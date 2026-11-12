// Package relay 实现 OpenAI 兼容请求的上游转发：调度内核串联、SSE 零缓冲透传、
// 首 token 超时与重试预算（任务书 4.4 / 4.5⑥）。
package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"evergate/internal/settings"
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
	LogWriter *LogWriter // 可为 nil（测试中不落日志）
	Settings  *settings.Manager
}

// Relay 转发器。零拷贝流式路径见 stream.go；本文件负责编排与重试。
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
	outcomeNextKey                           // 换下一个 key（同渠道）
	outcomeNextChannel                       // 换下一候选渠道
)

// hitRecord 记录实际命中的渠道/key 与首个 token 延迟（供请求日志）。
type hitRecord struct {
	channelID        int64
	channelName      string
	keyID            int64
	upstreamModel    string
	firstTokenMS     int64
	promptTokens     int64
	completionTokens int64
	totalTokens      int64
	cachedTokens     int64
	reasoningTokens  int64
	usageSeen        bool
}

// HandleChatCompletions 处理 POST /v1/chat/completions（含 stream:true SSE）。
// 无论成败，结束统一异步落一条请求日志（含重试轨迹）。
func (r *Relay) HandleChatCompletions(c *gin.Context) {
	start := r.opt.Now()
	hit := &hitRecord{}
	trace, status, errType, errMsg := r.dispatch(c, hit)
	r.finishRequest(c, start, hit, trace, status, errType, errMsg)
}

// dispatch 执行完整编排，返回（重试轨迹, 最终状态码, 错误类型, 错误消息）。
func (r *Relay) dispatch(c *gin.Context, hit *hitRecord) ([]store.RetryAttempt, int, string, string) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "读取请求体失败")
		return nil, http.StatusBadRequest, "invalid_request_error", "读取请求体失败"
	}
	model, stream, ok := parseChatRequest(body)
	if !ok {
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "请求体必须是含 model 字段的 JSON")
		return nil, http.StatusBadRequest, "invalid_request_error", "缺少 model 字段"
	}
	c.Set("evergate.model", model)
	c.Set("evergate.stream", stream)
	if status, errType, errMsg, ok := r.enforceTokenGovernance(c, model); !ok {
		errKind := "invalid_request_error"
		if errType == "quota_exceeded" {
			errKind = "insufficient_quota"
		}
		writeOpenAIError(c, status, errKind, errType, errMsg)
		return nil, status, errType, errMsg
	}

	var channels []*store.Channel
	if err := r.opt.DB.Where("enabled = ?", true).Find(&channels).Error; err != nil {
		writeOpenAIError(c, http.StatusInternalServerError, "server_error", "server_error", "查询渠道失败")
		return nil, http.StatusInternalServerError, "server_error", err.Error()
	}
	candidates := r.opt.Router.Route(model, channels)
	if isResponsesRequest(c) {
		candidates = preferResponsesNative(candidates)
		if !anyResponsesNative(candidates) {
			if responsesNeedsNativeBridge(body) {
				writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "responses_native_required",
					"请求携带 previous_response_id/background/store 等 Responses 原生状态字段，但候选渠道均未声明 Responses 能力")
				return nil, http.StatusBadRequest, "invalid_request_error", "responses_native_required"
			}
			converted, toolRegistry, cerr := responsesToChat(body)
			if cerr != nil {
				writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", cerr.Error())
				return nil, http.StatusBadRequest, "invalid_request_error", cerr.Error()
			}
			body = converted
			c.Set(responsesToolIdentitiesKey, toolRegistry)
			setResponsesBridgeBody(c, converted)
		} else {
			setResponsesOriginalBody(c, body)
		}
	}

	d := r.currentDefaults()
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
			attemptBody := body
			if isResponsesRequest(c) {
				if cand.Channel.SupportsResponses {
					attemptBody = responsesOriginalBody(c)
					c.Set(responsesNativeKey, true)
				} else {
					converted, cached := responsesBridgeBody(c)
					if !cached {
						var cerr error
						var registry *responsesToolRegistry
						converted, registry, cerr = responsesToChat(responsesOriginalBody(c))
						if cerr != nil {
							start := r.opt.Now()
							trace = append(trace, store.RetryAttempt{
								ChannelID: cand.Channel.ID, ChannelName: cand.Channel.Name, KeyID: key.ID,
								Attempt: len(trace) + 1, ErrorType: "responses_bridge", ErrorMessage: cerr.Error(),
								ElapsedMS: r.opt.Now().Sub(start).Milliseconds(),
							})
							continue loop
						}
						c.Set(responsesToolIdentitiesKey, registry)
						setResponsesBridgeBody(c, converted)
					}
					attemptBody = converted
					c.Set(responsesNativeKey, false)
				}
			}
			out := r.attemptOne(c, cand, key, attemptBody, model, stream, &trace, hit)
			switch out {
			case outcomeDone:
				return trace, c.Writer.Status(), "", ""
			case outcomeNextChannel:
				continue loop
			}
		}
	}

	// 全部渠道耗尽 / 预算用尽 → 503 + OpenAI 错误体（验收 #7）
	msg := "all upstream channels exhausted"
	errType := "server_error"
	if n := len(trace); n > 0 {
		last := trace[n-1]
		if last.ErrorMessage != "" {
			msg += ": " + last.ErrorMessage
		}
		if last.ErrorType != "" {
			errType = last.ErrorType
		}
	}
	r.opt.Logger.Warn("relay exhausted",
		slog.String("model", model),
		slog.Int("attempts", len(trace)),
	)
	writeOpenAIError(c, http.StatusServiceUnavailable, "server_error", "all_channels_exhausted", msg)
	return trace, http.StatusServiceUnavailable, errType, msg
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

