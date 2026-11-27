package health

import (
	"testing"

	"evergate/internal/store"
)

// 只有进入 / 离开 down 才发事件，同状态重复探测不发。
func TestProberNotifiesOnlyOnDownTransitions(t *testing.T) {
	type event struct {
		name    string
		summary string
	}
	var events []event
	p := NewProber(ProberOptions{OnEvent: func(name, summary string, _ map[string]any) {
		events = append(events, event{name, summary})
	}})
	ch := &store.Channel{ID: 7, Name: "mock"}
	result := ProbeResult{StatusCode: 0, Error: "dial tcp: refused"}

	p.notifyStatusChange(ch, store.ChannelStatusActive, store.ChannelStatusDown, result)
	p.notifyStatusChange(ch, store.ChannelStatusDown, store.ChannelStatusDown, result)
	p.notifyStatusChange(ch, store.ChannelStatusDown, store.ChannelStatusActive, ProbeResult{StatusCode: 200})
	p.notifyStatusChange(ch, store.ChannelStatusActive, store.ChannelStatusDegraded, ProbeResult{StatusCode: 200})

	if len(events) != 2 || events[0].name != "channel_down" || events[1].name != "channel_recovered" {
		t.Fatalf("events = %+v, want down + recovered only", events)
	}
}
