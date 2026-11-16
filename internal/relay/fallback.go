package relay

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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
		writeOpenAIError(c, http.StatusInternalServerError, "server_error", "server_error", err.Error())
		return nil, http.StatusInternalServerError, "server_error", err.Error()
	}

	var channels []*store.Channel
	if err := r.opt.DB.Where("enabled = ?", true).Find(&channels).Error; err != nil {
		writeOpenAIError(c, http.StatusInternalServerError, "server_error", "server_error", "查询渠道失败")
		return nil, http.StatusInternalServerError, "server_error", err.Error()
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
			trace = append(trace, store.RetryAttempt{
				ChannelName: "模型降级", Attempt: len(trace) + 1,
				ErrorType: entry.reason, ErrorMessage: fallbackReasonText(entry.reason),
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
		const msg = "请求携带 previous_response_id/background/store 等 Responses 原生状态字段，但候选渠道均未声明 Responses 能力"
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "responses_native_required", msg)
		return trace, http.StatusBadRequest, "responses_native_required", msg
	}

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
	r.opt.Logger.Warn("relay exhausted", "model", originalModel, "attempts", len(trace))
	writeOpenAIError(c, http.StatusServiceUnavailable, "server_error", "all_channels_exhausted", msg)
	return trace, http.StatusServiceUnavailable, errType, msg
}

func (r *Relay) modelSequence(c *gin.Context, originalModel string) ([]fallbackEntry, error) {
	token := tokenSnapshotFromContext(c)
	if token == nil || token.DisableFallback || r.opt.DB == nil {
		return []fallbackEntry{{model: originalModel}}, nil
	}
	var rule store.ModelFallback
	err := r.opt.DB.Where("model = ?", originalModel).First(&rule).Error
	if err == gorm.ErrRecordNotFound {
		return []fallbackEntry{{model: originalModel}}, nil
	}
	if err != nil {
		return nil, err
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
	if isMessagesRequest(c) {
		clearMessagesBridgeBody(c)
		candidates = preferAnthropicNative(candidates)
		if isMessagesCountTokens(c) {
			candidates = filterAnthropicNative(candidates)
			if len(candidates) == 0 {
				const msg = "count_tokens 只能路由到声明 Anthropic 原生能力的渠道"
				return modelDispatchResult{status: http.StatusBadRequest, errType: "messages_native_required", errMsg: msg}
			}
		} else if !anyAnthropicNative(candidates) {
			converted, cerr := messagesToChat(originalBody)
			if cerr != nil {
				return modelDispatchResult{status: http.StatusBadRequest, errType: "invalid_request_error", errMsg: cerr.Error()}
			}
			bridgeBody = converted
			setMessagesBridgeBody(c, converted)
		}
	}
	if isResponsesRequest(c) {
		setResponsesOriginalBody(c, originalBody)
		clearResponsesBridgeBody(c)
		candidates = preferResponsesNative(candidates)
		if !anyResponsesNative(candidates) {
			if responsesNeedsNativeBridge(originalBody) {
				return modelDispatchResult{nativeRequired: true}
			}
			converted, toolRegistry, cerr := responsesToChat(originalBody)
			if cerr != nil {
				return modelDispatchResult{status: http.StatusBadRequest, errType: "invalid_request_error", errMsg: cerr.Error()}
			}
			bridgeBody = converted
			c.Set(responsesToolIdentitiesKey, toolRegistry)
			setResponsesBridgeBody(c, converted)
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
							*trace = append(*trace, store.RetryAttempt{
								ChannelID: cand.Channel.ID, ChannelName: cand.Channel.Name, KeyID: key.ID,
								Attempt: len(*trace) + 1, ErrorType: "responses_bridge", ErrorMessage: cerr.Error(),
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
			if isMessagesRequest(c) {
				if cand.Channel.SupportsAnthropic {
					attemptBody = messagesOriginalBody(c)
					c.Set(messagesNativeKey, true)
				} else {
					converted, cached := messagesBridgeBody(c)
					if !cached {
						var cerr error
						converted, cerr = messagesToChat(messagesOriginalBody(c))
						if cerr != nil {
							start := r.opt.Now()
							*trace = append(*trace, store.RetryAttempt{
								ChannelID: cand.Channel.ID, ChannelName: cand.Channel.Name, KeyID: key.ID,
								Attempt: len(*trace) + 1, ErrorType: "messages_bridge", ErrorMessage: cerr.Error(),
								ElapsedMS: r.opt.Now().Sub(start).Milliseconds(),
							})
							return modelDispatchResult{status: http.StatusBadRequest, errType: "invalid_request_error", errMsg: cerr.Error()}
						}
						setMessagesBridgeBody(c, converted)
						bridgeBody = converted
					}
					attemptBody = converted
					c.Set(messagesNativeKey, false)
				}
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

func fallbackReasonText(reason string) string {
	if reason == fallbackReasonBudget {
		return "令牌剩余额度低于阈值，按降级链切换模型"
	}
	return "原模型渠道耗尽，按降级链切换模型"
}

func errorKind(errType string) string {
	if errType == "quota_exceeded" {
		return "insufficient_quota"
	}
	return "invalid_request_error"
}
