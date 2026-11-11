package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"evergate/internal/router"
	"evergate/internal/store"
	"github.com/gin-gonic/gin"
)

const responsesProtocol = "responses"
const responsesToolIdentitiesKey = "evergate.responses_tool_identities"
const responsesNativeKey = "evergate.responses_native"
const responsesOriginalBodyKey = "evergate.responses_original_body"
const responsesBridgeBodyKey = "evergate.responses_bridge_body"

type responsesToolIdentity struct {
	Name         string
	Namespace    string
	Custom       bool
	UpstreamName string
}

type responsesParsedInput struct {
	Items           []responsesInputItem
	AdditionalTools []responsesTool
	ReasoningEffort string
}

type responsesToolRegistry struct {
	byUpstream         map[string]responsesToolIdentity
	upstreamByOriginal map[string]string
	used               map[string]bool
}

type responsesInputItem struct {
	Type      string                     `json:"type"`
	ID        string                     `json:"id"`
	Role      string                     `json:"role"`
	CallID    string                     `json:"call_id"`
	Name      string                     `json:"name"`
	Namespace string                     `json:"namespace"`
	Arguments string                     `json:"arguments"`
	Input     string                     `json:"input"`
	Content   json.RawMessage            `json:"content"`
	Output    json.RawMessage            `json:"output"`
	Tools     []responsesTool            `json:"tools"`
	Reasoning map[string]json.RawMessage `json:"reasoning"`
}

type responsesTextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Strict      *bool           `json:"strict"`
	Parameters  map[string]any  `json:"parameters"`
	Tools       []responsesTool `json:"tools"`
	External    *bool           `json:"external_web_access"`
	Async       *bool           `json:"async"`
	Format      json.RawMessage `json:"format"`
	DeferLoad   *bool           `json:"defer_loading"`
	Callers     []string        `json:"allowed_callers"`
}

type responsesRequest struct {
	Model             string                     `json:"model"`
	Instructions      string                     `json:"instructions"`
	Input             json.RawMessage            `json:"input"`
	Tools             []responsesTool            `json:"tools"`
	ToolChoice        any                        `json:"tool_choice"`
	ParallelToolCalls *bool                      `json:"parallel_tool_calls"`
	MaxOutputTokens   *int                       `json:"max_output_tokens"`
	Temperature       *float64                   `json:"temperature"`
	TopP              *float64                   `json:"top_p"`
	Stream            bool                       `json:"stream"`
	Reasoning         map[string]json.RawMessage `json:"reasoning"`
	Store             *bool                      `json:"store"`
	Include           []string                   `json:"include"`
	PromptCacheKey    string                     `json:"prompt_cache_key"`
	Text              *struct {
		Verbosity string `json:"verbosity"`
	} `json:"text"`
	ClientMetadata map[string]string `json:"client_metadata"`
}

type chatRequest struct {
	Model             string        `json:"model"`
	Messages          []chatMessage `json:"messages"`
	Tools             []chatTool    `json:"tools,omitempty"`
	ToolChoice        any           `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool         `json:"parallel_tool_calls,omitempty"`
	MaxTokens         *int          `json:"max_tokens,omitempty"`
	Temperature       *float64      `json:"temperature,omitempty"`
	TopP              *float64      `json:"top_p,omitempty"`
	Stream            bool          `json:"stream"`
	ReasoningEffort   string        `json:"reasoning_effort,omitempty"`
	Verbosity         string        `json:"verbosity,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Strict      *bool          `json:"strict,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type chatToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function chatCallFunc `json:"function"`
}

type chatCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// HandleResponses 处理 OpenAI Responses API：Responses 能力渠道原生透传，
// 其余渠道在 dispatch 阶段回落到 Chat Completions 桥接。
func (r *Relay) HandleResponses(c *gin.Context) {
	start := r.opt.Now()
	hit := &hitRecord{}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "读取请求体失败")
		return
	}
	if _, _, ok := parseChatRequest(body); !ok {
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "请求体必须是含 model 字段的 JSON")
		return
	}
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(strings.NewReader(string(body)))
	c.Set("evergate.protocol", responsesProtocol)
	setResponsesOriginalBody(c, body)

	trace, status, errType, errMsg := r.dispatch(c, hit)
	if r.opt.LogWriter != nil {
		tokenID, tokenName := tokenFromContext(c)
		total := r.opt.Now().Sub(start)
		r.opt.LogWriter.Submit(&store.RequestLog{
			TokenID: tokenID, TokenName: tokenName,
			ChannelID: hit.channelID, ChannelName: hit.channelName, KeyID: hit.keyID,
			Model: hitLoggedModel(c), UpstreamModel: hit.upstreamModel,
			Stream: isStreamRequest(c), StatusCode: status, ErrorType: errType, ErrorMessage: errMsg,
			RetryTrace: trace, FirstTokenMS: hit.firstTokenMS, TotalMS: total.Milliseconds(),
			PromptTokens: hit.promptTokens, CompletionTokens: hit.completionTokens,
			TotalTokens: hit.totalTokens, CachedTokens: hit.cachedTokens, ReasoningTokens: hit.reasoningTokens,
			IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
		})
	}
}

func responsesToChat(body []byte) ([]byte, *responsesToolRegistry, error) {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, nil, fmt.Errorf("请求体必须是合法 JSON: %w", err)
	}
	for key := range wire {
		switch key {
		case "model", "instructions", "input", "tools", "tool_choice", "parallel_tool_calls",
			"max_output_tokens", "temperature", "top_p", "stream", "reasoning", "store",
			"include", "prompt_cache_key", "text", "client_metadata":
		default:
			return nil, nil, fmt.Errorf("unsupported Responses request field %q", key)
		}
	}
	var req responsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, nil, fmt.Errorf("请求体必须是合法 JSON: %w", err)
	}
	if req.Model == "" {
		return nil, nil, fmt.Errorf("model 不能为空")
	}
	out := chatRequest{
		Model:             req.Model,
		ParallelToolCalls: req.ParallelToolCalls, MaxTokens: req.MaxOutputTokens,
		Temperature: req.Temperature, TopP: req.TopP, Stream: req.Stream,
	}
	if req.Instructions != "" {
		out.Messages = append(out.Messages, chatMessage{Role: "system", Content: req.Instructions})
	}
	parsedInput, err := parseResponsesInput(req.Input)
	if err != nil {
		return nil, nil, err
	}
	allTools := make([]responsesTool, 0, len(req.Tools)+len(parsedInput.AdditionalTools))
	allTools = append(allTools, req.Tools...)
	allTools = append(allTools, parsedInput.AdditionalTools...)
	tools, toolRegistry, err := responsesToolsToChat(allTools, "")
	if err != nil {
		return nil, nil, err
	}
	out.Tools = tools
	inputMessages, merr := responsesItemsToMessages(parsedInput.Items, toolRegistry)
	if merr != nil {
		return nil, nil, merr
	}
	out.Messages = append(out.Messages, inputMessages...)
	out.ToolChoice = responsesToolChoiceToChat(req.ToolChoice, toolRegistry)
	if raw, ok := req.Reasoning["effort"]; ok {
		var effort string
		if json.Unmarshal(raw, &effort) == nil && effort != "" {
			out.ReasoningEffort = effort
		}
	}
	if parsedInput.ReasoningEffort != "" {
		out.ReasoningEffort = parsedInput.ReasoningEffort
	}
	if req.Text != nil && req.Text.Verbosity != "" {
		out.Verbosity = req.Text.Verbosity
	}
	converted, err := json.Marshal(out)
	if err != nil {
		return nil, nil, err
	}
	return converted, toolRegistry, nil
}

func parseResponsesInput(raw json.RawMessage) (responsesParsedInput, error) {
	if len(raw) == 0 {
		return responsesParsedInput{}, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil && text != "" {
		content, err := json.Marshal(text)
		if err != nil {
			return responsesParsedInput{}, err
		}
		return responsesParsedInput{Items: []responsesInputItem{{Type: "message", Role: "user", Content: content}}}, nil
	}
	var items []responsesInputItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return responsesParsedInput{}, fmt.Errorf("input 必须是字符串或 item 数组: %w", err)
	}
	result := responsesParsedInput{Items: items}
	for _, item := range items {
		switch item.Type {
		case "additional_tools":
			result.AdditionalTools = append(result.AdditionalTools, item.Tools...)
		case "configuration_update":
			if raw, ok := item.Reasoning["effort"]; ok {
				var effort string
				if json.Unmarshal(raw, &effort) == nil && effort != "" {
					result.ReasoningEffort = effort
				}
			}
		}
	}
	return result, nil
}

