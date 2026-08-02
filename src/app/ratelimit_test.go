package app

import (
	"testing"
	"time"
)

// TestRateLimiter_GlobalQuota 验证：全局限流下，无论来源如何，请求量合计达到额度即拒绝。
func TestRateLimiter_GlobalQuota(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Enabled:   true,
		PerMinute: 3,
		PerHour:   0, // 未设置则不限
		PerDay:    0,
	})

	// 前 3 个放行
	for i := 0; i < 3; i++ {
		if !rl.Allow() {
			t.Fatalf("第 %d 个请求应放行", i+1)
		}
	}
	// 第 4 个拒绝
	if rl.Allow() {
		t.Fatal("超过每分钟额度后应拒绝")
	}
}

func TestRateLimiter_Disabled(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{Enabled: false, PerMinute: 1})
	for i := 0; i < 100; i++ {
		if !rl.Allow() {
			t.Fatal("限流关闭时不应拒绝任何请求")
		}
	}
}

// TestRateLimiter_WindowReset 验证：窗口过期后计数重置。
// 通过直接改写窗口起始时间模拟"1 分钟后"，避免真实等待。
func TestRateLimiter_WindowReset(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{Enabled: true, PerMinute: 1})

	if !rl.Allow() {
		t.Fatal("首个请求应放行")
	}
	if rl.Allow() {
		t.Fatal("同窗口内第二个请求应拒绝")
	}

	// 模拟一分钟窗口过期
	rl.mu.Lock()
	rl.minuteStart = time.Now().Add(-time.Minute)
	rl.mu.Unlock()

	if !rl.Allow() {
		t.Fatal("窗口重置后应放行")
	}
}

// TestRateLimiter_HourAndDayLimits 验证：分/时/天三级限制同时生效。
func TestRateLimiter_HourAndDayLimits(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Enabled:   true,
		PerMinute: 1000, // 分钟额度宽松，专注验证小时/天
		PerHour:   5,
		PerDay:    10,
	})

	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Fatalf("小时额度内第 %d 个应放行", i+1)
		}
	}
	if rl.Allow() {
		t.Fatal("达到小时额度后应拒绝")
	}

	// 模拟一小时窗口过期（分钟窗口同步过期，重置分钟计数）
	rl.mu.Lock()
	rl.minuteStart = time.Now().Add(-time.Minute)
	rl.hourStart = time.Now().Add(-time.Hour)
	rl.mu.Unlock()

	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Fatalf("新小时窗口第 %d 个应放行", i+1)
		}
	}
	if rl.Allow() {
		t.Fatal("新小时窗口达到额度后应拒绝")
	}

	// 模拟一天窗口过期（累计已消费 10 个 = 达到每日额度上限，重置后放行）
	rl.mu.Lock()
	rl.minuteStart = time.Now().Add(-time.Minute)
	rl.hourStart = time.Now().Add(-time.Hour)
	rl.dayStart = time.Now().Add(-25 * time.Hour)
	rl.mu.Unlock()

	if !rl.Allow() {
		t.Fatal("每日窗口重置后应放行")
	}
}
