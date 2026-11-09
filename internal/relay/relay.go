// Package relay 实现 OpenAI 兼容请求的上游转发：调度内核串联、SSE 零缓冲透传、
// 首 token 超时与重试预算（任务书 4.4 / 4.5⑥）。
package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/config"
	"evergate/internal/pool"
	"evergate/internal/retry"
	"evergate/internal/router"
	"evergate/internal/store"
)

// Options 是 relay 装配依赖。
type Options struct {
	DB        *gorm.DB
	Router    *router.Router
	Pool      *pool.Manager
	MasterKey string
	Defaults  config.Defaults
	Logger    *slog.Logger
	Now       func() time.Time
}

// Relay 转发器。零拷贝路径见 stream.go；本文件负责编排与重试。
type Relay struct {
	opt        Options
	mu         sync.Mutex
	transports map[time.Duration]*http.Transport
}

// New 创建 Relay。
func New(opt Options) *Relay {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	return &Relay{opt: opt, transports: make(map[time.Duration]*http.Transport)}
}

// attemptOutcome 单次尝试的结果，决定下一步走向。
type attemptOutcome int

const (
	outcomeDone        attemptOutcome = iota // 已向客户端写完响应（成功/透传/流结束）
	outcomeNextKey                           // 换下一个 key（同渠道或下一渠道）
	outcomeNextChannel                       // 直接换下一候选渠道
)

// HandleChatCompletions 处理 POST /v1/chat/completions（含 stream:true SSE）。
func (r *Relay) HandleChatCompletions(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "读取请求体失败")
		return
	}
	model, stream, ok := parseChatRequest(body)
	if !ok {
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "请求体必须是含 model 字段的 JSON")
		return
	}

	var channels []*store.Channel
	if err := r.opt.DB.Where("enabled = ?", true).Find(&channels).Error; err != nil {
		writeOpenAIError(c, http.StatusInternalServerError, "server_error", "server_error", "查询渠道失败")
		return
	}
	candidates := r.opt.Router.Route(model, channels)

	d := r.opt.Defaults
	budget := retry.NewBudget(d.RetryBudget.Channels, d.RetryBudget.KeysPerChannel, d.RetryBudget.Total.Duration, r.opt.Now())
	trace := make([]store.RetryAttempt, 0, 4)

loop:
	for ci, cand := range candidates {
		var keys []*store.ChannelKey
		if err := r.opt.DB.Where("channel_id = ? AND enabled = ?", cand.Channel.ID, true).Find(&keys).Error; err != nil {
			continue
		}
		usedKeys := make(map[int64]bool)
		for ki := 1; ki <= d.RetryBudget.KeysPerChannel; ki++ {
			if !budget.Allow(ci+1, ki, r.opt.Now()) {
				break loop // 重试预算耗尽
			}
			key := r.opt.Pool.Acquire(cand.Channel.ID, keys)
			if key == nil || usedKeys[key.ID] {
				continue loop // 渠道无可用 key（或只剩已试过的 key）→ 换下一候选渠道
			}
			usedKeys[key.ID] = true
			out := r.attemptOne(c, cand, key, body, model, stream, &trace)
			switch out {
			case outcomeDone:
				return
			case outcomeNextChannel:
				continue loop
			}
		}
	}

	// 全部渠道耗尽 / 预算用尽 → 503 + OpenAI 错误体（验收 #7）
	msg := "all upstream channels exhausted"
	if n := len(trace); n > 0 {
		last := trace[n-1]
		if last.ErrorMessage != "" {
			msg += ": " + last.ErrorMessage
		}
	}
	r.opt.Logger.Warn("relay exhausted",
		slog.String("model", model),
		slog.Int("attempts", len(trace)),
	)
	writeOpenAIError(c, http.StatusServiceUnavailable, "server_error", "all_channels_exhausted", msg)
}

