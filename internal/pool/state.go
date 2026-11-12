// Package pool 实现 key 池调度与 cooldown 状态机（任务书 4.5②③⑦）。
package pool

import (
	"sync"
	"time"

	"evergate/internal/store"
)

// DefaultCooldownSchedule 退避档位表（任务书 4.5③：30s → 2m → 10m，上限 30m）。
var DefaultCooldownSchedule = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
}

// ScheduleFrom 由配置的 base/max 生成退避档位表（{1,4,20}×base 逐级封顶于 max，
// 与任务书 30s→2m→10m→30m 一致）。
func ScheduleFrom(base, max time.Duration) []time.Duration {
	apply := func(mult time.Duration) time.Duration {
		d := base * mult
		if d > max {
			return max
		}
		return d
	}
	s := []time.Duration{apply(1), apply(4), apply(20), max}
	out := s[:1]
	for _, d := range s[1:] {
		if d > out[len(out)-1] {
			out = append(out, d)
		}
	}
	return out
}

// KeySnapshot 状态机持久化快照（写回 channel_keys 表）。
type KeySnapshot struct {
	KeyID            int64     `json:"key_id"`
	State            string    `json:"state"`
	ConsecutiveFails int       `json:"consecutive_fails"`
	CooldownUntil    time.Time `json:"cooldown_until"`
	Level            int       `json:"level"`
}

// StateMachine 每 key 的 cooldown 状态机（内存权威，定期持久化）。
//
// 状态流转（任务书 4.5③）：
//
//	active --连续失败 N 次--> cooldown(按档位表退避)
//	cooldown --到期--> half_open（放行 1 个真实请求作探测）
//	half_open --成功--> active（计数清零）
//	half_open --失败--> cooldown（退避档位 +1，封顶 30m）
//	429+Retry-After --> cooldown_until = now + Retry-After（优先于默认退避）
//	401/403 --> disabled（不参与退避，管理台标记）
type StateMachine struct {
	mu       sync.Mutex
	failN    int
	schedule []time.Duration
	states   map[int64]*keyState
	now      func() time.Time
}

type keyState struct {
	state         string
	consecFails   int
	cooldownUntil time.Time
	level         int
	probeInFlight bool
}

// NewStateMachine failThreshold 为连续失败阈值，schedule 为退避档位表，now 为可注入时钟。
func NewStateMachine(failThreshold int, schedule []time.Duration, now func() time.Time) *StateMachine {
	if failThreshold < 1 {
		failThreshold = 3
	}
	if len(schedule) == 0 {
		schedule = DefaultCooldownSchedule
	}
	if now == nil {
		now = time.Now
	}
	return &StateMachine{
		failN:    failThreshold,
		schedule: schedule,
		states:   make(map[int64]*keyState),
		now:      now,
	}
}

// UpdateConfig 原子替换失败阈值与退避表；已存在的 level/cooldown 状态保留，
// 新配置从下一次状态流转开始生效。
func (sm *StateMachine) UpdateConfig(failThreshold int, schedule []time.Duration) {
	if failThreshold < 1 {
		failThreshold = 3
	}
	if len(schedule) == 0 {
		schedule = DefaultCooldownSchedule
	}
	schedule = append([]time.Duration(nil), schedule...)
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.failN = failThreshold
	sm.schedule = schedule
}

func (sm *StateMachine) get(keyID int64) *keyState {
	s, ok := sm.states[keyID]
	if !ok {
		s = &keyState{state: store.KeyStateActive}
		sm.states[keyID] = s
	}
	return s
}

// State 返回 key 当前状态（观测用）。
func (sm *StateMachine) State(keyID int64) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.get(keyID).state
}

// OnSuccess 成功：任何状态回到 active，计数与档位清零（含 half_open 探测成功）。
func (sm *StateMachine) OnSuccess(keyID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.get(keyID)
	s.state = store.KeyStateActive
	s.consecFails = 0
	s.cooldownUntil = time.Time{}
	s.level = 0
	s.probeInFlight = false
}

