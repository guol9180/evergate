// Package provider 定义跨协议上游适配的公共能力；具体代理编排仍在 relay。
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"evergate/internal/store"
)

const (
	defaultAnthropicVersion = "2023-06-01"
	modelListPageSize       = 100
	maxPages                = 10
	maxModels               = 500
	maxModelListBody        = 4 << 20
)

// HTTPClient 与 net/http.Client 兼容，供测试注入。
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ModelListEndpoint 描述渠道能力对应的模型目录端点与鉴权方式。
type ModelListEndpoint struct {
	Protocol  string
	Path      string
	Authorize func(*http.Request, string)
}

// ModelListEndpointFor 按 Anthropic → Gemini → OpenAI 兼容的优先级选择端点。
func ModelListEndpointFor(ch *store.Channel) ModelListEndpoint {
	switch {
	case ch.SupportsAnthropic:
		return ModelListEndpoint{
			Protocol: "anthropic",
			Path:     "/v1/models",
			Authorize: func(req *http.Request, key string) {
				req.Header.Set("x-api-key", key)
				req.Header.Set("anthropic-version", defaultAnthropicVersion)
			},
		}
	case ch.SupportsGemini:
		return ModelListEndpoint{
			Protocol: "gemini",
			Path:     "/v1beta/models",
			Authorize: func(req *http.Request, key string) {
				req.Header.Set("x-goog-api-key", key)
			},
		}
	default:
		return ModelListEndpoint{
			Protocol: "openai-compatible",
			Path:     "/models",
			Authorize: func(req *http.Request, key string) {
				req.Header.Set("Authorization", "Bearer "+key)
			},
		}
	}
}

// ModelListURL 返回渠道的模型目录地址。OpenCode Go 的 OpenAI 推理基址位于
// /inference/go/openai/v1，但目录固定在同源 /inference/go/v1/models。
func ModelListURL(ch *store.Channel) string {
	base := strings.TrimSuffix(ch.BaseURL, "/")
	endpoint := ModelListEndpointFor(ch)
	if endpoint.Protocol != "openai-compatible" {
		return base + endpoint.Path
	}
	u, err := url.Parse(base)
	if err != nil {
		return base + endpoint.Path
	}
	if suffix := "/openai/v1"; strings.HasSuffix(strings.ToLower(u.Path), suffix) {
		u.Path = u.Path[:len(u.Path)-len(suffix)] + "/v1/models"
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}
	return base + endpoint.Path
}

// ModelListResult 是一次模型目录拉取的有界结果。
type ModelListResult struct {
	Models    []string
	Protocol  string
	Pages     int
	Truncated bool
	// Prices 仅当上游目录带 OpenRouter 风格 pricing.prompt/completion（per token）时填充，键为上游模型名
	Prices map[string]ModelPriceHint
}

// ModelPriceHint 上游目录声明的价格（USD / 百万 token）；缓存价只在目录给出时为正，0 表示未声明。
type ModelPriceHint struct {
	InputMicrosPerMillion      int64 `json:"input_micros_per_million"`
	OutputMicrosPerMillion     int64 `json:"output_micros_per_million"`
	CacheReadMicrosPerMillion  int64 `json:"cache_read_micros_per_million,omitempty"`
	CacheWriteMicrosPerMillion int64 `json:"cache_write_micros_per_million,omitempty"`
}

// parsePerTokenUSD 把 OpenRouter 风格的 per-token 价格字符串换算为微美元 / 百万 token：v × 1e6 token × 1e6 micros。
func parsePerTokenUSD(raw string) (int64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v <= 0 || v > 1 {
		return 0, false
	}
	return int64(math.Round(v * 1e12)), true
}

type modelListPayload struct {
	Data []struct {
		ID      string `json:"id"`
		Pricing *struct {
			Prompt          string `json:"prompt"`
			Completion      string `json:"completion"`
			InputCacheRead  string `json:"input_cache_read"`
			InputCacheWrite string `json:"input_cache_write"`
		} `json:"pricing"`
	} `json:"data"`
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
	HasMore       bool   `json:"has_more"`
	LastID        string `json:"last_id"`
	NextPageToken string `json:"nextPageToken"`
}