// HandleModels 处理 GET /v1/models：启用渠道模型去重合并。
func (r *Relay) HandleModels(c *gin.Context) {
	var channels []*store.Channel
	if err := r.opt.DB.Where("enabled = ?", true).Find(&channels).Error; err != nil {
		writeOpenAIError(c, http.StatusInternalServerError, "server_error", "server_error", "查询渠道失败")
		return
	}
	seen := make(map[string]bool)
	for _, ch := range channels {
		for _, m := range ch.Models {
			seen[m] = true
		}
	}
	models := make([]string, 0, len(seen))
	for m := range seen {
		models = append(models, m)
	}
	sortStrings(models)
	data := make([]gin.H, 0, len(models))
	for _, m := range models {
		data = append(data, gin.H{"id": m, "object": "model", "created": 0, "owned_by": "evergate"})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

// attemptOne 执行一次上游尝试并分类处置。
func (r *Relay) attemptOne(c *gin.Context, cand router.Candidate, key *store.ChannelKey, body []byte, model string, stream bool, trace *[]store.RetryAttempt) attemptOutcome {
	ch := cand.Channel
	connect, firstToken, overall := resolveTimeouts(ch, r.opt.Defaults)
	appendTrace := func(status int, errType, errMsg string, attemptStart time.Time) {
		*trace = append(*trace, store.RetryAttempt{
			ChannelID: ch.ID, ChannelName: ch.Name, KeyID: key.ID,
			Attempt: len(*trace) + 1, ErrorType: errType, ErrorMessage: errMsg,
			ElapsedMS: r.opt.Now().Sub(attemptStart).Milliseconds(),
		})
	}

	plainKey, err := store.Decrypt(key.KeyCipher, r.opt.MasterKey)
	if err != nil {
		r.opt.Pool.ReportFailure(key.ID, retry.Classification{Kind: retry.KindAuth})
		start := r.opt.Now()
		appendTrace(0, "decrypt", "上游 key 解密失败（master_key 不匹配或数据损坏）", start)
		return outcomeNextKey
	}

	upBody, err := replaceModel(body, cand.UpstreamModel)
	if err != nil {
		start := r.opt.Now()
		appendTrace(0, "internal", "重构上游请求体失败: "+err.Error(), start)
		return outcomeNextKey
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), overall)
	defer cancel()

	url := strings.TrimSuffix(ch.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(upBody))
	if err != nil {
		start := r.opt.Now()
		appendTrace(0, "internal", err.Error(), start)
		return outcomeNextKey
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+plainKey)
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	client := &http.Client{Transport: r.transport(connect)}
	start := r.opt.Now()
	resp, err := client.Do(req)
	if err != nil {
		cls := retry.Classify(retry.Result{Err: err})
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			cls = retry.Classification{Kind: retry.KindRetryable}
			err = errors.New("overall timeout")
		}
		r.opt.Pool.ReportFailure(key.ID, cls)
		appendTrace(0, string(cls.Kind), err.Error(), start)
		return outcomeNextKey
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		if stream {
			return r.streamResponse(c, resp, key, cand, firstToken, cancel, trace)
		}
		// 非流式：响应到达即首 token，在窗口内读完整体
		data, rerr := readAllWithFirstTokenTimeout(resp.Body, ctx, cancel, firstToken)
		if rerr != nil {
			cls := retry.Classify(retry.Result{Err: rerr})
			r.opt.Pool.ReportFailure(key.ID, cls)
			appendTrace(0, string(cls.Kind), rerr.Error(), start)
			return outcomeNextKey
		}
		r.opt.Pool.ReportSuccess(key.ID)
		appendTrace(http.StatusOK, "", "", start)
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
		c.Data(http.StatusOK, ct, data)
		return outcomeDone
	}

	// 非 200：读错误体分类
	errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	cls := retry.Classify(retry.Result{
		StatusCode:  resp.StatusCode,
		RetryAfter:  retry.ParseRetryAfter(resp.Header.Get("Retry-After"), r.opt.Now()),
		BodySnippet: string(errBody),
	})
	switch cls.Kind {
	case retry.KindRetryable, retry.KindAuth:
		r.opt.Pool.ReportFailure(key.ID, cls)
		msg := string(errBody)
		if len(msg) > 200 {
			msg = msg[:200]
		}
		appendTrace(resp.StatusCode, string(cls.Kind), msg, start)
		return outcomeNextKey
	default:
		// 不可重试 / 内容策略：直接透传给客户端（验收 #7 之外的 4xx 契约）
		appendTrace(resp.StatusCode, string(cls.Kind), "", start)
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
		c.Data(resp.StatusCode, ct, errBody)
		return outcomeDone
	}
}

// transport 按连接超时缓存 Transport（渠道可覆盖 connect 超时）。
func (r *Relay) transport(connect time.Duration) *http.Transport {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.transports[connect]
	if t == nil {
		t = &http.Transport{
			DialContext:         (&net.Dialer{Timeout: connect}).DialContext,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        32,
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     90 * time.Second,
		}
		r.transports[connect] = t
	}
	return t
}

// resolveTimeouts 渠道级覆盖 > 全局默认。
func resolveTimeouts(ch *store.Channel, d config.Defaults) (connect, firstToken, overall time.Duration) {
	connect = d.ConnectTimeout.Duration
	firstToken = d.FirstTokenTimeout.Duration
	overall = d.OverallTimeout.Duration
	if ch.Timeouts.ConnectSec != nil {
		connect = time.Duration(*ch.Timeouts.ConnectSec) * time.Second
	}
	if ch.Timeouts.FirstTokenSec != nil {
		firstToken = time.Duration(*ch.Timeouts.FirstTokenSec) * time.Second
	}
	if ch.Timeouts.OverallSec != nil {
		overall = time.Duration(*ch.Timeouts.OverallSec) * time.Second
	}
	return
}

// parseChatRequest 提取 model 与 stream 标志。
func parseChatRequest(body []byte) (model string, stream bool, ok bool) {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		return "", false, false
	}
	raw, exists := wire["model"]
	if !exists {
		return "", false, false
	}
	if err := json.Unmarshal(raw, &model); err != nil || model == "" {
		return "", false, false
	}
	if s, exists := wire["stream"]; exists {
		_ = json.Unmarshal(s, &stream)
	}
	return model, stream, true
}

// replaceModel 用映射后的上游模型名替换请求体中的 model 字段，其余字段原样保留。
func replaceModel(body []byte, upstreamModel string) ([]byte, error) {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, err
	}
	m, err := json.Marshal(upstreamModel)
	if err != nil {
		return nil, err
	}
	wire["model"] = m
	return json.Marshal(wire)
}

// writeOpenAIError 输出 OpenAI 格式错误体（对外契约）。
func writeOpenAIError(c *gin.Context, status int, typ, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"message": message, "type": typ, "code": code}})
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
