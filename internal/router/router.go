// Package router 实现模型 → 渠道候选列表路由（任务书 4.5①）。
package router

import (
	"math"
	"math/rand"
	"sort"
	"sync"

	"evergate/internal/store"
)

// Router 模型路由器：enabled 且支持该模型的渠道，按 priority 升序分组，
// 同优先级内按 weight 加权随机排序。亲和提升由 relay 在候选列表上完成。
type Router struct {
	mu   sync.Mutex // math/rand.Rand 非并发安全；请求路径会并发调用 Route
	rand *rand.Rand
}

// New 以指定种子创建（同种子路由结果可复现，便于测试）。
func New(seed int64) *Router {
	return &Router{rand: rand.New(rand.NewSource(seed))}
}

// Candidate 单个候选渠道（含模型映射后的上游模型名）。
type Candidate struct {
	Channel       *store.Channel
	UpstreamModel string
}

// Route 返回候选渠道列表（顺序即尝试顺序）。
func (r *Router) Route(model string, channels []*store.Channel) []Candidate {
	matched := make([]*store.Channel, 0, len(channels))
	for _, ch := range channels {
		if ch.Enabled && SupportsModel(ch, model) {
			matched = append(matched, ch)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].Priority < matched[j].Priority
	})
	out := make([]Candidate, 0, len(matched))
	for i := 0; i < len(matched); {
		j := i + 1
		for j < len(matched) && matched[j].Priority == matched[i].Priority {
			j++
		}
		group := matched[i:j]
		r.shuffleWeighted(group)
		for _, ch := range group {
			out = append(out, Candidate{Channel: ch, UpstreamModel: UpstreamModel(ch.ModelMapping, model)})
		}
		i = j
	}
	return out
}

// shuffleWeighted 同优先级组内加权随机排序（Efraimidis–Spirakis：key = u^(1/w)，降序）。
func (r *Router) shuffleWeighted(group []*store.Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	type weightedChannel struct {
		channel *store.Channel
		key     float64
	}
	weighted := make([]weightedChannel, len(group))
	for i, ch := range group {
		w := float64(ch.Weight)
		if w <= 0 {
			w = 1
		}
		weighted[i] = weightedChannel{
			channel: ch,
			key:     math.Pow(r.rand.Float64(), 1/w),
		}
	}
	sort.SliceStable(weighted, func(i, j int) bool { return weighted[i].key > weighted[j].key })
	for i := range weighted {
		group[i] = weighted[i].channel
	}
}

// SupportsModel 判断渠道声明的模型列表是否包含 model。
func SupportsModel(ch *store.Channel, model string) bool {
	if ch == nil {
		return false
	}
	for _, m := range ch.Models {
		if m == model {
			return true
		}
	}
	return false
}

// UpstreamModel 将对外模型名映射为渠道上游模型名；空值保持原模型。
func UpstreamModel(mapping map[string]string, model string) string {
	if v, ok := mapping[model]; ok && v != "" {
		return v
	}
	return model
}

// PublicModelCatalog 返回启用渠道的对外模型目录，按名称排序并去重。
func PublicModelCatalog(channels []*store.Channel) []string {
	seen := make(map[string]bool)
	for _, ch := range channels {
		if ch == nil || !ch.Enabled {
			continue
		}
		for _, model := range ch.Models {
			seen[model] = true
		}
	}
	models := make([]string, 0, len(seen))
	for model := range seen {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}
