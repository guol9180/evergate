package pool

import (
	"sync"
	"time"

	"evergate/internal/retry"
	"evergate/internal/store"
)

// Manager key 池调度器：渠道内按权重轮询，跳过 cooldown/disabled；
// cooldown 到期放行单个 half_open 探测请求（任务书 4.5②③）。
type Manager struct {
	mu sync.Mutex
	sm *StateMachine
	rr map[int64]*wrrPicker
}

// NewManager failThreshold/schedule 传参给状态机，now 为可注入时钟。
func NewManager(failThreshold int, schedule []time.Duration, now func() time.Time) *Manager {
	return &Manager{
		sm: NewStateMachine(failThreshold, schedule, now),
		rr: make(map[int64]*wrrPicker),
	}
}

// StateMachine 暴露内部状态机（持久化/恢复用）。
func (m *Manager) StateMachine() *StateMachine { return m.sm }

// UpdateCooldownConfig 热更新 key 状态机配置。
func (m *Manager) UpdateCooldownConfig(failThreshold int, schedule []time.Duration) {
	m.sm.UpdateConfig(failThreshold, schedule)
}

// Acquire 为渠道选择下一个可用 key；无可用 key 返回 nil（视为渠道故障）。
// cooldown 已到期的 key 以 half_open 探测身份参与，同 key 同时只放行 1 个探测。
func (m *Manager) Acquire(channelID int64, keys []*store.ChannelKey) *store.ChannelKey {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.acquireLocked(channelID, keys)
}

// AcquirePreferred 优先返回指定 key（会话亲和）：仅当该 key 仍启用且处于
// active / 可探测状态时命中；否则等价于 Acquire 的普通调度，绝不复活不可用 key。
func (m *Manager) AcquirePreferred(channelID int64, keys []*store.ChannelKey, preferredKeyID int64) *store.ChannelKey {
	m.mu.Lock()
	defer m.mu.Unlock()
	if preferredKeyID != 0 {
		for _, k := range keys {
			if k.ID != preferredKeyID {
				continue
			}
			if eligible, probe := m.keyEligible(k); eligible {
				if probe {
					m.sm.beginProbe(k.ID)
				}
				return k
			}
			break // 粘性 key 存在但不可用 → 回退普通调度
		}
	}
	return m.acquireLocked(channelID, keys)
}

// acquireLocked 是 Acquire 的无锁实现，调用方必须持有 m.mu。
func (m *Manager) acquireLocked(channelID int64, keys []*store.ChannelKey) *store.ChannelKey {
	eligible := make([]*store.ChannelKey, 0, len(keys))
	probeCandidates := make(map[int64]bool)
	for _, k := range keys {
		if ok, probe := m.keyEligible(k); ok {
			eligible = append(eligible, k)
			if probe {
				probeCandidates[k.ID] = true
			}
		}
	}
	if len(eligible) == 0 {
		return nil
	}
	picker := m.rr[channelID]
	if picker == nil {
		picker = newWRRPicker()
		m.rr[channelID] = picker
	}
	picked := picker.pick(eligible)
	if picked != nil && probeCandidates[picked.ID] {
		m.sm.beginProbe(picked.ID)
	}
	return picked
}

// keyEligible 判定 key 是否可参与调度；probe 表示本次会占用 half_open 探测名额。
// 调用方必须持有 m.mu。
func (m *Manager) keyEligible(k *store.ChannelKey) (eligible, probe bool) {
	if !k.Enabled {
		return false, false
	}
	switch m.sm.State(k.ID) {
	case store.KeyStateActive:
		return true, false
	case store.KeyStateCooldown:
		// 到期且探测名额未被占用 → 可作为探测候选
		if m.sm.cooldownExpiredByID(k.ID) && !m.sm.probeOccupied(k.ID) {
			return true, true
		}
	case store.KeyStateHalfOpen:
		if !m.sm.probeOccupied(k.ID) {
			return true, true
		}
	}
	return false, false
}

// ReportSuccess 上报一次成功。
func (m *Manager) ReportSuccess(keyID int64) { m.sm.OnSuccess(keyID) }

// CancelProbe 释放 half_open 探测名额且不改变状态，用于 inconclusive 探测。
func (m *Manager) CancelProbe(keyID int64) { m.sm.ReleaseProbe(keyID) }

// ReportFailure 按错误分类驱动状态机：
// auth → 禁用；rate_limited+Retry-After → 按 Retry-After 冷却；rate_limited → 计失败；
// transient → 计失败（跨请求累积达阈值冷却 key，持续故障的渠道最终 down——任务书语义；
// 单请求内"跳渠道不试同渠道第二把 key"由 relay 层负责——bifrost：服务器过载换 key 无益）；
// model_unavailable / non_retryable / content_policy 不惩罚 key。
func (m *Manager) ReportFailure(keyID int64, c retry.Classification) {
	switch c.Kind {
	case retry.KindAuth:
		m.sm.OnAuthFailure(keyID)
	case retry.KindRateLimited:
		if c.RetryAfter > 0 {
			m.sm.OnRateLimited(keyID, c.RetryAfter)
		} else {
			m.sm.OnFailure(keyID)
		}
	case retry.KindTransient:
		m.sm.OnFailure(keyID)
	default:
		// 请求本身的问题，key 无辜，不改变状态。
	}
}

// Reset 手动重新启用某 key（管理员操作优先）。
func (m *Manager) Reset(keyID int64) { m.sm.Reset(keyID) }

// DeriveChannelStatus 由 key 状态推导渠道健康（任务书 4.5⑦）：
// 无可用 key 或全部不可用 → down；部分不可用 → degraded；否则 active。
func DeriveChannelStatus(keys []*store.ChannelKey, stateFn func(int64) string) string {
	enabled := 0
	available := 0
	cooldownOrDisabled := 0
	for _, k := range keys {
		if !k.Enabled {
			continue
		}
		enabled++
		switch state := stateFn(k.ID); state {
		case store.KeyStateActive, store.KeyStateHalfOpen:
			available++
		default: // cooldown / disabled
			cooldownOrDisabled++
		}
	}
	switch {
	case enabled == 0 || available == 0:
		return store.ChannelStatusDown
	case cooldownOrDisabled > 0:
		return store.ChannelStatusDegraded
	default:
		return store.ChannelStatusActive
	}
}
