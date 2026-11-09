package health

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"evergate/internal/pool"
	"evergate/internal/retry"
	"evergate/internal/store"
)

func setup(t *testing.T) (*gorm.DB, *pool.Manager, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	master, _ := store.GenerateRandomKey()
	clock := func() time.Time { return time.Unix(1000000, 0).UTC() }
	m := pool.NewManager(3, pool.DefaultCooldownSchedule, clock)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db, m, master
}

// 验收 #8：重启进程后 cooldown 状态从 DB 恢复。
func TestCooldownPersistRestoreRoundtrip(t *testing.T) {
	db, m, master := setup(t)
	cipher, _ := store.Encrypt("sk-x", master)
	key := &store.ChannelKey{ChannelID: 1, KeyCipher: cipher, Enabled: true, State: store.KeyStateActive}
	if err := db.Create(key).Error; err != nil {
		t.Fatal(err)
	}
	// key1 连续失败 3 次 → cooldown（30s 档）
	for i := 0; i < 3; i++ {
		m.ReportFailure(key.ID, poolTestClassification())
	}
	if m.StateMachine().State(key.ID) != store.KeyStateCooldown {
		t.Fatal("应进入 cooldown")
	}
	// 持久化
	persistOnce(db, m, nil)
	var row store.ChannelKey
	db.First(&row, key.ID)
	if row.State != store.KeyStateCooldown || row.ConsecutiveFails != 3 {
		t.Fatalf("DB 状态未更新: %+v", row)
	}
	if row.CooldownUntil == nil || row.CooldownUntil.Unix() != time.Unix(1000000, 0).Add(30*time.Second).Unix() {
		t.Fatalf("cooldown_until 不正确: %v", row.CooldownUntil)
	}
	// 模拟重启：新 manager 从 DB 恢复
	clock2 := func() time.Time { return time.Unix(1000000, 0).UTC() }
	m2 := pool.NewManager(3, pool.DefaultCooldownSchedule, clock2)
	if err := RestoreCooldowns(db, m2); err != nil {
		t.Fatal(err)
	}
	if m2.StateMachine().State(key.ID) != store.KeyStateCooldown {
		t.Fatal("重启后应恢复 cooldown 状态")
	}
	// 恢复的冷却按原到期时间生效（30s 内不可用）
	if m2.StateMachine().State(key.ID) == store.KeyStateActive {
		t.Fatal("冷却期内不应恢复 active")
	}
}

func TestRestoreSkipsActiveKeys(t *testing.T) {
	db, m, master := setup(t)
	cipher, _ := store.Encrypt("sk-y", master)
	key := &store.ChannelKey{ChannelID: 1, KeyCipher: cipher, Enabled: true, State: store.KeyStateActive}
	db.Create(key)
	if err := RestoreCooldowns(db, m); err != nil {
		t.Fatal(err)
	}
	// active key 无需恢复：快照为空
	if snaps := m.StateMachine().Snapshot(); len(snaps) != 0 {
		t.Fatalf("active key 不应产生快照: %v", snaps)
	}
}

// poolTestClassification 返回计失败的限流分类（无 Retry-After → 走阈值路径）。
func poolTestClassification() retry.Classification {
	return retry.Classification{Kind: retry.KindRateLimited}
}
