package alert

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"evergate/internal/store"
)

func TestBuildPayloadPresets(t *testing.T) {
	text := "channel_down: mock"
	details := map[string]any{"channel": "mock", "status": "down"}
	cases := []struct {
		preset string
		want   string
	}{
		{PresetGeneric, text},
		{PresetWeCom, text},
		{PresetFeishu, text},
		{PresetTelegram, text},
		{PresetBark, text},
	}
	for _, tc := range cases {
		payload, err := BuildPayload(tc.preset, text, "channel_down", details)
		if err != nil {
			t.Fatalf("%s: %v", tc.preset, err)
		}
		var wire map[string]any
		if err := json.Unmarshal(payload, &wire); err != nil {
			t.Fatalf("%s: %v", tc.preset, err)
		}
		if !containsText(wire, tc.want) {
			t.Fatalf("%s payload does not carry %q: %s", tc.preset, tc.want, payload)
		}
		if strings.Contains(string(payload), "sk-") {
			t.Fatalf("%s payload leaked key material: %s", tc.preset, payload)
		}
	}
	if _, err := BuildPayload("nope", text, "x", nil); err == nil {
		t.Fatal("unknown preset should fail")
	}
}

func containsText(wire map[string]any, want string) bool {
	for _, value := range wire {
		switch typed := value.(type) {
		case string:
			if strings.Contains(typed, want) {
				return true
			}
		case map[string]any:
			if containsText(typed, want) {
				return true
			}
		}
	}
	return false
}

func newAlertFixture(t *testing.T) (*Manager, *atomic.Int32, *[]string) {
	t.Helper()
	var hits atomic.Int32
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	db, err := store.Open(filepath.Join(t.TempDir(), "alert.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	clock := time.Unix(3000, 0).UTC()
	m := New(db, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return clock })
	if _, err := m.Save(store.AlertSetting{
		Enabled: true, Preset: PresetGeneric, URL: server.URL,
		Events: []string{EventChannelDown},
	}); err != nil {
		t.Fatalf("save setting: %v", err)
	}
	return m, &hits, &bodies
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("condition not met in time")
	}
}

func TestNotifySendsSubscribedEventOnly(t *testing.T) {
	m, hits, bodies := newAlertFixture(t)
	m.Notify(EventChannelDown, "mock", map[string]any{"channel": "mock"})
	waitFor(t, func() bool { return hits.Load() == 1 })

	m.Notify(EventKeyDisabled, "key #1", nil) // 未订阅 → 不发
	m.Notify(EventChannelDown, "mock", nil)   // 冷却期内同摘要 → 不发
	time.Sleep(50 * time.Millisecond)
	if hits.Load() != 1 {
		t.Fatalf("webhook hits = %d, want 1", hits.Load())
	}
	if !strings.Contains((*bodies)[0], `"event":"channel_down"`) {
		t.Fatalf("payload missing event: %s", (*bodies)[0])
	}

	records, err := m.Recent(10)
	if err != nil || len(records) != 1 || records[0].Result != "sent" {
		t.Fatalf("records = %+v err=%v", records, err)
	}
}

func TestSendTestIgnoresEnabledAndRecords(t *testing.T) {
	m, hits, _ := newAlertFixture(t)
	if _, err := m.Save(store.AlertSetting{Enabled: false, Preset: PresetGeneric, URL: m.mustSettingURL(t)}); err != nil {
		t.Fatalf("save disabled setting: %v", err)
	}
	record, err := m.SendTest(nil)
	if err != nil {
		t.Fatalf("send test: %v", err)
	}
	if hits.Load() != 1 || record.Result != "test" || record.Draft {
		t.Fatalf("hits=%d record=%+v", hits.Load(), record)
	}
	records, _ := m.Recent(10)
	if len(records) == 0 || records[0].Result != "test" {
		t.Fatalf("test record missing: %+v", records)
	}
}

