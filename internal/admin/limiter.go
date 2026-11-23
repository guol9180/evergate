package admin

import (
	"net"
	"sync"
	"time"
)

const (
	loginFailureLimit  = 5
	loginFailureWindow = 10 * time.Minute
	loginBlockDuration = 5 * time.Minute
	loginLimiterCap    = 1024
)

type loginState struct {
	failures     int
	windowStart  time.Time
	blockedUntil time.Time
	lastSeen     time.Time
}

type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]loginState
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: make(map[string]loginState)}
}

func (l *loginLimiter) blockedFor(key string, now time.Time) time.Duration {
	if l == nil || key == "" {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(now)
	state, ok := l.entries[key]
	if !ok {
		return 0
	}
	if now.Before(state.blockedUntil) {
		return state.blockedUntil.Sub(now)
	}
	if !state.windowStart.IsZero() && now.Sub(state.windowStart) >= loginFailureWindow {
		delete(l.entries, key)
	}
	return 0
}

func (l *loginLimiter) recordFailure(key string, now time.Time) {
	if l == nil || key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(now)
	state := l.entries[key]
	if state.windowStart.IsZero() || now.Sub(state.windowStart) >= loginFailureWindow {
		state = loginState{windowStart: now}
	}
	state.failures++
	state.lastSeen = now
	if state.failures >= loginFailureLimit {
		state.blockedUntil = now.Add(loginBlockDuration)
	}
	l.entries[key] = state
}

func (l *loginLimiter) recordSuccess(key string) {
	if l == nil || key == "" {
		return
	}
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func (l *loginLimiter) pruneLocked(now time.Time) {
	for key, state := range l.entries {
		expiredWindow := !state.windowStart.IsZero() && now.Sub(state.windowStart) >= loginFailureWindow
		unblocked := state.blockedUntil.IsZero() || !now.Before(state.blockedUntil)
		if expiredWindow && unblocked {
			delete(l.entries, key)
		}
	}
	if len(l.entries) < loginLimiterCap {
		return
	}
	var oldestKey string
	var oldest time.Time
	for key, state := range l.entries {
		if oldestKey == "" || state.lastSeen.Before(oldest) {
			oldestKey, oldest = key, state.lastSeen
		}
	}
	if oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}

func isLoopback(clientIP string) bool {
	ip := net.ParseIP(clientIP)
	return ip != nil && ip.IsLoopback()
}