func responsesItemsToMessages(items []responsesInputItem, registry *responsesToolRegistry) ([]chatMessage, error) {
	messages := make([]chatMessage, 0, len(items))
	assistantWithTools := -1
	for _, item := range items {
		switch item.Type {
		case "", "message":
			role := item.Role
			if role == "developer" {
				role = "system"
			}
			content, cerr := contentText(item.Content)
			if cerr != nil {
				return nil, cerr
			}
			msg := chatMessage{Role: role, Content: content}
			messages = append(messages, msg)
			assistantWithTools = -1
		case "additional_tools", "configuration_update":
			// 已在 parseResponsesInput / tools registry 阶段处理。
		case "function_call":
			name := registry.UpstreamName(item.Namespace, item.Name, false)
			appendToolCall(&messages, &assistantWithTools, item.CallID, name, item.Arguments)
		case "custom_tool_call":
			arguments, err := json.Marshal(map[string]string{"input": item.Input})
			if err != nil {
				return nil, err
			}
			name := registry.UpstreamName(item.Namespace, item.Name, true)
			appendToolCall(&messages, &assistantWithTools, item.CallID, name, string(arguments))
		case "function_call_output":
			output, oerr := outputText(item.Output)
			if oerr != nil {
				return nil, oerr
			}
			messages = append(messages, chatMessage{Role: "tool", ToolCallID: item.CallID, Content: output})
			assistantWithTools = -1
		case "custom_tool_call_output":
			output, oerr := outputText(item.Output)
			if oerr != nil {
				return nil, oerr
			}
			messages = append(messages, chatMessage{Role: "tool", ToolCallID: item.CallID, Content: output})
			assistantWithTools = -1
		case "reasoning":
			// EverGate 不生成 encrypted reasoning，因此正常不会收到该项。
		default:
			return nil, fmt.Errorf("unsupported Responses input item type %q", item.Type)
		}
	}
	return messages, nil
}

func appendToolCall(msgs *[]chatMessage, assistantWithTools *int, callID, name, arguments string) {
	if *assistantWithTools < 0 {
		*msgs = append(*msgs, chatMessage{Role: "assistant"})
		*assistantWithTools = len(*msgs) - 1
	}
	(*msgs)[*assistantWithTools].ToolCalls = append((*msgs)[*assistantWithTools].ToolCalls, chatToolCall{
		ID: callID, Type: "function",
		Function: chatCallFunc{Name: name, Arguments: arguments},
	})
}

func newResponsesToolRegistry() *responsesToolRegistry {
	return &responsesToolRegistry{
		byUpstream:         make(map[string]responsesToolIdentity),
		upstreamByOriginal: make(map[string]string),
		used:               make(map[string]bool),
	}
}

func responsesToolOriginalKey(namespace, name string, custom bool) string {
	return normalizeResponsesNamespace(namespace) + "\x00" + name + "\x00" + strconv.FormatBool(custom)
}

func normalizeResponsesNamespace(namespace string) string {
	if namespace == "functions" {
		return ""
	}
	return namespace
}

func (r *responsesToolRegistry) Register(name, namespace string, custom bool) (string, error) {
	identity := responsesToolIdentity{Name: name, Namespace: normalizeResponsesNamespace(namespace), Custom: custom}
	originalKey := responsesToolOriginalKey(identity.Namespace, name, custom)
	if existing, ok := r.upstreamByOriginal[originalKey]; ok {
		if previous := r.byUpstream[existing]; previous != identity {
			return "", fmt.Errorf("Responses tool %q in namespace %q has conflicting declarations", name, identity.Namespace)
		}
		return existing, nil
	}

	candidate := chatToolUpstreamName(identity.Namespace, name, custom)
	if len(candidate) > 64 {
		return "", fmt.Errorf("Responses tool %q in namespace %q cannot map to a valid Chat tool name", name, identity.Namespace)
	}
	if r.used[candidate] {
		return "", fmt.Errorf("Responses tool %q in namespace %q maps to an already used Chat tool name %q", name, identity.Namespace, candidate)
	}
	identity.UpstreamName = candidate
	r.used[candidate] = true
	r.upstreamByOriginal[originalKey] = candidate
	r.byUpstream[candidate] = identity
	return candidate, nil
}

