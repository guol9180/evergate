package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"evergate/internal/store"
	"github.com/gin-gonic/gin"
)

const responsesProtocol = "responses"

type responsesInputItem struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Content   json.RawMessage `json:"content"`
	Output    json.RawMessage `json:"output"`
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
	ClientMetadata    map[string]string          `json:"client_metadata"`
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

// HandleResponses 处理 Codex 使用的 OpenAI Responses API，并在网关内部
// 转换为已有 Chat Completions 调度链路。
func (r *Relay) HandleResponses(c *gin.Context) {
	start := r.opt.Now()
	hit := &hitRecord{}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "读取请求体失败")
		return
	}
	converted, err := responsesToChat(body)
	if err != nil {
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", err.Error())
		return
	}
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(strings.NewReader(string(converted)))
	c.Set("evergate.protocol", responsesProtocol)

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
			IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
		})
	}
}

func responsesToChat(body []byte) ([]byte, error) {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("请求体必须是合法 JSON: %w", err)
	}
	for key := range wire {
		switch key {
		case "model", "instructions", "input", "tools", "tool_choice", "parallel_tool_calls",
			"max_output_tokens", "temperature", "top_p", "stream", "reasoning", "store",
			"include", "prompt_cache_key", "client_metadata":
		default:
			return nil, fmt.Errorf("unsupported Responses request field %q", key)
		}
	}
	var req responsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("请求体必须是合法 JSON: %w", err)
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model 不能为空")
	}
	out := chatRequest{
		Model: req.Model, ToolChoice: req.ToolChoice,
		ParallelToolCalls: req.ParallelToolCalls, MaxTokens: req.MaxOutputTokens,
		Temperature: req.Temperature, TopP: req.TopP, Stream: req.Stream,
	}
	if req.Instructions != "" {
		out.Messages = append(out.Messages, chatMessage{Role: "system", Content: req.Instructions})
	}
	messages, err := responsesInputToMessages(req.Input)
	if err != nil {
		return nil, err
	}
	out.Messages = append(out.Messages, messages...)
	tools, err := responsesToolsToChat(req.Tools)
	if err != nil {
		return nil, err
	}
	out.Tools = tools
	if raw, ok := req.Reasoning["effort"]; ok {
		var effort string
		if json.Unmarshal(raw, &effort) == nil && effort != "" {
			out.ReasoningEffort = effort
		}
	}
	return json.Marshal(out)
}

func responsesInputToMessages(raw json.RawMessage) ([]chatMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil && text != "" {
		return []chatMessage{{Role: "user", Content: text}}, nil
	}
	var items []responsesInputItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("input 必须是字符串或 item 数组: %w", err)
	}
	msgs := make([]chatMessage, 0, len(items))
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
			msgs = append(msgs, msg)
			assistantWithTools = -1
		case "function_call":
			if assistantWithTools < 0 {
				msgs = append(msgs, chatMessage{Role: "assistant"})
				assistantWithTools = len(msgs) - 1
			}
			msgs[assistantWithTools].ToolCalls = append(msgs[assistantWithTools].ToolCalls, chatToolCall{
				ID: item.CallID, Type: "function",
				Function: chatCallFunc{Name: item.Name, Arguments: item.Arguments},
			})
		case "function_call_output":
			output, oerr := outputText(item.Output)
			if oerr != nil {
				return nil, oerr
			}
			msgs = append(msgs, chatMessage{Role: "tool", ToolCallID: item.CallID, Content: output})
			assistantWithTools = -1
		case "reasoning":
			// EverGate 不生成 encrypted reasoning，因此正常不会收到该项。
		default:
			return nil, fmt.Errorf("unsupported Responses input item type %q", item.Type)
		}
	}
	return msgs, nil
}

func responsesToolsToChat(tools []responsesTool) ([]chatTool, error) {
	out := make([]chatTool, 0, len(tools))
	for _, tool := range tools {
		switch tool.Type {
		case "function":
			out = append(out, chatTool{Type: "function", Function: chatFunction{
				Name: tool.Name, Description: tool.Description, Strict: tool.Strict, Parameters: tool.Parameters,
			}})
		case "namespace":
			nested, err := responsesToolsToChat(tool.Tools)
			if err != nil {
				return nil, err
			}
			out = append(out, nested...)
		case "web_search":
			if tool.External != nil && *tool.External {
				return nil, fmt.Errorf("Responses web_search tool is not supported by Chat Completions upstreams")
			}
		default:
			return nil, fmt.Errorf("unsupported Responses tool type %q", tool.Type)
		}
	}
	return out, nil
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

func convertChatCompletionToResponses(body []byte, model string, now time.Time) ([]byte, error) {
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
			output = append(output, map[string]any{
				"id": responseItemID("fc", call.ID), "type": "function_call", "status": "completed",
				"call_id": call.ID, "name": call.Function.Name, "arguments": call.Function.Arguments,
			})
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
