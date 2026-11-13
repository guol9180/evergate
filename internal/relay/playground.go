package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"evergate/internal/store"
)

// ctxPlayground 标记管理台 Playground 来源（请求日志 Source=playground）。
const ctxPlayground = "evergate.playground"

const (
	playgroundMaxBody   = 64 << 10
	playgroundMaxPrompt = 16000
	playgroundMaxSystem = 8000
	playgroundMaxModel  = 100
)

type playgroundRequest struct {
	Protocol string `json:"protocol"`
	Model    string `json:"model"`
	System   string `json:"system"`
	Prompt   string `json:"prompt"`
	Stream   bool   `json:"stream"`
}

// playgroundMeta 是管理台调试面板展示的命中链路摘要；不含任何 key 明文。
type playgroundMeta struct {
	Protocol      string               `json:"protocol"`
	Model         string               `json:"model"`
	Status        int                  `json:"status"`
	ChannelID     int64                `json:"channel_id"`
	ChannelName   string               `json:"channel_name"`
	UpstreamModel string               `json:"upstream_model"`
	FirstTokenMS  int64                `json:"first_token_ms"`
	TotalMS       int64                `json:"total_ms"`
	Usage         playgroundUsage      `json:"usage"`
	RetryTrace    []store.RetryAttempt `json:"retry_trace"`
	ErrorType     string               `json:"error_type"`
	ErrorMessage  string               `json:"error_message"`
}

type playgroundUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	CachedTokens     int64 `json:"cached_tokens"`
	ReasoningTokens  int64 `json:"reasoning_tokens"`
	UsageSeen        bool  `json:"usage_seen"`
}

// HandlePlayground 执行管理台调试请求：复用 relay 调度内核，但不经过下游令牌鉴权，
// 仅由 admin 会话中间件保护（路由注册见 admin.RegisterRoutes）。
func (r *Relay) HandlePlayground(c *gin.Context) {
	req, err := parsePlaygroundRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": err.Error()})
		return
	}
	body, err := buildPlaygroundBody(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": err.Error()})
		return
	}

	writer := newPlaygroundWriter(c.Writer)
	inner, _ := gin.CreateTestContext(writer)
	inner.Request = c.Request.Clone(c.Request.Context())
	inner.Request.Body = io.NopCloser(bytes.NewReader(body))
	inner.Request.ContentLength = int64(len(body))
	inner.Request.Header.Set("Content-Type", "application/json")
	inner.Set(ctxPlayground, true)
	if req.Protocol == "responses" {
		inner.Set("evergate.protocol", responsesProtocol)
		setResponsesOriginalBody(inner, body)
	}

	start := r.opt.Now()
	hit := &hitRecord{}
	trace, status, errType, errMsg := r.dispatch(inner, hit)
	elapsed := r.opt.Now().Sub(start)
	r.finishRequest(inner, start, hit, trace, status, errType, errMsg)

	meta := playgroundMeta{
		Protocol: req.Protocol, Model: req.Model, Status: status,
		ChannelID: hit.channelID, ChannelName: hit.channelName,
		UpstreamModel: hit.upstreamModel, FirstTokenMS: hit.firstTokenMS,
		TotalMS: elapsed.Milliseconds(), RetryTrace: trace,
		ErrorType: errType, ErrorMessage: errMsg,
		Usage: playgroundUsage{
			PromptTokens: hit.promptTokens, CompletionTokens: hit.completionTokens,
			TotalTokens: hit.totalTokens, CachedTokens: hit.cachedTokens,
			ReasoningTokens: hit.reasoningTokens, UsageSeen: hit.usageSeen,
		},
	}
	if trace == nil {
		meta.RetryTrace = []store.RetryAttempt{}
	}

	if writer.streaming {
		writePlaygroundMeta(c.Writer, meta)
		return
	}

	// 非流式（或流式请求在首包前失败）：返回 admin API 统一信封。
	var response any = string(writer.body.Bytes())
	if json.Valid(writer.body.Bytes()) {
		response = json.RawMessage(append([]byte(nil), writer.body.Bytes()...))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"response": response, "meta": meta}})
}

