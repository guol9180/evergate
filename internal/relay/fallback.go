package relay

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/affinity"
	"evergate/internal/config"
	"evergate/internal/reqconfig"
	"evergate/internal/retry"
	"evergate/internal/router"
	"evergate/internal/store"
)

const (
	fallbackReasonBudget    = "budget_threshold"
	fallbackReasonExhausted = "channel_exhausted"
)

type fallbackEntry struct {
	model  string
	reason string
}

type modelDispatchResult struct {
	done            bool
	nativeRequired  bool
	status          int
	errType, errMsg string
}

func (r *Relay) dispatchWithFallbacks(c *gin.Context, hit *hitRecord, body []byte, originalModel string, stream bool, override *reqconfig.Config) ([]store.RetryAttempt, int, string, string) {
	c.Set("evergate.original_model", originalModel)
	entries, err := r.modelSequence(c, originalModel)
	if err != nil {
		r.logInternalError("resolve model fallback sequence failed", err)
		const msg = "failed to resolve model fallback sequence"
		writeOpenAIError(c, http.StatusInternalServerError, "server_error", "model_sequence_failed", msg)
		return nil, http.StatusInternalServerError, "model_sequence_failed", msg
	}

	channels, err := r.enabledChannels()
	if err != nil {
		r.logInternalError("load enabled channels failed", err)
		const msg = "failed to load enabled channels"
		writeOpenAIError(c, http.StatusInternalServerError, "server_error", "channels_unavailable", msg)
		return nil, http.StatusInternalServerError, "channels_unavailable", msg
	}

	d := r.currentDefaults()
	if override.Retry != nil {
		if override.Retry.Channels != nil {
			d.RetryBudget.Channels = *override.Retry.Channels
		}
		if override.Retry.KeysPerChannel != nil {
			d.RetryBudget.KeysPerChannel = *override.Retry.KeysPerChannel
		}
		if override.Retry.Total != nil {
			d.RetryBudget.Total = config.Duration{Duration: *override.Retry.Total}
		}
	}
	deadline := r.opt.Now().Add(d.RetryBudget.Total.Duration)
	if d.RetryBudget.Total.Duration <= 0 {
		deadline = time.Time{}
	}

	trace := make([]store.RetryAttempt, 0, 4)
	nativeRequired := false
	for _, entry := range entries {
		if entry.model != originalModel {
			start := r.opt.Now()
			// 边界只落结构化字段（reason / from / to），展示文案由前端按 reason 本地化，数据层不写中文
			trace = append(trace, store.RetryAttempt{
				Attempt:   len(trace) + 1,
				ErrorType: entry.reason,
				ElapsedMS: r.opt.Now().Sub(start).Milliseconds(),
				Reason:    entry.reason, FromModel: originalModel, ToModel: entry.model,
			})
		}
		c.Set("evergate.model", entry.model)
		result := r.dispatchModel(c, hit, body, originalModel, entry.model, stream, channels, d, deadline, override, &trace)
		if result.done {
			return trace, result.status, result.errType, result.errMsg
		}
		if result.status != 0 {
			writeOpenAIError(c, result.status, errorKind(result.errType), result.errType, result.errMsg)
			return trace, result.status, result.errType, result.errMsg
		}
		nativeRequired = nativeRequired || result.nativeRequired
	}

	if nativeRequired {
		const msg = "request uses native Responses state (previous_response_id/background/store) but no candidate channel supports the Responses API"
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "responses_native_required", msg)
		return trace, http.StatusBadRequest, "responses_native_required", msg
	}

	msg := "all upstream channels exhausted"
	errType := "server_error"
	if n := len(trace); n > 0 {
		last := trace[n-1]
		if last.ErrorMessage != "" && !isInternalTraceType(last.ErrorType) {
			msg += ": " + last.ErrorMessage
		}
		if last.ErrorType != "" {
			errType = last.ErrorType
		}
	}
	r.opt.Logger.Warn("relay exhausted", "model", originalModel, "attempts", len(trace))
	writeOpenAIError(c, http.StatusServiceUnavailable, "server_error", "all_channels_exhausted", msg)
	return trace, http.StatusServiceUnavailable, errType, msg
}

// isInternalTraceType 判断重试轨迹里的错误类型是否来自网关内部（解密 / 转换 / 重构），
// 这类文案不得回显给下游；上游返回的错误消息则允许透出。
func isInternalTraceType(errType string) bool {
	switch errType {
	case "internal", "decrypt", "responses_bridge", "messages_bridge", "gemini_bridge":
		return true
	}
	return false
}