func (r *responsesToolRegistry) Identity(upstreamName string) responsesToolIdentity {
	if identity, ok := r.byUpstream[upstreamName]; ok {
		return identity
	}
	return responsesToolIdentity{Name: upstreamName, UpstreamName: upstreamName}
}

func (r *responsesToolRegistry) UpstreamName(namespace, name string, custom bool) string {
	normalized := normalizeResponsesNamespace(namespace)
	if upstreamName, ok := r.upstreamByOriginal[responsesToolOriginalKey(normalized, name, custom)]; ok {
		return upstreamName
	}
	return chatToolUpstreamName(normalized, name, custom)
}

func sanitizeChatToolName(value string) string {
	var sb strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
	}
	if sb.Len() == 0 {
		return "_"
	}
	return sb.String()
}

func chatToolUpstreamName(namespace, name string, custom bool) string {
	segments := make([]string, 0, 2)
	if namespace != "" {
		segments = append(segments, sanitizeChatToolName(namespace))
	}
	segments = append(segments, sanitizeChatToolName(name))
	base := strings.Join(segments, "__")
	if len(base) <= 64 {
		return base
	}
	suffix := "_" + responseItemID("t", responsesToolOriginalKey(namespace, name, custom))[2:]
	if len(suffix) >= 64 {
		return suffix[:64]
	}
	return base[:64-len(suffix)] + suffix
}

func responsesToolsToChat(tools []responsesTool, namespace string) ([]chatTool, *responsesToolRegistry, error) {
	out := make([]chatTool, 0, len(tools))
	registry := newResponsesToolRegistry()
	if err := appendResponsesToolsToChat(&out, tools, namespace, registry); err != nil {
		return nil, nil, err
	}
	return out, registry, nil
}

func appendResponsesToolsToChat(out *[]chatTool, tools []responsesTool, namespace string, registry *responsesToolRegistry) error {
	for _, tool := range tools {
		switch tool.Type {
		case "function":
			upstreamName, err := registry.Register(tool.Name, namespace, false)
			if err != nil {
				return err
			}
			if !containsChatTool(*out, upstreamName) {
				*out = append(*out, chatTool{Type: "function", Function: chatFunction{
					Name: upstreamName, Description: tool.Description, Strict: tool.Strict, Parameters: tool.Parameters,
				}})
			}
		case "namespace":
			nestedNamespace := tool.Name
			if nestedNamespace == "functions" {
				nestedNamespace = ""
			}
			if err := appendResponsesToolsToChat(out, tool.Tools, nestedNamespace, registry); err != nil {
				return err
			}
		case "web_search":
			if tool.External != nil && *tool.External {
				return fmt.Errorf("Responses web_search tool is not supported by Chat Completions upstreams")
			}
		case "custom":
			function, err := customToolChatFunction(tool)
			if err != nil {
				return err
			}
			upstreamName, err := registry.Register(tool.Name, namespace, true)
			if err != nil {
				return err
			}
			function.Name = upstreamName
			if !containsChatTool(*out, upstreamName) {
				*out = append(*out, chatTool{Type: "function", Function: function})
			}
		default:
			return fmt.Errorf("unsupported Responses tool type %q", tool.Type)
		}
	}
	return nil
}

func containsChatTool(tools []chatTool, name string) bool {
	for _, tool := range tools {
		if tool.Function.Name == name {
			return true
		}
	}
	return false
}

