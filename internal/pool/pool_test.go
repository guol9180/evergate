package pool

import (
	"testing"
	"time"

	"evergate/internal/retry"
	"evergate/internal/store"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func newSM(clock *fakeClock) *StateMachine {
	return NewStateMachine(3, DefaultCooldownSchedule, clock.Now)
}

func TestScheduleFrom(t *testing.T) {
	got := ScheduleFrom(30*time.Second, 30*time.Minute)
	want := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tier[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	t.Run("小base封顶去重", func(t *testing.T) {
		got := ScheduleFrom(time.Second, 2*time.Second)
		if len(got) != 2 || got[0] != time.Second || got[1] != 2*time.Second {
			t.Fatalf("got %v", got)
		}
	})
}

func TestActiveToFailsToCooldown(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sm := newSM(clock)
	// 1、2 次失败保持 active
	sm.OnFailure(1)
	sm.OnFailure(1)
	if sm.State(1) != store.KeyStateActive {
		t.Fatalf("state = %s, want active", sm.State(1))
	}
	// 第 3 次触发 cooldown（30s 档）
	sm.OnFailure(1)
	if sm.State(1) != store.KeyStateCooldown {
		t.Fatalf("state = %s, want cooldown", sm.State(1))
	}
	if sm.cooldownExpiredByID(1) {
		t.Fatal("刚进入冷却不应已到期")
	}
	clock.Advance(29 * time.Second)
	if sm.cooldownExpiredByID(1) {
		t.Fatal("29s 时不应到期")
	}
	clock.Advance(1 * time.Second)
	if !sm.cooldownExpiredByID(1) {
		t.Fatal("30s 时应到期")
	}
}

func TestCooldownExpiryToHalfOpenProbe(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sm := newSM(clock)
	for i := 0; i < 3; i++ {
		sm.OnFailure(1)
	}
	clock.Advance(30 * time.Second)
	if !sm.cooldownExpiredByID(1) {
		t.Fatal("应已到期")
	}
	// 到期后转 half_open 并占用探测名额
	sm.beginProbe(1)
	if sm.State(1) != store.KeyStateHalfOpen {
		t.Fatalf("state = %s, want half_open", sm.State(1))
	}
	if !sm.probeOccupied(1) {
		t.Fatal("探测名额应被占用")
	}
}

func TestHalfOpenSuccessResetsAll(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sm := newSM(clock)
	for i := 0; i < 3; i++ {
		sm.OnFailure(1)
	}
	clock.Advance(30 * time.Second)
	sm.beginProbe(1)
	sm.OnSuccess(1)
	if sm.State(1) != store.KeyStateActive {
		t.Fatalf("state = %s, want active", sm.State(1))
	}
	if sm.probeOccupied(1) {
		t.Fatal("探测名额应释放")
	}
	// 计数清零：需再失败 3 次才冷却
	sm.OnFailure(1)
	sm.OnFailure(1)
	if sm.State(1) != store.KeyStateActive {
		t.Fatalf("重置后 2 次失败不应冷却，state = %s", sm.State(1))
	}
}

func TestHalfOpenFailureEscalatesSchedule(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sm := newSM(clock)
	for i := 0; i < 3; i++ {
		sm.OnFailure(1)
	}
	// 第一次冷却 30s，探测失败 → 下一档 2m
	clock.Advance(30 * time.Second)
	sm.beginProbe(1)
	sm.OnFailure(1)
	if sm.State(1) != store.KeyStateCooldown {
		t.Fatalf("state = %s, want cooldown", sm.State(1))
	}
	clock.Advance(2*time.Minute - time.Second)
	if sm.cooldownExpiredByID(1) {
		t.Fatal("2m 档位不应提前到期")
	}
	clock.Advance(time.Second)
	if !sm.cooldownExpiredByID(1) {
		t.Fatal("2m 档位应到期")
	}
	// 第二次探测失败 → 10m 档
	sm.beginProbe(1)
	sm.OnFailure(1)
	clock.Advance(10 * time.Minute)
	sm.beginProbe(1)
	sm.OnFailure(1)
	// 第三次探测失败 → 封顶 30m
	clock.Advance(30 * time.Minute)
	sm.beginProbe(1)
	sm.OnFailure(1)
	clock.Advance(30*time.Minute - time.Second)
	if sm.cooldownExpiredByID(1) {
		t.Fatal("封顶 30m 档位不应提前到期")
	}
}

func TestStateMachineUpdateConfigHotApplies(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sm := newSM(clock)
	sm.UpdateConfig(1, []time.Duration{20 * time.Millisecond, time.Minute})

	sm.OnFailure(1)
	clock.Advance(15 * time.Millisecond)
	if sm.cooldownExpiredByID(1) {
		t.Fatal("新 20ms 档位未到 15ms 不应过期")
	}
	clock.Advance(5 * time.Millisecond)
	if !sm.cooldownExpiredByID(1) {
		t.Fatal("新 20ms 档位到 20ms 应过期")
	}
	sm.beginProbe(1)
	sm.OnFailure(1)
	if sm.State(1) != store.KeyStateCooldown {
		t.Fatalf("热更新后状态 = %s, want cooldown", sm.State(1))
	}
	wantUntil := time.Unix(0, 0).Add(20*time.Millisecond + time.Minute)
	if until := sm.Snapshot()[0].CooldownUntil; !until.Equal(wantUntil) {
		t.Fatalf("第二档未使用热更新 schedule: %v", until)
	}
}

func TestRateLimitedUsesRetryAfterImmediately(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sm := newSM(clock)
	// 零失败也立即冷却（Retry-After 优先于阈值与默认退避）
	sm.OnRateLimited(1, 60*time.Second)
	if sm.State(1) != store.KeyStateCooldown {
		t.Fatalf("state = %s, want cooldown", sm.State(1))
	}
	clock.Advance(59 * time.Second)
	if sm.cooldownExpiredByID(1) {
		t.Fatal("59s 不应到期")
	}
	clock.Advance(time.Second)
	if !sm.cooldownExpiredByID(1) {
		t.Fatal("60s 应到期")
	}
}

func TestAuthFailureDisables(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sm := newSM(clock)
	sm.OnAuthFailure(1)
	if sm.State(1) != store.KeyStateDisabled {
		t.Fatalf("state = %s, want disabled", sm.State(1))
	}
	// disabled 不随时间恢复
	clock.Advance(24 * time.Hour)
	if sm.cooldownExpiredByID(1) {
		t.Fatal("disabled 不应因时间到期")
	}
	// 手动 Reset 恢复 active
	sm.Reset(1)
	if sm.State(1) != store.KeyStateActive {
		t.Fatalf("state = %s, want active", sm.State(1))
	}
}

func TestSnapshotRestoreRoundtrip(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sm := newSM(clock)
	for i := 0; i < 3; i++ {
		sm.OnFailure(1)
	}
	sm.OnAuthFailure(2)
	snap := sm.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot len = %d, want 2", len(snap))
	}
	// 模拟重启：新状态机从快照恢复
	clock2 := &fakeClock{t: clock.t}
	sm2 := newSM(clock2)
	sm2.Restore(snap)
	if sm2.State(1) != store.KeyStateCooldown {
		t.Fatalf("恢复后 state = %s, want cooldown", sm2.State(1))
	}
	if sm2.State(2) != store.KeyStateDisabled {
		t.Fatalf("恢复后 state = %s, want disabled", sm2.State(2))
	}
	// 恢复的冷却随时间到期
	clock2.Advance(30 * time.Second)
	if !sm2.cooldownExpiredByID(1) {
		t.Fatal("恢复的冷却应在原到期时间到期")
	}
}

func TestSmoothWRRSequence(t *testing.T) {
	// nginx 经典序列：权重 {5,1,1} → a,a,b,a,c,a,a
	mk := func(id int64, w int) *store.ChannelKey { return &store.ChannelKey{ID: id, Weight: w, Enabled: true} }
	keys := []*store.ChannelKey{mk(1, 5), mk(2, 1), mk(3, 1)}
	p := newWRRPicker()
	want := []int64{1, 1, 2, 1, 3, 1, 1}
	for i, w := range want {
		if got := p.pick(keys); got.ID != w {
			t.Fatalf("pick[%d] = %d, want %d", i, got.ID, w)
		}
	}
}

func TestWRRWeightNonPositiveDefaultsOne(t *testing.T) {
	keys := []*store.ChannelKey{{ID: 1, Weight: 0, Enabled: true}, {ID: 2, Weight: -5, Enabled: true}}
	p := newWRRPicker()
	seen := map[int64]bool{}
	for i := 0; i < 10; i++ {
		k := p.pick(keys)
		seen[k.ID] = true
	}
	if !seen[1] || !seen[2] {
		t.Fatalf("权重非正应按 1 轮询，seen=%v", seen)
	}
}

func newMgr(clock *fakeClock) *Manager {
	return NewManager(3, DefaultCooldownSchedule, clock.Now)
}

func TestManagerAcquireSkipsUnavailable(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	m := newMgr(clock)
	keys := []*store.ChannelKey{
		{ID: 1, Enabled: true, Weight: 1},
		{ID: 2, Enabled: false, Weight: 1}, // 管理禁用
	}
	if got := m.Acquire(10, keys); got == nil || got.ID != 1 {
		t.Fatalf("应选中 key1, got %v", got)
	}
	// key1 冷却
	m.ReportFailure(1, retry.Classification{Kind: retry.KindRateLimited})
	m.ReportFailure(1, retry.Classification{Kind: retry.KindRateLimited})
	m.ReportFailure(1, retry.Classification{Kind: retry.KindRateLimited})
	if got := m.Acquire(10, keys); got != nil {
		t.Fatalf("唯一可用 key 冷却时应返回 nil, got %v", got)
	}
	// 到期后作为 half_open 探测放行一次
	clock.Advance(30 * time.Second)
	if got := m.Acquire(10, keys); got == nil || got.ID != 1 {
		t.Fatalf("到期后应放行探测, got %v", got)
	}
	// 探测在途，不再放行
	if got := m.Acquire(10, keys); got != nil {
		t.Fatalf("探测在途不应再放行同 key, got %v", got)
	}
}

func TestManagerReportFailureMapping(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	m := newMgr(clock)
	// auth → disabled
	m.ReportFailure(1, retry.Classification{Kind: retry.KindAuth})
	if m.StateMachine().State(1) != store.KeyStateDisabled {
		t.Fatal("auth 应禁用")
	}
	// 429 + Retry-After → 立即按 60s 冷却
	m.ReportFailure(2, retry.Classification{Kind: retry.KindRateLimited, RetryAfter: 60 * time.Second})
	if m.StateMachine().State(2) != store.KeyStateCooldown {
		t.Fatal("429 应冷却")
	}
	clock.Advance(59 * time.Second)
	if m.StateMachine().cooldownExpiredByID(2) {
		t.Fatal("59s 不应到期")
	}
	// non_retryable / content_policy → 不惩罚
	m.ReportFailure(3, retry.Classification{Kind: retry.KindNonRetryable})
	m.ReportFailure(3, retry.Classification{Kind: retry.KindContentPolicy})
	if m.StateMachine().State(3) != store.KeyStateActive {
		t.Fatal("透传类错误不应惩罚 key")
	}
}

func TestDeriveChannelStatus(t *testing.T) {
	mkKeys := func(states ...string) []*store.ChannelKey {
		keys := []*store.ChannelKey{}
		for i, s := range states {
			k := &store.ChannelKey{ID: int64(i + 1), Enabled: true}
			keys = append(keys, k)
			_ = s
		}
		return keys
	}
	stateFn := func(states map[int64]string) func(int64) string {
		return func(id int64) string { return states[id] }
	}
	cases := []struct {
		name   string
		keys   []*store.ChannelKey
		states map[int64]string
		want   string
	}{
		{"全部active", mkKeys("a", "a"), map[int64]string{1: store.KeyStateActive, 2: store.KeyStateActive}, store.ChannelStatusActive},
		{"部分cooldown", mkKeys("a", "c"), map[int64]string{1: store.KeyStateActive, 2: store.KeyStateCooldown}, store.ChannelStatusDegraded},
		{"全部cooldown", mkKeys("c", "c"), map[int64]string{1: store.KeyStateCooldown, 2: store.KeyStateCooldown}, store.ChannelStatusDown},
		{"全部disabled", mkKeys("d", "d"), map[int64]string{1: store.KeyStateDisabled, 2: store.KeyStateDisabled}, store.ChannelStatusDown},
		{"half_open算可用", mkKeys("h", "c"), map[int64]string{1: store.KeyStateHalfOpen, 2: store.KeyStateCooldown}, store.ChannelStatusDegraded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DeriveChannelStatus(c.keys, stateFn(c.states)); got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
	t.Run("无启用key", func(t *testing.T) {
		keys := []*store.ChannelKey{{ID: 1, Enabled: false}}
		if got := DeriveChannelStatus(keys, func(int64) string { return store.KeyStateActive }); got != store.ChannelStatusDown {
			t.Fatalf("got %s, want down", got)
		}
	})
}
