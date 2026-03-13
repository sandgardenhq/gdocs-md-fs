package ragfs

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

// syncSafeHandler is a thread-safe Handler for testing the sync loop.
type syncSafeHandler struct {
	mu          sync.Mutex
	readContent []byte
	writeErr    error
	lastWritten []byte
	writeCalled bool
}

func (h *syncSafeHandler) Read(_ context.Context, _ string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.readContent, nil
}
func (h *syncSafeHandler) Write(_ context.Context, _ string, data []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writeCalled = true
	h.lastWritten = make([]byte, len(data))
	copy(h.lastWritten, data)
	return h.writeErr
}
func (h *syncSafeHandler) List(_ context.Context, _ string) ([]Entry, error) {
	return nil, fmt.Errorf("not implemented")
}
func (h *syncSafeHandler) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("not implemented")
}
func (h *syncSafeHandler) Rename(_ context.Context, _, _ string) error {
	return fmt.Errorf("not implemented")
}
func (h *syncSafeHandler) Stat(_ context.Context, _ string) (*Entry, error) {
	return nil, fmt.Errorf("not implemented")
}
func (h *syncSafeHandler) Create(_ context.Context, _ string, _ bool) (*Entry, error) {
	return nil, fmt.Errorf("not implemented")
}

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

func TestNewServer_DefaultSyncInterval(t *testing.T) {
	s := NewServer(nil, "/tmp/test-mount")
	if s.syncInterval != time.Second {
		t.Errorf("syncInterval: got %v, want 1s", s.syncInterval)
	}
}

func TestNewServer_WithSyncInterval(t *testing.T) {
	s := NewServer(nil, "/tmp/test-mount", WithSyncInterval(500*time.Millisecond))
	if s.syncInterval != 500*time.Millisecond {
		t.Errorf("syncInterval: got %v, want 500ms", s.syncInterval)
	}
}

func TestServer_RegisterAndUnregisterDirty(t *testing.T) {
	s := NewServer(nil, "/tmp/test-mount")
	f := &File{path: "doc.md"}

	s.registerDirty(f)
	s.dirtyMu.Lock()
	if _, ok := s.dirtyFiles[f]; !ok {
		t.Error("file should be registered as dirty")
	}
	s.dirtyMu.Unlock()

	s.unregisterDirty(f)
	s.dirtyMu.Lock()
	if _, ok := s.dirtyFiles[f]; ok {
		t.Error("file should be unregistered after unregisterDirty")
	}
	s.dirtyMu.Unlock()
}

func TestServer_SyncFlushesRegisteredDirtyFiles(t *testing.T) {
	h := &syncSafeHandler{readContent: []byte{}}
	s := NewServer(h, "/tmp/test-mount", WithSyncInterval(10*time.Millisecond))
	s.dirtyFiles = make(map[*File]struct{})
	s.stopSync = make(chan struct{})

	f := &File{
		handler: h,
		cache:   s.cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
	}

	// Write to make dirty.
	_, _ = f.Write(context.Background(), nil, []byte("# Synced\n"), 0)

	s.registerDirty(f)

	// Start sync loop.
	go s.syncLoop()

	// Wait for sync to fire.
	time.Sleep(50 * time.Millisecond)
	close(s.stopSync)

	h.mu.Lock()
	called := h.writeCalled
	written := string(h.lastWritten)
	h.mu.Unlock()

	if !called {
		t.Error("sync loop should have flushed dirty file via handler.Write")
	}
	if written != "# Synced\n" {
		t.Errorf("sync wrote %q, want %q", written, "# Synced\n")
	}
}

func TestFileWrite_RegistersDirtyWithServer(t *testing.T) {
	h := &stubHandler{readContent: []byte{}}
	s := NewServer(h, "/tmp/test-mount")
	s.dirtyFiles = make(map[*File]struct{})
	s.stopSync = make(chan struct{})

	f := &File{
		handler: h,
		cache:   s.cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
		server:  s,
	}

	_, _ = f.Write(context.Background(), nil, []byte("# Content\n"), 0)

	s.dirtyMu.Lock()
	_, ok := s.dirtyFiles[f]
	s.dirtyMu.Unlock()

	if !ok {
		t.Error("Write should register file as dirty with server")
	}
}

func TestFileFlush_UnregistersDirtyFromServer(t *testing.T) {
	h := &stubHandler{readContent: []byte{}}
	s := NewServer(h, "/tmp/test-mount")
	s.dirtyFiles = make(map[*File]struct{})
	s.stopSync = make(chan struct{})

	f := &File{
		handler: h,
		cache:   s.cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
		server:  s,
	}

	_, _ = f.Write(context.Background(), nil, []byte("# Content\n"), 0)
	_ = f.Flush(context.Background(), nil)

	s.dirtyMu.Lock()
	_, ok := s.dirtyFiles[f]
	s.dirtyMu.Unlock()

	if ok {
		t.Error("Flush should unregister file from server dirty set")
	}
}

func TestServer_UnmountWaitsForFinalFlush(t *testing.T) {
	h := &syncSafeHandler{readContent: []byte{}}
	s := NewServer(h, "/tmp/test-mount", WithSyncInterval(time.Hour))
	// Don't call Mount; manually start the sync loop.
	go s.syncLoop()

	f := &File{
		handler: h,
		cache:   s.cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
		server:  s,
	}

	_, _ = f.Write(context.Background(), nil, []byte("# Final flush\n"), 0)
	s.registerDirty(f)

	// Unmount (without a real FUSE server, so set s.server = nil to skip
	// the actual FUSE unmount). The sync loop final flush must still run.
	s.server = nil
	err := s.Unmount()
	if err != nil {
		t.Fatalf("Unmount: %v", err)
	}

	// After Unmount returns, the final flush must have completed.
	h.mu.Lock()
	called := h.writeCalled
	written := string(h.lastWritten)
	h.mu.Unlock()

	if !called {
		t.Error("Unmount must wait for syncLoop to flush dirty files before returning")
	}
	if written != "# Final flush\n" {
		t.Errorf("final flush wrote %q, want %q", written, "# Final flush\n")
	}
}

func TestServer_UnmountTwiceDoesNotPanic(t *testing.T) {
	s := NewServer(nil, "/tmp/test-mount")
	go s.syncLoop()

	// Calling stopUnmount twice must not panic (double close of channel).
	// We call the exported Unmount twice but need s.server non-nil to reach
	// the close path. Use a minimal shim to avoid the early return.
	s.server = nil

	// Directly test the stop mechanism: first close signals the loop.
	s.stopUnmount()
	// Second call must not panic.
	s.stopUnmount()
}

func TestFileOpen_TruncRegistersDirtyWithServer(t *testing.T) {
	h := &stubHandler{}
	s := NewServer(h, "/tmp/test-mount")
	s.dirtyFiles = make(map[*File]struct{})
	s.stopSync = make(chan struct{})

	f := &File{
		handler: h,
		cache:   s.cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
		server:  s,
	}

	_, _, _ = f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_TRUNC))

	s.dirtyMu.Lock()
	_, ok := s.dirtyFiles[f]
	s.dirtyMu.Unlock()

	if !ok {
		t.Error("Open O_TRUNC should register file as dirty with server")
	}
}