func customToolChatFunction(tool responsesTool) (chatFunction, error) {
	if tool.Async != nil && *tool.Async {
		return chatFunction{}, fmt.Errorf("Responses async custom tool %q is not supported", tool.Name)
	}
	if tool.DeferLoad != nil && *tool.DeferLoad {
		return chatFunction{}, fmt.Errorf("Responses deferred custom tool %q is not supported", tool.Name)
	}
	for _, caller := range tool.Callers {
		if caller != "direct" {
			return chatFunction{}, fmt.Errorf("Responses custom tool %q caller %q is not supported", tool.Name, caller)
		}
	}
	inputDescription := "Raw input for this custom tool, passed as a plain string."
	if len(tool.Format) > 0 && string(tool.Format) != "null" {
		var format struct {
			Type       string `json:"type"`
			Syntax     string `json:"syntax"`
			Definition string `json:"definition"`
		}
		if err := json.Unmarshal(tool.Format, &format); err != nil {
			return chatFunction{}, fmt.Errorf("invalid Responses custom tool %q format: %w", tool.Name, err)
		}
		switch format.Type {
		case "", "text":
			if format.Syntax != "" || format.Definition != "" {
				return chatFunction{}, fmt.Errorf("Responses custom tool %q text format cannot carry grammar fields", tool.Name)
			}
		case "grammar":
			if format.Syntax != "lark" && format.Syntax != "regex" {
				return chatFunction{}, fmt.Errorf("Responses custom tool %q grammar syntax %q is not supported", tool.Name, format.Syntax)
			}
			if strings.TrimSpace(format.Definition) == "" {
				return chatFunction{}, fmt.Errorf("Responses custom tool %q grammar definition cannot be empty", tool.Name)
			}
			inputDescription += " It must match grammar syntax: " + format.Syntax + ".\ngrammar definition:\n" + format.Definition
		default:
			return chatFunction{}, fmt.Errorf("Responses custom tool %q format %q is not supported", tool.Name, format.Type)
		}
	}
	strict := false
	return chatFunction{
		Name:        tool.Name,
		Description: tool.Description,
		Strict:      &strict,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": inputDescription,
				},
			},
			"required":             []string{"input"},
			"additionalProperties": false,
		},
	}, nil
}

func responsesToolChoiceToChat(choice any, registry *responsesToolRegistry) any {
	choiceMap, ok := choice.(map[string]any)
	if !ok {
		return choice
	}
	name, _ := choiceMap["name"].(string)
	if name == "" {
		return choice
	}
	namespace, _ := choiceMap["namespace"].(string)
	if choiceMap["type"] == "custom" {
		return map[string]any{
			"type":     "function",
			"function": map[string]any{"name": registry.UpstreamName(namespace, name, true)},
		}
	}
	if choiceMap["type"] == "function" {
		return map[string]any{
			"type":     "function",
			"function": map[string]any{"name": registry.UpstreamName(namespace, name, false)},
		}
	}
	return choice
}

func contentText(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var parts []responsesTextPart
	if json.Unmarshal(raw, &parts) == nil {
		var sb strings.Builder
		for _, part := range parts {
			if part.Type == "input_text" || part.Type == "output_text" {
				sb.WriteString(part.Text)
			} else {
				return nil, fmt.Errorf("unsupported Responses content part type %q", part.Type)
			}
		}
		return sb.String(), nil
	}
	return nil, fmt.Errorf("unsupported Responses content format")
}

func outputText(raw json.RawMessage) (any, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	return contentText(raw)
}

func isResponsesRequest(c *gin.Context) bool {
	v, _ := c.Get("evergate.protocol")
	s, _ := v.(string)
	return s == responsesProtocol
}

func isResponsesNative(c *gin.Context) bool {
	value, _ := c.Get(responsesNativeKey)
	native, _ := value.(bool)
	return native
}

func setResponsesOriginalBody(c *gin.Context, body []byte) {
	c.Set(responsesOriginalBodyKey, append([]byte(nil), body...))
}

func responsesOriginalBody(c *gin.Context) []byte {
	value, _ := c.Get(responsesOriginalBodyKey)
	body, _ := value.([]byte)
	return body
}

func setResponsesBridgeBody(c *gin.Context, body []byte) {
	c.Set(responsesBridgeBodyKey, append([]byte(nil), body...))
}

