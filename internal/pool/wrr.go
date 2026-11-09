package pool

import "evergate/internal/store"

// wrrPicker 平滑加权轮询（nginx 风格）：权重高的 key 均匀领先而非连续出现。
type wrrPicker struct {
	current map[int64]int64
}

func newWRRPicker() *wrrPicker {
	return &wrrPicker{current: make(map[int64]int64)}
}

// pick 从候选中按平滑加权轮询选出下一个 key。
func (w *wrrPicker) pick(keys []*store.ChannelKey) *store.ChannelKey {
	if len(keys) == 0 {
		return nil
	}
	var total int64
	for _, k := range keys {
		wt := int64(keyWeight(k))
		w.current[k.ID] += wt
		total += wt
	}
	var best *store.ChannelKey
	var bestCur int64
	first := true
	for _, k := range keys {
		cur := w.current[k.ID]
		if first || cur > bestCur {
			best, bestCur, first = k, cur, false
		}
	}
	w.current[best.ID] -= total
	return best
}

func keyWeight(k *store.ChannelKey) int {
	if k.Weight <= 0 {
		return 1
	}
	return k.Weight
}
