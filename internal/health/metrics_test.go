package health

import (
	"path/filepath"
	"testing"
	"time"

	"evergate/internal/pool"
	"evergate/internal/retry"
	"evergate/internal/store"
)

func TestChannelMetricSamples(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "health-metrics.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ch := &store.Channel{
		Name: "metrics-ch", Type: "openai-compatible", BaseURL: "http://127.0.0.1:1",
		Weight: 1, Models: []string{"m"}, Enabled: true, Status: store.ChannelStatusActive,
	}
	if err := db.Create(ch).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	active := &store.ChannelKey{ChannelID: ch.ID, KeyCipher: "x", Enabled: true, Weight: 1, State: store.KeyStateActive}
	cooling := &store.ChannelKey{ChannelID: ch.ID, KeyCipher: "y", Enabled: true, Weight: 1, State: store.KeyStateActive}
	if err := db.Create(active).Error; err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := db.Create(cooling).Error; err != nil {
		t.Fatalf("create key: %v", err)
	}

	p := pool.NewManager(1, []time.Duration{time.Minute}, time.Now)
	p.ReportFailure(cooling.ID, retry.Classification{Kind: retry.KindRateLimited, RetryAfter: time.Minute})

	samples := ChannelMetricSamples(db, p)
	values := make(map[string]float64)
	for _, sample := range samples {
		key := sample.Name + "|" + sample.Labels["channel"] + "|" + sample.Labels["status"] + sample.Labels["state"]
		values[key] = sample.Value
	}
	channelLabel := "1"
	for _, want := range []string{
		"evergate_channel_status|" + channelLabel + "|degraded",
		"evergate_channel_keys|" + channelLabel + "|" + "active",
	} {
		if _, ok := values[want]; !ok {
			t.Fatalf("missing sample %q in %#v", want, samples)
		}
	}
	if got := values["evergate_channel_keys|"+channelLabel+"|"+"cooldown"]; got != 1 {
		t.Fatalf("cooldown key count = %v, want 1 (samples %#v)", got, samples)
	}
	if got := values["evergate_channel_keys|"+channelLabel+"|"+"active"]; got != 1 {
		t.Fatalf("active key count = %v, want 1", got)
	}
}
