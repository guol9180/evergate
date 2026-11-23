package relay

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/router"
)

const (
	messagesProtocol        = "messages"
	messagesNativeKey       = "evergate.messages_native"
	messagesOriginalBodyKey = "evergate.messages_original_body"
	messagesBridgeBodyKey   = "evergate.messages_bridge_body"
	messagesCountTokensKey  = "evergate.messages_count_tokens"
	// defaultAnthropicVersion 在客户端未携带 anthropic-version 时使用官方稳定版本头。
	defaultAnthropicVersion = "2023-06-01"
)

// HandleMessages 处理 Anthropic Messages 原生入口：声明能力的渠道 M→M 透传，
// 其余渠道在 dispatch 阶段回落到 Chat Completions 桥接。
func (r *Relay) HandleMessages(c *gin.Context) {
	r.handleMessages(c, false)
}

// HandleMessagesCountTokens 处理 Anthropic count_tokens；只有原生渠道能提供准确计数。
func (r *Relay) HandleMessagesCountTokens(c *gin.Context) {
	r.handleMessages(c, true)
}

func (r *Relay) handleMessages(c *gin.Context, countTokens bool) {
	start := r.opt.Now()
	hit := &hitRecord{}
	req := r.beginRequest(c, "messages")
	defer req.end()
	c.Set("evergate.protocol", messagesProtocol)
	if countTokens {
		c.Set(messagesCountTokensKey, true)
	}

	body, err := readRelayRequestBody(c)
	if err != nil {
		status, errType, message := requestBodyReadError(err)
		r.recordRequestMetrics(c, status, errType)
		writeOpenAIError(c, status, errType, errType, message)
		req.setResult(nil, status, errType, message)
		return
	}
	if _, _, ok := parseChatRequest(body); !ok {
		r.recordRequestMetrics(c, http.StatusBadRequest, "invalid_request_error")
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "请求体必须是含 model 字段的 JSON")
		req.setResult(nil, http.StatusBadRequest, "invalid_request_error", "缺少 model 字段")
		return
	}
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	setMessagesOriginalBody(c, body)

	trace, status, errType, errMsg := r.dispatch(c, hit)
	r.finishRequest(c, start, hit, trace, status, errType, errMsg)
	req.setResult(trace, status, errType, errMsg)
}

type messagesMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type messagesBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text,omitempty"`
	ID           string          `json:"id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	Content      json.RawMessage `json:"content,omitempty"`
	IsError      bool            `json:"is_error,omitempty"`
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
	Thinking     string          `json:"thinking,omitempty"`
	Signature    string          `json:"signature,omitempty"`
	Source       json.RawMessage `json:"source,omitempty"`
}

type messagesTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  map[string]any  `json:"input_schema"`
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
}

// messagesToChat 把 Messages 请求桥接为 Chat Completions；不可表达的字段显式报错，
// 避免静默改变 thinking / prompt caching 的语义。
func messagesToChat(body []byte) ([]byte, error) {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("请求体必须是合法 JSON: %w", err)
	}
	allowed := map[string]bool{
		"model": true, "max_tokens": true, "messages": true, "system": true,
		"tools": true, "tool_choice": true, "temperature": true, "top_p": true,
		"stop_sequences": true, "stream": true, "metadata": true, "thinking": true,
	}
	for key := range wire {
		if !allowed[key] {
			return nil, fmt.Errorf("unsupported Messages request field %q", key)
		}
	}
	if raw, ok := wire["thinking"]; ok && !isJSONNull(raw) {
		return nil, fmt.Errorf("thinking 无法桥接到 Chat Completions，请为该模型配置支持 Anthropic 原生协议的渠道")
	}

	var req struct {
		Model         string            `json:"model"`
		MaxTokens     *int              `json:"max_tokens"`
		System        json.RawMessage   `json:"system"`
		Messages      []messagesMessage `json:"messages"`
		Tools         []messagesTool    `json:"tools"`
		ToolChoice    json.RawMessage   `json:"tool_choice"`
		Temperature   *float64          `json:"temperature"`
		TopP          *float64          `json:"top_p"`
		StopSequences []string          `json:"stop_sequences"`
		Stream        bool              `json:"stream"`
		Metadata      struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("请求体必须是合法 JSON: %w", err)
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("messages 不能为空")
	}

	out := map[string]any{"model": req.Model, "stream": req.Stream}
	if req.MaxTokens != nil {
		out["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		out["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		out["top_p"] = *req.TopP
	}
	if len(req.StopSequences) > 0 {
		out["stop"] = req.StopSequences
	}
	if req.Metadata.UserID != "" {
		out["user"] = req.Metadata.UserID
	}

	converted := make([]chatMessage, 0, len(req.Messages)+1)
	if len(req.System) > 0 && !isJSONNull(req.System) {
		text, err := messagesSystemText(req.System)
		if err != nil {
			return nil, fmt.Errorf("system: %w", err)
		}
		if text != "" {
			converted = append(converted, chatMessage{Role: "system", Content: text})
		}
	}
	for i, msg := range req.Messages {
		items, err := bridgeMessagesMessage(msg)
		if err != nil {
			return nil, fmt.Errorf("messages[%d]: %w", i, err)
		}
		converted = append(converted, items...)
	}
	out["messages"] = converted

	if len(req.Tools) > 0 {
		tools := make([]chatTool, 0, len(req.Tools))
		for i, tool := range req.Tools {
			if hasCacheControl(tool.CacheControl) {
				return nil, fmt.Errorf("tools[%d]: cache_control 无法桥接到 Chat Completions", i)
			}
			if tool.Name == "" {
				return nil, fmt.Errorf("tools[%d]: name 不能为空", i)
			}
			parameters := tool.InputSchema
			if parameters == nil {
				parameters = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			tools = append(tools, chatTool{Type: "function", Function: chatFunction{
				Name: tool.Name, Description: tool.Description, Parameters: parameters,
			}})
		}
		out["tools"] = tools
	}
	if len(req.ToolChoice) > 0 && !isJSONNull(req.ToolChoice) {
		choice, parallel, err := messagesToolChoice(req.ToolChoice)
		if err != nil {
			return nil, err
		}
		out["tool_choice"] = choice
		if parallel != nil {
			out["parallel_tool_calls"] = *parallel
		}
	}
	return json.Marshal(out)
}

func messagesSystemText(raw json.RawMessage) (string, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var blocks []messagesBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", fmt.Errorf("必须是字符串或内容块数组")
	}
	var sb strings.Builder
	for _, block := range blocks {
		if hasCacheControl(block.CacheControl) {
			return "", fmt.Errorf("cache_control 无法桥接到 Chat Completions")
		}
		if block.Type != "text" {
			return "", fmt.Errorf("不支持的内容块类型 %q", block.Type)
		}
		sb.WriteString(block.Text)
	}
	return sb.String(), nil
}

func bridgeMessagesMessage(msg messagesMessage) ([]chatMessage, error) {
	if msg.Role != "user" && msg.Role != "assistant" {
		return nil, fmt.Errorf("不支持的 role %q", msg.Role)
	}
	var text string
	if json.Unmarshal(msg.Content, &text) == nil {
		return []chatMessage{{Role: msg.Role, Content: text}}, nil
	}
	var blocks []messagesBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil, fmt.Errorf("content 必须是字符串或内容块数组")
	}

	var texts []string
	var calls []chatToolCall
	var toolMessages []chatMessage
	for _, block := range blocks {
		if hasCacheControl(block.CacheControl) {
			return nil, fmt.Errorf("cache_control 无法桥接到 Chat Completions")
		}
		switch block.Type {
		case "text":
			texts = append(texts, block.Text)
		case "tool_use":
			if msg.Role != "assistant" {
				return nil, fmt.Errorf("tool_use 只能出现在 assistant 消息")
			}
			arguments := "{}"
			if len(block.Input) > 0 && !isJSONNull(block.Input) {
				arguments = string(block.Input)
			}
			calls = append(calls, chatToolCall{ID: block.ID, Type: "function", Function: chatCallFunc{
				Name: block.Name, Arguments: arguments,
			}})
		case "tool_result":
			if msg.Role != "user" {
				return nil, fmt.Errorf("tool_result 只能出现在 user 消息")
			}
			content, err := messagesToolResultText(block.Content)
			if err != nil {
				return nil, err
			}
			toolMessages = append(toolMessages, chatMessage{Role: "tool", ToolCallID: block.ToolUseID, Content: content})
		case "thinking", "redacted_thinking":
			return nil, fmt.Errorf("thinking 内容块无法桥接到 Chat Completions")
		case "image", "document":
			return nil, fmt.Errorf("%s 内容块无法桥接到 Chat Completions", block.Type)
		default:
			return nil, fmt.Errorf("不支持的内容块类型 %q", block.Type)
		}
	}
	joined := strings.Join(texts, "")
	if msg.Role == "assistant" {
		if len(calls) == 0 && joined == "" {
			return []chatMessage{{Role: "assistant", Content: ""}}, nil
		}
		content := any(nil)
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

func messagesToolResultText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var blocks []messagesBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", fmt.Errorf("tool_result.content 必须是字符串或内容块数组")
	}
	var sb strings.Builder
	for _, block := range blocks {
		if hasCacheControl(block.CacheControl) {
			return "", fmt.Errorf("cache_control 无法桥接到 Chat Completions")
		}
		if block.Type != "text" {
			return "", fmt.Errorf("tool_result 不支持的内容块类型 %q", block.Type)
		}
		sb.WriteString(block.Text)
	}
	return sb.String(), nil
}

func messagesToolChoice(raw json.RawMessage) (any, *bool, error) {
	var choice struct {
		Type                   string `json:"type"`
		Name                   string `json:"name"`
		DisableParallelToolUse *bool  `json:"disable_parallel_tool_use"`
	}
	if err := json.Unmarshal(raw, &choice); err != nil {
		return nil, nil, fmt.Errorf("tool_choice 必须是对象")
	}
	var parallel *bool
	if choice.DisableParallelToolUse != nil {
		value := !*choice.DisableParallelToolUse
		parallel = &value
	}
	switch choice.Type {
	case "auto":
		return "auto", parallel, nil
	case "any":
		return "required", parallel, nil
	case "none":
		return "none", parallel, nil
	case "tool":
		if choice.Name == "" {
			return nil, nil, fmt.Errorf("tool_choice.name 不能为空")
		}
		return map[string]any{"type": "function", "function": map[string]any{"name": choice.Name}}, parallel, nil
	default:
		return nil, nil, fmt.Errorf("不支持的 tool_choice.type %q", choice.Type)
	}
}

// convertChatCompletionToMessages 把 Chat Completions 非流式响应桥接为 Messages 响应。
func convertChatCompletionToMessages(body []byte, model string, now time.Time) ([]byte, error) {
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
	blocks := make([]map[string]any, 0, 1+len(choice.Message.ToolCalls))
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": text})
	}
	for _, call := range choice.Message.ToolCalls {
		id := call.ID
		if id == "" {
			id = anthropicMessageID(now)
		}
		blocks = append(blocks, map[string]any{
			"type": "tool_use", "id": id, "name": call.Function.Name,
			"input": messagesToolInput(call.Function.Arguments),
		})
	}
	usage := map[string]int64{"input_tokens": 0, "output_tokens": 0}
	if wire.Usage != nil {
		prompt, completion, _, cached, _ := wire.Usage.normalize()
		usage["input_tokens"] = prompt
		usage["output_tokens"] = completion
		if cached > 0 {
			usage["cache_read_input_tokens"] = cached
		}
	}
	response := map[string]any{
		"id": anthropicMessageID(now), "type": "message", "role": "assistant",
		"model": model, "content": blocks,
		"stop_reason": messagesStopReason(choice.FinishReason), "stop_sequence": nil,
		"usage": usage,
	}
	return json.Marshal(response)
}

func chatMessageText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("上游响应 content 格式不受支持")
	}
	var sb strings.Builder
	for _, part := range parts {
		if part.Type != "text" && part.Type != "output_text" {
			return "", fmt.Errorf("上游响应 content part %q 无法桥接", part.Type)
		}
		sb.WriteString(part.Text)
	}
	return sb.String(), nil
}

func messagesToolInput(arguments string) any {
	if strings.TrimSpace(arguments) == "" {
		return map[string]any{}
	}
	var input any
	if err := json.Unmarshal([]byte(arguments), &input); err == nil && input != nil {
		return input
	}
	return map[string]any{"raw_arguments": arguments}
}

func messagesStopReason(reason string) string {
	switch reason {
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	default:
		return "end_turn"
	}
}

func anthropicMessageID(now time.Time) string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "msg_" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("msg_%x", now.UnixNano())
}

func hasCacheControl(raw json.RawMessage) bool {
	return len(raw) > 0 && !isJSONNull(raw)
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func setMessagesOriginalBody(c *gin.Context, body []byte) {
	c.Set(messagesOriginalBodyKey, body)
}

func messagesOriginalBody(c *gin.Context) []byte {
	value, _ := c.Get(messagesOriginalBodyKey)
	body, _ := value.([]byte)
	return body
}

func setMessagesBridgeBody(c *gin.Context, body []byte) {
	c.Set(messagesBridgeBodyKey, body)
}

func messagesBridgeBody(c *gin.Context) ([]byte, bool) {
	value, ok := c.Get(messagesBridgeBodyKey)
	if !ok {
		return nil, false
	}
	body, _ := value.([]byte)
	return body, body != nil
}

func clearMessagesBridgeBody(c *gin.Context) {
	c.Set(messagesBridgeBodyKey, []byte(nil))
}

func isMessagesRequest(c *gin.Context) bool {
	value, _ := c.Get("evergate.protocol")
	protocol, _ := value.(string)
	return protocol == messagesProtocol
}

func isMessagesNative(c *gin.Context) bool {
	value, _ := c.Get(messagesNativeKey)
	native, _ := value.(bool)
	return native
}

func isMessagesCountTokens(c *gin.Context) bool {
	value, _ := c.Get(messagesCountTokensKey)
	count, _ := value.(bool)
	return count
}

func anyAnthropicNative(candidates []router.Candidate) bool {
	for _, candidate := range candidates {
		if candidate.Channel.SupportsAnthropic {
			return true
		}
	}
	return false
}

func preferAnthropicNative(candidates []router.Candidate) []router.Candidate {
	out := append([]router.Candidate(nil), candidates...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			leftNative, rightNative := out[j-1].Channel.SupportsAnthropic, out[j].Channel.SupportsAnthropic
			if !leftNative && rightNative {
				out[j-1], out[j] = out[j], out[j-1]
			}
		}
	}
	return out
}

func filterAnthropicNative(candidates []router.Candidate) []router.Candidate {
	out := make([]router.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Channel.SupportsAnthropic {
			out = append(out, candidate)
		}
	}
	return out
}

// writeMessagesUpstreamError 把 Anthropic 错误体转换为 EverGate 统一的 OpenAI 错误格式。
func writeMessagesUpstreamError(c *gin.Context, status int, body []byte, secret string) {
	var wire struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &wire)
	message := strings.TrimSpace(redactCredentials(wire.Error.Message, secret))
	if message == "" {
		message = upstreamErrorMessage(status, body, secret)
	}
	code := redactCredentials(wire.Error.Type, secret)
	if code == "" {
		code = "upstream_error"
	}
	kind := "invalid_request_error"
	if status >= http.StatusInternalServerError {
		kind = "server_error"
	}
	writeOpenAIError(c, status, kind, code, message)
}
