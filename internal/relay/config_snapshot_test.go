package relay

import (
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"evergate/internal/store"
)

// 快照必须挡住直接改库，直到显式失效才重建——这是热路径不查配置的前提。
func TestConfigSnapshotCachesUntilInvalidated(t *testing.T) {
	env := newEnv(t)
	env.addChannel(t, "first", "http://127.0.0.1:9/v1", 0, []string{"m1"}, nil, "sk")

	channels, err := env.enabledChannels()
	if err != nil || len(channels) != 1 {
		t.Fatalf("initial snapshot = %d channels, err %v", len(channels), err)
	}

	second := &store.Channel{Name: "second", Type: "openai-compatible", BaseURL: "http://127.0.0.1:9/v1", Models: []string{"m2"}, Enabled: true}
	if err := env.db.Create(second).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if cached, _ := env.enabledChannels(); len(cached) != 1 {
		t.Fatalf("snapshot should stay stale until invalidated, got %d", len(cached))
	}

	env.InvalidateConfig()
	if refreshed, _ := env.enabledChannels(); len(refreshed) != 2 {
		t.Fatalf("snapshot after invalidation = %d channels, want 2", len(refreshed))
	}
}

func TestFallbackRuleSnapshotRefreshesAfterInvalidation(t *testing.T) {
	env := newEnv(t)
	if _, found, err := env.fallbackRule("expensive"); err != nil || found {
		t.Fatalf("empty snapshot: found=%v err=%v", found, err)
	}
	rule := &store.ModelFallback{Model: "expensive", Fallbacks: []string{"cheap"}}
	if err := env.db.Create(rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, found, _ := env.fallbackRule("expensive"); found {
		t.Fatal("rule should be invisible before invalidation")
	}
	env.InvalidateConfig()
	got, found, err := env.fallbackRule("expensive")
	if err != nil || !found || len(got.Fallbacks) != 1 || got.Fallbacks[0] != "cheap" {
		t.Fatalf("rule after invalidation = %+v found=%v err=%v", got, found, err)
	}
}

// 热路径不再逐请求查渠道与降级规则：快照就绪后一次对话请求不应再读这两张表。
func TestChatRequestDoesNotReadConfigTables(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "cached", up.server.URL, 1, []string{"gpt-x"}, nil, "sk-cached")
	_, plain := env.addToken(t, "snapshot-token", nil)
	gateway := env.authGateway(t)
	body := `{"model":"gpt-x","messages":[]}`

	if resp, respBody := postWithSession(t, gateway, "/v1/chat/completions", plain, "warm", body); resp.StatusCode != 200 {
		t.Fatalf("warm-up: %d %s", resp.StatusCode, respBody)
	}

	var configQueries int32
	countConfig := func(db *gorm.DB) {
		switch db.Statement.Table {
		case "channels", "model_fallbacks":
			atomic.AddInt32(&configQueries, 1)
		}
	}
	if err := env.db.Callback().Query().After("gorm:query").Register("test:count_config", countConfig); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() { _ = env.db.Callback().Query().Remove("test:count_config") })

	if resp, respBody := postWithSession(t, gateway, "/v1/chat/completions", plain, "warm", body); resp.StatusCode != 200 {
		t.Fatalf("cached request: %d %s", resp.StatusCode, respBody)
	}
	if got := atomic.LoadInt32(&configQueries); got != 0 {
		t.Fatalf("warm snapshot should serve without config queries, got %d", got)
	}
}
