package relay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/retry"
	"evergate/internal/router"
	"evergate/internal/store"
)

type responsesStreamTool struct {
	id          string
	callID      string
	name        string
	arguments   strings.Builder
	outputIndex int
	done        bool
	custom      bool
	namespace   string
}

type responsesStream struct {
	w           *sseWriter
	model       string
	now         time.Time
	sequence    int
	responseID  string
	created     int64
	createdSent bool

	messageID   string
	messageIdx  int
	text        strings.Builder
	messageOpen bool
	output      []map[string]any

	tools    map[int]*responsesStreamTool
	finishRS bool
	usage    map[string]any

	toolRegistry *responsesToolRegistry
}

// streamResponsesResponse converts an upstream Chat Completions SSE stream to
// Responses semantic events. As with the native path, nothing is written before
// the first upstream chunk, preserving the pre-first-token retry window.
func (r *Relay) streamResponsesResponse(c *gin.Context, resp *http.Response, key *store.ChannelKey, cand router.Candidate, firstToken time.Duration, cancel context.CancelFunc, trace *[]store.RetryAttempt, hit *hitRecord, model string, attemptStart time.Time) attemptOutcome {
	start := attemptStart
	appendTrace := func(errType, errMsg string) {
		*trace = append(*trace, store.RetryAttempt{
			ChannelID: cand.Channel.ID, ChannelName: cand.Channel.Name, KeyID: key.ID,
			Attempt: len(*trace) + 1, ErrorType: errType, ErrorMessage: errMsg,
			ElapsedMS: r.opt.Now().Sub(start).Milliseconds(),
		})
	}
	br := bufio.NewReaderSize(resp.Body, 32*1024)
	var timeout atomic.Bool
	timer := time.AfterFunc(firstToken, func() { timeout.Store(true); cancel() })
	first, err := br.ReadBytes('\n')
	timer.Stop()
	if err != nil {
		if timeout.Load() {
			err = retry.ErrFirstTokenTimeout
		}
		cls := retry.Classify(retry.Result{Err: err})
		r.opt.Pool.ReportFailure(key.ID, cls)
		appendTrace(string(cls.Kind), err.Error())
		return outcomeNextChannel
	}
	if cls, isErr := parseSSEErrorEvent(first); isErr {
		appendTrace(string(cls.Kind), "upstream stream error event")
		switch cls.Kind {
		case retry.KindRateLimited, retry.KindAuth:
			r.opt.Pool.ReportFailure(key.ID, cls)
			return outcomeNextKey
		default:
			return outcomeNextChannel
		}
	}

	r.opt.Pool.ReportSuccess(key.ID)
	appendTrace("", "")
	recordHit(hit, cand.Channel, key, cand.UpstreamModel, r.opt.Now().Sub(start))
	state := &responsesStream{
		w: newSSEWriter(c.Writer), model: model, now: r.opt.Now(),
		tools: map[int]*responsesStreamTool{}, toolRegistry: responsesToolIdentities(c),
	}
	state.w.header()
	stopKeepalive := state.w.startKeepalive()
	defer stopKeepalive()
	if lineData, ok := sseData(first); ok {
		state.consume(lineData)
	}

	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) > 0 {
			if data, ok := sseData(line); ok {
				state.consume(data)
				if state.finishRS {
					return outcomeDone
				}
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				state.finish()
				return outcomeDone
			}
			r.opt.Logger.Error("upstream responses stream interrupted", "error", rerr.Error(), "channel", cand.Channel.Name)
			state.fail("upstream stream interrupted: " + rerr.Error())
			return outcomeDone
		}
	}
}

