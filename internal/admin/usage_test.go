package admin

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"evergate/internal/store"
	"evergate/internal/usage"
)

func TestUsagePricingAndReportAPI(t *testing.T) {
	env := newEnv(t)
	cookie, _ := login(t, env, "admin", "admin123")
	env.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`)
	cookie, _ = login(t, env, "admin", "newpass123")

	code, body, _ := env.req(t, "PUT", "/admin/api/usage/prices/default", cookie,
		`{"input_micros_per_million":1000000,"output_micros_per_million":2000000}`)
	if code != http.StatusOK || !strings.Contains(body, `"input_micros_per_million":1000000`) {
		t.Fatalf("set default price: %d %s", code, body)
	}
	ch := &store.Channel{Name: "usage-ch", Type: "openai-compatible", BaseURL: "http://usage", Models: []string{"m"}, Enabled: true}
	if err := env.db.Create(ch).Error; err != nil {
		t.Fatal(err)
	}
	code, body, _ = env.req(t, "POST", "/admin/api/usage/prices/overrides", cookie,
		fmt.Sprintf(`{"channel_id":%d,"model":"m","input_micros_per_million":3000000,"output_micros_per_million":4000000}`, ch.ID))
	if code != http.StatusOK || !strings.Contains(body, `"channel_id":1`) {
		t.Fatalf("set override: %d %s", code, body)
	}
	if code, body, _ = env.req(t, "POST", "/admin/api/usage/prices/overrides", cookie,
		`{"channel_id":999,"model":"m","input_micros_per_million":1,"output_micros_per_million":1}`); code != http.StatusBadRequest {
		t.Fatalf("missing channel override should 400: %d %s", code, body)
	}
	if code, body, _ = env.req(t, "PUT", "/admin/api/usage/prices/default", cookie,
		`{"input_micros_per_million":-1,"output_micros_per_million":1}`); code != http.StatusBadRequest {
		t.Fatalf("negative price should 400: %d %s", code, body)
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	logs := []store.RequestLog{
		{CreatedAt: today.Add(time.Hour), ChannelID: ch.ID, ChannelName: ch.Name, Model: "m", TokenID: 1, TokenName: "t", StatusCode: 200, PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5, CostMicros: 17, Cost: 0.000017, CostStatus: usage.CostStatusPriced},
		{CreatedAt: today.Add(2 * time.Hour), ChannelID: ch.ID, ChannelName: ch.Name, Model: "m", TokenID: 1, TokenName: "t", StatusCode: 200, CostStatus: usage.CostStatusNoUsage},
	}
	if err := env.db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if err := usage.RecomputeDay(context.Background(), env.db, today); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	code, body, _ = env.req(t, "GET", "/admin/api/usage/report", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("report: %d %s", code, body)
	}
	for _, want := range []string{`"requests":2`, `"success":2`, `"tokens_in":3`, `"tokens_out":2`, `"cost_micros":17`, `"unpriced_requests":1`, `"name":"usage-ch"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("report missing %s: %s", want, body)
		}
	}
	old := today.AddDate(0, 0, -30).Format("2006-01-02")
	code, body, _ = env.req(t, "GET", "/admin/api/usage/report?start="+old, cookie, "")
	if code != http.StatusBadRequest || !strings.Contains(body, "保留窗口") {
		t.Fatalf("out of retention should 400: %d %s", code, body)
	}

	var price store.ModelPrice
	if err := env.db.Where("channel_id = ? AND model = ?", ch.ID, "m").First(&price).Error; err != nil {
		t.Fatal(err)
	}
	if code, body, _ = env.req(t, "DELETE", fmt.Sprintf("/admin/api/usage/prices/overrides/%d", price.ID), cookie, ""); code != http.StatusOK || strings.Contains(body, `"channel_id":1`) {
		t.Fatalf("delete override: %d %s", code, body)
	}
}
