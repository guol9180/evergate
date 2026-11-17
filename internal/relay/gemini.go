package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"evergate/internal/router"
)

const (
	geminiProtocol        = "gemini"
	geminiNativeKey       = "evergate.gemini_native"
	geminiOriginalBodyKey = "evergate.gemini_original_body"
	geminiBridgeBodyKey   = "evergate.gemini_bridge_body"
	geminiMethodKey       = "evergate.gemini_method"

	geminiMethodGenerate = "generateContent"
	geminiMethodStream   = "streamGenerateContent"
	geminiMethodCount    = "countTokens"
)

// RejectQueryKey 禁止 Gemini 的 ?key= 鉴权，避免下游 key 进入访问日志。
func RejectQueryKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Query("key") != "" {
			writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "api_key_in_query",
				"Gemini API key must be sent via x-goog-api-key; query parameter key is not allowed")
			c.Abort()
			return
		}
		c.Next()
	}
}

// HandleGemini 处理 Gemini 原生入口；模型来自路径，body 不携带 model。
func (r *Relay) HandleGemini(c *gin.Context) {
	start := r.opt.Now()
	hit := &hitRecord{}
	req := r.beginRequest(c, "gemini")
	defer req.end()
	c.Set("evergate.protocol", geminiProtocol)

	model, method, err := parseGeminiAction(c.Param("action"))
	if err != nil {
		r.recordRequestMetrics(c, http.StatusBadRequest, "invalid_request_error")
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_gemini_path", err.Error())
		req.setResult(nil, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	c.Set(geminiMethodKey, method)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		r.recordRequestMetrics(c, http.StatusBadRequest, "invalid_request_error")
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "读取请求体失败")
		req.setResult(nil, http.StatusBadRequest, "invalid_request_error", "读取请求体失败")
		return
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		r.recordRequestMetrics(c, http.StatusBadRequest, "invalid_request_error")
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "请求体必须是合法 JSON 对象")
		req.setResult(nil, http.StatusBadRequest, "invalid_request_error", "请求体必须是合法 JSON 对象")
		return
	}
	setGeminiOriginalBody(c, body)

	dispatchBody, err := geminiDispatchBody(body, model, method)
	if err != nil {
		r.recordRequestMetrics(c, http.StatusBadRequest, "invalid_request_error")
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "重构请求体失败")
		req.setResult(nil, http.StatusBadRequest, "invalid_request_error", "重构请求体失败")
		return
	}
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(dispatchBody))

	trace, status, errType, errMsg := r.dispatch(c, hit)
	r.finishRequest(c, start, hit, trace, status, errType, errMsg)
	req.setResult(trace, status, errType, errMsg)
}

func geminiDispatchBody(body []byte, model, method string) ([]byte, error) {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, err
	}
	modelJSON, _ := json.Marshal(model)
	streamJSON, _ := json.Marshal(method == geminiMethodStream)
	wire["model"] = modelJSON
	wire["stream"] = streamJSON
	return json.Marshal(wire)
}

func parseGeminiAction(action string) (model, method string, err error) {
	path := strings.TrimPrefix(action, "/")
	index := strings.LastIndex(path, ":")
	if index <= 0 || index == len(path)-1 {
		return "", "", fmt.Errorf("路径必须形如 /v1beta/models/{model}:generateContent")
	}
	model = strings.TrimPrefix(path[:index], "models/")
	method = path[index+1:]
	switch method {
	case geminiMethodGenerate, geminiMethodStream, geminiMethodCount:
		return model, method, nil
	default:
		return "", "", fmt.Errorf("不支持的 Gemini 方法 %q", method)
	}
}

type geminiPart struct {
	Text             string          `json:"text,omitempty"`
	Thought          *bool           `json:"thought,omitempty"`
	FunctionCall     *geminiFuncCall `json:"functionCall,omitempty"`
	FunctionResponse *geminiFuncResp `json:"functionResponse,omitempty"`
	InlineData       json.RawMessage `json:"inlineData,omitempty"`
	FileData         json.RawMessage `json:"fileData,omitempty"`
}

type geminiFuncCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type geminiFuncResp struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiTool struct {
	FunctionDeclarations []struct {
		Name                 string          `json:"name"`
		Description          string          `json:"description"`
		Parameters           map[string]any  `json:"parameters"`
		ParametersJSONSchema json.RawMessage `json:"parametersJsonSchema"`
	} `json:"functionDeclarations"`
}

// geminiToChat 把 Gemini 请求桥接为 Chat Completions；不可等价表达的字段显式 400。
func geminiToChat(body []byte, model string, stream bool) ([]byte, error) {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("请求体必须是合法 JSON: %w", err)
	}
	allowed := map[string]bool{
		"contents": true, "systemInstruction": true, "system_instruction": true,
		"tools": true, "toolConfig": true, "tool_config": true,
		"generationConfig": true, "generation_config": true,
		"safetySettings": true, "safety_settings": true,
	}
	for key := range wire {
		if !allowed[key] {
			return nil, fmt.Errorf("unsupported Gemini request field %q", key)
		}
	}
	if raw := geminiRaw(wire, "safetySettings", "safety_settings"); len(raw) > 0 && !isJSONNull(raw) && strings.TrimSpace(string(raw)) != "[]" {
		return nil, fmt.Errorf("safetySettings 无法桥接到 Chat Completions，请为该模型配置支持 Gemini 原生协议的渠道")
	}
	if _, ok := wire["systemInstruction"]; !ok {
		if raw, exists := wire["system_instruction"]; exists {
			wire["systemInstruction"] = raw
		}
	}
	if _, ok := wire["toolConfig"]; !ok {
		if raw, exists := wire["tool_config"]; exists {
			wire["toolConfig"] = raw
		}
	}
	if _, ok := wire["generationConfig"]; !ok {
		if raw, exists := wire["generation_config"]; exists {
			wire["generationConfig"] = raw
		}
	}

	var req struct {
		Contents          []geminiContent `json:"contents"`
		SystemInstruction *struct {
			Parts []geminiPart `json:"parts"`
		} `json:"systemInstruction"`
		Tools      []geminiTool `json:"tools"`
		ToolConfig *struct {
			FunctionCallingConfig struct {
				Mode                 string   `json:"mode"`
				AllowedFunctionNames []string `json:"allowedFunctionNames"`
			} `json:"functionCallingConfig"`
		} `json:"toolConfig"`
		GenerationConfig *struct {
			Temperature      *float64        `json:"temperature"`
			TopP             *float64        `json:"topP"`
			TopK             *int            `json:"topK"`
			MaxOutputTokens  *int            `json:"maxOutputTokens"`
			StopSequences    []string        `json:"stopSequences"`
			CandidateCount   *int            `json:"candidateCount"`
			PresencePenalty  *float64        `json:"presencePenalty"`
			FrequencyPenalty *float64        `json:"frequencyPenalty"`
			Seed             *int64          `json:"seed"`
			ResponseMimeType string          `json:"responseMimeType"`
			ResponseSchema   json.RawMessage `json:"responseSchema"`
			ThinkingConfig   json.RawMessage `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("请求体必须是合法 JSON: %w", err)
	}
	if len(req.Contents) == 0 {
		return nil, fmt.Errorf("contents 不能为空")
	}
	if req.GenerationConfig != nil {
		if len(req.GenerationConfig.ThinkingConfig) > 0 && !isJSONNull(req.GenerationConfig.ThinkingConfig) {
			return nil, fmt.Errorf("generationConfig.thinkingConfig 无法桥接到 Chat Completions")
		}
		if req.GenerationConfig.ResponseMimeType != "" || (len(req.GenerationConfig.ResponseSchema) > 0 && !isJSONNull(req.GenerationConfig.ResponseSchema)) {
			return nil, fmt.Errorf("generationConfig.responseSchema/responseMimeType 无法桥接到 Chat Completions")
		}
		if req.GenerationConfig.TopK != nil {
			return nil, fmt.Errorf("generationConfig.topK 无法桥接到 Chat Completions")
		}
		if req.GenerationConfig.CandidateCount != nil && *req.GenerationConfig.CandidateCount > 1 {
			return nil, fmt.Errorf("generationConfig.candidateCount > 1 无法桥接到 Chat Completions")
		}
	}

	out := map[string]any{"model": model, "stream": stream}
	var messages []chatMessage
	if req.SystemInstruction != nil {
		text, err := geminiPartsText(req.SystemInstruction.Parts, "systemInstruction")
		if err != nil {
			return nil, err
		}
		if text != "" {
			messages = append(messages, chatMessage{Role: "system", Content: text})
		}
	}
	pendingCalls := make(map[string][]string)
	callSeq := 0
	for i, content := range req.Contents {
		role := content.Role
		if role == "" {
			role = "user"
		}
		if role != "user" && role != "model" {
			return nil, fmt.Errorf("contents[%d]: 不支持的 role %q", i, role)
		}
		converted, err := bridgeGeminiContent(content, role, pendingCalls, &callSeq)
		if err != nil {
			return nil, fmt.Errorf("contents[%d]: %w", i, err)
		}
		messages = append(messages, converted...)
	}
	out["messages"] = messages

	if len(req.Tools) > 0 {
		tools := make([]chatTool, 0, len(req.Tools))
		for i, tool := range req.Tools {
			if len(tool.FunctionDeclarations) == 0 {
				return nil, fmt.Errorf("tools[%d]: 仅支持 functionDeclarations，其他内置工具无法桥接", i)
			}
			for j, decl := range tool.FunctionDeclarations {
				if decl.Name == "" {
					return nil, fmt.Errorf("tools[%d].functionDeclarations[%d]: name 不能为空", i, j)
				}
				if len(decl.ParametersJSONSchema) > 0 && !isJSONNull(decl.ParametersJSONSchema) {
					return nil, fmt.Errorf("tools[%d].functionDeclarations[%d]: parametersJsonSchema 无法桥接", i, j)
				}
				parameters := decl.Parameters
				if parameters == nil {
					parameters = map[string]any{"type": "object", "properties": map[string]any{}}
				}
				tools = append(tools, chatTool{Type: "function", Function: chatFunction{
					Name: decl.Name, Description: decl.Description, Parameters: parameters,
				}})
			}
		}
		out["tools"] = tools
	}
	if req.ToolConfig != nil {
		choice, err := geminiToolChoice(req.ToolConfig.FunctionCallingConfig.Mode, req.ToolConfig.FunctionCallingConfig.AllowedFunctionNames)
		if err != nil {
			return nil, err
		}
		out["tool_choice"] = choice
	}
	if cfg := req.GenerationConfig; cfg != nil {
		if cfg.Temperature != nil {
			out["temperature"] = *cfg.Temperature
		}
		if cfg.TopP != nil {
			out["top_p"] = *cfg.TopP
		}
		if cfg.MaxOutputTokens != nil {
			out["max_tokens"] = *cfg.MaxOutputTokens
		}
		if len(cfg.StopSequences) > 0 {
			out["stop"] = cfg.StopSequences
		}
		if cfg.PresencePenalty != nil {
			out["presence_penalty"] = *cfg.PresencePenalty
		}
		if cfg.FrequencyPenalty != nil {
			out["frequency_penalty"] = *cfg.FrequencyPenalty
		}
		if cfg.Seed != nil {
			out["seed"] = *cfg.Seed
		}
	}
	return json.Marshal(out)
}

func bridgeGeminiContent(content geminiContent, role string, pendingCalls map[string][]string, callSeq *int) ([]chatMessage, error) {
	var texts []string
	var calls []chatToolCall
	var toolMessages []chatMessage
	for _, part := range content.Parts {
		if part.Thought != nil && *part.Thought {
			return nil, fmt.Errorf("thought 内容无法桥接到 Chat Completions")
		}
		if len(part.InlineData) > 0 || len(part.FileData) > 0 {
			return nil, fmt.Errorf("inlineData/fileData 无法桥接到 Chat Completions")
		}
		if part.FunctionCall != nil {
			if role != "model" {
				return nil, fmt.Errorf("functionCall 只能出现在 model 内容")
			}
			if part.FunctionCall.Name == "" {
				return nil, fmt.Errorf("functionCall.name 不能为空")
			}
			*callSeq = *callSeq + 1
			id := fmt.Sprintf("call_%d_%s", *callSeq, part.FunctionCall.Name)
			arguments := "{}"
			if len(part.FunctionCall.Args) > 0 && !isJSONNull(part.FunctionCall.Args) {
				arguments = string(part.FunctionCall.Args)
			}
			calls = append(calls, chatToolCall{ID: id, Type: "function", Function: chatCallFunc{
				Name: part.FunctionCall.Name, Arguments: arguments,
			}})
			pendingCalls[part.FunctionCall.Name] = append(pendingCalls[part.FunctionCall.Name], id)
			continue
		}
		if part.FunctionResponse != nil {
			if role != "user" {
				return nil, fmt.Errorf("functionResponse 只能出现在 user 内容")
			}
			id := ""
			queue := pendingCalls[part.FunctionResponse.Name]
			if len(queue) > 0 {
				id, pendingCalls[part.FunctionResponse.Name] = queue[0], queue[1:]
			}
			if id == "" {
				id = "call_response_" + part.FunctionResponse.Name
			}
			content, err := geminiFunctionResponseText(part.FunctionResponse.Response)
			if err != nil {
				return nil, err
			}
			toolMessages = append(toolMessages, chatMessage{Role: "tool", ToolCallID: id, Content: content})
			continue
		}
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	joined := strings.Join(texts, "")
	if role == "model" {
		if joined == "" && len(calls) == 0 {
			return []chatMessage{{Role: "assistant", Content: ""}}, nil
		}
		var content any
		if joined != "" {
			content = joined
		}
		return []chatMessage{{Role: "assistant", Content: content, ToolCalls: calls}}, nil
	}
	out := append([]chatMessage(nil), toolMessages...)
	if joined != "" || len(out) == 0 {
		out = append(out, chatMessage{Role: "user", Content: joined})
	}
	return out, nil
}

func geminiPartsText(parts []geminiPart, field string) (string, error) {
	var sb strings.Builder
	for _, part := range parts {
		if part.Thought != nil && *part.Thought {
			return "", fmt.Errorf("%s: thought 内容无法桥接到 Chat Completions", field)
		}
		if part.FunctionCall != nil || part.FunctionResponse != nil || len(part.InlineData) > 0 || len(part.FileData) > 0 {
			return "", fmt.Errorf("%s: 只支持 text parts", field)
		}
		sb.WriteString(part.Text)
	}
	return sb.String(), nil
}

func geminiFunctionResponseText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("functionResponse.response 必须是对象或字符串")
	}
	if result, ok := obj["result"]; ok {
		data, _ := json.Marshal(result)
		return string(data), nil
	}
	data, _ := json.Marshal(obj)
	return string(data), nil
}

func geminiToolChoice(mode string, allowed []string) (any, error) {
	if len(allowed) > 1 {
		return nil, fmt.Errorf("toolConfig.allowedFunctionNames 多于一个时无法桥接")
	}
	if len(allowed) == 1 {
		return map[string]any{"type": "function", "function": map[string]any{"name": allowed[0]}}, nil
	}
	switch strings.ToUpper(mode) {
	case "", "AUTO":
		return "auto", nil
	case "ANY":
		return "required", nil
	case "NONE":
		return "none", nil
	default:
		return nil, fmt.Errorf("不支持的 toolConfig.functionCallingConfig.mode %q", mode)
	}
}

// convertChatCompletionToGemini 把 Chat Completions 非流式响应桥接为 Gemini 响应。
func convertChatCompletionToGemini(body []byte, model string) ([]byte, error) {
	var wire struct {
		Choices []struct {
			Message struct {
				Content   json.RawMessage `json:"content"`
				ToolCalls []chatToolCall  `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *usageWire `json:"usage"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("解析上游响应失败: %w", err)
	}
	if len(wire.Choices) == 0 {
		return nil, fmt.Errorf("上游响应缺少 choices")
	}
	choice := wire.Choices[0]
	text, err := chatMessageText(choice.Message.Content)
	if err != nil {
		return nil, err
	}
	parts := make([]map[string]any, 0, 1+len(choice.Message.ToolCalls))
	if strings.TrimSpace(text) != "" {
		parts = append(parts, map[string]any{"text": text})
	}
	for _, call := range choice.Message.ToolCalls {
		parts = append(parts, map[string]any{
			"functionCall": map[string]any{
				"name": call.Function.Name,
				"args": messagesToolInput(call.Function.Arguments),
			},
		})
	}
	usage := geminiUsage{}
	if wire.Usage != nil {
		prompt, completion, total, cached, _ := wire.Usage.normalize()
		usage.PromptTokenCount = prompt
		usage.CandidatesTokenCount = completion
		usage.TotalTokenCount = total
		usage.CachedContentTokenCount = cached
	}
	response := map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": parts},
			"finishReason": geminiFinishReason(choice.FinishReason),
			"index":        0,
		}},
		"usageMetadata": usage,
		"modelVersion":  model,
	}
	return json.Marshal(response)
}

func geminiFinishReason(reason string) string {
	switch reason {
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "SAFETY"
	default:
		return "STOP"
	}
}

type geminiUsage struct {
	PromptTokenCount          int64 `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount      int64 `json:"candidatesTokenCount,omitempty"`
	ThoughtsTokenCount        int64 `json:"thoughtsTokenCount,omitempty"`
	ToolUsePromptTokenCount   int64 `json:"toolUsePromptTokenCount,omitempty"`
	TotalTokenCount           int64 `json:"totalTokenCount,omitempty"`
	CachedContentTokenCount   int64 `json:"cachedContentTokenCount,omitempty"`
	PromptTokenCountSnake     int64 `json:"prompt_token_count,omitempty"`
	CandidatesTokenCountSnake int64 `json:"candidates_token_count,omitempty"`
	ThoughtsTokenCountSnake   int64 `json:"thoughts_token_count,omitempty"`
	ToolUsePromptTokenSnake   int64 `json:"tool_use_prompt_token_count,omitempty"`
	TotalTokenCountSnake      int64 `json:"total_token_count,omitempty"`
	CachedContentTokenSnake   int64 `json:"cached_content_token_count,omitempty"`
}

func (u geminiUsage) normalize() (prompt, completion, total, cached int64) {
	prompt = firstPositive(u.PromptTokenCount, u.PromptTokenCountSnake) +
		firstPositive(u.ToolUsePromptTokenCount, u.ToolUsePromptTokenSnake)
	completion = firstPositive(u.CandidatesTokenCount, u.CandidatesTokenCountSnake) +
		firstPositive(u.ThoughtsTokenCount, u.ThoughtsTokenCountSnake)
	total = firstPositive(u.TotalTokenCount, u.TotalTokenCountSnake, prompt+completion)
	cached = firstPositive(u.CachedContentTokenCount, u.CachedContentTokenSnake)
	return
}

func parseGeminiUsage(data []byte, hit *hitRecord) {
	var wire struct {
		UsageMetadata      *geminiUsage `json:"usageMetadata"`
		UsageMetadataSnake *geminiUsage `json:"usage_metadata"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return
	}
	usage := wire.UsageMetadata
	if usage == nil {
		usage = wire.UsageMetadataSnake
	}
	if usage == nil {
		return
	}
	prompt, completion, total, cached := usage.normalize()
	hit.usageSeen = true
	if prompt > 0 {
		hit.promptTokens = prompt
	}
	if completion > 0 {
		hit.completionTokens = completion
	}
	if total > 0 {
		hit.totalTokens = total
	} else {
		hit.totalTokens = hit.promptTokens + hit.completionTokens
	}
	if cached > 0 {
		hit.cachedTokens = cached
	}
}

func geminiRaw(wire map[string]json.RawMessage, names ...string) json.RawMessage {
	for _, name := range names {
		if raw, ok := wire[name]; ok {
			return raw
		}
	}
	return nil
}

func setGeminiOriginalBody(c *gin.Context, body []byte) {
	c.Set(geminiOriginalBodyKey, body)
}

func geminiOriginalBody(c *gin.Context) []byte {
	value, _ := c.Get(geminiOriginalBodyKey)
	body, _ := value.([]byte)
	return body
}

func setGeminiBridgeBody(c *gin.Context, body []byte) {
	c.Set(geminiBridgeBodyKey, body)
}

func geminiBridgeBody(c *gin.Context) ([]byte, bool) {
	value, ok := c.Get(geminiBridgeBodyKey)
	if !ok {
		return nil, false
	}
	body, _ := value.([]byte)
	return body, body != nil
}

func clearGeminiBridgeBody(c *gin.Context) {
	c.Set(geminiBridgeBodyKey, []byte(nil))
}

func isGeminiRequest(c *gin.Context) bool {
	value, _ := c.Get("evergate.protocol")
	protocol, _ := value.(string)
	return protocol == geminiProtocol
}

func isGeminiNative(c *gin.Context) bool {
	value, _ := c.Get(geminiNativeKey)
	native, _ := value.(bool)
	return native
}

func geminiMethod(c *gin.Context) string {
	value, _ := c.Get(geminiMethodKey)
	method, _ := value.(string)
	return method
}

func isGeminiStreamRequest(c *gin.Context) bool {
	return geminiMethod(c) == geminiMethodStream
}

func isGeminiCountTokens(c *gin.Context) bool {
	return geminiMethod(c) == geminiMethodCount
}

func anyGeminiNative(candidates []router.Candidate) bool {
	for _, candidate := range candidates {
		if candidate.Channel.SupportsGemini {
			return true
		}
	}
	return false
}

func preferGeminiNative(candidates []router.Candidate) []router.Candidate {
	out := append([]router.Candidate(nil), candidates...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			leftNative, rightNative := out[j-1].Channel.SupportsGemini, out[j].Channel.SupportsGemini
			if !leftNative && rightNative {
				out[j-1], out[j] = out[j], out[j-1]
			}
		}
	}
	return out
}

func filterGeminiNative(candidates []router.Candidate) []router.Candidate {
	out := make([]router.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Channel.SupportsGemini {
			out = append(out, candidate)
		}
	}
	return out
}

// writeGeminiUpstreamError 把 Gemini 错误体转换为 EverGate 统一的 OpenAI 错误格式。
func writeGeminiUpstreamError(c *gin.Context, status int, body []byte) {
	var wire struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &wire)
	message := strings.TrimSpace(wire.Error.Message)
	if message == "" {
		message = upstreamErrorMessage(status, body)
	}
	code := wire.Error.Status
	if code == "" {
		code = "upstream_error"
	}
	kind := "invalid_request_error"
	if status >= http.StatusInternalServerError {
		kind = "server_error"
	}
	writeOpenAIError(c, status, kind, code, message)
}

// geminiUpstreamPath 返回原生上游路径；模型位于路径而非 body。
func geminiUpstreamPath(channelBase, upstreamModel string) string {
	return strings.TrimSuffix(channelBase, "/") + "/v1beta/models/" + upstreamModel + ":"
}

func geminiRequestURL(channelBase, upstreamModel, method string) string {
	base := geminiUpstreamPath(channelBase, upstreamModel) + method
	if method == geminiMethodStream {
		return base + "?alt=sse"
	}
	return base
}

func geminiErrorFrame(message string) []byte {
	payload, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    http.StatusBadGateway,
			"message": message,
			"status":  "INTERNAL",
		},
	})
	return append(append([]byte("data: "), payload...), '\n', '\n')
}
