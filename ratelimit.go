package main

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu      sync.Mutex
	enabled bool
	perMin  int
	perHour int
	perDay  int
	clients map[string]*clientWindow
}

type clientWindow struct {
	minuteCount int
	minuteStart time.Time
	hourCount   int
	hourStart   time.Time
	dayCount    int
	dayStart    time.Time
}

func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		enabled: cfg.Enabled,
		perMin:  cfg.PerMinute,
		perHour: cfg.PerHour,
		perDay:  cfg.PerDay,
		clients: make(map[string]*clientWindow),
	}
}

// Allow 检查是否允许该 IP 的请求，返回是否放行
func (rl *RateLimiter) Allow(ip string) bool {
	if !rl.enabled {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	c, ok := rl.clients[ip]
	if !ok {
		c = &clientWindow{
			minuteStart: now,
			hourStart:   now,
			dayStart:    now,
		}
		rl.clients[ip] = c
	}

	// 重置过期窗口
	if now.Sub(c.minuteStart) >= time.Minute {
		c.minuteCount = 0
		c.minuteStart = now
	}
	if now.Sub(c.hourStart) >= time.Hour {
		c.hourCount = 0
		c.hourStart = now
	}
	if now.Sub(c.dayStart) >= 24*time.Hour {
		c.dayCount = 0
		c.dayStart = now
	}

	// 检查限制
	if rl.perMin > 0 && c.minuteCount >= rl.perMin {
		return false
	}
	if rl.perHour > 0 && c.hourCount >= rl.perHour {
		return false
	}
	if rl.perDay > 0 && c.dayCount >= rl.perDay {
		return false
	}

	// 计数
	c.minuteCount++
	c.hourCount++
	c.dayCount++

	return true
}

// Cleanup 定期清理过期记录，防止内存泄漏
func (rl *RateLimiter) Cleanup() {
	if !rl.enabled {
		return
	}

	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for range ticker.C {
			rl.mu.Lock()
			now := time.Now()
			for ip, c := range rl.clients {
				if now.Sub(c.dayStart) >= 25*time.Hour {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()
}
