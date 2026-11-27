package relay

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"evergate/internal/router"
	"evergate/internal/store"
)

// bridgeSpec 描述一种非 OpenAI Chat 协议在分派时需要做的桥接动作。
// 协议之间的差异全部收敛在这张表里，dispatchModel 只按表执行，
// 避免每种协议各写一遍结构相同的候选偏好、桥接体缓存与失败分支。
type bridgeSpec struct {
	// match 判断当前请求是否属于该协议；同一请求最多命中一项。
	match func(*gin.Context) bool
	// prepare 在路由前写入协议原始请求体等一次性状态。
	prepare func(c *gin.Context, body []byte)
	// nativeKey 标记本次尝试是否走原生协议，供响应转换阶段读取。
	nativeKey    string
	originalBody func(*gin.Context) []byte

	// 桥接体缓存三件套：进入分派时清空，命中原生渠道时不用，回退到 chat 时写入。
	clearBridge func(*gin.Context)
	bridgeBody  func(*gin.Context) ([]byte, bool)
	setBridge   func(*gin.Context, []byte)

	// convert 把协议原始请求转换为 OpenAI Chat 请求；协议附带的 context 状态由实现自行写入。
	convert func(c *gin.Context, body []byte, model string) ([]byte, error)

	preferNative func([]router.Candidate) []router.Candidate
	anyNative    func([]router.Candidate) bool
	supports     func(*store.Channel) bool

	// countTokens / onlyNative / nativeErr* 约束只能走原生渠道的计次请求。
	countTokens       func(*gin.Context) bool
	onlyNative        func([]router.Candidate) []router.Candidate
	nativeErrType     string
	nativeErrMsg      string
	nativeRequired    func(body []byte) bool
	skipOnConvertFail bool
	convertErrType    string
}

var bridgeSpecs = []bridgeSpec{
	{
		match:        isResponsesRequest,
		prepare:      setResponsesOriginalBody,
		nativeKey:    responsesNativeKey,
		originalBody: responsesOriginalBody,
		clearBridge:  clearResponsesBridgeBody,
		bridgeBody:   responsesBridgeBody,
		setBridge:    setResponsesBridgeBody,
		convert: func(c *gin.Context, body []byte, _ string) ([]byte, error) {
			converted, registry, err := responsesToChat(body)
			if err != nil {
				return nil, err
			}
			c.Set(responsesToolIdentitiesKey, registry)
			return converted, nil
		},
		preferNative:   preferResponsesNative,
		anyNative:      anyResponsesNative,
		supports:       func(ch *store.Channel) bool { return ch.SupportsResponses },
		nativeRequired: responsesNeedsNativeBridge,
		// Responses 桥接失败只影响当前渠道，其余渠道（含原生渠道）仍可尝试。
		skipOnConvertFail: true,
		convertErrType:    "responses_bridge",
	},
	{
		match:        isMessagesRequest,
		nativeKey:    messagesNativeKey,
		originalBody: messagesOriginalBody,
		clearBridge:  clearMessagesBridgeBody,
		bridgeBody:   messagesBridgeBody,
		setBridge:    setMessagesBridgeBody,
		convert: func(_ *gin.Context, body []byte, _ string) ([]byte, error) {
			return messagesToChat(body)
		},
		preferNative:   preferAnthropicNative,
		anyNative:      anyAnthropicNative,
		supports:       func(ch *store.Channel) bool { return ch.SupportsAnthropic },
		countTokens:    isMessagesCountTokens,
		onlyNative:     filterAnthropicNative,
		nativeErrType:  "messages_native_required",
		nativeErrMsg:   "count_tokens can only be routed to channels with native Anthropic support",
		convertErrType: "messages_bridge",
	},
	{
		match:        isGeminiRequest,
		nativeKey:    geminiNativeKey,
		originalBody: geminiOriginalBody,
		clearBridge:  clearGeminiBridgeBody,
		bridgeBody:   geminiBridgeBody,
		setBridge:    setGeminiBridgeBody,
		convert: func(c *gin.Context, body []byte, model string) ([]byte, error) {
			return geminiToChat(body, model, isGeminiStreamRequest(c))
		},
		preferNative:   preferGeminiNative,
		anyNative:      anyGeminiNative,
		supports:       func(ch *store.Channel) bool { return ch.SupportsGemini },
		countTokens:    isGeminiCountTokens,
		onlyNative:     filterGeminiNative,
		nativeErrType:  "gemini_native_required",
		nativeErrMsg:   "countTokens can only be routed to channels with native Gemini support",
		convertErrType: "gemini_bridge",
	},
}