// ListModels 读取协议对应的模型目录；最多 10 页、500 个唯一模型。
func ListModels(ctx context.Context, client HTTPClient, ch *store.Channel, key string) (ModelListResult, error) {
	if ch == nil {
		return ModelListResult{}, errors.New("channel is nil")
	}
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := ModelListEndpointFor(ch)
	result := ModelListResult{Protocol: endpoint.Protocol}
	seen := make(map[string]bool)
	cursor := ""

	for page := 1; page <= maxPages; page++ {
		query := make(url.Values)
		switch endpoint.Protocol {
		case "anthropic":
			query.Set("limit", fmt.Sprint(modelListPageSize))
			if cursor != "" {
				query.Set("after_id", cursor)
			}
		case "gemini":
			query.Set("pageSize", fmt.Sprint(modelListPageSize))
			if cursor != "" {
				query.Set("pageToken", cursor)
			}
		}

		rawURL := ModelListURL(ch)
		if len(query) > 0 {
			rawURL += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return result, fmt.Errorf("构造模型列表请求: %w", err)
		}
		endpoint.Authorize(req, key)
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return result, fmt.Errorf("请求模型列表: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxModelListBody+1))
		closeErr := resp.Body.Close()
		if readErr != nil {
			return result, fmt.Errorf("读取模型列表响应: %w", readErr)
		}
		if closeErr != nil {
			return result, fmt.Errorf("关闭模型列表响应: %w", closeErr)
		}
		if len(body) > maxModelListBody {
			return result, fmt.Errorf("模型列表响应超过 %d 字节上限", maxModelListBody)
		}
		result.Pages = page
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			snippet := summarizeModelListBody(body, key)
			if snippet == "" {
				return result, fmt.Errorf("模型列表 GET %s 返回 HTTP %d", endpoint.Path, resp.StatusCode)
			}
			return result, fmt.Errorf("模型列表 GET %s 返回 HTTP %d: %s", endpoint.Path, resp.StatusCode, snippet)
		}

		var payload modelListPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			return result, fmt.Errorf("解析模型列表响应: %w", err)
		}
		pageModels := payloadModels(payload)
		for _, item := range payload.Data {
			id := strings.TrimSpace(item.ID)
			if id == "" || item.Pricing == nil {
				continue
			}
			in, okIn := parsePerTokenUSD(item.Pricing.Prompt)
			out, okOut := parsePerTokenUSD(item.Pricing.Completion)
			if !okIn || !okOut {
				continue
			}
			if result.Prices == nil {
				result.Prices = map[string]ModelPriceHint{}
			}
			hint := ModelPriceHint{InputMicrosPerMillion: in, OutputMicrosPerMillion: out}
			if cacheRead, ok := parsePerTokenUSD(item.Pricing.InputCacheRead); ok {
				hint.CacheReadMicrosPerMillion = cacheRead
			}
			if cacheWrite, ok := parsePerTokenUSD(item.Pricing.InputCacheWrite); ok {
				hint.CacheWriteMicrosPerMillion = cacheWrite
			}
			result.Prices[id] = hint
		}
		for _, model := range pageModels {
			if model == "" || seen[model] {
				continue
			}
			if len(result.Models) >= maxModels {
				result.Truncated = true
				break
			}
			seen[model] = true
			result.Models = append(result.Models, model)
		}
		if result.Truncated {
			break
		}

		switch endpoint.Protocol {
		case "anthropic":
			if !payload.HasMore {
				cursor = ""
				break
			}
			cursor = payload.LastID
			if cursor == "" && len(pageModels) > 0 {
				cursor = pageModels[len(pageModels)-1]
			}
		case "gemini":
			cursor = payload.NextPageToken
		}
		if cursor == "" {
			break
		}
		if page == maxPages {
			result.Truncated = true
		}
	}

	sort.Strings(result.Models)
	return result, nil
}

func payloadModels(payload modelListPayload) []string {
	models := make([]string, 0, len(payload.Data)+len(payload.Models))
	for _, item := range payload.Data {
		if id := strings.TrimSpace(item.ID); id != "" {
			models = append(models, id)
		}
	}
	for _, item := range payload.Models {
		name := strings.TrimSpace(item.Name)
		name = strings.TrimPrefix(name, "models/")
		if name != "" {
			models = append(models, name)
		}
	}
	return models
}

func summarizeModelListBody(body []byte, key string) string {
	snippet := strings.Join(strings.Fields(strings.ToValidUTF8(string(body), " ")), " ")
	if key != "" {
		snippet = strings.ReplaceAll(snippet, key, "***")
	}
	if len(snippet) > 300 {
		snippet = snippet[:300] + "…"
	}
	return snippet
}