// OnFailure 普通失败（网络/5xx/408/409 等）。
func (sm *StateMachine) OnFailure(keyID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.get(keyID)
	s.consecFails++
	switch s.state {
	case store.KeyStateActive:
		if s.consecFails >= sm.failN {
			sm.enterCooldown(s)
		}
	case store.KeyStateHalfOpen:
		// 探测失败：退避档位 +1 后重新冷却（档位表封顶）。
		s.probeInFlight = false
		sm.enterCooldown(s)
	}
	// cooldown 中的 key 不会被调度，维持原状。
}

// OnRateLimited 429：立即按 Retry-After 冷却（优先于默认退避，不推进档位）。
func (sm *StateMachine) OnRateLimited(keyID int64, retryAfter time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.get(keyID)
	s.consecFails++
	s.probeInFlight = false
	s.state = store.KeyStateCooldown
	s.cooldownUntil = sm.now().Add(retryAfter)
}

// OnAuthFailure 401/403：key 无效/被封，直接 disabled，不参与退避。
func (sm *StateMachine) OnAuthFailure(keyID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.get(keyID)
	s.state = store.KeyStateDisabled
	s.probeInFlight = false
}

// Reset 手动 enable：管理员操作永远优先于自动状态。
func (sm *StateMachine) Reset(keyID int64) {
	sm.OnSuccess(keyID)
}

// enterCooldown 按档位表进入冷却；level 封顶于最后一档。
func (sm *StateMachine) enterCooldown(s *keyState) {
	if s.level > len(sm.schedule)-1 {
		s.level = len(sm.schedule) - 1
	}
	d := sm.schedule[s.level]
	s.cooldownUntil = sm.now().Add(d)
	s.state = store.KeyStateCooldown
	s.level++
}

// cooldownExpiredByID 冷却是否已到期（给 Manager 调度判断用）；未冷却过（零值）恒为未到期。
func (sm *StateMachine) cooldownExpiredByID(keyID int64) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.get(keyID)
	if s.cooldownUntil.IsZero() {
		return false
	}
	return !sm.now().Before(s.cooldownUntil)
}

// probeOccupied half_open 探测名额是否已被占用。
func (sm *StateMachine) probeOccupied(keyID int64) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.get(keyID).probeInFlight
}

// beginProbe 占用 half_open 探测名额；若仍在 cooldown 则先转 half_open。
func (sm *StateMachine) beginProbe(keyID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.get(keyID)
	s.state = store.KeyStateHalfOpen
	s.probeInFlight = true
}

// ReleaseProbe 释放 half_open 探测名额但保留 half_open 状态。
// 用于“探测路径不支持”等 inconclusive 结果：本次探测不能证明 key 好坏，
// 也不能永久占住唯一探测名额。
func (sm *StateMachine) ReleaseProbe(keyID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.get(keyID)
	s.probeInFlight = false
}

// Snapshot 导出全部 key 状态快照（定期持久化到 DB 用）。
func (sm *StateMachine) Snapshot() []KeySnapshot {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	out := make([]KeySnapshot, 0, len(sm.states))
	for id, s := range sm.states {
		out = append(out, KeySnapshot{
			KeyID:            id,
			State:            s.state,
			ConsecutiveFails: s.consecFails,
			CooldownUntil:    s.cooldownUntil,
			Level:            s.level,
		})
	}
	return out
}

// Restore 从 DB 快照恢复状态（重启后冷却不丢，避免瞬间打死坏 key）。
func (sm *StateMachine) Restore(snap []KeySnapshot) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for _, k := range snap {
		sm.states[k.KeyID] = &keyState{
			state:         k.State,
			consecFails:   k.ConsecutiveFails,
			cooldownUntil: k.CooldownUntil,
			level:         k.Level,
		}
	}
}
