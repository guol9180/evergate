package admin

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"evergate/internal/store"
)

func TestLogAndUsageFiltersRejectInvalidParams(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	cases := []struct{ path, field string }{
		{"/admin/api/logs?channel_id=abc", "channel_id"},
		{"/admin/api/logs?token_id=1x", "token_id"},
		{"/admin/api/logs?status=2xx", "status"},
		{"/admin/api/logs?start=yesterday", "start"},
		{"/admin/api/logs?end=2026-13-01T00:00:00Z", "end"},
		{"/admin/api/logs/stats?channel_id=abc", "channel_id"},
		{"/admin/api/logs/export?token_id=abc", "token_id"},
		{"/admin/api/usage/report?channel_id=abc", "channel_id"},
		{"/admin/api/usage/report?token_id=abc", "token_id"},
	}
	for _, tc := range cases {
		code, body, _ := env.req(t, "GET", tc.path, cookie, "")
		if code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d %s", tc.path, code, body)
		}
		if !strings.Contains(body, tc.field) {
			t.Fatalf("%s: response should name field %q: %s", tc.path, tc.field, body)
		}
	}
	// 合法值不受影响
	if code, body, _ := env.req(t, "GET", "/admin/api/logs?channel_id=1&status=200", cookie, ""); code != http.StatusOK {
		t.Fatalf("valid filters should pass: %d %s", code, body)
	}
}

func TestLogExportEscapesFormulasAndCarriesUsageColumns(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	log := store.RequestLog{
		CreatedAt: time.Now().UTC(), Source: "api", Model: "m", ChannelName: "=SUM(A1)",
		TokenName: "-2+3", ErrorMessage: "@cmd", CostStatus: "priced", PriceSource: "override",
		CachedTokens: 11, CacheWriteTokens: 12, ReasoningTokens: 13, StatusCode: 200,
		RetryTrace: []store.RetryAttempt{{Attempt: 1}, {Attempt: 2}},
	}
	if err := env.db.Create(&log).Error; err != nil {
		t.Fatalf("seed log: %v", err)
	}
	code, body, _ := env.req(t, "GET", "/admin/api/logs/export", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("export: %d %s", code, body)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(body, "\uFEFF"))).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want header + 1 row, got %d", len(rows))
	}
	header := rows[0]
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	for _, col := range []string{"source", "cached_tokens", "cache_write_tokens", "reasoning_tokens", "retry_count", "price_source"} {
		if _, ok := index[col]; !ok {
			t.Fatalf("missing column %q in %v", col, header)
		}
	}
	row := rows[1]
	if got := row[index["channel"]]; got != "'=SUM(A1)" {
		t.Fatalf("formula cell not escaped: %q", got)
	}
	if got := row[index["token"]]; got != "'-2+3" {
		t.Fatalf("negative formula cell not escaped: %q", got)
	}
	if got := row[index["error_message"]]; got != "'@cmd" {
		t.Fatalf("at-formula cell not escaped: %q", got)
	}
	if row[index["cached_tokens"]] != "11" || row[index["cache_write_tokens"]] != "12" ||
		row[index["reasoning_tokens"]] != "13" || row[index["retry_count"]] != "2" {
		t.Fatalf("usage columns not exported: %v", row)
	}
}

