package app

import (
	"sync"
	"time"
)

const defaultRecentBufferSize = 50

type recentAccess struct {
	URL         string    `json:"url"`
	Orientation string    `json:"orientation"`
	Format      string    `json:"format"`
	At          time.Time `json:"at"`
}

// recentBuffer 定容环形缓冲。每次访问追加，满了就覆盖最旧。
// 用于追溯最近返回的图片，避免喜欢的图找不到。
type recentBuffer struct {
	mu    sync.Mutex
	items []recentAccess
	head  int // next write position
	full  bool
	cap   int
}

func newRecentBuffer(capacity int) *recentBuffer {
	if capacity <= 0 {
		capacity = defaultRecentBufferSize
	}
	return &recentBuffer{
		items: make([]recentAccess, capacity),
		cap:   capacity,
	}
}

func (r *recentBuffer) Add(item recentAccess) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.items[r.head] = item
	r.head = (r.head + 1) % r.cap
	if r.head == 0 {
		r.full = true
	}
	r.mu.Unlock()
}

// Recent 返回最新的 limit 条记录（按时间倒序）。limit<=0 返回全部。
func (r *recentBuffer) Recent(limit int) []recentAccess {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	size := r.sizeLocked()
	if limit <= 0 || limit > size {
		limit = size
	}
	out := make([]recentAccess, limit)
	for i := 0; i < limit; i++ {
		idx := (r.head - 1 - i + r.cap) % r.cap
		out[i] = r.items[idx]
	}
	return out
}

func (r *recentBuffer) Size() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sizeLocked()
}

func (r *recentBuffer) sizeLocked() int {
	if r.full {
		return r.cap
	}
	return r.head
}