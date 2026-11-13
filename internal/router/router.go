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
			out = append(out, Candidate{Channel: ch, UpstreamModel: mapModel(ch.ModelMapping, model)})
		}
		i = j
	}
	return out
}

// shuffleWeighted 同优先级组内加权随机排序（Efraimidis–Spirakis：key = u^(1/w)，降序）。
func (r *Router) shuffleWeighted(group []*store.Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]float64, len(group))
	for i, ch := range group {
		w := float64(ch.Weight)
		if w <= 0 {
			w = 1
		}
		keys[i] = math.Pow(r.rand.Float64(), 1/w)
	}
	sort.SliceStable(group, func(i, j int) bool { return keys[i] > keys[j] })
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

func mapModel(mapping map[string]string, model string) string {
	if v, ok := mapping[model]; ok && v != "" {
		return v
	}
	return model
}
