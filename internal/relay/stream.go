package relay

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/retry"
	"evergate/internal/router"
	"evergate/internal/store"
)

// keepaliveInterval SSE 空闲心跳间隔。测试会缩短该值以模拟客户端静默超时。
var keepaliveInterval = 5 * time.Second

// streamResponse 处理流式透传：首 chunk 前是可重试窗口，首 chunk 写给客户端后
// 永不重试，只能以 SSE error event 明确上报（任务书 4.5⑥ 红线）。
func (r *Relay) streamResponse(c *gin.Context, resp *http.Response, key *store.ChannelKey, cand router.Candidate, firstToken time.Duration, cancel context.CancelFunc, trace *[]store.RetryAttempt, hit *hitRecord, attemptStart time.Time) attemptOutcome {
	if isResponsesRequest(c) {
		return r.streamResponsesResponse(c, resp, key, cand, firstToken, cancel, trace, hit, hitLoggedModel(c), attemptStart)
	}
	start := attemptStart
	appendTrace := func(errType, errMsg string) {
		*trace = append(*trace, store.RetryAttempt{
			ChannelID: cand.Channel.ID, ChannelName: cand.Channel.Name, KeyID: key.ID,
			Attempt: len(*trace) + 1, ErrorType: errType, ErrorMessage: errMsg,
			ElapsedMS: r.opt.Now().Sub(start).Milliseconds(),
		})
		r.metricAttempt(cand.Channel.ID, cand.UpstreamModel, errType)
	}

	br := bufio.NewReaderSize(resp.Body, 32*1024)

	// === 可重试窗口：读取首个 chunk ===
	var ftFired atomic.Bool
	timer := time.AfterFunc(firstToken, func() { ftFired.Store(true); cancel() })
	first, firstDataLine, err := readFirstSSEDataLine(br)
	timer.Stop()
	if err != nil {
		if ftFired.Load() {
			err = retry.ErrFirstTokenTimeout
		}
		cls := retry.Classify(retry.Result{Err: err})
		r.reportKeyFailure(key.ID, cls, err.Error())
		appendTrace(string(cls.Kind), err.Error())
		return outcomeNextChannel
	}

	// 首 chunk 若是 SSE 错误事件（骑在 200 上的失败，bifrost stream_error 经验），
	// 此时尚未写给客户端，仍在可重试窗口内，按推断状态正常分类换候选。
	if cls, isErr := parseSSEErrorEvent(firstDataLine); isErr {
		appendTrace(string(cls.Kind), "upstream stream error event")
		switch cls.Kind {
		case retry.KindRateLimited, retry.KindAuth:
			r.reportKeyFailure(key.ID, cls, "upstream stream error event")
			return outcomeNextKey
		default:
			r.recordKeyFailure(key.ID, "upstream stream error event")
			return outcomeNextChannel
		}
	}

	// === 红线：首 chunk 即写给客户端，此后不再重试 ===
	r.reportKeySuccess(key.ID)
	appendTrace("", "")
	r.recordHit(c, hit, cand.Channel, key, cand.UpstreamModel, r.opt.Now().Sub(start))

	w := newChatSSEWriter(c.Writer, hitLoggedModel(c))
	w.header()
	w.writeFlush(first)

	stopKeepalive := w.startKeepalive()
	defer stopKeepalive()

	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) > 0 {
			w.writeFlush(line)
			if data, ok := sseData(line); ok {
				parseUsage([]byte(data), hit)
			}
			// 流中途的错误事件：透传给客户端（红线：不重试），解析语义仅用于日志
			if cls, isErr := parseSSEErrorEvent(line); isErr {
				r.opt.Logger.Error("upstream sse error event",
					"kind", string(cls.Kind),
					"channel", cand.Channel.Name,
				)
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return outcomeDone // 正常流结束
			}
			// 首 token 后失败：明确 SSE 错误，不重试、不重复（验收 #4）
			r.opt.Logger.Error("upstream stream interrupted",
				"error", rerr.Error(),
				"channel", cand.Channel.Name,
			)
			errPayload, _ := json.Marshal(map[string]any{
				"error": map[string]any{
					"message": "upstream stream interrupted: " + rerr.Error(),
					"type":    "server_error",
					"code":    "upstream_stream_error",
				},
			})
			w.writeFlush(append(append([]byte("data: "), errPayload...), '\n', '\n'))
			return outcomeDone
		}
	}
}

// readFirstSSEDataLine buffers comments, empty lines, and event preamble until
// the first data line. Only data commits the retry window.
func readFirstSSEDataLine(br *bufio.Reader) (raw, dataLine []byte, err error) {
	for {
		line, readErr := br.ReadBytes('\n')
		raw = append(raw, line...)
		if len(line) > 0 {
			if _, ok := sseData(line); ok {
				return raw, line, nil
			}
		}
		if readErr != nil {
			return raw, nil, readErr
		}
	}
}

