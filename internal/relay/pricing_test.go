package relay

import (
	"context"
	"net/http"
	"testing"

	"evergate/internal/store"
	"evergate/internal/usage"
)

func TestRequestCostUsesSnapshotAndDoesNotRewriteHistory(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	})
	env.addChannel(t, "cost", up.server.URL, 0, []string{"m"}, nil, "sk")
	if err := env.usage.SetDefault(context.Background(), 1_000_000, 2_000_000); err != nil {
		t.Fatalf("set default price: %v", err)
	}

	if resp, body := env.chat(t, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("first request: %d %s", resp.StatusCode, body)
	}
	if err := env.usage.SetDefault(context.Background(), 2_000_000, 4_000_000); err != nil {
		t.Fatalf("update default price: %v", err)
	}
	if resp, body := env.chat(t, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("second request: %d %s", resp.StatusCode, body)
	}

	env.logWriter.Close()
	var logs []store.RequestLog
	if err := env.db.Order("id asc").Find(&logs).Error; err != nil {
		t.Fatalf("read logs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("logs = %d, want 2", len(logs))
	}
	if logs[0].CostStatus != usage.CostStatusPriced || logs[0].CostMicros != 7 || logs[1].CostMicros != 14 {
		t.Fatalf("cost history should use price at request time: %#v", logs)
	}
}

func TestChannelModelPriceOverridesDefault(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	})
	env.addChannel(t, "override", up.server.URL, 0, []string{"m"}, nil, "sk")
	if err := env.usage.SetDefault(context.Background(), 1_000_000, 1_000_000); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if _, err := env.usage.UpsertOverride(context.Background(), 1, "m", 10_000_000, 20_000_000); err != nil {
		t.Fatalf("set override: %v", err)
	}
	if resp, body := env.chat(t, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("request: %d %s", resp.StatusCode, body)
	}
	env.logWriter.Close()
	var log store.RequestLog
	if err := env.db.First(&log).Error; err != nil {
		t.Fatalf("read log: %v", err)
	}
	if log.CostMicros != 70 || log.CostStatus != usage.CostStatusPriced {
		t.Fatalf("override cost = %#v, want 70 micros", log)
	}
}
