package admin

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"evergate/internal/store"
)

// updateChannel 只写可编辑列：表单打开后后台探测写入的 last_probe_* / status / catalog_diff 不能被整行回写覆盖。
func TestUpdateChannelKeepsProbeFieldsAndWritesEditableColumns(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	id := createChannelForTest(t, env, cookie, "probe-me")
	probeSeed := store.Channel{
		LastProbeResult: "failure", LastProbeError: "upstream 503", LastProbeStatusCode: 503,
		Status: store.ChannelStatusDegraded, CatalogDiff: store.ChannelCatalogDiff{Added: []string{"m2"}},
	}
	if err := env.db.Model(&store.Channel{}).Where("id = ?", id).Updates(&probeSeed).Error; err != nil {
		t.Fatalf("seed probe fields: %v", err)
	}

	code, body, _ := env.req(t, "PUT", fmt.Sprintf("/admin/api/channels/%d", id), cookie,
		`{"name":"renamed","type":"openai-compatible","base_url":"https://example.com/v1","models":["a","b"],"model_mapping":{"a":"up"},"priority":3,"weight":7,"supports_responses":true,"timeout_config":{"connect_sec":5}}`)
	if code != http.StatusOK {
		t.Fatalf("update: %d %s", code, body)
	}
	var ch store.Channel
	if err := env.db.First(&ch, id).Error; err != nil {
		t.Fatal(err)
	}
	editable := map[string]bool{
		"name": ch.Name == "renamed", "base_url": ch.BaseURL == "https://example.com/v1",
		"models": len(ch.Models) == 2 && ch.Models[1] == "b", "mapping": ch.ModelMapping["a"] == "up",
		"priority": ch.Priority == 3, "weight": ch.Weight == 7, "responses": ch.SupportsResponses,
		"timeouts": ch.Timeouts.ConnectSec != nil && *ch.Timeouts.ConnectSec == 5,
	}
	for field, ok := range editable {
		if !ok {
			t.Errorf("editable column %s not written: %+v", field, ch)
		}
	}
	probe := map[string]bool{
		"last_probe_result": ch.LastProbeResult == "failure",
		"last_probe_error":  ch.LastProbeError == "upstream 503",
		"status_code":       ch.LastProbeStatusCode == 503,
		"status":            ch.Status == store.ChannelStatusDegraded,
		"catalog_diff":      len(ch.CatalogDiff.Added) == 1,
	}
	for field, ok := range probe {
		if !ok {
			t.Errorf("probe-owned column %s was overwritten by the form save: %+v", field, ch)
		}
	}
	// 响应 DTO 与落库一致：名称 / 模型已更新，探测结果仍是 failure（status 是按 key 实时派生的，不等于 DB 列）
	for _, want := range []string{`"name":"renamed"`, `"last_probe_result":"failure"`, `"last_probe_status_code":503`} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %s: %s", want, body)
		}
	}
}

// 渠道列表 key 计数：旧实现整表加载密文行，新实现 GROUP BY 总数 + 只取启用 key 的 id。
func TestListChannelsCountsKeysWithGroupedQuery(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	id := createChannelForTest(t, env, cookie, "counted")
	if code, body, _ := env.req(t, "POST", fmt.Sprintf("/admin/api/channels/%d/keys", id), cookie, `{"keys":"sk-1\nsk-2\nsk-3"}`); code != http.StatusOK {
		t.Fatalf("add keys: %d %s", code, body)
	}
	var keys []*store.ChannelKey
	if err := env.db.Where("channel_id = ?", id).Order("id asc").Find(&keys).Error; err != nil || len(keys) != 3 {
		t.Fatalf("seed keys: %v %d", err, len(keys))
	}
	if code, body, _ := env.req(t, "PUT", fmt.Sprintf("/admin/api/keys/%d", keys[1].ID), cookie, `{"enabled":false}`); code != http.StatusOK {
		t.Fatalf("disable key: %d %s", code, body)
	}
	code, body, _ := env.req(t, "GET", "/admin/api/channels", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("list: %d %s", code, body)
	}
	for _, want := range []string{`"key_count":3`, `"available_key_count":2`} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing %s: %s", want, body)
		}
	}
}

func BenchmarkChannelKeySummary(b *testing.B) {
	env := newEnvOpts(b, true)
	const channels, keysPerChannel = 20, 200
	cipher := strings.Repeat("c", 120) // 与真实 AES-GCM base64 密文同量级
	rows := make([]*store.ChannelKey, 0, channels*keysPerChannel)
	for c := 1; c <= channels; c++ {
		ch := &store.Channel{Name: fmt.Sprintf("bench-%d", c), Type: "openai-compatible", BaseURL: "https://example.com", Enabled: true, Status: store.ChannelStatusActive}
		if err := env.db.Create(ch).Error; err != nil {
			b.Fatal(err)
		}
		for k := 0; k < keysPerChannel; k++ {
			rows = append(rows, &store.ChannelKey{ChannelID: ch.ID, KeyCipher: cipher, Enabled: k%4 != 0, Weight: 1, State: store.KeyStateActive})
		}
	}
	if err := env.db.CreateInBatches(rows, 200).Error; err != nil {
		b.Fatal(err)
	}
	api := &API{opt: Options{DB: env.db}}

	b.Run("full_rows", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var keys []*store.ChannelKey
			if err := env.db.Order("id asc").Find(&keys).Error; err != nil {
				b.Fatal(err)
			}
			totals := make(map[int64]int64)
			for _, key := range keys {
				totals[key.ChannelID]++
			}
			if totals[1] != keysPerChannel {
				b.Fatalf("total = %d", totals[1])
			}
		}
	})
	b.Run("grouped", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			totals, enabled, err := api.channelKeySummary()
			if err != nil {
				b.Fatal(err)
			}
			if totals[1] != keysPerChannel || len(enabled) == 0 {
				b.Fatalf("total = %d enabled = %d", totals[1], len(enabled))
			}
		}
	})
}