func responsesBridgeBody(c *gin.Context) ([]byte, bool) {
	value, exists := c.Get(responsesBridgeBodyKey)
	if !exists {
		return nil, false
	}
	body, _ := value.([]byte)
	return body, true
}

func anyResponsesNative(candidates []router.Candidate) bool {
	for _, candidate := range candidates {
		if candidate.Channel.SupportsResponses {
			return true
		}
	}
	return false
}

func preferResponsesNative(candidates []router.Candidate) []router.Candidate {
	out := append([]router.Candidate(nil), candidates...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			leftNative, rightNative := out[j-1].Channel.SupportsResponses, out[j].Channel.SupportsResponses
			if !leftNative && rightNative {
				out[j-1], out[j] = out[j], out[j-1]
			}
		}
	}
	return out
}

func responsesNeedsNativeBridge(body []byte) bool {
	var wire struct {
		PreviousResponseID string `json:"previous_response_id"`
		Background         *bool  `json:"background"`
		Store              *bool  `json:"store"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return true
	}
	return wire.PreviousResponseID != "" ||
		(wire.Background != nil && *wire.Background) ||
		(wire.Store != nil && *wire.Store)
}

func responsesToolIdentities(c *gin.Context) *responsesToolRegistry {
	value, _ := c.Get(responsesToolIdentitiesKey)
	registry, _ := value.(*responsesToolRegistry)
	if registry == nil {
		return newResponsesToolRegistry()
	}
	return registry
}

func toolIdentity(registry *responsesToolRegistry, name string) responsesToolIdentity {
	return registry.Identity(name)
}

func customToolInput(arguments string) string {
	var wire struct {
		Input *string `json:"input"`
	}
	if err := json.Unmarshal([]byte(arguments), &wire); err == nil && wire.Input != nil {
		return *wire.Input
	}
	return arguments
}

func convertChatCompletionToResponses(body []byte, model string, now time.Time, registry *responsesToolRegistry) ([]byte, error) {
	var src struct {
		ID      string `json:"id"`
		Created int64  `json:"created"`
		Choices []struct {
			Message struct {
				Role      string         `json:"role"`
				Content   string         `json:"content"`
				ToolCalls []chatToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &src); err != nil {
		return nil, err
	}
	output := make([]map[string]any, 0, 2)
	if len(src.Choices) > 0 {
		msg := src.Choices[0].Message
		if msg.Content != "" {
			output = append(output, map[string]any{
				"id": responseItemID("msg", src.ID), "type": "message", "role": "assistant", "status": "completed",
				"content": []map[string]any{{"type": "output_text", "text": msg.Content, "annotations": []any{}, "logprobs": []any{}}},
			})
		}
		for _, call := range msg.ToolCalls {
			identity := toolIdentity(registry, call.Function.Name)
			responseItem := map[string]any{
				"call_id": call.ID, "name": identity.Name, "status": "completed",
			}
			if identity.Namespace != "" {
				responseItem["namespace"] = identity.Namespace
			}
			if identity.Custom {
				input := customToolInput(call.Function.Arguments)
				responseItem["id"] = responseItemID("ctc", call.ID)
				responseItem["type"] = "custom_tool_call"
				responseItem["input"] = input
			} else {
				responseItem["id"] = responseItemID("fc", call.ID)
				responseItem["type"] = "function_call"
				responseItem["arguments"] = call.Function.Arguments
			}
			output = append(output, responseItem)
		}
	}
	usage := map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
	if src.Usage != nil {
		usage = map[string]any{
			"input_tokens": src.Usage.PromptTokens, "output_tokens": src.Usage.CompletionTokens,
			"total_tokens": src.Usage.TotalTokens,
		}
	}
	return json.Marshal(map[string]any{
		"id": responseItemID("resp", src.ID), "object": "response", "created_at": src.Created,
		"status": "completed", "completed_at": now.Unix(), "model": model,
		"output": output, "usage": usage, "store": false,
	})
}

func responseItemID(prefix, source string) string {
	sum := sha256.Sum256([]byte(source))
	return prefix + "_" + hex.EncodeToString(sum[:12])
}
