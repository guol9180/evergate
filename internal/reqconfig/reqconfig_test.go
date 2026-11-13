package reqconfig

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseEmpty(t *testing.T) {
	cfg, err := Parse("  ")
	if err != nil {
		t.Fatalf("empty header: %v", err)
	}
	if cfg.Timeouts != nil || cfg.Retry != nil || len(cfg.ChannelIDs) != 0 || cfg.SessionAffinity != nil {
		t.Fatalf("空 header 不应产生覆盖: %+v", cfg)
	}
	if got := cfg.Summary(); got != "" {
		t.Fatalf("空覆盖摘要应为空, got %q", got)
	}
}

func TestParseFullConfig(t *testing.T) {
	cfg, err := Parse(`{
		"timeouts":{"connect":"30s","first_token":"120s","overall":"600s"},
		"retry":{"channels":2,"keys_per_channel":1,"total":"45s"},
		"channels":[3,1,3],
		"session_affinity":false
	}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Timeouts == nil || cfg.Timeouts.Connect == nil || *cfg.Timeouts.Connect != 30*time.Second {
		t.Fatalf("connect = %+v", cfg.Timeouts)
	}
	if cfg.Retry == nil || cfg.Retry.Channels == nil || *cfg.Retry.Channels != 2 {
		t.Fatalf("retry = %+v", cfg.Retry)
	}
	if len(cfg.ChannelIDs) != 2 || cfg.ChannelIDs[0] != 3 || cfg.ChannelIDs[1] != 1 {
		t.Fatalf("channels 去重保序失败: %v", cfg.ChannelIDs)
	}
	if cfg.SessionAffinity == nil || *cfg.SessionAffinity {
		t.Fatalf("session_affinity = %v", cfg.SessionAffinity)
	}
	summary := cfg.Summary()
	if len(summary) > MaxSummaryBytes {
		t.Fatalf("摘要超长: %d", len(summary))
	}
	for _, want := range []string{`"connect_ms":30000`, `"channels":`, `"session_affinity":false`} {
		if !strings.Contains(summary, want) {
			t.Fatalf("摘要缺少 %q: %s", want, summary)
		}
	}
	if strings.Contains(summary, "30s") {
		t.Fatalf("摘要应为数值毫秒，不应回显原始时长串: %s", summary)
	}
}

func TestDurationClampToSafetyFloor(t *testing.T) {
	cfg, err := Parse(`{"timeouts":{"connect":"10ms","first_token":"10ms","overall":"1s"},"retry":{"total":"1s"}}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if *cfg.Timeouts.Connect != minConnect || *cfg.Timeouts.FirstToken != minFirstToken {
		t.Fatalf("超时下限收敛失败: %+v", cfg.Timeouts)
	}
	if *cfg.Timeouts.Overall != minOverall {
		t.Fatalf("overall 下限收敛失败: %v", *cfg.Timeouts.Overall)
	}
	if *cfg.Retry.Total != minRetryTotal {
		t.Fatalf("retry.total 下限收敛失败: %v", *cfg.Retry.Total)
	}
}

func TestInvalidConfigs(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		path string
	}{
		{"非对象", `[]`, ""},
		{"null", `null`, ""},
		{"未知顶层字段", `{"api_key":"sk-x"}`, "api_key"},
		{"未知嵌套字段", `{"timeouts":{"connect":"5s","bogus":1}}`, "timeouts.bogus"},
		{"未知重试字段", `{"retry":{"channels":1,"bogus":2}}`, "retry.bogus"},
		{"时长类型错误", `{"timeouts":{"connect":5}}`, "timeouts.connect"},
		{"非法时长", `{"timeouts":{"connect":"5"}}`, "timeouts.connect"},
		{"负时长", `{"timeouts":{"connect":"-1s"}}`, "timeouts.connect"},
		{"超上限时长", `{"timeouts":{"connect":"61s"}}`, "timeouts.connect"},
		{"超上限 overall", `{"timeouts":{"overall":"31m"}}`, "timeouts.overall"},
		{"overall 小于 first_token", `{"timeouts":{"first_token":"120s","overall":"60s"}}`, "timeouts.overall"},
		{"渠道数 0", `{"retry":{"channels":0}}`, "retry.channels"},
		{"渠道数超限", `{"retry":{"channels":11}}`, "retry.channels"},
		{"key 数超限", `{"retry":{"keys_per_channel":11}}`, "retry.keys_per_channel"},
		{"重试总时长超限", `{"retry":{"total":"601s"}}`, "retry.total"},
		{"channels 非数组", `{"channels":"1"}`, "channels"},
		{"channels 空", `{"channels":[]}`, "channels"},
		{"channels 过多", `{"channels":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17]}`, "channels"},
		{"channel 非整数", `{"channels":[1.5]}`, "channels[0]"},
		{"channel 非正", `{"channels":[0]}`, "channels[0]"},
		{"亲和类型错误", `{"session_affinity":"off"}`, "session_affinity"},
		{"敏感字段", `{"model":"gpt-4o"}`, "model"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.raw)
			if err == nil {
				t.Fatalf("应拒绝: %s", c.raw)
			}
			re, ok := err.(*Error)
			if !ok {
				t.Fatalf("错误类型 = %T", err)
			}
			if re.Path != c.path {
				t.Fatalf("path = %q, want %q (err=%v)", re.Path, c.path, re)
			}
		})
	}
}

func TestHeaderSizeLimit(t *testing.T) {
	raw := `{"channels":[1],"x":"` + strings.Repeat("a", MaxHeaderBytes) + `"}`
	if _, err := Parse(raw); err == nil {
		t.Fatal("超长 header 应被拒绝")
	}
}

func TestSummaryBoundedForManyChannels(t *testing.T) {
	ids := make([]int64, 0, MaxChannelIDs)
	for i := int64(1); i <= MaxChannelIDs; i++ {
		ids = append(ids, i)
	}
	raw, _ := json.Marshal(map[string]any{"channels": ids})
	cfg, err := Parse(string(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	summary := cfg.Summary()
	if len(summary) > MaxSummaryBytes {
		t.Fatalf("摘要超长: %d", len(summary))
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(summary), &decoded); err != nil {
		t.Fatalf("摘要必须仍是合法 JSON: %v (%s)", err, summary)
	}
	if decoded["channels_count"].(float64) != MaxChannelIDs || decoded["channels_elided"] != true {
		t.Fatalf("超长渠道列表应折叠计数: %s", summary)
	}
}

func TestParseAcceptsRootObjectWhitespace(t *testing.T) {
	if _, err := Parse("\n\t{\"session_affinity\":true}\n"); err != nil {
		t.Fatalf("空白包裹的 JSON 应可解析: %v", err)
	}
	if _, err := Parse(`{"session_affinity":true}{"x":1}`); err == nil {
		t.Fatal("JSON 之后的多余内容应被拒绝")
	}
}