func (s *responsesStream) consume(data string) {
	if data == "[DONE]" {
		s.finish()
		s.finishRS = true
		return
	}
	if strings.Contains(data, `"error"`) {
		var wire map[string]json.RawMessage
		if json.Unmarshal([]byte(data), &wire) == nil {
			if raw, exists := wire["error"]; exists && string(raw) != "null" {
				var errObj struct {
					Message string `json:"message"`
				}
				_ = json.Unmarshal(raw, &errObj)
				if errObj.Message == "" {
					errObj.Message = "upstream stream error"
				}
				s.fail(errObj.Message)
				s.finishRS = true
				return
			}
		}
	}
	var chunk struct {
		ID      string `json:"id"`
		Created int64  `json:"created"`
		Choices []struct {
			Delta struct {
				Content   string           `json:"content"`
				ToolCalls []streamToolCall `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return
	}
	if chunk.ID != "" {
		s.responseID = chunk.ID
	}
	if chunk.Created != 0 {
		s.created = chunk.Created
	}
	if chunk.Usage != nil {
		s.usage = map[string]any{
			"input_tokens": chunk.Usage.PromptTokens, "output_tokens": chunk.Usage.CompletionTokens,
			"total_tokens": chunk.Usage.TotalTokens,
		}
	}
	if len(chunk.Choices) == 0 {
		return
	}
	delta := chunk.Choices[0].Delta
	if delta.Content != "" {
		s.ensureCreated()
		s.startMessage()
		s.text.WriteString(delta.Content)
		s.event("response.output_text.delta", map[string]any{
			"item_id": s.messageID, "output_index": s.messageIdx, "content_index": 0,
			"delta": delta.Content, "logprobs": []any{},
		})
	}
	for _, call := range delta.ToolCalls {
		s.ensureCreated()
		tool := s.tools[call.Index]
		name := call.Function.Name
		if name == "" && tool != nil {
			name = tool.name
		}
		if tool == nil {
			identity := toolIdentity(s.toolRegistry, name)
			idPrefix := "fc"
			if identity.Custom {
				idPrefix = "ctc"
			}
			tool = &responsesStreamTool{
				id: responseItemID(idPrefix, chunk.ID+"-"+call.ID+"-"+name), outputIndex: -1,
				name: identity.Name, callID: call.ID, custom: identity.Custom, namespace: identity.Namespace,
			}
			s.tools[call.Index] = tool
		}
		if call.ID != "" {
			tool.callID = call.ID
		}
		if call.Function.Name != "" {
			if tool.outputIndex < 0 {
				identity := toolIdentity(s.toolRegistry, call.Function.Name)
				tool.custom = identity.Custom
				tool.namespace = identity.Namespace
				tool.name = identity.Name
			}
		}
		if tool.outputIndex < 0 {
			tool.outputIndex = len(s.output)
			if tool.custom {
				item := map[string]any{"id": tool.id, "type": "custom_tool_call", "status": "in_progress", "call_id": tool.callID, "name": tool.name, "input": ""}
				if tool.namespace != "" {
					item["namespace"] = tool.namespace
				}
				s.output = append(s.output, item)
			} else {
				item := map[string]any{"id": tool.id, "type": "function_call", "status": "in_progress", "call_id": tool.callID, "name": tool.name, "arguments": ""}
				if tool.namespace != "" {
					item["namespace"] = tool.namespace
				}
				s.output = append(s.output, item)
			}
			s.event("response.output_item.added", map[string]any{"output_index": tool.outputIndex, "item": s.output[tool.outputIndex]})
		}
		if call.Function.Arguments != "" {
			tool.arguments.WriteString(call.Function.Arguments)
			if !tool.custom {
				s.output[tool.outputIndex]["arguments"] = tool.arguments.String()
				s.event("response.function_call_arguments.delta", map[string]any{
					"item_id": tool.id, "output_index": tool.outputIndex, "delta": call.Function.Arguments,
				})
			}
		}
	}
	if chunk.Choices[0].FinishReason != "" {
		s.finish()
		s.finishRS = true
	}
}

func (s *responsesStream) ensureCreated() {
	if s.createdSent {
		return
	}
	s.createdSent = true
	if s.responseID == "" {
		s.responseID = "stream"
	}
	if s.created == 0 {
		s.created = s.now.Unix()
	}
	s.responseID = responseItemID("resp", s.responseID)
	s.event("response.created", map[string]any{"response": s.responseEnvelope("in_progress", nil)})
}

func (s *responsesStream) startMessage() {
	if s.messageOpen {
		return
	}
	s.ensureCreated()
	s.messageOpen = true
	s.messageID = responseItemID("msg", s.responseID)
	item := map[string]any{"id": s.messageID, "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}
	s.output = append(s.output, item)
	s.messageIdx = len(s.output) - 1
	idx := s.messageIdx
	s.event("response.output_item.added", map[string]any{"output_index": idx, "item": item})
	part := map[string]any{"type": "output_text", "text": "", "annotations": []any{}, "logprobs": []any{}}
	s.event("response.content_part.added", map[string]any{"item_id": s.messageID, "output_index": idx, "content_index": 0, "part": part})
}

func (s *responsesStream) finish() {
	if s.finishRS {
		return
	}
	s.ensureCreated()
	if s.messageOpen {
		text := s.text.String()
		idx := s.messageIdx
		s.event("response.output_text.done", map[string]any{"item_id": s.messageID, "output_index": idx, "content_index": 0, "text": text, "logprobs": []any{}})
		part := map[string]any{"type": "output_text", "text": text, "annotations": []any{}, "logprobs": []any{}}
		s.event("response.content_part.done", map[string]any{"item_id": s.messageID, "output_index": idx, "content_index": 0, "part": part})
		s.output[idx] = map[string]any{"id": s.messageID, "type": "message", "role": "assistant", "status": "completed", "content": []map[string]any{part}}
		s.event("response.output_item.done", map[string]any{"output_index": idx, "item": s.output[idx]})
	}
	indexes := make([]int, 0, len(s.tools))
	for i := range s.tools {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	for _, i := range indexes {
		tool := s.tools[i]
		if tool.done || tool.outputIndex < 0 {
			continue
		}
		tool.done = true
		var item map[string]any
		if tool.custom {
			input := customToolInput(tool.arguments.String())
			s.event("response.custom_tool_call_input.delta", map[string]any{
				"item_id": tool.id, "output_index": tool.outputIndex, "delta": input,
			})
			s.event("response.custom_tool_call_input.done", map[string]any{
				"item_id": tool.id, "output_index": tool.outputIndex, "input": input,
			})
			item = map[string]any{"id": tool.id, "type": "custom_tool_call", "status": "completed", "call_id": tool.callID, "name": tool.name, "input": input}
		} else {
			s.event("response.function_call_arguments.done", map[string]any{"item_id": tool.id, "output_index": tool.outputIndex, "arguments": tool.arguments.String()})
			item = map[string]any{"id": tool.id, "type": "function_call", "status": "completed", "call_id": tool.callID, "name": tool.name, "arguments": tool.arguments.String()}
		}
		if tool.namespace != "" {
			item["namespace"] = tool.namespace
		}
		s.output[tool.outputIndex] = item
		s.event("response.output_item.done", map[string]any{"output_index": tool.outputIndex, "item": item})
	}
	s.event("response.completed", map[string]any{"response": s.responseEnvelope("completed", nil)})
	s.finishRS = true
}

func (s *responsesStream) fail(message string) {
	s.ensureCreated()
	s.event("response.failed", map[string]any{"response": s.responseEnvelope("failed", map[string]any{"code": "upstream_stream_error", "message": message})})
}

func (s *responsesStream) responseEnvelope(status string, errObj map[string]any) map[string]any {
	usage := s.usage
	if usage == nil {
		usage = map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
	}
	output := s.output
	if output == nil {
		output = []map[string]any{}
	}
	return map[string]any{
		"id": s.responseID, "object": "response", "created_at": s.created,
		"status": status, "completed_at": s.now.Unix(), "model": s.model,
		"output": output, "error": errObj, "usage": usage, "store": false,
	}
}

func (s *responsesStream) event(kind string, fields map[string]any) {
	fields["type"] = kind
	fields["sequence_number"] = s.sequence
	s.sequence++
	payload, err := json.Marshal(fields)
	if err != nil {
		return
	}
	s.w.writeFlush(append(append([]byte("event: "+kind+"\ndata: "), payload...), '\n', '\n'))
}

type streamToolCall struct {
	Index    int          `json:"index"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function chatCallFunc `json:"function"`
}

func sseData(line []byte) (string, bool) {
	s := strings.TrimSpace(string(line))
	if after, ok := strings.CutPrefix(s, "data:"); ok {
		return strings.TrimSpace(after), true
	}
	return "", false
}
