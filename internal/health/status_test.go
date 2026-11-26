package health

import (
	"testing"

	"evergate/internal/store"
)

func TestDeriveStatus(t *testing.T) {
	activeChannel := func() *store.Channel { return &store.Channel{Enabled: true} }
	stateFn := func(states map[int64]string) func(int64) string {
		return func(id int64) string { return states[id] }
	}
	keys := func(states ...string) []*store.ChannelKey {
		out := make([]*store.ChannelKey, 0, len(states))
		for i := range states {
			out = append(out, &store.ChannelKey{ID: int64(i + 1), Enabled: true})
		}
		return out
	}
	cases := []struct {
		name    string
		channel *store.Channel
		keys    []*store.ChannelKey
		states  map[int64]string
		want    string
	}{
		{"all active", activeChannel(), keys("a", "a"), map[int64]string{1: store.KeyStateActive, 2: store.KeyStateActive}, store.ChannelStatusActive},
		{"half open usable", activeChannel(), keys("h", "c"), map[int64]string{1: store.KeyStateHalfOpen, 2: store.KeyStateCooldown}, store.ChannelStatusDegraded},
		{"all cooldown", activeChannel(), keys("c", "c"), map[int64]string{1: store.KeyStateCooldown, 2: store.KeyStateCooldown}, store.ChannelStatusDown},
		{"all disabled", activeChannel(), keys("d", "d"), map[int64]string{1: store.KeyStateDisabled, 2: store.KeyStateDisabled}, store.ChannelStatusDown},
		{"no enabled key", activeChannel(), []*store.ChannelKey{{ID: 1, Enabled: false}}, map[int64]string{1: store.KeyStateActive}, store.ChannelStatusDown},
		{"channel disabled", &store.Channel{Enabled: false}, keys("a"), map[int64]string{1: store.KeyStateActive}, store.ChannelStatusDown},
		{"probe inconclusive", &store.Channel{Enabled: true, LastProbeResult: ProbeResultInconclusive}, keys("a"), map[int64]string{1: store.KeyStateActive}, store.ChannelStatusInconclusive},
		{"probe failure", &store.Channel{Enabled: true, LastProbeResult: ProbeResultFailure}, keys("a"), map[int64]string{1: store.KeyStateActive}, store.ChannelStatusDegraded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeriveStatus(tc.channel, tc.keys, stateFn(tc.states)); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
