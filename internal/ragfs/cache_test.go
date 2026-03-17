package ragfs

import (
	"testing"
	"time"
)

func TestWithMetaTTL(t *testing.T) {
	c := NewCache(WithMetaTTL(5 * time.Second))
	if c.metaTTL != 5*time.Second {
		t.Errorf("metaTTL = %v, want 5s", c.metaTTL)
	}
}

func TestGetMeta_ReturnsNilWhenEmpty(t *testing.T) {
	c := NewCache()
	if got := c.GetMeta("nonexistent"); got != nil {
		t.Errorf("GetMeta on empty cache returned %v, want nil", got)
	}
}

func TestGetMeta_ReturnsCachedEntry(t *testing.T) {
	c := NewCache()
	e := &Entry{Name: "test.md"}
	c.PutMeta("test.md", e)

	got := c.GetMeta("test.md")
	if got == nil {
		t.Fatal("GetMeta returned nil, want entry")
	}
	if got.Name != "test.md" {
		t.Errorf("GetMeta name = %q, want %q", got.Name, "test.md")
	}
}

func TestGetMeta_ExpiredEntry(t *testing.T) {
	c := NewCache(WithMetaTTL(1 * time.Millisecond))
	c.PutMeta("test.md", &Entry{Name: "test.md"})
	time.Sleep(5 * time.Millisecond)

	if got := c.GetMeta("test.md"); got != nil {
		t.Errorf("GetMeta on expired entry returned %v, want nil", got)
	}
}

func TestPutMetaList_And_GetMetaList(t *testing.T) {
	c := NewCache()
	entries := []Entry{
		{Name: "a.md"},
		{Name: "b.md"},
	}
	c.PutMetaList("dir", entries)

	got := c.GetMetaList("dir")
	if len(got) != 2 {
		t.Fatalf("GetMetaList returned %d entries, want 2", len(got))
	}
	if got[0].Name != "a.md" || got[1].Name != "b.md" {
		t.Errorf("GetMetaList entries = %v, want [a.md, b.md]", got)
	}
}

func TestInvalidatePrefix_RemovesMatchingEntries(t *testing.T) {
	c := NewCache()
	c.PutMeta("dir/a.md", &Entry{Name: "a.md"})
	c.PutMeta("dir/b.md", &Entry{Name: "b.md"})
	c.PutMeta("other/c.md", &Entry{Name: "c.md"})
	c.PutContent("dir/a.md", []byte("content a"))
	c.PutContent("dir/b.md", []byte("content b"))
	c.PutContent("other/c.md", []byte("content c"))

	c.InvalidatePrefix("dir/")

	if c.GetMeta("dir/a.md") != nil {
		t.Error("dir/a.md meta should be invalidated")
	}
	if c.GetMeta("dir/b.md") != nil {
		t.Error("dir/b.md meta should be invalidated")
	}
	if c.GetMeta("other/c.md") == nil {
		t.Error("other/c.md meta should still be cached")
	}
	if c.GetContent("dir/a.md") != nil {
		t.Error("dir/a.md content should be invalidated")
	}
	if c.GetContent("other/c.md") == nil {
		t.Error("other/c.md content should still be cached")
	}
}

func TestEviction_WhenMaxSizeExceeded(t *testing.T) {
	c := NewCache(WithMaxSize(20))
	c.PutContent("a", []byte("1234567890")) // 10 bytes
	c.PutContent("b", []byte("1234567890")) // 10 bytes, total 20
	c.PutContent("c", []byte("1234567890")) // 10 bytes, should evict "a"

	if c.GetContent("a") != nil {
		t.Error("oldest entry 'a' should have been evicted")
	}
	if c.GetContent("c") == nil {
		t.Error("newest entry 'c' should be cached")
	}
}
