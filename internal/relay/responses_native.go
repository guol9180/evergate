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

type sseEvent struct {
	raw  []byte
	name string
	data []byte
}

func (r *Relay) streamNativeResponsesResponse(c *gin.Context, resp *http.Response, key *store.ChannelKey, cand router.Candidate, firstToken time.Duration, cancel context.CancelFunc, trace *[]store.RetryAttempt, hit *hitRecord, attemptStart time.Time) attemptOutcome {
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
	first, err := readSSEEvent(br)
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
	if cls, isErr := parseResponsesSSEErrorEvent(first); isErr {
		appendTrace(0, string(cls.Kind), "upstream Responses stream error event")
		switch cls.Kind {
		case retry.KindRateLimited, retry.KindAuth:
			r.reportKeyFailure(key.ID, cls, "upstream Responses stream error event")
			return outcomeNextKey
		default:
			r.recordKeyFailure(key.ID, "upstream Responses stream error event")
			return outcomeNextChannel
		}
	}

	r.reportKeySuccess(key.ID)
	appendTrace(http.StatusOK, "", "")
	r.recordHit(c, hit, cand.Channel, key, cand.UpstreamModel, r.opt.Now().Sub(attemptStart))
	parseResponsesEventUsage(first.data, hit)

	w := newSSEWriter(c.Writer)
	w.header()
	w.writeFlush(first.raw)
	stopKeepalive := w.startKeepalive()
	defer stopKeepalive()

	for {
		event, rerr := readSSEEvent(br)
		if len(event.raw) > 0 {
			parseResponsesEventUsage(event.data, hit)
			if cls, isErr := parseResponsesSSEErrorEvent(event); isErr {
				r.opt.Logger.Error("upstream responses stream error event",
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
			r.opt.Logger.Error("upstream native responses stream interrupted",
				"error", rerr.Error(),
				"channel", cand.Channel.Name,
			)
			payload, _ := json.Marshal(map[string]any{
				"type": "response.failed",
				"response": map[string]any{
					"error": map[string]any{
						"code": "upstream_stream_error", "message": "upstream stream interrupted: " + rerr.Error(),
					},
				},
			})
			w.writeFlush([]byte("event: response.failed\ndata: " + string(payload) + "\n\n"))
			return outcomeDone
		}
	}
}

func readSSEEvent(br *bufio.Reader) (sseEvent, error) {
	var event sseEvent
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			event.raw = append(event.raw, line...)
			trimmed := strings.TrimRight(string(line), "\r\n")
			if trimmed == "" {
				return event, nil
			}
			if after, ok := strings.CutPrefix(trimmed, "event:"); ok {
				event.name = strings.TrimSpace(after)
			} else if after, ok := strings.CutPrefix(trimmed, "data:"); ok {
				if len(event.data) > 0 {
					event.data = append(event.data, '\n')
				}
				event.data = append(event.data, []byte(strings.TrimSpace(after))...)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && len(event.raw) > 0 {
				return event, nil
			}
			return event, err
		}
	}
}

func parseResponsesSSEErrorEvent(event sseEvent) (retry.Classification, bool) {
	payload := strings.TrimSpace(string(event.data))
	if payload == "" || payload == "[DONE]" {
		return retry.Classification{}, false
	}
	if event.name != "response.failed" && !strings.Contains(payload, `"response.failed"`) {
		return retry.Classification{}, false
	}
	var wire struct {
		Type  string `json:"type"`
		Error struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"error"`
		Response struct {
			Error struct {
				Type string `json:"type"`
				Code string `json:"code"`
			} `json:"error"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(payload), &wire); err != nil {
		return retry.Classification{Kind: retry.KindTransient}, true
	}
	errType, code := wire.Response.Error.Type, wire.Response.Error.Code
	if errType == "" && code == "" {
		errType, code = wire.Error.Type, wire.Error.Code
	}
	if errType == "" {
		errType = wire.Type
	}
	status := retry.StreamErrorStatus(errType, code)
	return classifyStreamError(status, payload), true
}
