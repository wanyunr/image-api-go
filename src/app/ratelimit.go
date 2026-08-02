package app

import (
	"sync"
	"time"
)

// RateLimiter 全局限流器。
//
// 设计说明：限流按整个服务所有请求的合计值计算（而非按 IP 分桶），
// 因此伪造 X-Forwarded-For 等转发头无法绕过限流，无需复杂的可信代理配置。
type RateLimiter struct {
	mu      sync.Mutex
	enabled bool
	perMin  int
	perHour int
	perDay  int

	minuteCount int
	minuteStart time.Time
	hourCount   int
	hourStart   time.Time
	dayCount    int
	dayStart    time.Time
}

func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	now := time.Now()
	return &RateLimiter{
		enabled:     cfg.Enabled,
		perMin:      cfg.PerMinute,
		perHour:     cfg.PerHour,
		perDay:      cfg.PerDay,
		minuteStart: now,
		hourStart:   now,
		dayStart:    now,
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

	// 重置过期窗口
	if now.Sub(rl.minuteStart) >= time.Minute {
		rl.minuteCount = 0
		rl.minuteStart = now
	}
	if now.Sub(rl.hourStart) >= time.Hour {
		rl.hourCount = 0
		rl.hourStart = now
	}
	if now.Sub(rl.dayStart) >= 24*time.Hour {
		rl.dayCount = 0
		rl.dayStart = now
	}

	// 检查限制
	if rl.perMin > 0 && rl.minuteCount >= rl.perMin {
		return false
	}
	if rl.perHour > 0 && rl.hourCount >= rl.perHour {
		return false
	}
	if rl.perDay > 0 && rl.dayCount >= rl.perDay {
		return false
	}

	// 计数
	rl.minuteCount++
	rl.hourCount++
	rl.dayCount++

	return true
}
