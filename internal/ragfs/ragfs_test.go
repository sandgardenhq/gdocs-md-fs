package ragfs

import (
	"os"
	"testing"
)

func TestNewServer_CapturesCurrentUID(t *testing.T) {
	s := NewServer(nil, "/tmp/test-mount")

	wantUID := uint32(os.Getuid())
	if s.uid != wantUID {
		t.Errorf("uid: got %d, want %d", s.uid, wantUID)
	}
}

func TestNewServer_CapturesCurrentGID(t *testing.T) {
	s := NewServer(nil, "/tmp/test-mount")

	wantGID := uint32(os.Getgid())
	if s.gid != wantGID {
		t.Errorf("gid: got %d, want %d", s.gid, wantGID)
	}
}

func TestNewServer_CreatesDefaultCache(t *testing.T) {
	s := NewServer(nil, "/tmp/test-mount")

	if s.cache == nil {
		t.Error("cache: expected non-nil default cache")
	}
}

func TestNewServer_AppliesCacheOption(t *testing.T) {
	s := NewServer(nil, "/tmp/test-mount", WithCacheOptions())

	if s.cache == nil {
		t.Error("cache: expected non-nil cache from WithCacheOptions")
	}
}

func TestNewServer_AppliesReadOnlyOption(t *testing.T) {
	s := NewServer(nil, "/tmp/test-mount", WithReadOnly(true))

	if !s.readOnly {
		t.Error("readOnly: expected true")
	}
}

func TestNewServer_StoresHandlerAndMountpoint(t *testing.T) {
	s := NewServer(nil, "/tmp/test-mount")

	if s.handler != nil {
		t.Error("handler: expected nil")
	}
	if s.mountpoint != "/tmp/test-mount" {
		t.Errorf("mountpoint: got %q, want %q", s.mountpoint, "/tmp/test-mount")
	}
}