// finishRequest 统一处理额度结算与请求日志，Chat/Responses 两条入口不得各自分叉。
func (r *Relay) finishRequest(c *gin.Context, start time.Time, hit *hitRecord, trace []store.RetryAttempt, status int, errType, errMsg string) {
	if status >= http.StatusOK && status < http.StatusBadRequest && errType == "" {
		usageErrType, usageErrMsg := r.settleTokenUsage(c, hit)
		if usageErrType != "" {
			errType, errMsg = usageErrType, usageErrMsg
		}
	}
	if r.opt.LogWriter == nil {
		return
	}
	tokenID, tokenName := tokenFromContext(c)
	total := r.opt.Now().Sub(start)
	r.opt.LogWriter.Submit(&store.RequestLog{
		TokenID: tokenID, TokenName: tokenName,
		ChannelID: hit.channelID, ChannelName: hit.channelName, KeyID: hit.keyID,
		Model: firstNonEmpty(hitLoggedModel(c), ""), UpstreamModel: hit.upstreamModel,
		Stream:     isStreamRequest(c),
		StatusCode: status, ErrorType: errType, ErrorMessage: errMsg,
		RetryTrace:   trace,
		FirstTokenMS: hit.firstTokenMS, TotalMS: total.Milliseconds(),
		PromptTokens: hit.promptTokens, CompletionTokens: hit.completionTokens,
		TotalTokens: hit.totalTokens, CachedTokens: hit.cachedTokens, ReasoningTokens: hit.reasoningTokens,
		IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
}

// enforceTokenGovernance 在调用上游前执行模型白名单与额度预检。
// TokenAuth 已负责身份、停用、过期与 IP 白名单；这里需要请求体中的 model。
func (r *Relay) enforceTokenGovernance(c *gin.Context, model string) (int, string, string, bool) {
	token := tokenSnapshotFromContext(c)
	if token == nil {
		return 0, "", "", true
	}
	if len(token.AllowedModels) > 0 && !modelAllowed(model, token.AllowedModels) {
		return http.StatusForbidden, "model_not_allowed", "model is not allowed for this token", false
	}
	if token.Quota > 0 {
		var used int64
		if err := r.opt.DB.
			Model(&store.Token{}).
			Where("id = ?", token.ID).
			Select("used_quota").
			Scan(&used).Error; err != nil {
			return http.StatusInternalServerError, "server_error", "query token quota failed", false
		}
		if used >= token.Quota {
			return http.StatusTooManyRequests, "quota_exceeded", "token quota exceeded", false
		}
	}
	return 0, "", "", true
}

func modelAllowed(model string, allowed []string) bool {
	for _, item := range allowed {
		if strings.TrimSpace(item) == model {
			return true
		}
	}
	return false
}

// settleTokenUsage 仅在客户端收到 2xx 且上游提供正数 usage 后扣减额度。
// 扣减使用条件 UPDATE 保证并发下 used_quota 不会越过 quota；
// 客户端断开、上游失败或 usage 缺失均不扣，并把原因带回请求日志。
func (r *Relay) settleTokenUsage(c *gin.Context, hit *hitRecord) (string, string) {
	token := tokenSnapshotFromContext(c)
	if token == nil || hit == nil {
		return "", ""
	}
	if err := c.Request.Context().Err(); err != nil {
		return "client_disconnected_no_usage_charge", "client disconnected before settlement; quota not charged"
	}
	charged := hit.promptTokens + hit.completionTokens
	if charged <= 0 {
		if !hit.usageSeen {
			return "usage_missing", "upstream success response did not include usage; quota not charged"
		}
		return "usage_zero", "upstream usage input+output tokens is zero; quota not charged"
	}
	result := r.opt.DB.Model(&store.Token{}).
		Where("id = ? AND (quota = 0 OR used_quota + ? <= quota)", token.ID, charged).
		UpdateColumn("used_quota", gorm.Expr("used_quota + ?", charged))
	if result.Error != nil {
		return "quota_update_failed", "update token quota failed: " + result.Error.Error()
	}
	if result.RowsAffected == 0 {
		return "quota_exceeded", "successful response usage exceeded remaining token quota; quota not charged"
	}
	return "", ""
}

// attemptOne 执行一次上游尝试并分类处置。
func (r *Relay) attemptOne(c *gin.Context, cand router.Candidate, key *store.ChannelKey, body []byte, model string, stream bool, trace *[]store.RetryAttempt, hit *hitRecord) attemptOutcome {
	ch := cand.Channel
	nativeResponses := isResponsesRequest(c) && isResponsesNative(c)
	connect, firstToken, overall := resolveTimeouts(ch, r.currentDefaults())
	appendTrace := func(status int, errType, errMsg string, attemptStart time.Time) {
		*trace = append(*trace, store.RetryAttempt{
			ChannelID: ch.ID, ChannelName: ch.Name, KeyID: key.ID,
			Attempt: len(*trace) + 1, ErrorType: errType, ErrorMessage: errMsg,
			ElapsedMS: r.opt.Now().Sub(attemptStart).Milliseconds(),
		})
	}

	plainKey, err := store.Decrypt(key.KeyCipher, r.opt.MasterKey)
	if err != nil {
		r.reportKeyFailure(key.ID, retry.Classification{Kind: retry.KindAuth}, "上游 key 解密失败（master_key 不匹配或数据损坏）")
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

	upstreamPath := "/chat/completions"
	if nativeResponses {
		upstreamPath = "/responses"
	}
	url := strings.TrimSuffix(ch.BaseURL, "/") + upstreamPath
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
		// 网络/连接/超时类错误：瞬时故障——本请求内跳渠道（不浪费同渠道第二把 key），
		// 但仍计入 key 失败（持续故障最终冷却，见 pool.Manager.ReportFailure）。
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = errors.New("overall timeout")
		}
		r.reportKeyFailure(key.ID, retry.Classification{Kind: retry.KindTransient}, err.Error())
		appendTrace(0, string(retry.KindTransient), err.Error(), start)
		return outcomeNextChannel
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		if stream {
			if nativeResponses {
				return r.streamNativeResponsesResponse(c, resp, key, cand, firstToken, cancel, trace, hit, start)
			}
			return r.streamResponse(c, resp, key, cand, firstToken, cancel, trace, hit, start)
		}
		// 非流式：响应到达即首 token，在窗口内读完整体
		data, rerr := readAllWithFirstTokenTimeout(resp.Body, ctx, cancel, firstToken)
		if rerr != nil {
			cls := retry.Classify(retry.Result{Err: rerr})
			r.reportKeyFailure(key.ID, cls, rerr.Error())
			appendTrace(0, string(cls.Kind), rerr.Error(), start)
			return outcomeNextChannel
		}
		r.reportKeySuccess(key.ID)
		appendTrace(http.StatusOK, "", "", start)
		recordHit(hit, ch, key, cand.UpstreamModel, r.opt.Now().Sub(start))
		parseUsage(data, hit)
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
		if isResponsesRequest(c) && !nativeResponses {
			converted, cerr := convertChatCompletionToResponses(data, model, r.opt.Now(), responsesToolIdentities(c))
			if cerr != nil {
				appendTrace(0, "internal", "转换 Responses 响应失败: "+cerr.Error(), start)
				writeOpenAIError(c, http.StatusBadGateway, "server_error", "responses_conversion_failed", cerr.Error())
				return outcomeDone
			}
			data = converted
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
	case retry.KindRateLimited, retry.KindAuth:
		msg := upstreamErrorMessage(resp.StatusCode, errBody)
		r.reportKeyFailure(key.ID, cls, msg)
		appendTrace(resp.StatusCode, string(cls.Kind), msg, start)
		return outcomeNextKey
	case retry.KindTransient, retry.KindModelUnavailable:
		// 服务器故障：计失败但本请求内换渠道；渠道缺模型：不惩罚 key，换渠道
		msg := upstreamErrorMessage(resp.StatusCode, errBody)
		if cls.Kind == retry.KindTransient {
			r.reportKeyFailure(key.ID, cls, msg)
		} else {
			r.recordKeyFailure(key.ID, msg)
		}
		appendTrace(resp.StatusCode, string(cls.Kind), msg, start)
		return outcomeNextChannel
	default:
		// 不可重试 / 内容策略：直接透传给客户端
		msg := upstreamErrorMessage(resp.StatusCode, errBody)
		r.recordKeyFailure(key.ID, msg)
		appendTrace(resp.StatusCode, string(cls.Kind), msg, start)
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
		c.Data(resp.StatusCode, ct, errBody)
		return outcomeDone
	}
}

func upstreamErrorMessage(status int, body []byte) string {
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = fmt.Sprintf("http %d (empty error body)", status)
	}
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

func (r *Relay) currentDefaults() config.Defaults {
	if r.opt.Settings != nil {
		return r.opt.Settings.Snapshot().ConfigDefaults()
	}
	return r.opt.Defaults
}

// reportKeySuccess 同步驱动状态机并持久化请求级成功统计。
func (r *Relay) reportKeySuccess(keyID int64) {
	r.opt.Pool.ReportSuccess(keyID)
	r.persistKeyAttempt(keyID, true, "")
}

// reportKeyFailure 按分类驱动状态机并持久化请求级失败统计。
func (r *Relay) reportKeyFailure(keyID int64, class retry.Classification, message string) {
	r.opt.Pool.ReportFailure(keyID, class)
	r.persistKeyAttempt(keyID, false, message)
}

// recordKeyFailure 仅记录失败统计，不惩罚 key（模型不存在/内容策略等 key 无辜场景）。
func (r *Relay) recordKeyFailure(keyID int64, message string) {
	r.persistKeyAttempt(keyID, false, message)
}

func (r *Relay) persistKeyAttempt(keyID int64, success bool, message string) {
	if r.opt.DB == nil {
		return
	}
	if len(message) > 300 {
		message = message[:300]
	}
	updates := map[string]any{"last_used_at": r.opt.Now().UTC()}
	if success {
		updates["success_count"] = gorm.Expr("success_count + 1")
		updates["last_error"] = ""
	} else {
		updates["fail_count"] = gorm.Expr("fail_count + 1")
		updates["last_error"] = message
	}
	if err := r.opt.DB.Model(&store.ChannelKey{}).Where("id = ?", keyID).Updates(updates).Error; err != nil {
		if r.opt.Logger != nil {
			r.opt.Logger.Warn("persist key attempt failed", slog.Int64("key_id", keyID), slog.Any("error", err))
		}
	}
}

// recordHit 记录成功命中的渠道/key（供请求日志）。
func recordHit(hit *hitRecord, ch *store.Channel, key *store.ChannelKey, upstreamModel string, firstTokenLatency time.Duration) {
	hit.channelID = ch.ID
	hit.channelName = ch.Name
	hit.keyID = key.ID
	hit.upstreamModel = upstreamModel
	hit.firstTokenMS = firstTokenLatency.Milliseconds()
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

func firstNonEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

// hitLoggedModel 从请求上下文取模型名（dispatch 内解析后写入）。
func hitLoggedModel(c *gin.Context) string {
	v, _ := c.Get("evergate.model")
	s, _ := v.(string)
	return s
}

// isStreamRequest 判断请求是否流式（dispatch 内解析后写入）。
func isStreamRequest(c *gin.Context) bool {
	v, _ := c.Get("evergate.stream")
	b, _ := v.(bool)
	return b
}
