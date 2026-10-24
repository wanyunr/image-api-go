package app

import (
	"sync"
	"time"
)

// fixedWindow 单个固定窗口计数器：到期重置，额度（limit>0 时）用尽即拒绝。
type fixedWindow struct {
	limit int
	span  time.Duration
	count int
	start time.Time
}

func (w *fixedWindow) resetIfExpired(now time.Time) {
	if now.Sub(w.start) >= w.span {
		w.count = 0
		w.start = now
	}
}

func (w *fixedWindow) exhausted() bool {
	return w.limit > 0 && w.count >= w.limit
}

// RateLimiter 全局限流器。
//
// 设计说明：限流按整个服务所有请求的合计值计算（而非按 IP 分桶），
// 因此伪造 X-Forwarded-For 等转发头无法绕过限流，无需复杂的可信代理配置。
type RateLimiter struct {
	mu      sync.Mutex
	enabled bool
	minute  fixedWindow
	hour    fixedWindow
	day     fixedWindow
}

func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	now := time.Now()
	return &RateLimiter{
		enabled: cfg.Enabled,
		minute:  fixedWindow{limit: cfg.PerMinute, span: time.Minute, start: now},
		hour:    fixedWindow{limit: cfg.PerHour, span: time.Hour, start: now},
		day:     fixedWindow{limit: cfg.PerDay, span: 24 * time.Hour, start: now},
	}
}

// Allow 检查是否允许请求通过，返回是否放行。
func (rl *RateLimiter) Allow() bool {
	if !rl.enabled {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windows := []*fixedWindow{&rl.minute, &rl.hour, &rl.day}

	for _, w := range windows {
		w.resetIfExpired(now)
	}
	for _, w := range windows {
		if w.exhausted() {
			return false
		}
	}
	for _, w := range windows {
		w.count++
	}
	return true
}
