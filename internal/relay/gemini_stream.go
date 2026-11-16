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

// streamNativeGeminiResponse 透传 Gemini SSE（data-only 帧、无 [DONE]、EOF 终止）。
func (r *Relay) streamNativeGeminiResponse(c *gin.Context, resp *http.Response, key *store.ChannelKey, cand router.Candidate, firstToken time.Duration, cancel context.CancelFunc, trace *[]store.RetryAttempt, hit *hitRecord, attemptStart time.Time) attemptOutcome {
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
	if cls, isErr := parseGeminiSSEErrorEvent(first); isErr {
		appendTrace(0, string(cls.Kind), "upstream Gemini stream error event")
		switch cls.Kind {
		case retry.KindRateLimited, retry.KindAuth:
			r.reportKeyFailure(key.ID, cls, "upstream Gemini stream error event")
			return outcomeNextKey
		default:
			r.recordKeyFailure(key.ID, "upstream Gemini stream error event")
			return outcomeNextChannel
		}
	}

	r.reportKeySuccess(key.ID)
	appendTrace(http.StatusOK, "", "")
	r.recordHit(c, hit, cand.Channel, key, cand.UpstreamModel, r.opt.Now().Sub(attemptStart))
	parseGeminiUsage(first.data, hit)

	w := newSSEWriter(c.Writer)
	w.header()
	w.writeFlush(first.raw)
	stopKeepalive := w.startKeepalive()
	defer stopKeepalive()

	for {
		event, rerr := readSSEEvent(br)
		if len(event.raw) > 0 {
			parseGeminiUsage(event.data, hit)
			if cls, isErr := parseGeminiSSEErrorEvent(event); isErr {
				r.opt.Logger.Error("upstream Gemini stream error event",
					"kind", string(cls.Kind),
					"channel", cand.Channel.Name,
				)
			}
			w.writeFlush(event.raw)
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return outcomeDone
			}
			r.opt.Logger.Error("upstream native Gemini stream interrupted",
				"error", rerr.Error(),
				"channel", cand.Channel.Name,
			)
			w.writeFlush(geminiErrorFrame("upstream stream interrupted: " + rerr.Error()))
			return outcomeDone
		}
	}
}

// streamGeminiBridgeResponse 把 Chat Completions SSE 转换为 Gemini data-only SSE。
func (r *Relay) streamGeminiBridgeResponse(c *gin.Context, resp *http.Response, key *store.ChannelKey, cand router.Candidate, firstToken time.Duration, cancel context.CancelFunc, trace *[]store.RetryAttempt, hit *hitRecord, model string, attemptStart time.Time) attemptOutcome {
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
	conv := newGeminiBridgeStream(w, model)
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
			r.opt.Logger.Error("upstream Gemini bridge stream interrupted",
				"error", rerr.Error(),
				"channel", cand.Channel.Name,
			)
			conv.fail("upstream stream interrupted: " + rerr.Error())
			return outcomeDone
		}
	}
}

func parseGeminiSSEErrorEvent(event sseEvent) (retry.Classification, bool) {
	payload := strings.TrimSpace(string(event.data))
	if payload == "" || payload == "[DONE]" {
		return retry.Classification{}, false
	}
	var wire struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &wire); err != nil {
		return retry.Classification{}, false
	}
	if wire.Error.Code == 0 && wire.Error.Status == "" {
		return retry.Classification{}, false
	}
	status := wire.Error.Code
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	return classifyStreamError(status, payload), true
}

type geminiBridgeStream struct {
	w            *sseWriter
	model        string
	tools        map[int]*geminiToolAccum
	usage        geminiUsage
	finishReason string
	finished     bool
}

type geminiToolAccum struct {
	name string
	args string
}

func newGeminiBridgeStream(w *sseWriter, model string) *geminiBridgeStream {
	return &geminiBridgeStream{w: w, model: model, tools: make(map[int]*geminiToolAccum)}
}

func (s *geminiBridgeStream) consume(data []byte, hit *hitRecord) {
	payload := strings.TrimSpace(string(data))
	if payload == "" {
		return
	}
	if payload == "[DONE]" {
		s.finish()
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
		recordUsage(*chunk.Usage, hit)
		candidates := hit.completionTokens
		thoughts := hit.reasoningTokens
		if thoughts > 0 && candidates >= thoughts {
			candidates -= thoughts
		}
		s.usage = geminiUsage{
			PromptTokenCount:     hit.promptTokens,
			CandidatesTokenCount: candidates,
			ThoughtsTokenCount:   thoughts,
			TotalTokenCount:      hit.totalTokens,
		}
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			s.emitParts([]any{map[string]any{"text": choice.Delta.Content}}, "")
		}
		for _, call := range choice.Delta.ToolCalls {
			acc := s.tools[call.Index]
			if acc == nil {
				acc = &geminiToolAccum{}
				s.tools[call.Index] = acc
			}
			if call.Function.Name != "" {
				acc.name = call.Function.Name
			}
			acc.args += call.Function.Arguments
		}
		if choice.FinishReason != "" {
			s.finishReason = geminiFinishReason(choice.FinishReason)
			s.finish()
		}
	}
}

func (s *geminiBridgeStream) finish() {
	if s.finished {
		return
	}
	parts := make([]any, 0, len(s.tools))
	indices := make([]int, 0, len(s.tools))
	for index := range s.tools {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		acc := s.tools[index]
		if acc == nil || acc.name == "" {
			continue
		}
		parts = append(parts, map[string]any{
			"functionCall": map[string]any{
				"name": acc.name,
				"args": messagesToolInput(acc.args),
			},
		})
	}
	reason := s.finishReason
	if reason == "" {
		reason = "STOP"
	}
	s.emitParts(parts, reason)
	s.finished = true
}

func (s *geminiBridgeStream) emitParts(parts []any, finishReason string) {
	candidate := map[string]any{
		"content": map[string]any{"role": "model", "parts": parts},
		"index":   0,
	}
	if finishReason != "" {
		candidate["finishReason"] = finishReason
	}
	frame := map[string]any{"candidates": []any{candidate}, "modelVersion": s.model}
	if finishReason != "" && (s.usage.PromptTokenCount > 0 || s.usage.CandidatesTokenCount > 0 ||
		s.usage.ThoughtsTokenCount > 0 || s.usage.TotalTokenCount > 0) {
		frame["usageMetadata"] = s.usage
	}
	data, err := json.Marshal(frame)
	if err != nil {
		return
	}
	s.w.writeFlush(append(append([]byte("data: "), data...), '\n', '\n'))
}

func (s *geminiBridgeStream) fail(message string) {
	if s.finished {
		return
	}
	s.w.writeFlush(geminiErrorFrame(message))
	s.finished = true
}
