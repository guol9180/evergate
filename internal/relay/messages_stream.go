package relay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/retry"
	"evergate/internal/router"
	"evergate/internal/store"
)

// streamNativeMessagesResponse 透传上游 Messages SSE：事件原样转发，
// 首个语义事件前可重试；EOF 早于 message_stop 视为流中断。
func (r *Relay) streamNativeMessagesResponse(c *gin.Context, resp *http.Response, key *store.ChannelKey, cand router.Candidate, firstToken time.Duration, cancel context.CancelFunc, trace *[]store.RetryAttempt, hit *hitRecord, attemptStart time.Time) attemptOutcome {
	appendTrace := func(status int, errType, errMsg string) {
		*trace = append(*trace, store.RetryAttempt{
			ChannelID: cand.Channel.ID, ChannelName: cand.Channel.Name, KeyID: key.ID,
			Attempt: len(*trace) + 1, StatusCode: status, ErrorType: errType, ErrorMessage: errMsg,
			ElapsedMS: r.opt.Now().Sub(attemptStart).Milliseconds(),
		})
	}

	br := bufio.NewReaderSize(resp.Body, 32*1024)
	var timeout atomic.Bool
	timer := time.AfterFunc(firstToken, func() { timeout.Store(true); cancel() })
	var first sseEvent
	var err error
	for {
		first, err = readSSEEvent(br)
		if err != nil || !isMessagesPingEvent(first) {
			break
		}
	}
	timer.Stop()
	if err != nil {
		if timeout.Load() {
			err = retry.ErrFirstTokenTimeout
		}
		cls := retry.Classify(retry.Result{Err: err})
		r.reportKeyFailure(key.ID, cls, err.Error())
		appendTrace(0, string(cls.Kind), err.Error())
		return outcomeNextChannel
	}
	if cls, isErr := parseMessagesSSEErrorEvent(first); isErr {
		appendTrace(0, string(cls.Kind), "upstream Messages stream error event")
		switch cls.Kind {
		case retry.KindRateLimited, retry.KindAuth:
			r.reportKeyFailure(key.ID, cls, "upstream Messages stream error event")
			return outcomeNextKey
		default:
			r.recordKeyFailure(key.ID, "upstream Messages stream error event")
			return outcomeNextChannel
		}
	}

	r.reportKeySuccess(key.ID)
	appendTrace(http.StatusOK, "", "")
	r.recordHit(c, hit, cand.Channel, key, cand.UpstreamModel, r.opt.Now().Sub(attemptStart))
	parseMessagesEventUsage(first.data, hit)

	w := newSSEWriter(c.Writer)
	w.header()
	w.writeFlush(first.raw)
	stopKeepalive := w.startKeepalive()
	defer stopKeepalive()

	sawStop := isMessagesStopEvent(first)
	for {
		event, rerr := readSSEEvent(br)
		if len(event.raw) > 0 {
			parseMessagesEventUsage(event.data, hit)
			if isMessagesStopEvent(event) {
				sawStop = true
			}
			if cls, isErr := parseMessagesSSEErrorEvent(event); isErr {
				r.opt.Logger.Error("upstream Messages stream error event",
					"kind", string(cls.Kind),
					"channel", cand.Channel.Name,
				)
			}
			w.writeFlush(event.raw)
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				if !sawStop {
					writeMessagesStreamError(w, "upstream stream interrupted: connection closed before message_stop")
				}
				return outcomeDone
			}
			r.opt.Logger.Error("upstream native Messages stream interrupted",
				"error", rerr.Error(),
				"channel", cand.Channel.Name,
			)
			writeMessagesStreamError(w, "upstream stream interrupted: "+rerr.Error())
			return outcomeDone
		}
	}
}

