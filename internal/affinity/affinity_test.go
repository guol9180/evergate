package affinity

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestRememberLookupAndTokenIsolation(t *testing.T) {
	table := New(8, time.Minute, nil)
	table.Remember(1, "gpt-x", "sess-1", 10, 20)

	entry, ok := table.Lookup(1, "gpt-x", "sess-1")
	if !ok || entry.ChannelID != 10 || entry.KeyID != 20 {
		t.Fatalf("lookup = %+v, %v; want channel 10 key 20", entry, ok)
	}
	if _, ok := table.Lookup(2, "gpt-x", "sess-1"); ok {
		t.Fatal("不同下游令牌不得复用同一 session 的亲和记录")
	}
	if _, ok := table.Lookup(1, "gpt-y", "sess-1"); ok {
		t.Fatal("不同模型不得复用同一 session 的亲和记录")
	}
	if _, ok := table.Lookup(1, "gpt-x", "sess-2"); ok {
		t.Fatal("不同 session 不得命中")
	}
}

func TestExpiryFallsBackAndCanRebuild(t *testing.T) {
	clock := &testClock{t: time.Unix(1_000_000, 0)}
	table := New(8, 10*time.Minute, clock.Now)
	table.Remember(1, "m", "s", 1, 2)

	clock.Advance(9*time.Minute + 59*time.Second)
	if _, ok := table.Lookup(1, "m", "s"); !ok {
		t.Fatal("TTL 内应命中")
	}
	clock.Advance(2 * time.Second)
	if _, ok := table.Lookup(1, "m", "s"); ok {
		t.Fatal("TTL 过期后应回退普通调度")
	}
	if table.Len() != 0 {
		t.Fatalf("过期记录应在查询时清理, len = %d", table.Len())
	}

	table.Remember(1, "m", "s", 3, 4)
	entry, ok := table.Lookup(1, "m", "s")
	if !ok || entry.ChannelID != 3 || entry.KeyID != 4 {
		t.Fatalf("重建后应命中新记录, got %+v %v", entry, ok)
	}
}

func TestLRUCapacityBound(t *testing.T) {
	table := New(2, time.Hour, nil)
	table.Remember(1, "m", "a", 1, 1)
	table.Remember(1, "m", "b", 2, 2)
	if _, ok := table.Lookup(1, "m", "a"); !ok {
		t.Fatal("a 应存在")
	}
	table.Remember(1, "m", "c", 3, 3) // 容量满 → 淘汰最久未用的 b
	if table.Len() != 2 {
		t.Fatalf("len = %d, want 2（LRU 有界）", table.Len())
	}
	if _, ok := table.Lookup(1, "m", "b"); ok {
		t.Fatal("b 应被 LRU 淘汰")
	}
	if _, ok := table.Lookup(1, "m", "a"); !ok {
		t.Fatal("a 刚被访问，不应被淘汰")
	}
	if _, ok := table.Lookup(1, "m", "c"); !ok {
		t.Fatal("c 应存在")
	}
}

func TestNormalizeSession(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"  sess-1\t", "sess-1"},
		{"会话-中文", "会话-中文"},
		{strings.Repeat("x", MaxSessionLen), strings.Repeat("x", MaxSessionLen)},
		{strings.Repeat("x", MaxSessionLen+1), ""},
	}
	for _, c := range cases {
		if got := NormalizeSession(c.in); got != c.want {
			t.Fatalf("NormalizeSession(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTableStoresOnlyFingerprint(t *testing.T) {
	table := New(4, time.Minute, nil)
	const session = "raw-session-secret-value"
	table.Remember(7, "m", session, 1, 2)

	table.mu.Lock()
	defer table.mu.Unlock()
	for key, el := range table.entries {
		if strings.Contains(fmt.Sprint(key), session) {
			t.Fatalf("表键包含 session 原文: %v", key)
		}
		rec := el.Value.(*record)
		if strings.Contains(fmt.Sprint(rec), session) {
			t.Fatalf("记录包含 session 原文: %+v", rec)
		}
	}
}

func TestForgetIfMatchesKeepsNewerRecord(t *testing.T) {
	table := New(4, time.Minute, nil)
	table.Remember(1, "m", "s", 1, 1)
	table.Remember(1, "m", "s", 2, 2)
	table.ForgetIfMatches(1, "m", "s", 1, 1) // 慢失败请求的旧记录，不该误删新记录
	entry, ok := table.Lookup(1, "m", "s")
	if !ok || entry.ChannelID != 2 {
		t.Fatalf("新记录应保留, got %+v %v", entry, ok)
	}
	table.ForgetIfMatches(1, "m", "s", 2, 2)
	if _, ok := table.Lookup(1, "m", "s"); ok {
		t.Fatal("匹配时应删除")
	}
}

func TestConcurrentAccess(t *testing.T) {
	table := New(64, time.Minute, nil)
	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			session := fmt.Sprintf("sess-%d", g%4) // 相同 session 并发读写
			for i := 0; i < 200; i++ {
				table.Remember(1, "m", session, int64(g+1), int64(i+1))
				_, _ = table.Lookup(1, "m", session)
				if i%17 == 0 {
					table.ForgetIfMatches(1, "m", session, int64(g+1), int64(i+1))
				}
			}
		}(g)
	}
	wg.Wait()
	if table.Len() > 64 {
		t.Fatalf("并发后容量越界: %d", table.Len())
	}
}