func TestConfigImportRollsBackOnLateFailure(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	before := env.setting.Snapshot()

	values := before.Values
	values.ConnectTimeoutMS = 1500
	settingsJSON, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"version":1,"channels":[{"name":"rollback-me","type":"openai-compatible","base_url":"https://example.com","models":["m"],"weight":1}],` +
		`"settings":` + string(settingsJSON) + `,` +
		`"price_default":{"input_micros_per_million":123,"output_micros_per_million":456},` +
		`"tokens":[{"name":"bad-token","allowed_ips":["not-an-ip"]}]}`

	code, body, _ := env.req(t, "POST", "/admin/api/config/import", cookie, payload)
	if code != http.StatusBadRequest || !strings.Contains(body, "bad-token") {
		t.Fatalf("late failure should be 400: %d %s", code, body)
	}

	var channels int64
	env.db.Model(&store.Channel{}).Where("name = ?", "rollback-me").Count(&channels)
	if channels != 0 {
		t.Fatalf("channel must roll back, got %d", channels)
	}
	var tokens int64
	env.db.Model(&store.Token{}).Where("name = ?", "bad-token").Count(&tokens)
	if tokens != 0 {
		t.Fatalf("token must not persist, got %d", tokens)
	}
	var prices int64
	env.db.Model(&store.ModelPrice{}).Count(&prices)
	if prices != 0 {
		t.Fatalf("price must roll back, got %d rows", prices)
	}
	if got := env.setting.Snapshot().ConnectTimeoutMS; got != before.ConnectTimeoutMS {
		t.Fatalf("settings must roll back, got %d want %d", got, before.ConnectTimeoutMS)
	}
	var row store.RuntimeSetting
	if err := env.db.First(&row, "id = ?", 1).Error; err != nil {
		t.Fatal(err)
	}
	if row.ConnectTimeoutMS != before.ConnectTimeoutMS {
		t.Fatalf("settings row must roll back, got %d", row.ConnectTimeoutMS)
	}
}

func TestConfigImportCommitsSettingsAndPrices(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	values := env.setting.Snapshot().Values
	values.ConnectTimeoutMS = 1500
	settingsJSON, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"version":1,"channels":[{"name":"commit-me","type":"openai-compatible","base_url":"https://example.com","models":["m"],"weight":1}],` +
		`"settings":` + string(settingsJSON) + `,` +
		`"price_default":{"input_micros_per_million":123,"output_micros_per_million":456},` +
		`"price_overrides":[{"channel_name":"commit-me","model":"m","input_micros_per_million":1,"output_micros_per_million":2}]}`
	if code, body, _ := env.req(t, "POST", "/admin/api/config/import", cookie, payload); code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	if got := env.setting.Snapshot().ConnectTimeoutMS; got != 1500 {
		t.Fatalf("settings snapshot not published after commit: %d", got)
	}
	global, overrides, err := env.usage.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if global == nil || global.InputMicrosPerMillion != 123 || len(overrides) != 1 || overrides[0].Model != "m" {
		t.Fatalf("prices not committed: %+v %+v", global, overrides)
	}
}

func TestChannelKeyAndTokenSecretsNeverSerialized(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	if code, body, _ := env.req(t, "POST", "/admin/api/channels", cookie, `{"name":"secret-holder","type":"openai-compatible","base_url":"https://example.com","models":["m"]}`); code != http.StatusOK {
		t.Fatalf("create channel: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "POST", "/admin/api/channels/1/keys", cookie, `{"keys":"sk-upstream-secret"}`); code != http.StatusOK {
		t.Fatalf("add key: %d %s", code, body)
	}
	var key store.ChannelKey
	if err := env.db.First(&key).Error; err != nil {
		t.Fatal(err)
	}
	if key.KeyCipher == "" {
		t.Fatal("test setup: cipher should exist")
	}
	if code, body, _ := env.req(t, "GET", "/admin/api/channels/1", cookie, ""); code != http.StatusOK {
		t.Fatalf("channel detail: %d %s", code, body)
	} else if strings.Contains(body, "key_cipher") || strings.Contains(body, key.KeyCipher) {
		t.Fatalf("channel detail leaked key material: %s", body)
	}

	if code, body, _ := env.req(t, "POST", "/admin/api/tokens", cookie, `{"name":"t"}`); code != http.StatusOK {
		t.Fatalf("create token: %d %s", code, body)
	}
	var token store.Token
	if err := env.db.First(&token).Error; err != nil {
		t.Fatal(err)
	}
	if code, body, _ := env.req(t, "GET", "/admin/api/tokens", cookie, ""); code != http.StatusOK {
		t.Fatalf("token list: %d %s", code, body)
	} else if strings.Contains(body, "key_hash") || strings.Contains(body, token.KeyHash) {
		t.Fatalf("token list leaked key hash: %s", body)
	}

	keyJSON, _ := json.Marshal(key)
	if strings.Contains(string(keyJSON), "key_cipher") || strings.Contains(string(keyJSON), key.KeyCipher) {
		t.Fatalf("ChannelKey JSON leaked cipher: %s", keyJSON)
	}
	tokenJSON, _ := json.Marshal(token)
	if strings.Contains(string(tokenJSON), "key_hash") || strings.Contains(string(tokenJSON), token.KeyHash) {
		t.Fatalf("Token JSON leaked hash: %s", tokenJSON)
	}
}

func TestCSVCellEscaping(t *testing.T) {
	cases := map[string]string{
		"":           "",
		"plain":      "plain",
		"=1+1":       "'=1+1",
		"+SUM(A1)":   "'+SUM(A1)",
		"-2":         "'-2",
		"@cmd":       "'@cmd",
		"\tleading":  "'\tleading",
		"normal=sum": "normal=sum",
	}
	for in, want := range cases {
		if got := csvCell(in); got != want {
			t.Fatalf("csvCell(%q) = %q, want %q", in, got, want)
		}
	}
}