// parseSSEErrorEvent 判断一行 SSE 数据是否为上游错误事件（data: {"error":{...}}），
// 是则返回按推断状态得出的分类（bifrost streamErrorStatus 思路）。
func parseSSEErrorEvent(line []byte) (retry.Classification, bool) {
	// 快速路径：绝大多数 chunk 不含 "error"，先做廉价字节检查避免逐 chunk JSON 解析
	if !bytes.Contains(line, []byte(`"error"`)) {
		return retry.Classification{}, false
	}
	s := strings.TrimSpace(string(line))
	if !strings.HasPrefix(s, "data:") {
		return retry.Classification{}, false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(s, "data:"))
	if payload == "" || payload == "[DONE]" {
		return retry.Classification{}, false
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &wire); err != nil {
		return retry.Classification{}, false
	}
	errRaw, ok := wire["error"]
	if !ok {
		return retry.Classification{}, false
	}
	var errObj struct {
		Type string `json:"type"`
		Code string `json:"code"`
	}
	_ = json.Unmarshal(errRaw, &errObj)
	status := retry.StreamErrorStatus(errObj.Type, errObj.Code)
	return classifyStreamError(status, payload), true
}

func classifyStreamError(status int, payload string) retry.Classification {
	cls := retry.Classify(retry.Result{StatusCode: status, BodySnippet: payload})
	// SSE 错误事件没有 Retry-After 头可言：推断为限流时给默认冷却（LiteLLM cooldown 语义）
	if status == 429 && cls.Kind == retry.KindRateLimited && cls.RetryAfter == 0 {
		cls.RetryAfter = 60 * time.Second
	}
	return cls
}

// readAllWithFirstTokenTimeout 非流式读取：在首 token 窗口内读完整个响应体。
func readAllWithFirstTokenTimeout(body io.Reader, ctx context.Context, cancel context.CancelFunc, firstToken time.Duration) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(body)
		done <- result{data, err}
	}()
	select {
	case res := <-done:
		return res.data, res.err
	case <-time.After(firstToken):
		cancel()
		<-done // 等待读 goroutine 因 ctx 取消退出，避免泄漏
		return nil, retry.ErrFirstTokenTimeout
	}
}

// sseWriter 客户端 SSE 写入器：主循环与 keepalive 心跳并发写，需加锁。
type sseWriter struct {
	w         gin.ResponseWriter
	mu        sync.Mutex
	lastWrite time.Time
	interval  time.Duration
	keepalive []byte
}

func newSSEWriter(w gin.ResponseWriter) *sseWriter {
	return &sseWriter{w: w, lastWrite: time.Now(), interval: keepaliveInterval}
}

// newChatSSEWriter 使用合法空增量帧保活。Ktor 等客户端会忽略 SSE 注释，
// 但对 data 事件应用静默超时；空 delta 不产生正文、角色、工具调用或 usage。
func newChatSSEWriter(w gin.ResponseWriter, model string) *sseWriter {
	s := newSSEWriter(w)
	payload, _ := json.Marshal(struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Index int      `json:"index"`
			Delta struct{} `json:"delta"`
		} `json:"choices"`
	}{
		ID:      "evergate-keepalive",
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []struct {
			Index int      `json:"index"`
			Delta struct{} `json:"delta"`
		}{{Index: 0}},
	})
	s.keepalive = append(append([]byte("data: "), payload...), '\n', '\n')
	return s
}

// header 写出 SSE 响应头（仅一次）。
func (s *sseWriter) header() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.w.Header().Set("Content-Type", "text/event-stream")
	s.w.Header().Set("Cache-Control", "no-cache")
	s.w.Header().Set("X-Accel-Buffering", "no")
	s.w.WriteHeader(http.StatusOK)
	s.w.Flush()
}

// writeFlush 写一段并立刻 flush（零缓冲红线）。
func (s *sseWriter) writeFlush(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.w.Write(p)
	s.w.Flush()
	s.lastWrite = time.Now()
}

// startKeepalive 空闲超过间隔时向客户端发送 ": keepalive" 注释，返回停止函数。
func (s *sseWriter) startKeepalive() (stop func()) {
	interval := s.interval
	if interval <= 0 {
		interval = keepaliveInterval
	}
	ticker := time.NewTicker(interval)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				s.mu.Lock()
				idle := time.Since(s.lastWrite)
				if idle >= interval {
					frame := s.keepalive
					if len(frame) == 0 {
						frame = []byte(": keepalive\n\n")
					}
					_, _ = s.w.Write(frame)
					s.w.Flush()
					s.lastWrite = time.Now()
				}
				s.mu.Unlock()
			}
		}
	}()
	return func() {
		ticker.Stop()
		close(done)
	}
}
