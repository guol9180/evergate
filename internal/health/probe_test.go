package health

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"evergate/internal/config"
	"evergate/internal/pool"
	"evergate/internal/settings"
	"evergate/internal/store"
)

type probeEnv struct {
	db       *gorm.DB
	pool     *pool.Manager
	master   string
	setting  *settings.Manager
	server   *httptest.Server
	prober   *Prober
	now      func() time.Time
	requests int64
	mutex    sync.Mutex
}

func newProbeEnv(t *testing.T, status int, delay time.Duration) *probeEnv {
	t.Helper()
	env := &probeEnv{mutex: sync.Mutex{}}
	env.now = func() time.Time { return time.Now().UTC() }
	env.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env.mutex.Lock()
		env.requests++
		env.mutex.Unlock()
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(env.server.Close)

	var err error
	env.db, err = store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(env.db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := env.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	env.master, _ = store.GenerateRandomKey()
	env.pool = pool.NewManager(3, pool.DefaultCooldownSchedule, env.now)
	env.setting, err = settings.Load(env.db, config.Default(), env.now)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	env.prober = NewProber(ProberOptions{
		DB: env.db, MasterKey: env.master, Pool: env.pool,
		Client: env.server.Client(), Now: env.now,
	})
	return env
}

func (e *probeEnv) createChannel(t *testing.T, name string, enabled bool) *store.Channel {
	t.Helper()
	ch := &store.Channel{
		Name: name, Type: "openai-compatible", BaseURL: e.server.URL,
		Models: []string{"test-model"}, Enabled: enabled, Status: store.ChannelStatusActive,
	}
	if err := e.db.Create(ch).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if !enabled {
		// GORM 对带 default:true 的 bool 零值会按默认值插入；
		// 生产停用走 toggle 的 Update，这里同样显式更新以模拟真实停用。
		if err := e.db.Model(ch).Update("enabled", false).Error; err != nil {
			t.Fatalf("disable channel: %v", err)
		}
		ch.Enabled = false
	}
	return ch
}

func (e *probeEnv) createKey(t *testing.T, channelID int64, state string, cooldownUntil *time.Time) *store.ChannelKey {
	t.Helper()
	cipher, err := store.Encrypt("sk-probe", e.master)
	if err != nil {
		t.Fatalf("encrypt key: %v", err)
	}
	key := &store.ChannelKey{
		ChannelID: channelID, KeyCipher: cipher, Enabled: true, Weight: 1,
		State: state, CooldownUntil: cooldownUntil,
	}
	if err := e.db.Create(key).Error; err != nil {
		t.Fatalf("create key: %v", err)
	}
	return key
}

func (e *probeEnv) expiredCooldown(t *testing.T) *time.Time {
	t.Helper()
	at := e.now().Add(-time.Second)
	return &at
}

func TestProbeSuccessRestoresCooldownKey(t *testing.T) {
	env := newProbeEnv(t, http.StatusOK, 0)
	ch := env.createChannel(t, "success", true)
	key := env.createKey(t, ch.ID, store.KeyStateCooldown, env.expiredCooldown(t))
	if err := RestoreCooldowns(env.db, env.pool); err != nil {
		t.Fatalf("restore cooldown: %v", err)
	}

	result, err := env.prober.ProbeChannel(context.Background(), ch, []*store.ChannelKey{key}, time.Second, time.Minute)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.Result != ProbeResultSuccess || result.Status != store.ChannelStatusActive {
		t.Fatalf("result = %#v", result)
	}
	if state := env.pool.StateMachine().State(key.ID); state != store.KeyStateActive {
		t.Fatalf("cooldown key should recover, state = %s", state)
	}
	var row store.ChannelKey
	if err := env.db.First(&row, key.ID).Error; err != nil {
		t.Fatalf("read key: %v", err)
	}
	// State 权威在内存状态机，随后由 StartCooldownPersister 刷回；
	// 探测自身只需立即确认统计与健康字段。
	if row.SuccessCount != 1 || row.LastUsedAt == nil {
		t.Fatalf("key row = %#v", row)
	}
	var channelRow store.Channel
	if err := env.db.First(&channelRow, ch.ID).Error; err != nil {
		t.Fatalf("read channel: %v", err)
	}
	if channelRow.Status != store.ChannelStatusActive || channelRow.LastProbeResult != ProbeResultSuccess ||
		channelRow.LastProbeStatusCode != http.StatusOK || channelRow.LastProbeAt == nil || channelRow.NextProbeAt == nil {
		t.Fatalf("channel health row = %#v", channelRow)
	}
}

func TestProbeFailureKeepsCooldown(t *testing.T) {
	env := newProbeEnv(t, http.StatusServiceUnavailable, 0)
	ch := env.createChannel(t, "failure", true)
	key := env.createKey(t, ch.ID, store.KeyStateCooldown, env.expiredCooldown(t))
	if err := RestoreCooldowns(env.db, env.pool); err != nil {
		t.Fatalf("restore cooldown: %v", err)
	}

	result, err := env.prober.ProbeChannel(context.Background(), ch, []*store.ChannelKey{key}, time.Second, time.Minute)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.Result != ProbeResultFailure || result.ErrorType != "transient" {
		t.Fatalf("failure result = %#v", result)
	}
	if state := env.pool.StateMachine().State(key.ID); state != store.KeyStateCooldown {
		t.Fatalf("failed probe should stay cooldown, state = %s", state)
	}
	var row store.ChannelKey
	env.db.First(&row, key.ID)
	if row.FailCount != 1 || row.LastError == "" {
		t.Fatalf("failure statistics not persisted: %#v", row)
	}
	var channelRow store.Channel
	env.db.First(&channelRow, ch.ID)
	if channelRow.Status != store.ChannelStatusDown || channelRow.LastProbeErrorType != "transient" || channelRow.NextProbeAt == nil {
		t.Fatalf("failure channel row = %#v", channelRow)
	}
}

func TestProbeInconclusiveKeepsHalfOpenAndReleasesSlot(t *testing.T) {
	env := newProbeEnv(t, http.StatusNotFound, 0)
	ch := env.createChannel(t, "inconclusive", true)
	key := env.createKey(t, ch.ID, store.KeyStateCooldown, env.expiredCooldown(t))
	if err := RestoreCooldowns(env.db, env.pool); err != nil {
		t.Fatalf("restore cooldown: %v", err)
	}

	result, err := env.prober.ProbeChannel(context.Background(), ch, []*store.ChannelKey{key}, time.Second, time.Minute)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.Result != ProbeResultInconclusive || result.Status != store.ChannelStatusInconclusive {
		t.Fatalf("inconclusive result = %#v", result)
	}
	if state := env.pool.StateMachine().State(key.ID); state != store.KeyStateHalfOpen {
		t.Fatalf("inconclusive should keep half_open, state = %s", state)
	}
	if picked := env.pool.Acquire(ch.ID, []*store.ChannelKey{key}); picked == nil {
		t.Fatal("inconclusive should release the only half-open probe slot")
	}
	var row store.ChannelKey
	env.db.First(&row, key.ID)
	if row.SuccessCount != 0 || row.FailCount != 0 {
		t.Fatalf("inconclusive should not change request statistics: %#v", row)
	}
}

func TestSchedulerSkipsDisabledChannel(t *testing.T) {
	env := newProbeEnv(t, http.StatusOK, 0)
	enabled := env.createChannel(t, "enabled", true)
	env.createKey(t, enabled.ID, store.KeyStateActive, nil)
	disabled := env.createChannel(t, "disabled", false)
	env.createKey(t, disabled.ID, store.KeyStateActive, nil)

	s := &scheduler{prober: env.prober, opt: SchedulerOptions{
		DB: env.db, MasterKey: env.master, Pool: env.pool,
		Settings: env.setting, Client: env.server.Client(), Logger: slog.Default(), Now: env.now,
	}}
	s.runOnce(context.Background())

	var row store.Channel
	if err := env.db.First(&row, disabled.ID).Error; err != nil {
		t.Fatalf("read disabled channel: %v", err)
	}
	if row.LastProbeAt != nil {
		t.Fatalf("disabled channel should not be probed: %#v", row)
	}
}

func TestSchedulerMarksNoKeyChannelDown(t *testing.T) {
	env := newProbeEnv(t, http.StatusOK, 0)
	ch := env.createChannel(t, "no-key", true)

	s := &scheduler{prober: env.prober, opt: SchedulerOptions{
		DB: env.db, MasterKey: env.master, Pool: env.pool,
		Settings: env.setting, Client: env.server.Client(), Logger: slog.Default(), Now: env.now,
	}}
	s.runOnce(context.Background())

	var row store.Channel
	if err := env.db.First(&row, ch.ID).Error; err != nil {
		t.Fatalf("read channel: %v", err)
	}
	if row.Status != store.ChannelStatusDown || row.NextProbeAt != nil {
		t.Fatalf("no-key channel should be down and not scheduled: %#v", row)
	}
	env.mutex.Lock()
	requests := env.requests
	env.mutex.Unlock()
	if requests != 0 {
		t.Fatalf("no-key channel should not call upstream, requests = %d", requests)
	}
}

func TestTimerWaitFireChangeAndCancel(t *testing.T) {
	if !timerWait(context.Background(), 5*time.Millisecond, nil) {
		t.Fatal("timer should fire after interval")
	}
	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	if timerWait(context.Background(), time.Minute, changes) {
		t.Fatal("settings change should rebuild timer instead of firing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if timerWait(ctx, time.Minute, nil) {
		t.Fatal("canceled scheduler should stop waiting")
	}
}

func TestSchedulerConcurrencyLimit(t *testing.T) {
	env := newProbeEnv(t, http.StatusOK, 25*time.Millisecond)
	for i := 0; i < 4; i++ {
		ch := env.createChannel(t, "concurrent-"+string(rune('A'+i)), true)
		env.createKey(t, ch.ID, store.KeyStateActive, nil)
	}
	next := env.setting.Snapshot()
	next.HealthProbeConcurrency = 2
	if _, err := env.setting.Update(next); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	current := 0
	peak := 0
	env.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env.mutex.Lock()
		current++
		if current > peak {
			peak = current
		}
		env.mutex.Unlock()
		time.Sleep(20 * time.Millisecond)
		env.mutex.Lock()
		current--
		env.mutex.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	s := &scheduler{prober: env.prober, opt: SchedulerOptions{
		DB: env.db, MasterKey: env.master, Pool: env.pool,
		Settings: env.setting, Client: env.server.Client(), Logger: slog.Default(), Now: env.now,
	}}
	s.runOnce(context.Background())
	env.mutex.Lock()
	finalPeak := peak
	env.mutex.Unlock()
	if finalPeak > 2 {
		t.Fatalf("peak concurrency = %d, want <= 2", finalPeak)
	}
}

func TestStartProbesRebuildsWaitOnSettingsChange(t *testing.T) {
	env := newProbeEnv(t, http.StatusOK, 0)
	ch := env.createChannel(t, "scheduler", true)
	env.createKey(t, ch.ID, store.KeyStateActive, nil)

	intervals := make(chan time.Duration, 2)
	stop := StartProbes(SchedulerOptions{
		DB: env.db, MasterKey: env.master, Pool: env.pool, Settings: env.setting,
		Client: env.server.Client(), Logger: slog.Default(), Now: env.now,
		WaitFunc: func(ctx context.Context, interval time.Duration, changes <-chan struct{}) bool {
			select {
			case intervals <- interval:
			case <-ctx.Done():
				return false
			}
			select {
			case <-changes:
				return false
			case <-ctx.Done():
				return false
			}
		},
	})
	defer stop()

	first := <-intervals
	next := env.setting.Snapshot()
	next.HealthProbeIntervalMS = 20_000
	if _, err := env.setting.Update(next); err != nil {
		t.Fatalf("update interval: %v", err)
	}
	second := <-intervals
	if second != 20*time.Second || first == second {
		t.Fatalf("wait interval should rebuild: first=%s second=%s", first, second)
	}
	env.mutex.Lock()
	requests := env.requests
	env.mutex.Unlock()
	if requests < 1 {
		t.Fatal("startup probe should run once")
	}
}
