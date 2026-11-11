package relay

import "encoding/json"

type usageWire struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	PromptTokensDetails struct {
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
}

func (u usageWire) normalize() (prompt, completion, total, cached, reasoning int64) {
	prompt = firstPositive(u.PromptTokens, u.InputTokens)
	completion = firstPositive(u.CompletionTokens, u.OutputTokens)
	total = firstPositive(u.TotalTokens, prompt+completion)
	cached = firstPositive(u.PromptTokensDetails.CachedTokens, u.InputTokensDetails.CachedTokens)
	reasoning = firstPositive(u.CompletionTokensDetails.ReasoningTokens, u.OutputTokensDetails.ReasoningTokens)
	return
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

func recordUsage(usage usageWire, hit *hitRecord) {
	prompt, completion, total, cached, reasoning := usage.normalize()
	hit.promptTokens = prompt
	hit.completionTokens = completion
	hit.totalTokens = total
	hit.cachedTokens = cached
	hit.reasoningTokens = reasoning
}