func TestSendTestWithDraftUsesDraftWithoutSaving(t *testing.T) {
	m, savedHits, _ := newAlertFixture(t)
	var draftHits atomic.Int32
	var draftBodies []string
	draftServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		draftBodies = append(draftBodies, string(data))
		draftHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(draftServer.Close)
	saved, err := m.Setting()
	if err != nil {
		t.Fatalf("setting: %v", err)
	}

	record, err := m.SendTest(&store.AlertSetting{
		Enabled: false, Preset: PresetWeCom, URL: draftServer.URL, Events: []string{EventKeyDisabled},
	})
	if err != nil {
		t.Fatalf("send draft test: %v", err)
	}
	if record.Result != "test" || record.Preset != PresetWeCom || !record.Draft {
		t.Fatalf("record = %+v", record)
	}
	if savedHits.Load() != 0 {
		t.Fatalf("saved webhook should not be called, hits=%d", savedHits.Load())
	}
	if draftHits.Load() != 1 || !strings.Contains(draftBodies[0], `"msgtype":"text"`) {
		t.Fatalf("draft webhook hits=%d bodies=%v", draftHits.Load(), draftBodies)
	}
	after, err := m.Setting()
	if err != nil || after.URL != saved.URL || after.Preset != saved.Preset || !after.Enabled {
		t.Fatalf("draft test must not change the saved setting: before=%+v after=%+v err=%v", saved, after, err)
	}
	records, _ := m.Recent(10)
	if len(records) != 1 || !records[0].Draft || records[0].Preset != PresetWeCom {
		t.Fatalf("records = %+v", records)
	}
}

func TestSendTestWithInvalidDraftFailsWithoutSending(t *testing.T) {
	m, hits, _ := newAlertFixture(t)
	if _, err := m.SendTest(&store.AlertSetting{Preset: "slack", URL: "https://x"}); !errors.Is(err, ErrInvalidSetting) {
		t.Fatalf("unknown preset should be ErrInvalidSetting, got %v", err)
	}
	if _, err := m.SendTest(&store.AlertSetting{Preset: PresetGeneric, URL: "ftp://x"}); !errors.Is(err, ErrInvalidSetting) {
		t.Fatalf("non-http url should be ErrInvalidSetting, got %v", err)
	}
	if _, err := m.SendTest(&store.AlertSetting{Preset: PresetGeneric, URL: "https://x", Events: []string{"nope"}}); !errors.Is(err, ErrInvalidSetting) {
		t.Fatalf("unknown event should be ErrInvalidSetting, got %v", err)
	}
	// 空 URL 通过校验（未启用时允许留空），但在发送阶段被拦下，且不会落到已保存的地址
	if _, err := m.SendTest(&store.AlertSetting{Preset: PresetGeneric, URL: ""}); err == nil || errors.Is(err, ErrInvalidSetting) {
		t.Fatalf("empty url should fail at send stage, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("no webhook should be called, hits=%d", hits.Load())
	}
	records, _ := m.Recent(10)
	for _, record := range records {
		if record.Result != "failed" {
			t.Fatalf("only failed records expected: %+v", records)
		}
	}
}

func (m *Manager) mustSettingURL(t *testing.T) string {
	t.Helper()
	setting, err := m.Setting()
	if err != nil {
		t.Fatalf("setting: %v", err)
	}
	return setting.URL
}

func TestSaveValidatesPresetURLAndEvents(t *testing.T) {
	m, _, _ := newAlertFixture(t)
	if _, err := m.Save(store.AlertSetting{Preset: "slack", URL: "https://example.com"}); err == nil {
		t.Fatal("unknown preset should fail")
	}
	if _, err := m.Save(store.AlertSetting{Preset: PresetGeneric, Enabled: true}); err == nil {
		t.Fatal("enabled without url should fail")
	}
	if _, err := m.Save(store.AlertSetting{Preset: PresetGeneric, URL: "ftp://x"}); err == nil {
		t.Fatal("non-http url should fail")
	}
	if _, err := m.Save(store.AlertSetting{Preset: PresetGeneric, URL: "https://x", Events: []string{"nope"}}); err == nil {
		t.Fatal("unknown event should fail")
	}
}