// streamMessagesBridgeResponse 把 Chat Completions SSE 转换为 Messages 事件序列。
func (r *Relay) streamMessagesBridgeResponse(c *gin.Context, resp *http.Response, key *store.ChannelKey, cand router.Candidate, firstToken time.Duration, cancel context.CancelFunc, trace *[]store.RetryAttempt, hit *hitRecord, model string, attemptStart time.Time) attemptOutcome {
	appendTrace := func(status int, errType, errMsg string) {
		*trace = append(*trace, store.RetryAttempt{
			ChannelID: cand.Channel.ID, ChannelName: cand.Channel.Name, KeyID: key.ID,
			Attempt: len(*trace) + 1, StatusCode: status, ErrorType: errType, ErrorMessage: errMsg,
			ElapsedMS: r.opt.Now().Sub(attemptStart).Milliseconds(),
		})
	}

	br := bufio.NewReaderSize(resp.Body, 32*1024)
	var timeout atomic.Bool
	timer := time.AfterFunc(firstToken, func() { timeout.Store(true); cancel() })
	var first sseEvent
	var err error
	for {
		first, err = readSSEEvent(br)
		if err != nil || strings.TrimSpace(string(first.data)) != "" {
			break
		}
	}
	timer.Stop()
	if err != nil {
		if timeout.Load() {
			err = retry.ErrFirstTokenTimeout
		}
		cls := retry.Classify(retry.Result{Err: err})
		r.reportKeyFailure(key.ID, cls, err.Error())
		appendTrace(0, string(cls.Kind), err.Error())
		return outcomeNextChannel
	}
	if cls, isErr := classifySSEErrorEvent(first); isErr {
		appendTrace(0, string(cls.Kind), "upstream stream error event")
		switch cls.Kind {
		case retry.KindRateLimited, retry.KindAuth:
			r.reportKeyFailure(key.ID, cls, "upstream stream error event")
			return outcomeNextKey
		default:
			r.recordKeyFailure(key.ID, "upstream stream error event")
			return outcomeNextChannel
		}
	}

	r.reportKeySuccess(key.ID)
	appendTrace(http.StatusOK, "", "")
	r.recordHit(c, hit, cand.Channel, key, cand.UpstreamModel, r.opt.Now().Sub(attemptStart))

	w := newSSEWriter(c.Writer)
	w.header()
	conv := newMessagesBridgeStream(w, model, r.opt.Now())
	conv.consume(first.data, hit)
	stopKeepalive := w.startKeepalive()
	defer stopKeepalive()

	for {
		event, rerr := readSSEEvent(br)
		if len(event.raw) > 0 {
			conv.consume(event.data, hit)
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				if !conv.finished {
					conv.fail("upstream stream interrupted: connection closed before finish_reason")
				}
				return outcomeDone
			}
			r.opt.Logger.Error("upstream bridge stream interrupted",
				"error", rerr.Error(),
				"channel", cand.Channel.Name,
			)
			conv.fail("upstream stream interrupted: " + rerr.Error())
			return outcomeDone
		}
	}
}

func classifySSEErrorEvent(event sseEvent) (retry.Classification, bool) {
	if cls, ok := parseMessagesSSEErrorEvent(event); ok {
		return cls, true
	}
	return parseSSEErrorEvent(event.raw)
}

func parseMessagesSSEErrorEvent(event sseEvent) (retry.Classification, bool) {
	payload := strings.TrimSpace(string(event.data))
	if payload == "" || payload == "[DONE]" {
		return retry.Classification{}, false
	}
	var wire struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &wire); err != nil {
		return retry.Classification{}, false
	}
	if event.name != "error" && wire.Type != "error" {
		return retry.Classification{}, false
	}
	status := retry.StreamErrorStatus(wire.Error.Type, "")
	return classifyStreamError(status, payload), true
}

func isMessagesPingEvent(event sseEvent) bool {
	if event.name == "ping" {
		return true
	}
	var wire struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(event.data, &wire) == nil && wire.Type == "ping"
}

func isMessagesStopEvent(event sseEvent) bool {
	if event.name == "message_stop" {
		return true
	}
	var wire struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(event.data, &wire) == nil && wire.Type == "message_stop"
}

