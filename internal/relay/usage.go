package relay

import (
	"encoding/json"
)

type usageWire struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	// Anthropic Messages usage：prompt caching 明细（input_tokens 不含这两项）。
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	PromptTokensDetails      struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	InputTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	// DeepSeek 上下文缓存：命中与未命中 token 分列，均含于 prompt_tokens。
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"`
}

// normalizedUsage 归一化后的用量：输入侧拆成普通输入、缓存读、缓存写，Prompt 为三者之和。
// OpenAI / Gemini / DeepSeek 的 cached 含于 prompt；Anthropic 的 input_tokens 不含缓存读写，这里补齐，
// 使各协议的「输入 token」「额度扣减」「成本」口径一致——缓存 token 不再漏计或重复计。
type normalizedUsage struct {
	Prompt, CacheRead, CacheWrite, Completion, Total, Reasoning int64
}

// regularInput 扣除缓存读写后的普通输入；按 Anthropic 形态向下游回写 usage 时使用。
func (n normalizedUsage) regularInput() int64 {
	regular := n.Prompt - n.CacheRead - n.CacheWrite
	if regular < 0 {
		return 0
	}
	return regular
}

// withTotal 上游未给 total 时按输入 + 输出推导；只对完整 usage 帧调用，分片帧的推导值没有意义。
func (n normalizedUsage) withTotal() normalizedUsage {
	if n.Total <= 0 {
		n.Total = n.Prompt + n.Completion
	}
	return n
}

func (u usageWire) normalize() normalizedUsage {
	n := normalizedUsage{
		CacheRead:  firstPositive(u.PromptTokensDetails.CachedTokens, u.InputTokensDetails.CachedTokens, u.PromptCacheHitTokens, u.CacheReadInputTokens),
		CacheWrite: u.CacheCreationInputTokens,
		Completion: firstPositive(u.CompletionTokens, u.OutputTokens),
		Reasoning:  firstPositive(u.CompletionTokensDetails.ReasoningTokens, u.OutputTokensDetails.ReasoningTokens),
	}
	// 输入计数有两种形态：OpenAI Chat / Responses 与 DeepSeek 的缓存含于输入计数，照抄即可；
	// Anthropic Messages 的缓存读写独立于 input_tokens，需要加回——否则缓存 token 不计费也不扣额度。
	anthropicShape := u.PromptTokens <= 0 &&
		u.PromptTokensDetails.CachedTokens <= 0 && u.InputTokensDetails.CachedTokens <= 0 && u.PromptCacheHitTokens <= 0
	n.Prompt = firstPositive(u.PromptTokens, u.InputTokens)
	if anthropicShape {
		n.Prompt += n.CacheRead + n.CacheWrite
	}
	// 上游数字自相矛盾（缓存多于输入）时收敛到输入总量，保证普通输入不为负。
	if n.CacheRead > n.Prompt {
		n.CacheRead = n.Prompt
	}
	if n.CacheWrite > n.Prompt-n.CacheRead {
		n.CacheWrite = n.Prompt - n.CacheRead
	}
	n.Total = u.TotalTokens
	return n
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func parseUsage(data []byte, hit *hitRecord) {
	var top struct {
		Usage *usageWire `json:"usage"`
	}
	if err := json.Unmarshal(data, &top); err != nil || top.Usage == nil {
		return
	}
	recordUsage(*top.Usage, hit)
}

func parseResponsesEventUsage(data []byte, hit *hitRecord) {
	var event struct {
		Response struct {
			Usage *usageWire `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &event); err != nil || event.Response.Usage == nil {
		parseUsage(data, hit)
		return
	}
	recordUsage(*event.Response.Usage, hit)
}

// recordUsage 用一次完整的 usage 覆盖命中记录（非流式响应或流结束帧）。
func recordUsage(usage usageWire, hit *hitRecord) {
	applyUsage(usage.normalize(), hit)
}

func applyUsage(n normalizedUsage, hit *hitRecord) {
	n = n.withTotal()
	hit.usageSeen = true
	hit.promptTokens, hit.completionTokens = n.Prompt, n.Completion
	hit.totalTokens, hit.reasoningTokens = n.Total, n.Reasoning
	hit.cachedTokens, hit.cacheWriteTokens = n.CacheRead, n.CacheWrite
}

// mergeUsage 合并分片到达的 usage：只覆盖本次为正数的字段，后到的完整帧自然胜出。
func mergeUsage(n normalizedUsage, hit *hitRecord) {
	hit.usageSeen = true
	if n.Prompt > 0 {
		hit.promptTokens = n.Prompt
	}
	if n.Completion > 0 {
		hit.completionTokens = n.Completion
	}
	if n.CacheRead > 0 {
		hit.cachedTokens = n.CacheRead
	}
	if n.CacheWrite > 0 {
		hit.cacheWriteTokens = n.CacheWrite
	}
	if n.Reasoning > 0 {
		hit.reasoningTokens = n.Reasoning
	}
	if n.Total > 0 {
		hit.totalTokens = n.Total
	} else {
		hit.totalTokens = hit.promptTokens + hit.completionTokens
	}
}
