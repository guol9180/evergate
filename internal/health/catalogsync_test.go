package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"evergate/internal/pool"
	"evergate/internal/store"
)

func TestComputeCatalogDiffRespectsMappingAndDedupes(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	ch := &store.Channel{Models: []string{"a", "b"}, ModelMapping: map[string]string{"b": "b-up"}}

	diff := ComputeCatalogDiff(ch, []string{"a", "b-up", "c", "c", " "}, now)
	if strings.Join(diff.Added, ",") != "c" || len(diff.Removed) != 0 || diff.UpstreamCount != 3 {
		t.Fatalf("diff = %+v", diff)
	}
	if !diff.CheckedAt.Equal(now) {
		t.Fatalf("checked_at = %v", diff.CheckedAt)
	}

	// 上游目录里 b-up 消失 → 渠道声明的 b 疑似下线（按上游名比较）
	diff = ComputeCatalogDiff(ch, []string{"a"}, now)
	if strings.Join(diff.Removed, ",") != "b-up" || len(diff.Added) != 0 {
		t.Fatalf("removed diff = %+v", diff)
	}
}

// 同步只写 catalog_diff，不改渠道模型列表，也不影响 key 状态机。
func TestSyncChannelCatalogPersistsDiffWithoutTouchingModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "keep"}, {"id": "brand-new"}}})
	}))
	t.Cleanup(server.Close)

	db, err := store.Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	master, _ := store.GenerateRandomKey()
	clock := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	pl := pool.NewManager(3, pool.DefaultCooldownSchedule, func() time.Time { return clock })
	ch := &store.Channel{
		Name: "cat", Type: "openai-compatible", BaseURL: server.URL + "/v1",
		Models: []string{"keep", "gone"}, Enabled: true, Status: store.ChannelStatusActive,
	}
	if err := db.Create(ch).Error; err != nil {
		t.Fatal(err)
	}
	cipher, _ := store.Encrypt("sk-cat", master)
	key := &store.ChannelKey{ChannelID: ch.ID, KeyCipher: cipher, Enabled: true, Weight: 1, State: store.KeyStateActive}
	if err := db.Create(key).Error; err != nil {
		t.Fatal(err)
	}
	opt := CatalogSyncOptions{
		DB: db, MasterKey: master, Pool: pl, Client: server.Client(),
		Now: func() time.Time { return clock }, Timeout: func() time.Duration { return 2 * time.Second },
	}

	diff, err := SyncChannelCatalog(context.Background(), opt, ch)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if strings.Join(diff.Added, ",") != "brand-new" || strings.Join(diff.Removed, ",") != "gone" || diff.UpstreamCount != 2 {
		t.Fatalf("diff = %+v", diff)
	}

	var stored store.Channel
	if err := db.First(&stored, ch.ID).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Join(stored.Models, ",") != "keep,gone" {
		t.Fatalf("channel models must not be rewritten: %v", stored.Models)
	}
	if len(stored.CatalogDiff.Added) != 1 || stored.CatalogDiff.Added[0] != "brand-new" || !stored.CatalogDiff.CheckedAt.Equal(clock) {
		t.Fatalf("persisted diff = %+v", stored.CatalogDiff)
	}
	if state := pl.StateMachine().State(key.ID); state != store.KeyStateActive {
		t.Fatalf("catalog sync must not change key state, got %s", state)
	}

	// 没有 key 的渠道：跳过并返回明确错误，不写 diff
	if err := db.Model(&store.ChannelKey{}).Where("id = ?", key.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	pl.Reset(key.ID)
	if _, err := SyncChannelCatalog(context.Background(), opt, ch); err != ErrNoKeyForCatalog {
		t.Fatalf("no-key sync err = %v", err)
	}
}
