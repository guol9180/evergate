package router

import (
	"testing"

	"evergate/internal/store"
)

func ch(id int64, priority, weight int, models ...string) *store.Channel {
	return &store.Channel{ID: id, Name: string(rune('A' - 1 + id)), Enabled: true, Priority: priority, Weight: weight, Models: models}
}

func TestRouteFiltersAndPriorityOrder(t *testing.T) {
	r := New(42)
	channels := []*store.Channel{
		ch(1, 1, 1, "m1"), // 命中 p1
		ch(2, 0, 1, "m1"), // 命中 p0（最优先）
		ch(3, 2, 1, "m1"), // 命中 p2
		ch(4, 0, 1, "m2"), // 模型不符
		{ID: 5, Name: "E", Enabled: false, Priority: 0, Weight: 1, Models: []string{"m1"}}, // 禁用
	}
	got := r.Route("m1", channels)
	if len(got) != 3 {
		t.Fatalf("候选数 = %d, want 3", len(got))
	}
	wantOrder := []int64{2, 1, 3}
	for i, c := range got {
		if c.Channel.ID != wantOrder[i] {
			t.Fatalf("候选[%d] = 渠道%d, want 渠道%d", i, c.Channel.ID, wantOrder[i])
		}
	}
}

func TestRouteNoMatch(t *testing.T) {
	r := New(1)
	got := r.Route("none", []*store.Channel{ch(1, 0, 1, "m1")})
	if len(got) != 0 {
		t.Fatalf("无匹配应返回空, got %d", len(got))
	}
}

func TestRouteWeightedShufflePrefersHeavy(t *testing.T) {
	r := New(7)
	heavy := ch(1, 0, 1000, "m")
	light := ch(2, 0, 1, "m")
	heavyFirst := 0
	rounds := 1000
	for i := 0; i < rounds; i++ {
		got := r.Route("m", []*store.Channel{heavy, light})
		if got[0].Channel.ID == 1 {
			heavyFirst++
		}
	}
	if ratio := float64(heavyFirst) / float64(rounds); ratio < 0.9 {
		t.Fatalf("高权重渠道排前比例 = %.2f, want >= 0.9", ratio)
	}
}

func TestRouteModelMapping(t *testing.T) {
	r := New(1)
	withMapping := ch(1, 0, 1, "gpt")
	withMapping.ModelMapping = map[string]string{"gpt": "upstream-gpt"}
	cases := []struct {
		name string
		ch   *store.Channel
		want string
	}{
		{"映射命中", withMapping, "upstream-gpt"},
		{"无映射同名透传", ch(2, 0, 1, "gpt"), "gpt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Route("gpt", []*store.Channel{c.ch})
			if len(got) != 1 || got[0].UpstreamModel != c.want {
				t.Fatalf("UpstreamModel = %v, want %s", got, c.want)
			}
		})
	}
	t.Run("空映射值透传", func(t *testing.T) {
		empty := ch(3, 0, 1, "gpt")
		empty.ModelMapping = map[string]string{"gpt": ""}
		got := r.Route("gpt", []*store.Channel{empty})
		if got[0].UpstreamModel != "gpt" {
			t.Fatalf("空映射值应透传, got %s", got[0].UpstreamModel)
		}
	})
}
