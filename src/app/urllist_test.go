package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadURLFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "urls.txt")
	content := "" +
		"# 注释行\n" +
		"\n" +
		"https://cdn.example.com/a.jpg\n" +
		"  https://cdn.example.com/b.jpg  \n" + // 前后空白应被去除
		"ftp://cdn.example.com/c.jpg\n" + // 非 http(s) 应被忽略
		"not-a-url\n" + // 非法 URL 应被忽略
		"http://cdn.example.com/d.jpg\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	urls, err := loadURLFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"https://cdn.example.com/a.jpg",
		"https://cdn.example.com/b.jpg",
		"http://cdn.example.com/d.jpg",
	}
	if !reflect.DeepEqual(urls, want) {
		t.Fatalf("解析结果 = %v, want %v", urls, want)
	}
}

func TestLoadOrCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "urls.txt")

	t.Run("不存在时生成默认文件", func(t *testing.T) {
		urls, created, err := loadOrCreate(path, "# default\n# Add one image URL per line\n", "竖屏")
		if err != nil {
			t.Fatal(err)
		}
		if !created {
			t.Fatal("应报告已创建")
		}
		if len(urls) != 0 {
			t.Fatalf("默认文件应为空列表: %v", urls)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("默认文件未生成: %v", err)
		}
	})

	t.Run("已存在时直接读取", func(t *testing.T) {
		urls, created, err := loadOrCreate(path, "# default\n", "竖屏")
		if err != nil {
			t.Fatal(err)
		}
		if created {
			t.Fatal("不应重复创建")
		}
		if len(urls) != 0 {
			t.Fatalf("读取结果 = %v, want empty", urls)
		}
	})
}

func TestDedupe(t *testing.T) {
	in := []string{"a", "b", "a", "c", "b"}
	if got := dedupe(in); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("dedupe = %v", got)
	}
}

func TestIsValidURL(t *testing.T) {
	valid := []string{"http://a.com/1.jpg", "https://a.com/1.jpg"}
	invalid := []string{"", "ftp://a.com/1.jpg", "javascript:alert(1)", "/relative/path", "a.com/1.jpg"}
	for _, s := range valid {
		if !isValidURL(s) {
			t.Errorf("isValidURL(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if isValidURL(s) {
			t.Errorf("isValidURL(%q) = true, want false", s)
		}
	}
}