func parseMessagesEventUsage(data []byte, hit *hitRecord) {
	var event struct {
		Message *struct {
			Usage *usageWire `json:"usage"`
		} `json:"message"`
		Usage *usageWire `json:"usage"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}
	if event.Message != nil && event.Message.Usage != nil {
		mergeAnthropicUsage(*event.Message.Usage, hit)
	}
	if event.Usage != nil {
		mergeAnthropicUsage(*event.Usage, hit)
	}
}

// mergeAnthropicUsage 兼容 message_start 的初始 usage 与 message_delta 的累计 usage，
// 后到的正数覆盖先前值，避免 output_tokens 被 0 冲掉。
func mergeAnthropicUsage(usage usageWire, hit *hitRecord) {
	prompt, completion, _, cached, _ := usage.normalize()
	hit.usageSeen = true
	if prompt > 0 {
		hit.promptTokens = prompt
	}
	if completion > 0 {
		hit.completionTokens = completion
	}
	hit.totalTokens = hit.promptTokens + hit.completionTokens
	if cached > 0 {
		hit.cachedTokens = cached
	}
}

func writeMessagesStreamError(w *sseWriter, message string) {
	payload, _ := json.Marshal(map[string]any{
		"type":  "error",
		"error": map[string]any{"type": "api_error", "message": message},
	})
	w.writeFlush([]byte("event: error\ndata: " + string(payload) + "\n\n"))
}

type messagesBridgeStream struct {
	w           *sseWriter
	id          string
	model       string
	started     bool
	nextIndex   int
	openIndex   int
	openKind    string
	toolBlocks  map[int]int
	sawTool     bool
	finished    bool
	stopReason  string
	inputTokens int64
	outTokens   int64
}

func newMessagesBridgeStream(w *sseWriter, model string, now time.Time) *messagesBridgeStream {
	return &messagesBridgeStream{
		w: w, id: anthropicMessageID(now), model: model,
		toolBlocks: make(map[int]int),
	}
}

func (s *messagesBridgeStream) consume(data []byte, hit *hitRecord) {
	payload := strings.TrimSpace(string(data))
	if payload == "" {
		return
	}
	if payload == "[DONE]" {
		s.endCleanly()
		return
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *usageWire `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return
	}
	if chunk.Usage != nil {
		prompt, completion, _, cached, _ := chunk.Usage.normalize()
		if prompt > 0 {
			s.inputTokens = prompt
			hit.usageSeen = true
			hit.promptTokens = prompt
		}
		if completion > 0 {
			s.outTokens = completion
			hit.usageSeen = true
			hit.completionTokens = completion
		}
		if cached > 0 {
			hit.cachedTokens = cached
		}
		hit.totalTokens = hit.promptTokens + hit.completionTokens
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			s.text(choice.Delta.Content)
		}
		for _, call := range choice.Delta.ToolCalls {
			s.tool(call.Index, call.ID, call.Function.Name, call.Function.Arguments)
		}
		if choice.FinishReason != "" {
			s.finish(messagesStopReason(choice.FinishReason))
		}
	}
}

func (s *messagesBridgeStream) text(text string) {
	s.ensureStart()
	if s.openKind != "text" {
		s.closeBlock()
		s.openIndex = s.nextIndex
		s.nextIndex++
		s.openKind = "text"
		s.emit("content_block_start", map[string]any{
			"type": "content_block_start", "index": s.openIndex,
			"content_block": map[string]any{"type": "text", "text": ""},
		})
	}
	s.emit("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": s.openIndex,
		"delta": map[string]any{"type": "text_delta", "text": text},
	})
}

func (s *messagesBridgeStream) tool(index int, id, name, arguments string) {
	s.ensureStart()
	blockIndex, seen := s.toolBlocks[index]
	if !seen {
		s.closeBlock()
		blockIndex = s.nextIndex
		s.nextIndex++
		s.toolBlocks[index] = blockIndex
		s.sawTool = true
		s.openIndex = blockIndex
		s.openKind = "tool"
		if id == "" {
			id = anthropicMessageID(time.Now())
		}
		if name == "" {
			name = "tool"
		}
		s.emit("content_block_start", map[string]any{
			"type": "content_block_start", "index": blockIndex,
			"content_block": map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{}},
		})
	} else if s.openKind != "tool" || s.openIndex != blockIndex {
		return
	}
	if arguments == "" {
		return
	}
	s.emit("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": blockIndex,
		"delta": map[string]any{"type": "input_json_delta", "partial_json": arguments},
	})
}

func (s *messagesBridgeStream) finish(stopReason string) {
	if s.finished {
		return
	}
	s.ensureStart()
	s.closeBlock()
	if s.sawTool {
		stopReason = "tool_use"
	}
	s.stopReason = stopReason
	s.emit("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": s.outTokens},
	})
	s.emit("message_stop", map[string]any{"type": "message_stop"})
	s.finished = true
}

func (s *messagesBridgeStream) endCleanly() {
	if s.finished {
		return
	}
	s.finish("end_turn")
}

func (s *messagesBridgeStream) fail(message string) {
	if s.finished {
		return
	}
	s.ensureStart()
	s.emit("error", map[string]any{
		"type":  "error",
		"error": map[string]any{"type": "api_error", "message": message},
	})
	s.finished = true
}

func (s *messagesBridgeStream) ensureStart() {
	if s.started {
		return
	}
	s.started = true
	s.emit("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": s.id, "type": "message", "role": "assistant", "model": s.model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": s.inputTokens, "output_tokens": 0},
		},
	})
}

func (s *messagesBridgeStream) closeBlock() {
	if s.openKind == "" {
		return
	}
	s.emit("content_block_stop", map[string]any{
		"type": "content_block_stop", "index": s.openIndex,
	})
	s.openKind = ""
}

func (s *messagesBridgeStream) emit(event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	s.w.writeFlush([]byte("event: " + event + "\ndata: " + string(data) + "\n\n"))
}