func (r *Relay) modelSequence(c *gin.Context, originalModel string) ([]fallbackEntry, error) {
	token := tokenSnapshotFromContext(c)
	if token == nil || token.DisableFallback || r.opt.DB == nil {
		return []fallbackEntry{{model: originalModel}}, nil
	}
	rule, found, err := r.fallbackRule(originalModel)
	if err != nil {
		return nil, err
	}
	if !found {
		return []fallbackEntry{{model: originalModel}}, nil
	}

	entries := make([]fallbackEntry, 0, len(rule.Fallbacks)+1)
	budgetTrigger := rule.BudgetThreshold > 0 && token.Quota > 0 && quotaRemainingFromContext(c) < rule.BudgetThreshold
	if !budgetTrigger {
		entries = append(entries, fallbackEntry{model: originalModel})
	}
	for _, model := range rule.Fallbacks {
		if len(token.AllowedModels) > 0 && !modelAllowed(model, token.AllowedModels) {
			continue
		}
		reason := fallbackReasonExhausted
		if budgetTrigger && len(entries) == 0 {
			reason = fallbackReasonBudget
		}
		entries = append(entries, fallbackEntry{model: model, reason: reason})
	}
	if len(entries) == 0 {
		entries = append(entries, fallbackEntry{model: originalModel})
	}
	return entries, nil
}

func (r *Relay) dispatchModel(
	c *gin.Context,
	hit *hitRecord, originalBody []byte, originalModel, model string, stream bool,
	channels []*store.Channel, d config.Defaults, deadline time.Time, override *reqconfig.Config,
	trace *[]store.RetryAttempt,
) modelDispatchResult {
	token := tokenSnapshotFromContext(c)
	if token != nil && len(token.AllowedModels) > 0 && !modelAllowed(model, token.AllowedModels) {
		return modelDispatchResult{}
	}

	modelChannels := channels
	if len(override.ChannelIDs) > 0 {
		if model == originalModel {
			filtered, err := filterChannels(channels, model, override.ChannelIDs)
			if err != nil {
				return modelDispatchResult{status: http.StatusBadRequest, errType: "invalid_request_config", errMsg: err.Error()}
			}
			modelChannels = filtered
		} else {
			modelChannels = filterChannelsByIDAndModel(channels, model, override.ChannelIDs)
		}
	}
	candidates := r.opt.Router.Route(model, modelChannels)

	tokenID, _ := tokenFromContext(c)
	session := affinity.NormalizeSession(c.GetHeader("X-Session-Id"))
	var sticky affinity.Entry
	stickyOK := false
	affinityEnabled := override.SessionAffinity == nil || *override.SessionAffinity
	if affinityEnabled && session != "" && tokenID != 0 {
		sticky, stickyOK = r.opt.Affinity.Lookup(tokenID, model, session)
		c.Set(ctxAffinitySession, session)
	}
	if stickyOK {
		candidates = promoteSticky(candidates, sticky)
	}

	bridgeBody := originalBody
	spec := bridgeSpecFor(c)
	if spec != nil {
		prep := r.prepareBridge(c, spec, candidates, originalBody, model)
		if prep.failure != nil {
			return *prep.failure
		}
		if prep.nativeRequired {
			return modelDispatchResult{nativeRequired: true}
		}
		candidates = prep.candidates
		if prep.bridgeBody != nil {
			bridgeBody = prep.bridgeBody
		}
	}

	budget := retry.NewBudget(d.RetryBudget.Channels, d.RetryBudget.KeysPerChannel, d.RetryBudget.Total.Duration, r.opt.Now())
	if !deadline.IsZero() {
		budget.Deadline = deadline
	}

loop:
	for ci, cand := range candidates {
		var keys []*store.ChannelKey
		if err := r.opt.DB.Where("channel_id = ? AND enabled = ?", cand.Channel.ID, true).Find(&keys).Error; err != nil {
			continue
		}
		usedKeys := make(map[int64]bool)
		for ki := 1; ki <= d.RetryBudget.KeysPerChannel; ki++ {
			if !budget.Allow(ci+1, ki, r.opt.Now()) {
				break loop
			}
			preferredKeyID := int64(0)
			if stickyOK && cand.Channel.ID == sticky.ChannelID && ki == 1 {
				preferredKeyID = sticky.KeyID
			}
			key := r.opt.Pool.AcquirePreferred(cand.Channel.ID, keys, preferredKeyID)
			if key == nil || usedKeys[key.ID] {
				continue loop
			}
			usedKeys[key.ID] = true
			attemptBody := bridgeBody
			if spec != nil {
				body, skip, failure := r.bridgeAttemptBody(c, spec, cand.Channel, key.ID, model, trace)
				if failure != nil {
					return *failure
				}
				if skip {
					continue loop
				}
				attemptBody = body
			}
			out := r.attemptOne(c, cand, key, attemptBody, model, stream, trace, hit, override.Timeouts)
			if out == outcomeDone {
				return modelDispatchResult{done: true, status: c.Writer.Status()}
			}
			if out == outcomeNextChannel {
				continue loop
			}
		}
	}
	return modelDispatchResult{}
}

func filterChannelsByIDAndModel(channels []*store.Channel, model string, ids []int64) []*store.Channel {
	byID := make(map[int64]*store.Channel, len(channels))
	for _, ch := range channels {
		byID[ch.ID] = ch
	}
	out := make([]*store.Channel, 0, len(ids))
	for _, id := range ids {
		if ch, ok := byID[id]; ok && router.SupportsModel(ch, model) {
			out = append(out, ch)
		}
	}
	return out
}

func errorKind(errType string) string {
	if errType == "quota_exceeded" {
		return "insufficient_quota"
	}
	return "invalid_request_error"
}