func parsePlaygroundRequest(c *gin.Context) (playgroundRequest, error) {
	var req playgroundRequest
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, playgroundMaxBody+1))
	if err != nil {
		return req, fmt.Errorf("读取请求体失败")
	}
	if len(data) > playgroundMaxBody {
		return req, fmt.Errorf("请求体超过 %dKB 上限", playgroundMaxBody/1024)
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return req, fmt.Errorf("请求体必须是合法 JSON")
	}
	if req.Protocol == "" {
		req.Protocol = "chat"
	}
	if req.Protocol != "chat" && req.Protocol != "responses" {
		return req, fmt.Errorf("protocol 仅支持 chat 或 responses")
	}
	req.Model = strings.TrimSpace(req.Model)
	req.System = strings.TrimSpace(req.System)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Model == "" || len(req.Model) > playgroundMaxModel {
		return req, fmt.Errorf("model 不能为空且不超过 %d 字符", playgroundMaxModel)
	}
	if req.Prompt == "" {
		return req, fmt.Errorf("prompt 不能为空")
	}
	if len(req.Prompt) > playgroundMaxPrompt {
		return req, fmt.Errorf("prompt 超过 %d 字符上限", playgroundMaxPrompt)
	}
	if len(req.System) > playgroundMaxSystem {
		return req, fmt.Errorf("system 超过 %d 字符上限", playgroundMaxSystem)
	}
	return req, nil
}

func buildPlaygroundBody(req playgroundRequest) ([]byte, error) {
	if req.Protocol == "responses" {
		payload := map[string]any{"model": req.Model, "input": req.Prompt, "stream": req.Stream}
		if req.System != "" {
			payload["instructions"] = req.System
		}
		return json.Marshal(payload)
	}
	messages := make([]map[string]string, 0, 2)
	if req.System != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.System})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.Prompt})
	return json.Marshal(map[string]any{"model": req.Model, "messages": messages, "stream": req.Stream})
}

func writePlaygroundMeta(w gin.ResponseWriter, meta playgroundMeta) {
	data, err := json.Marshal(meta)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: evergate.meta\ndata: %s\n\n", data)
	w.Flush()
}

// playgroundWriter 把 relay 输出桥接到管理台响应：
//   - SSE（Content-Type: text/event-stream）实时转发，保证逐段渲染；
//   - 其他响应先缓冲，结束后由 HandlePlayground 包成 admin 统一信封。
//
// 状态码、大小与 Written 均为包装器自身语义，避免 relay 读到 admin writer 的状态。
type playgroundWriter struct {
	gin.ResponseWriter
	header    http.Header
	body      bytes.Buffer
	streaming bool
	decided   bool
	status    int
}

func newPlaygroundWriter(admin gin.ResponseWriter) *playgroundWriter {
	return &playgroundWriter{ResponseWriter: admin, header: make(http.Header), status: http.StatusOK}
}

// Header 使用独立 header map：relay 的 Content-Length/Content-Type 不得污染管理台响应。
func (w *playgroundWriter) Header() http.Header { return w.header }

func (w *playgroundWriter) WriteHeader(code int) {
	w.status = code
	if !w.decided {
		w.decided = true
		w.streaming = strings.HasPrefix(w.header.Get("Content-Type"), "text/event-stream")
	}
	if w.streaming {
		for key, values := range w.header {
			for _, value := range values {
				w.ResponseWriter.Header().Add(key, value)
			}
		}
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *playgroundWriter) WriteHeaderNow() {
	if w.streaming {
		w.ResponseWriter.WriteHeaderNow()
	}
}

func (w *playgroundWriter) Write(data []byte) (int, error) {
	if !w.decided {
		w.WriteHeader(w.status)
	}
	if w.streaming {
		return w.ResponseWriter.Write(data)
	}
	return w.body.Write(data)
}

func (w *playgroundWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *playgroundWriter) Flush() {
	if w.streaming {
		w.ResponseWriter.Flush()
	}
}

func (w *playgroundWriter) Status() int { return w.status }

func (w *playgroundWriter) Size() int {
	if w.streaming {
		return w.ResponseWriter.Size()
	}
	return w.body.Len()
}

func (w *playgroundWriter) Written() bool { return w.decided }

var _ gin.ResponseWriter = (*playgroundWriter)(nil)
