package relay

import (
	"testing"
	"time"
)

func TestTransportBucketSharesNearbyTimeouts(t *testing.T) {
	r := New(Options{})
	if a, b := r.transport(3*time.Second), r.transport(5*time.Second); a != b {
		t.Fatal("3s and 5s should share the 5s bucket")
	}
	if a, c := r.transport(5*time.Second), r.transport(6*time.Second); a == c {
		t.Fatal("6s should land in the 10s bucket")
	}
	if z, a := r.transport(0), r.transport(transportBucket); z != a {
		t.Fatal("zero timeout should fall back to the first bucket")
	}
}

func TestTransportCacheIsBounded(t *testing.T) {
	r := New(Options{})
	for i := 1; i <= transportCacheLimit+8; i++ {
		r.transport(time.Duration(i*17) * time.Second)
	}
	if len(r.transports) != transportCacheLimit {
		t.Fatalf("cache size = %d, want %d", len(r.transports), transportCacheLimit)
	}
	if t2 := r.transport(3 * time.Second); t2 == nil {
		t.Fatal("evicted bucket should be rebuilt on demand")
	}
}
