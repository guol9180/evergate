// Package ratelimit 提供单进程内的请求限流计数：固定分钟窗口的 RPM 与并发上限。
// 只在内存里计数，重启即清零；多实例部署时各实例独立计数（项目边界不做分布式限流）。
package ratelimit

import (
	"sync"
	"time"
)

// Limiter 按 subject（下游令牌 / 上游 key 的 id）计数。
type Limiter struct {
	now func() time.Time

	mu       sync.Mutex
	minute   int64
	counts   map[int64]int
	inflight map[int64]int
}

func New(now func() time.Time) *Limiter {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Limiter{
		now:      now,
		counts:   make(map[int64]int),
		inflight: make(map[int64]int),
	}
}

// AllowRPM 记一次 RPM 计数；超限返回 false 与距下个窗口的等待时间。
func (l *Limiter) AllowRPM(subject int64, limit int) (time.Duration, bool) {
	if limit <= 0 {
		return 0, true
	}
	now := l.now()
	minute := now.Unix() / 60

	l.mu.Lock()
	defer l.mu.Unlock()
	if minute != l.minute {
		l.minute = minute
		l.counts = make(map[int64]int)
	}
	if l.counts[subject] >= limit {
		next := time.Unix((minute+1)*60, 0).Sub(now)
		if next < 0 {
			next = 0
		}
		return next, false
	}
	l.counts[subject]++
	return 0, true
}

// Acquire 尝试占用一个并发名额；成功返回释放函数（必须调用）。
func (l *Limiter) Acquire(subject int64, limit int) (func(), bool) {
	if limit <= 0 {
		return func() {}, true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[subject] >= limit {
		return nil, false
	}
	l.inflight[subject]++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.inflight[subject]--
			if l.inflight[subject] <= 0 {
				delete(l.inflight, subject)
			}
		})
	}, true
}

// Inflight 返回当前在途数（管理台展示 / 测试断言）。
func (l *Limiter) Inflight(subject int64) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inflight[subject]
}