func bridgeSpecFor(c *gin.Context) *bridgeSpec {
	for i := range bridgeSpecs {
		if bridgeSpecs[i].match(c) {
			return &bridgeSpecs[i]
		}
	}
	return nil
}

type bridgePrep struct {
	candidates     []router.Candidate
	bridgeBody     []byte
	nativeRequired bool
	failure        *modelDispatchResult
}

// prepareBridge 把候选渠道调整为优先原生，并在没有原生渠道时预先产出 chat 桥接体。
func (r *Relay) prepareBridge(c *gin.Context, spec *bridgeSpec, candidates []router.Candidate, body []byte, model string) bridgePrep {
	if spec.prepare != nil {
		spec.prepare(c, body)
	}
	spec.clearBridge(c)
	prep := bridgePrep{candidates: spec.preferNative(candidates)}

	if spec.countTokens != nil && spec.countTokens(c) {
		prep.candidates = spec.onlyNative(prep.candidates)
		if len(prep.candidates) == 0 {
			prep.failure = &modelDispatchResult{status: http.StatusBadRequest, errType: spec.nativeErrType, errMsg: spec.nativeErrMsg}
		}
		return prep
	}
	if spec.anyNative(prep.candidates) {
		return prep
	}
	if spec.nativeRequired != nil && spec.nativeRequired(body) {
		prep.nativeRequired = true
		return prep
	}
	converted, err := spec.convert(c, spec.originalBody(c), model)
	if err != nil {
		prep.failure = &modelDispatchResult{status: http.StatusBadRequest, errType: "invalid_request_error", errMsg: err.Error()}
		return prep
	}
	spec.setBridge(c, converted)
	prep.bridgeBody = converted
	return prep
}

// bridgeAttemptBody 选出当前渠道要发送的请求体：原生渠道发协议原始体，其余渠道发 chat 桥接体。
// skip 表示桥接失败应放弃该渠道继续尝试，failure 表示直接终止本次分派。
func (r *Relay) bridgeAttemptBody(c *gin.Context, spec *bridgeSpec, ch *store.Channel, keyID int64, model string, trace *[]store.RetryAttempt) (body []byte, skip bool, failure *modelDispatchResult) {
	if spec.supports(ch) {
		c.Set(spec.nativeKey, true)
		return spec.originalBody(c), false, nil
	}
	converted, cached := spec.bridgeBody(c)
	if !cached {
		var err error
		converted, err = spec.convert(c, spec.originalBody(c), model)
		if err != nil {
			start := r.opt.Now()
			*trace = append(*trace, store.RetryAttempt{
				ChannelID: ch.ID, ChannelName: ch.Name, KeyID: keyID,
				Attempt: len(*trace) + 1, ErrorType: spec.convertErrType, ErrorMessage: err.Error(),
				ElapsedMS: r.opt.Now().Sub(start).Milliseconds(),
			})
			if spec.skipOnConvertFail {
				return nil, true, nil
			}
			return nil, false, &modelDispatchResult{status: http.StatusBadRequest, errType: "invalid_request_error", errMsg: err.Error()}
		}
		spec.setBridge(c, converted)
	}
	c.Set(spec.nativeKey, false)
	return converted, false, nil
}
