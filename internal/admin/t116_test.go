package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"evergate/internal/store"
)

func seedFacetLogs(t *testing.T, env *testEnv) {
	t.Helper()
	for _, row := range []struct {
		model   string
		channel int64
		status  int
	}{{"gpt-4o", 1, 200}, {"gpt-4o", 1, 200}, {"gpt-4o", 2, 503}, {"claude", 2, 401}} {
		log := store.RequestLog{
			CreatedAt: time.Now().UTC(), Model: row.model, ChannelID: row.channel, ChannelName: "c", StatusCode: row.status,
		}
		if err := env.db.Create(&log).Error; err != nil {
			t.Fatalf("seed log: %v", err)
		}
	}
}

func TestLogStatusGroupFilters(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	seedFacetLogs(t, env)
	cases := map[string]int{"success": 2, "error": 2, "401": 1}
	for status, want := range cases {
		code, body, _ := env.req(t, "GET", "/admin/api/logs?status="+status, cookie, "")
		if code != http.StatusOK {
			t.Fatalf("status=%s: %d %s", status, code, body)
		}
		var page struct {
			Data struct {
				Total int64 `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			t.Fatal(err)
		}
		if page.Data.Total != int64(want) {
			t.Fatalf("status=%s: want %d rows, got %d", status, want, page.Data.Total)
		}
	}
}

func TestLogModelFacetsFollowFilters(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	seedFacetLogs(t, env)
	code, body, _ := env.req(t, "GET", "/admin/api/logs/models", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("models: %d %s", code, body)
	}
	var out struct {
		Data struct {
			Items []struct {
				Model    string `json:"model"`
				Requests int64  `json:"requests"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data.Items) != 2 || out.Data.Items[0].Model != "gpt-4o" || out.Data.Items[0].Requests != 3 || out.Data.Items[1].Model != "claude" {
		t.Fatalf("facets should be gpt-4o×3 then claude×1, got %+v", out.Data.Items)
	}
	// 候选跟随其他筛选：渠道 2 + 失败分组里只有 claude（渠道 2 的 gpt-4o 是 503，但 status=401 只剩 claude）
	code, body, _ = env.req(t, "GET", "/admin/api/logs/models?channel_id=2&status=error", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"model":"claude"`) || !strings.Contains(body, `"model":"gpt-4o"`) {
		t.Fatalf("facets should follow channel + status filters: %d %s", code, body)
	}
	code, body, _ = env.req(t, "GET", "/admin/api/logs/models?channel_id=2&status=401", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"model":"claude"`) || strings.Contains(body, `"model":"gpt-4o"`) {
		t.Fatalf("facets should follow channel filter: %d %s", code, body)
	}
	if code, body, _ = env.req(t, "GET", "/admin/api/logs/models?status=bogus", cookie, ""); code != http.StatusBadRequest {
		t.Fatalf("invalid status on facets should be 400: %d %s", code, body)
	}
}
