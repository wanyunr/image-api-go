package app

import (
	"testing"
	"time"
)

func TestRecentBufferOrderAndLimit(t *testing.T) {
	r := newRecentBuffer(3)
	base := time.Now()
	for i := 0; i < 3; i++ {
		r.Add(recentAccess{URL: string(rune('a' + i)), At: base.Add(time.Duration(i) * time.Second)})
	}

	items := r.Recent(0) // limit<=0 返回全部
	if len(items) != 3 {
		t.Fatalf("长度 = %d, want 3", len(items))
	}
	// 倒序：最新在前。
	if items[0].URL != "c" || items[2].URL != "a" {
		t.Fatalf("顺序不符: %+v", items)
	}

	if got := r.Recent(2); len(got) != 2 || got[0].URL != "c" || got[1].URL != "b" {
		t.Fatalf("limit=2 结果不符: %+v", got)
	}
	if got := r.Recent(99); len(got) != 3 {
		t.Fatalf("limit 超过容量应返回全部, got %d", len(got))
	}
}

func TestRecentBufferRingOverwrite(t *testing.T) {
	r := newRecentBuffer(2)
	for _, url := range []string{"a", "b", "c"} {
		r.Add(recentAccess{URL: url})
	}
	if r.Size() != 2 {
		t.Fatalf("容量固定为 2, size = %d", r.Size())
	}
	items := r.Recent(0)
	if items[0].URL != "c" || items[1].URL != "b" {
		t.Fatalf("最旧的 a 应被覆盖: %+v", items)
	}
}

func TestRecentBufferDefaultCapacity(t *testing.T) {
	r := newRecentBuffer(0) // 非法容量回退默认值
	if r.capacity != defaultRecentBufferSize {
		t.Fatalf("capacity = %d, want %d", r.capacity, defaultRecentBufferSize)
	}
}
