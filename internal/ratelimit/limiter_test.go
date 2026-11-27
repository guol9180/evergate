package ratelimit

import (
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) Now() time.Time          { return f.t }
func (f *fakeClock) Advance(d time.Duration) { f.t = f.t.Add(d) }

func TestAllowRPMBoundaryAndWindowReset(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0).UTC()}
	l := New(clock.Now)
	for i := 0; i < 3; i++ {
		if wait, ok := l.AllowRPM(1, 3); !ok {
			t.Fatalf("request %d should pass, wait %s", i+1, wait)
		}
	}
	wait, ok := l.AllowRPM(1, 3)
	if ok || wait <= 0 || wait > time.Minute {
		t.Fatalf("4th request should be limited with retry-after, got ok=%v wait=%s", ok, wait)
	}
	if _, ok := l.AllowRPM(2, 3); !ok {
		t.Fatal("limits must be per subject")
	}
	if _, ok := l.AllowRPM(1, 0); !ok {
		t.Fatal("limit 0 means unlimited")
	}
	clock.Advance(time.Minute)
	if _, ok := l.AllowRPM(1, 3); !ok {
		t.Fatal("new minute window should reset the counter")
	}
}

func TestAcquireConcurrency(t *testing.T) {
	l := New(nil)
	release, ok := l.Acquire(7, 1)
	if !ok {
		t.Fatal("first acquire should pass")
	}
	if _, ok := l.Acquire(7, 1); ok {
		t.Fatal("second acquire should be rejected at limit 1")
	}
	if _, ok := l.Acquire(8, 1); !ok {
		t.Fatal("concurrency must be per subject")
	}
	if l.Inflight(7) != 1 {
		t.Fatalf("inflight = %d, want 1", l.Inflight(7))
	}
	release()
	release() // 重复释放必须安全
	if l.Inflight(7) != 0 {
		t.Fatalf("inflight after release = %d, want 0", l.Inflight(7))
	}
	if release, ok := l.Acquire(7, 1); !ok {
		t.Fatal("slot should be reusable")
	} else {
		release()
	}
	if release, ok := l.Acquire(9, 0); !ok {
		t.Fatal("limit 0 means unlimited")
	} else {
		release()
	}
}
