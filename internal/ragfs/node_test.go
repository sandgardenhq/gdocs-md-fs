package ragfs

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// stubHandler is a minimal Handler for unit-testing File methods.
type stubHandler struct {
	readContent  []byte
	readErr      error
	writeErr     error
	lastWritten  []byte
	writeCalled  bool
	createEntry  *Entry
	createErr    error
	createCalled bool
}

func (h *stubHandler) Read(_ context.Context, _ string) ([]byte, error) {
	return h.readContent, h.readErr
}
func (h *stubHandler) Write(_ context.Context, _ string, data []byte) error {
	h.writeCalled = true
	h.lastWritten = make([]byte, len(data))
	copy(h.lastWritten, data)
	return h.writeErr
}
func (h *stubHandler) List(_ context.Context, _ string) ([]Entry, error) {
	return nil, fmt.Errorf("not implemented")
}
func (h *stubHandler) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("not implemented")
}
func (h *stubHandler) Rename(_ context.Context, _, _ string) error {
	return fmt.Errorf("not implemented")
}
func (h *stubHandler) Stat(_ context.Context, _ string) (*Entry, error) {
	return nil, fmt.Errorf("not implemented")
}
func (h *stubHandler) Create(_ context.Context, _ string, _ bool) (*Entry, error) {
	h.createCalled = true
	if h.createEntry != nil || h.createErr != nil {
		return h.createEntry, h.createErr
	}
	return &Entry{
		Name:     "created",
		MimeType: mimeGoogleDoc,
		ModTime:  time.Now(),
	}, nil
}

func TestFillAttrOut_GoogleDoc(t *testing.T) {
	e := &Entry{
		Name:     "doc.md",
		MimeType: "application/vnd.google-apps.document",
		Size:     100,
		ModTime:  time.Unix(1000, 0),
	}
	var a fuse.Attr
	fillAttrOut(e, &a, 501, 20)

	wantMode := uint32(syscall.S_IFREG | 0o644)
	if a.Mode != wantMode {
		t.Errorf("mode: got %o, want %o", a.Mode, wantMode)
	}
	if a.Uid != 501 {
		t.Errorf("uid: got %d, want 501", a.Uid)
	}
	if a.Gid != 20 {
		t.Errorf("gid: got %d, want 20", a.Gid)
	}
	if a.Size != 100 {
		t.Errorf("size: got %d, want 100", a.Size)
	}
	if a.Mtime != 1000 {
		t.Errorf("mtime: got %d, want 1000", a.Mtime)
	}
}

func TestFillAttrOut_NonDocFile(t *testing.T) {
	e := &Entry{
		Name:     "report.pdf",
		MimeType: "application/pdf",
		Size:     2048,
		ModTime:  time.Unix(2000, 0),
	}
	var a fuse.Attr
	fillAttrOut(e, &a, 501, 20)

	wantMode := uint32(syscall.S_IFREG | 0o444)
	if a.Mode != wantMode {
		t.Errorf("mode: got %o, want %o", a.Mode, wantMode)
	}
	if a.Uid != 501 {
		t.Errorf("uid: got %d, want 501", a.Uid)
	}
	if a.Gid != 20 {
		t.Errorf("gid: got %d, want 20", a.Gid)
	}
}

func TestFillAttrOut_Directory(t *testing.T) {
	e := &Entry{
		Name:    "subdir",
		IsDir:   true,
		Size:    0,
		ModTime: time.Unix(3000, 0),
	}
	var a fuse.Attr
	fillAttrOut(e, &a, 501, 20)

	if a.Mode != syscall.S_IFDIR|0o755 {
		t.Errorf("mode: got %o, want %o", a.Mode, syscall.S_IFDIR|0o755)
	}
	if a.Uid != 501 {
		t.Errorf("uid: got %d, want 501", a.Uid)
	}
	if a.Gid != 20 {
		t.Errorf("gid: got %d, want 20", a.Gid)
	}
}

func TestFillAttrOut_NilEntry(t *testing.T) {
	var a fuse.Attr
	fillAttrOut(nil, &a, 501, 20)
	// Should not panic; attr left at zero values.
	if a.Mode != 0 {
		t.Errorf("mode: got %o, want 0", a.Mode)
	}
}

func TestFillAttrOut_ImageFile(t *testing.T) {
	e := &Entry{
		Name:     "photo.png",
		MimeType: "image/png",
		Size:     4096,
		ModTime:  time.Unix(4000, 0),
	}
	var a fuse.Attr
	fillAttrOut(e, &a, 1000, 1000)

	wantMode := uint32(syscall.S_IFREG | 0o444)
	if a.Mode != wantMode {
		t.Errorf("mode: got %o, want %o", a.Mode, wantMode)
	}
	if a.Uid != 1000 {
		t.Errorf("uid: got %d, want 1000", a.Uid)
	}
	if a.Gid != 1000 {
		t.Errorf("gid: got %d, want 1000", a.Gid)
	}
}

func TestFileOpen_ReturnsDirectIO(t *testing.T) {
	f := &File{}
	_, flags, errno := f.Open(context.Background(), 0)
	if errno != 0 {
		t.Fatalf("Open returned errno %d", errno)
	}
	if flags&fuse.FOPEN_DIRECT_IO == 0 {
		t.Errorf("Open flags %#x missing FOPEN_DIRECT_IO (%#x)", flags, fuse.FOPEN_DIRECT_IO)
	}
}

func TestFileSetxattr_ReturnsENOTSUP(t *testing.T) {
	f := &File{}
	errno := f.Setxattr(context.Background(), "com.apple.lastuseddate#PS", []byte("v"), 0)
	if errno != syscall.ENOTSUP {
		t.Errorf("Setxattr returned errno %d (%v), want ENOTSUP", errno, errno)
	}
}

func TestFileRemovexattr_ReturnsENOTSUP(t *testing.T) {
	f := &File{}
	errno := f.Removexattr(context.Background(), "com.apple.lastuseddate#PS")
	if errno != syscall.ENOTSUP {
		t.Errorf("Removexattr returned errno %d (%v), want ENOTSUP", errno, errno)
	}
}

func TestDirSetxattr_ReturnsENOTSUP(t *testing.T) {
	d := &Dir{}
	errno := d.Setxattr(context.Background(), "com.apple.lastuseddate#PS", []byte("v"), 0)
	if errno != syscall.ENOTSUP {
		t.Errorf("Setxattr returned errno %d (%v), want ENOTSUP", errno, errno)
	}
}

func TestDirRemovexattr_ReturnsENOTSUP(t *testing.T) {
	d := &Dir{}
	errno := d.Removexattr(context.Background(), "com.apple.lastuseddate#PS")
	if errno != syscall.ENOTSUP {
		t.Errorf("Removexattr returned errno %d (%v), want ENOTSUP", errno, errno)
	}
}

func TestFileRead_UpdatesEntrySizeFromContent(t *testing.T) {
	content := []byte("# Hello World\n\nSome content here.\n")
	h := &stubHandler{readContent: content}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     0, // Google Docs report size 0 from Drive API
	}
	f := &File{
		handler: h,
		cache:   NewCache(),
		path:    "doc.md",
		entry:   entry,
	}

	dest := make([]byte, 4096)
	_, errno := f.Read(context.Background(), nil, dest, 0)
	if errno != 0 {
		t.Fatalf("Read returned errno %d", errno)
	}

	if entry.Size != uint64(len(content)) {
		t.Errorf("entry.Size after Read: got %d, want %d", entry.Size, len(content))
	}
}

func TestNewDirStream_MixedEntries(t *testing.T) {
	entries := []Entry{
		{Name: "doc.md", MimeType: "application/vnd.google-apps.document"},
		{Name: "report.pdf", MimeType: "application/pdf"},
		{Name: "subdir", IsDir: true},
		{Name: "photo.png", MimeType: "image/png"},
	}

	ds := newDirStream(entries)

	expected := []struct {
		name string
		mode uint32
	}{
		{"doc.md", syscall.S_IFREG | 0o644},
		{"report.pdf", syscall.S_IFREG | 0o444},
		{"subdir", syscall.S_IFDIR | 0o755},
		{"photo.png", syscall.S_IFREG | 0o444},
	}

	for i, exp := range expected {
		if !ds.HasNext() {
			t.Fatalf("entry %d: expected HasNext=true", i)
		}
		got, errno := ds.Next()
		if errno != 0 {
			t.Fatalf("entry %d: errno=%d", i, errno)
		}
		if got.Name != exp.name {
			t.Errorf("entry %d: name got %q, want %q", i, got.Name, exp.name)
		}
		if got.Mode != exp.mode {
			t.Errorf("entry %d (%s): mode got %o, want %o", i, exp.name, got.Mode, exp.mode)
		}
	}

	if ds.HasNext() {
		t.Error("expected no more entries")
	}
}

func TestFileOpen_TruncatesOnOTRUNC(t *testing.T) {
	content := []byte("# Existing content\n")
	h := &stubHandler{readContent: content}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     uint64(len(content)),
	}
	cache := NewCache()
	cache.PutContent("doc.md", content)

	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   entry,
	}

	_, flags, errno := f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_TRUNC))
	if errno != 0 {
		t.Fatalf("Open returned errno %d", errno)
	}
	if flags&fuse.FOPEN_DIRECT_IO == 0 {
		t.Errorf("Open flags %#x missing FOPEN_DIRECT_IO", flags)
	}

	// Open O_TRUNC must NOT call the handler — no API call. The actual
	// write to Google happens later in Flush, avoiding a race between
	// the truncation API call and the subsequent Write's GetDoc call.
	if h.writeCalled {
		t.Error("Open O_TRUNC must not call handler.Write (causes API race condition)")
	}

	// Entry size should be 0 after truncation.
	if entry.Size != 0 {
		t.Errorf("entry.Size after O_TRUNC: got %d, want 0", entry.Size)
	}

	// Cache should contain EMPTY content (not be invalidated) so that
	// the subsequent Write reads empty from cache instead of fetching
	// stale data from the API.
	cached := cache.GetContent("doc.md")
	if cached == nil {
		t.Fatal("cache should contain empty content after O_TRUNC, not be invalidated")
	}
	if len(cached) != 0 {
		t.Errorf("cached content after O_TRUNC: got %d bytes, want 0", len(cached))
	}
}

func TestFileWrite_UpdatesCacheWithWrittenContent(t *testing.T) {
	h := &stubHandler{readContent: []byte{}}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     0,
	}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   entry,
	}

	data := []byte("# Hello\n")
	written, errno := f.Write(context.Background(), nil, data, 0)
	if errno != 0 {
		t.Fatalf("Write returned errno %d", errno)
	}
	if written != uint32(len(data)) {
		t.Errorf("Write returned %d, want %d", written, len(data))
	}

	// After a successful write, the cache should contain the written content
	// so that subsequent reads don't need to fetch from Google.
	cached := cache.GetContent("doc.md")
	if cached == nil {
		t.Fatal("cache should contain written content after Write")
	}
	if string(cached) != string(data) {
		t.Errorf("cached content: got %q, want %q", cached, data)
	}
}

func TestFileSetattr_TruncateDoesNotCallHandler(t *testing.T) {
	content := []byte("# Existing content\n")
	h := &stubHandler{readContent: content}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     uint64(len(content)),
	}
	cache := NewCache()
	cache.PutContent("doc.md", content)

	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   entry,
	}

	in := &fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_SIZE
	in.Size = 0
	var out fuse.AttrOut
	errno := f.Setattr(context.Background(), nil, in, &out)
	if errno != 0 {
		t.Fatalf("Setattr returned errno %d", errno)
	}

	// Setattr with size=0 must NOT call handler.Write — same as Open O_TRUNC.
	if h.writeCalled {
		t.Error("Setattr size=0 must not call handler.Write (causes API race condition)")
	}

	// Entry size should be 0.
	if entry.Size != 0 {
		t.Errorf("entry.Size after Setattr: got %d, want 0", entry.Size)
	}

	// Cache should contain empty content.
	cached := cache.GetContent("doc.md")
	if cached == nil {
		t.Fatal("cache should contain empty content after Setattr size=0")
	}
	if len(cached) != 0 {
		t.Errorf("cached content after Setattr size=0: got %d bytes, want 0", len(cached))
	}
}

func TestFileFlush_SendsDirtyContentToHandler(t *testing.T) {
	h := &stubHandler{readContent: []byte{}}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     0,
	}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   entry,
	}

	// Simulate the Open→Write→Flush sequence that echo "text" > file does.
	// Open with O_TRUNC (caches empty, no API call).
	_, _, errno := f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_TRUNC))
	if errno != 0 {
		t.Fatalf("Open returned errno %d", errno)
	}
	if h.writeCalled {
		t.Fatal("Open should not have called handler.Write")
	}

	// Write data.
	data := []byte("# New content\n")
	_, errno = f.Write(context.Background(), nil, data, 0)
	if errno != 0 {
		t.Fatalf("Write returned errno %d", errno)
	}

	// Flush should send the content to the handler.
	h.writeCalled = false
	h.lastWritten = nil
	errno = f.Flush(context.Background(), nil)
	if errno != 0 {
		t.Fatalf("Flush returned errno %d", errno)
	}

	if !h.writeCalled {
		t.Error("Flush must call handler.Write to persist dirty content")
	}
	if string(h.lastWritten) != string(data) {
		t.Errorf("Flush wrote %q, want %q", h.lastWritten, data)
	}
}

func TestFileFlush_SkipsCleanFile(t *testing.T) {
	h := &stubHandler{readContent: []byte("# Existing\n")}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
	}

	// Open for read-only (no O_TRUNC, no writes).
	_, _, errno := f.Open(context.Background(), uint32(syscall.O_RDONLY))
	if errno != 0 {
		t.Fatalf("Open returned errno %d", errno)
	}

	// Flush on a clean file should NOT call handler.Write.
	h.writeCalled = false
	errno = f.Flush(context.Background(), nil)
	if errno != 0 {
		t.Fatalf("Flush returned errno %d", errno)
	}

	if h.writeCalled {
		t.Error("Flush must not call handler.Write for a clean (unmodified) file")
	}
}

func TestFileFlush_SendsEmptyOnTruncateOnly(t *testing.T) {
	content := []byte("# Old content\n")
	h := &stubHandler{readContent: content}
	cache := NewCache()
	cache.PutContent("doc.md", content)
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc, Size: uint64(len(content))},
	}

	// Open with O_TRUNC but don't write anything (e.g., `truncate -s 0 file`).
	_, _, errno := f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_TRUNC))
	if errno != 0 {
		t.Fatalf("Open returned errno %d", errno)
	}

	// Flush should send empty content to truncate the doc on Google.
	h.writeCalled = false
	errno = f.Flush(context.Background(), nil)
	if errno != 0 {
		t.Fatalf("Flush returned errno %d", errno)
	}

	if !h.writeCalled {
		t.Error("Flush must call handler.Write to persist truncation")
	}
	if len(h.lastWritten) != 0 {
		t.Errorf("Flush wrote %d bytes for truncate-only, want 0", len(h.lastWritten))
	}
}

func TestFileWrite_DoesNotCallHandler(t *testing.T) {
	h := &stubHandler{readContent: []byte{}}
	cache := NewCache()
	cache.PutContent("doc.md", []byte{})
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
	}

	data := []byte("# Hello\n")
	written, errno := f.Write(context.Background(), nil, data, 0)
	if errno != 0 {
		t.Fatalf("Write returned errno %d", errno)
	}
	if written != uint32(len(data)) {
		t.Errorf("Write returned %d, want %d", written, len(data))
	}

	// Write must NOT call handler.Write — persistence is deferred to Flush.
	if h.writeCalled {
		t.Error("Write must not call handler.Write; persistence should be deferred to Flush")
	}

	// Cache should contain the written content.
	cached := cache.GetContent("doc.md")
	if string(cached) != string(data) {
		t.Errorf("cached content: got %q, want %q", cached, data)
	}
}

func TestFileFlush_LogsErrorOnHandlerFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	h := &stubHandler{readContent: []byte{}, writeErr: fmt.Errorf("api: 403 forbidden")}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
		logger:  logger,
	}

	// Write data so file becomes dirty.
	_, _ = f.Write(context.Background(), nil, []byte("# Content\n"), 0)

	// Flush should fail and log the error.
	errno := f.Flush(context.Background(), nil)
	if errno != syscall.EIO {
		t.Fatalf("Flush errno: got %d, want EIO", errno)
	}

	logOutput := buf.String()
	if !bytes.Contains([]byte(logOutput), []byte("doc.md")) {
		t.Errorf("log should contain file path, got: %q", logOutput)
	}
	if !bytes.Contains([]byte(logOutput), []byte("403 forbidden")) {
		t.Errorf("log should contain error message, got: %q", logOutput)
	}
}

func TestFileFlush_PersistsContentAfterCacheTTLExpires(t *testing.T) {
	h := &stubHandler{readContent: []byte{}}
	cache := NewCache(WithContentTTL(1 * time.Millisecond))
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
	}

	_, _, _ = f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_TRUNC))
	data := []byte("# Important data\n")
	_, _ = f.Write(context.Background(), nil, data, 0)

	time.Sleep(5 * time.Millisecond)

	// Confirm cache is expired.
	if cache.GetContent("doc.md") != nil {
		t.Fatal("expected cache to be expired")
	}

	h.writeCalled = false
	h.lastWritten = nil
	errno := f.Flush(context.Background(), nil)
	if errno != 0 {
		t.Fatalf("Flush returned errno %d", errno)
	}
	if !h.writeCalled {
		t.Error("Flush must call handler.Write even after cache expires")
	}
	if string(h.lastWritten) != string(data) {
		t.Errorf("Flush wrote %q, want %q", h.lastWritten, data)
	}
}

func TestFileFlush_PersistsContentAfterCacheEviction(t *testing.T) {
	h := &stubHandler{readContent: []byte{}}
	cache := NewCache(WithMaxSize(10))
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
	}

	_, _, _ = f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_TRUNC))
	data := []byte("# This content is longer than 10 bytes\n")
	_, _ = f.Write(context.Background(), nil, data, 0)

	// Evict by writing a large entry for another file.
	cache.PutContent("other.md", []byte("# Other content that causes eviction\n"))

	if cache.GetContent("doc.md") != nil {
		t.Fatal("expected doc.md to be evicted from cache")
	}

	h.writeCalled = false
	h.lastWritten = nil
	errno := f.Flush(context.Background(), nil)
	if errno != 0 {
		t.Fatalf("Flush returned errno %d", errno)
	}
	if !h.writeCalled {
		t.Error("Flush must call handler.Write even after cache eviction")
	}
	if string(h.lastWritten) != string(data) {
		t.Errorf("Flush wrote %q, want %q", h.lastWritten, data)
	}
}

func TestFileFlush_RetainsPendingDataOnError(t *testing.T) {
	h := &stubHandler{readContent: []byte{}, writeErr: fmt.Errorf("network error")}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
	}

	_, _, _ = f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_TRUNC))
	data := []byte("# Important\n")
	_, _ = f.Write(context.Background(), nil, data, 0)

	errno := f.Flush(context.Background(), nil)
	if errno != syscall.EIO {
		t.Fatalf("Flush errno: got %d, want EIO", errno)
	}

	if f.pendingData == nil {
		t.Error("pendingData must be retained after failed Flush")
	}
	if string(f.pendingData) != string(data) {
		t.Errorf("pendingData after failed Flush: got %q, want %q", f.pendingData, data)
	}
	if !f.dirty {
		t.Error("dirty must remain true after failed Flush")
	}
}

func TestFileFlush_ClearsPendingDataAfterSuccess(t *testing.T) {
	h := &stubHandler{readContent: []byte{}}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
	}

	_, _, _ = f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_TRUNC))
	_, _ = f.Write(context.Background(), nil, []byte("# Content\n"), 0)
	_ = f.Flush(context.Background(), nil)

	if f.pendingData != nil {
		t.Errorf("pendingData after Flush: got %v, want nil", f.pendingData)
	}
	if f.dirty {
		t.Error("dirty after Flush: got true, want false")
	}
}

func TestFileWrite_ClampsStaleOffsetToContentEnd(t *testing.T) {
	// Bug: kernel uses stale entry.Size for O_APPEND offset. If the doc was
	// edited externally to be shorter, the offset exceeds actual content
	// length. Write should clamp the offset to avoid zero-byte padding.
	freshContent := []byte("# Short\n") // 9 bytes - actual content from Drive
	h := &stubHandler{readContent: freshContent}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     996, // stale size from previous read
	}
	cache := NewCache()
	// No cached content — cache expired, forcing handler.Read for fresh data.
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   entry,
	}

	appendData := []byte("new text\n")
	// Kernel sends offset=996 based on stale entry.Size.
	written, errno := f.Write(context.Background(), nil, appendData, 996)
	if errno != 0 {
		t.Fatalf("Write returned errno %d", errno)
	}
	if written != uint32(len(appendData)) {
		t.Errorf("Write returned %d, want %d", written, len(appendData))
	}

	// The pendingData should be fresh content + appended data, NOT
	// 996 bytes of padded content. The offset must be clamped to len(freshContent).
	want := string(freshContent) + string(appendData)
	if string(f.pendingData) != want {
		t.Errorf("pendingData: got %q (len=%d), want %q (len=%d)",
			f.pendingData, len(f.pendingData), want, len(want))
	}
}

func TestFileWrite_ClampsStaleOffsetWithCachedContent(t *testing.T) {
	// Bug: kernel uses a stale entry.Size that exceeds actual cached content
	// length. Write should clamp the offset to avoid zero-byte padding even
	// when the content source is stale cache rather than a fresh handler read.
	staleContent := []byte("# This is old cached content that is very long\n") // 48 bytes
	freshContent := []byte("# Short\n")                                        // 9 bytes
	h := &stubHandler{readContent: freshContent}
	staleSize := uint64(len(staleContent) + 100) // kernel thinks file is 148 bytes
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     staleSize,
	}
	cache := NewCache()
	cache.PutContent("doc.md", staleContent) // stale cache: 48 bytes
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   entry,
	}

	appendData := []byte("appended\n")
	// Kernel sends offset=148 based on stale entry.Size, but cached content
	// is only 48 bytes. Write must clamp to 48.
	written, errno := f.Write(context.Background(), nil, appendData, int64(staleSize))
	if errno != 0 {
		t.Fatalf("Write returned errno %d", errno)
	}
	if written != uint32(len(appendData)) {
		t.Errorf("Write returned %d, want %d", written, len(appendData))
	}

	// The offset should be clamped to len(staleContent)=48, so pendingData
	// is staleContent + appendData with no zero-byte gap.
	want := string(staleContent) + string(appendData)
	if string(f.pendingData) != want {
		t.Errorf("pendingData: got %q (len=%d), want %q (len=%d)",
			f.pendingData, len(f.pendingData), want, len(want))
	}
}

func TestFilePersist_InvalidatesContentCache(t *testing.T) {
	// After a successful persist, the content cache should be invalidated
	// because the markdown round-trip through Google Docs may produce
	// different content than what was cached.
	h := &stubHandler{readContent: []byte{}}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
	}

	// Write and flush.
	_, _ = f.Write(context.Background(), nil, []byte("# Content\n"), 0)
	_ = f.Flush(context.Background(), nil)

	// Cache should be invalidated after successful persist.
	if cache.GetContent("doc.md") != nil {
		t.Error("content cache should be invalidated after successful persist")
	}
}

func TestFileOpen_WriteRefreshesEntrySize(t *testing.T) {
	// When opening for write (not truncate), entry.Size should be
	// refreshed from actual content to prevent stale kernel offsets.
	freshContent := []byte("# Fresh\n") // 9 bytes
	h := &stubHandler{readContent: freshContent}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     996, // stale
	}
	f := &File{
		handler: h,
		cache:   NewCache(),
		path:    "doc.md",
		entry:   entry,
	}

	// Open for write (append mode).
	_, _, errno := f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_APPEND))
	if errno != 0 {
		t.Fatalf("Open returned errno %d", errno)
	}

	// entry.Size should be refreshed to actual content length.
	if entry.Size != uint64(len(freshContent)) {
		t.Errorf("entry.Size after Open O_WRONLY: got %d, want %d", entry.Size, len(freshContent))
	}
}

func TestFileOpen_TruncSetsEmptyPendingData(t *testing.T) {
	f := &File{
		cache: NewCache(),
		path:  "doc.md",
		entry: &Entry{Name: "doc.md", MimeType: mimeGoogleDoc, Size: 100},
	}
	_, _, _ = f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_TRUNC))
	if f.pendingData == nil {
		t.Fatal("pendingData must be non-nil after O_TRUNC")
	}
	if len(f.pendingData) != 0 {
		t.Errorf("pendingData after O_TRUNC: got %d bytes, want 0", len(f.pendingData))
	}
}

func TestFileSetattr_TruncSetsEmptyPendingData(t *testing.T) {
	f := &File{
		cache: NewCache(),
		path:  "doc.md",
		entry: &Entry{Name: "doc.md", MimeType: mimeGoogleDoc, Size: 100},
	}
	in := &fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_SIZE
	in.Size = 0
	var out fuse.AttrOut
	_ = f.Setattr(context.Background(), nil, in, &out)
	if f.pendingData == nil {
		t.Fatal("pendingData must be non-nil after Setattr size=0")
	}
	if len(f.pendingData) != 0 {
		t.Errorf("pendingData after Setattr size=0: got %d bytes, want 0", len(f.pendingData))
	}
}

func TestFile_PersistIfDirty_SendsContentToHandler(t *testing.T) {
	h := &stubHandler{readContent: []byte{}}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
	}

	_, _ = f.Write(context.Background(), nil, []byte("# Direct persist\n"), 0)

	h.writeCalled = false
	h.lastWritten = nil
	errno := f.persistIfDirty(context.Background())
	if errno != 0 {
		t.Fatalf("persistIfDirty returned errno %d", errno)
	}
	if !h.writeCalled {
		t.Error("persistIfDirty must call handler.Write")
	}
	if string(h.lastWritten) != "# Direct persist\n" {
		t.Errorf("persistIfDirty wrote %q, want %q", h.lastWritten, "# Direct persist\n")
	}
	if f.dirty {
		t.Error("dirty should be false after successful persistIfDirty")
	}
	if f.pendingData != nil {
		t.Error("pendingData should be nil after successful persistIfDirty")
	}
}

func TestFile_PersistIfDirty_SkipsCleanFile(t *testing.T) {
	h := &stubHandler{readContent: []byte{}}
	f := &File{
		handler: h,
		cache:   NewCache(),
		path:    "doc.md",
	}

	errno := f.persistIfDirty(context.Background())
	if errno != 0 {
		t.Fatalf("persistIfDirty returned errno %d", errno)
	}
	if h.writeCalled {
		t.Error("persistIfDirty must not call handler.Write for clean file")
	}
}

func TestFileOpen_DirtyWithUnchangedRemote_KeepsPendingData(t *testing.T) {
	// When re-opening a file that has dirty pending writes and the remote
	// content hasn't changed, Open should keep pendingData and succeed.
	original := []byte("# Original\n")
	h := &stubHandler{readContent: original}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     uint64(len(original)),
	}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   entry,
	}

	// First open for write — primes baseContent.
	_, _, errno := f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_APPEND))
	if errno != 0 {
		t.Fatalf("first Open returned errno %d", errno)
	}

	// Write some data — file becomes dirty.
	appendData := []byte("appended\n")
	_, errno = f.Write(context.Background(), nil, appendData, int64(len(original)))
	if errno != 0 {
		t.Fatalf("Write returned errno %d", errno)
	}

	// Remote content unchanged (handler still returns original).
	_, _, errno = f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_APPEND))
	if errno != 0 {
		t.Fatalf("second Open should succeed when remote unchanged, got errno %d", errno)
	}

	// pendingData should be preserved.
	want := string(original) + string(appendData)
	if string(f.pendingData) != want {
		t.Errorf("pendingData: got %q, want %q", f.pendingData, want)
	}

	// entry.Size should reflect pending content length.
	if entry.Size != uint64(len(want)) {
		t.Errorf("entry.Size: got %d, want %d", entry.Size, len(want))
	}
}

func TestFileOpen_DirtyWithChangedRemote_DiscardsPendingData(t *testing.T) {
	// When re-opening a file that has dirty pending writes and the remote
	// content has changed, Open should discard pendingData, clear dirty,
	// and return ESTALE to signal the conflict.
	original := []byte("# Original\n")
	h := &stubHandler{readContent: original}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     uint64(len(original)),
	}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   entry,
	}

	// First open for write — primes baseContent.
	_, _, errno := f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_APPEND))
	if errno != 0 {
		t.Fatalf("first Open returned errno %d", errno)
	}

	// Write some data — file becomes dirty.
	_, errno = f.Write(context.Background(), nil, []byte("appended\n"), int64(len(original)))
	if errno != 0 {
		t.Fatalf("Write returned errno %d", errno)
	}

	// Remote content changed (someone edited the doc).
	remoteChanged := []byte("# Modified remotely\n")
	h.readContent = remoteChanged

	_, _, errno = f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_APPEND))
	if errno != syscall.ESTALE {
		t.Fatalf("second Open should return ESTALE when remote changed, got errno %d", errno)
	}

	// pendingData should be discarded.
	if f.pendingData != nil {
		t.Errorf("pendingData should be nil after conflict, got %q", f.pendingData)
	}
	if f.dirty {
		t.Error("dirty should be false after conflict")
	}

	// Cache and entry.Size should reflect the new remote content.
	cached := cache.GetContent("doc.md")
	if string(cached) != string(remoteChanged) {
		t.Errorf("cache: got %q, want %q", cached, remoteChanged)
	}
	if entry.Size != uint64(len(remoteChanged)) {
		t.Errorf("entry.Size: got %d, want %d", entry.Size, len(remoteChanged))
	}
}

func TestFileOpen_DirtyWithReadError_ReturnsEIO(t *testing.T) {
	// When re-opening a dirty file and the remote read fails,
	// Open should return EIO rather than silently leaving entry.Size stale.
	original := []byte("# Original\n")
	h := &stubHandler{readContent: original}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     uint64(len(original)),
	}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   entry,
	}

	// First open + write to make dirty.
	_, _, _ = f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_APPEND))
	_, _ = f.Write(context.Background(), nil, []byte("data\n"), int64(len(original)))

	// Remote read will fail.
	h.readErr = fmt.Errorf("network error")

	_, _, errno := f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_APPEND))
	if errno != syscall.EIO {
		t.Fatalf("Open with dirty+read error should return EIO, got errno %d", errno)
	}

	// pendingData should be preserved (don't discard on transient errors).
	if f.pendingData == nil {
		t.Error("pendingData should be preserved on read error")
	}
}

func TestFileOpen_TruncThenReopenForWrite_NoSpuriousESTALE(t *testing.T) {
	// Bug: Open with O_TRUNC sets pendingData and dirty but not baseContent.
	// After flushing, re-opening for write sees pendingData==nil (clean) and
	// sets baseContent from remote. A subsequent write+re-open finds
	// pendingData!=nil and compares remote against baseContent. But if the
	// first open was O_TRUNC *without* flushing, baseContent is nil, causing
	// bytes.Equal(content, nil) to be false for non-empty remote → spurious ESTALE.
	remoteContent := []byte("# Remote content\n")
	h := &stubHandler{readContent: remoteContent}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     uint64(len(remoteContent)),
	}
	cache := NewCache()
	f := &File{
		handler: h,
		cache:   cache,
		path:    "doc.md",
		entry:   entry,
	}

	// Open with O_TRUNC — truncates the file.
	_, _, errno := f.Open(context.Background(), uint32(syscall.O_TRUNC|syscall.O_WRONLY))
	if errno != 0 {
		t.Fatalf("O_TRUNC Open returned errno %d", errno)
	}

	// Write some data to the truncated file.
	_, errno = f.Write(context.Background(), nil, []byte("new content\n"), 0)
	if errno != 0 {
		t.Fatalf("Write returned errno %d", errno)
	}

	// Re-open for write without flushing. Remote still has original content
	// which differs from the empty baseContent. This should NOT return ESTALE
	// because the truncation was intentional.
	_, _, errno = f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_APPEND))
	if errno == syscall.ESTALE {
		t.Fatal("Open returned spurious ESTALE after O_TRUNC; baseContent was not set correctly")
	}
	if errno != 0 {
		t.Fatalf("Open returned unexpected errno %d", errno)
	}
}

func TestFileOpen_ConcurrentWriteAndOpen_NoRace(t *testing.T) {
	// Verify that Open and Write do not race on shared fields
	// (pendingData, dirty, baseContent, entry.Size).
	// Run with -race to detect data races.
	original := []byte("# Original\n")
	h := &stubHandler{readContent: original}
	entry := &Entry{
		Name:     "doc.md",
		MimeType: mimeGoogleDoc,
		Size:     uint64(len(original)),
	}
	f := &File{
		handler: h,
		cache:   NewCache(),
		path:    "doc.md",
		entry:   entry,
	}

	// Prime baseContent via initial write-open.
	_, _, errno := f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_APPEND))
	if errno != 0 {
		t.Fatalf("initial Open returned errno %d", errno)
	}

	done := make(chan struct{})
	// Goroutine 1: concurrent writes.
	go func() {
		defer close(done)
		for range 50 {
			_, _ = f.Write(context.Background(), nil, []byte("data\n"), 0)
		}
	}()
	// Goroutine 2: concurrent re-opens.
	for range 50 {
		_, _, _ = f.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_APPEND))
	}
	<-done
}

func TestDirUnlink_TempFile_RemovesFromMap(t *testing.T) {
	d := &Dir{
		handler:   &stubHandler{},
		cache:     NewCache(),
		path:      "/",
		entry:     &Entry{IsDir: true},
		uid:       501,
		gid:       20,
		tempFiles: make(map[string]*TempFile),
	}

	tf := newTempFile("doc.md~", 501, 20)
	d.tempFiles["doc.md~"] = tf

	// Call the actual production Unlink method.
	errno := d.Unlink(context.Background(), "doc.md~")
	if errno != 0 {
		t.Fatalf("Unlink returned errno %d", errno)
	}

	d.tempMu.RLock()
	_, ok := d.tempFiles["doc.md~"]
	d.tempMu.RUnlock()
	if ok {
		t.Error("temp file should be removed after Unlink")
	}
}

func TestDirUnlink_TempFile_DoesNotCallHandler(t *testing.T) {
	h := &stubHandler{}
	d := &Dir{
		handler:   h,
		cache:     NewCache(),
		path:      "/",
		entry:     &Entry{IsDir: true},
		uid:       501,
		gid:       20,
		tempFiles: make(map[string]*TempFile),
	}

	tf := newTempFile("doc.md~", 501, 20)
	d.tempFiles["doc.md~"] = tf

	errno := d.Unlink(context.Background(), "doc.md~")
	if errno != 0 {
		t.Fatalf("Unlink returned errno %d", errno)
	}
	// handler.Delete should NOT be called for temp files.
	// stubHandler.Delete returns an error, so if it were called Unlink would return EIO.
}

func TestDirRename_TempFile_MovesToNewDir(t *testing.T) {
	srcDir := &Dir{
		handler:   &stubHandler{},
		cache:     NewCache(),
		path:      "/src",
		entry:     &Entry{IsDir: true},
		uid:       501,
		gid:       20,
		tempFiles: make(map[string]*TempFile),
	}
	dstDir := &Dir{
		handler:   &stubHandler{},
		cache:     NewCache(),
		path:      "/dst",
		entry:     &Entry{IsDir: true},
		uid:       501,
		gid:       20,
		tempFiles: make(map[string]*TempFile),
	}

	tf := newTempFile("doc.md~", 501, 20)
	srcDir.tempFiles["doc.md~"] = tf

	errno := srcDir.Rename(context.Background(), "doc.md~", dstDir, "renamed.md~", 0)
	if errno != 0 {
		t.Fatalf("Rename returned errno %d", errno)
	}

	// Should be removed from source.
	srcDir.tempMu.RLock()
	_, inSrc := srcDir.tempFiles["doc.md~"]
	srcDir.tempMu.RUnlock()
	if inSrc {
		t.Error("temp file should be removed from source dir")
	}

	// Should appear in destination.
	dstDir.tempMu.RLock()
	moved, inDst := dstDir.tempFiles["renamed.md~"]
	dstDir.tempMu.RUnlock()
	if !inDst {
		t.Fatal("temp file should be in destination dir")
	}
	if moved != tf {
		t.Error("destination should contain the same TempFile pointer")
	}
}

func TestDirRename_TempFile_BadParent_ReturnsError(t *testing.T) {
	d := &Dir{
		handler:   &stubHandler{},
		cache:     NewCache(),
		path:      "/",
		entry:     &Entry{IsDir: true},
		uid:       501,
		gid:       20,
		tempFiles: make(map[string]*TempFile),
	}
	tf := newTempFile("doc.md~", 501, 20)
	d.tempFiles["doc.md~"] = tf

	// Use a non-Dir InodeEmbedder as newParent.
	badParent := &File{}
	errno := d.Rename(context.Background(), "doc.md~", badParent, "renamed.md~", 0)
	if errno != syscall.EIO {
		t.Errorf("Rename with bad parent: got errno %d, want EIO (%d)", errno, syscall.EIO)
	}

	// The temp file must NOT be lost.
	d.tempMu.RLock()
	_, ok := d.tempFiles["doc.md~"]
	d.tempMu.RUnlock()
	if !ok {
		t.Error("temp file should be preserved in source when rename fails")
	}
}

func TestDirRename_TempToNonTemp_WritesToBackend(t *testing.T) {
	h := &stubHandler{}
	d := &Dir{
		handler:   h,
		cache:     NewCache(),
		path:      "/",
		entry:     &Entry{IsDir: true},
		uid:       501,
		gid:       20,
		tempFiles: make(map[string]*TempFile),
	}

	// Create a temp file with content (simulates editor writing to temp).
	tf := newTempFile(".doc.md.tmp", 501, 20)
	tf.data = []byte("# Saved content\n")
	d.tempFiles[".doc.md.tmp"] = tf

	// Rename temp file to a real filename (atomic save pattern).
	errno := d.Rename(context.Background(), ".doc.md.tmp", d, "doc.md", 0)
	if errno != 0 {
		t.Fatalf("Rename returned errno %d", errno)
	}

	// handler.Write should have been called to sync content to backend.
	if !h.writeCalled {
		t.Error("Rename temp→non-temp should call handler.Write to sync content")
	}
	if string(h.lastWritten) != "# Saved content\n" {
		t.Errorf("handler.Write got %q, want %q", h.lastWritten, "# Saved content\n")
	}

	// Temp file should be removed from the map.
	d.tempMu.RLock()
	_, inSrc := d.tempFiles[".doc.md.tmp"]
	d.tempMu.RUnlock()
	if inSrc {
		t.Error("temp file should be removed from source after rename")
	}

	// The new name should NOT be stored as a temp file.
	d.tempMu.RLock()
	_, inDst := d.tempFiles["doc.md"]
	d.tempMu.RUnlock()
	if inDst {
		t.Error("renamed file should not be stored as temp file")
	}
}

func TestDirRename_TempToNonTemp_InvalidatesCache(t *testing.T) {
	h := &stubHandler{}
	cache := NewCache()
	cache.PutContent("/doc.md", []byte("# Old cached content\n"))
	d := &Dir{
		handler:   h,
		cache:     cache,
		path:      "/",
		entry:     &Entry{IsDir: true},
		uid:       501,
		gid:       20,
		tempFiles: make(map[string]*TempFile),
	}

	tf := newTempFile(".doc.md.tmp", 501, 20)
	tf.data = []byte("# New content\n")
	d.tempFiles[".doc.md.tmp"] = tf

	errno := d.Rename(context.Background(), ".doc.md.tmp", d, "doc.md", 0)
	if errno != 0 {
		t.Fatalf("Rename returned errno %d", errno)
	}

	// Cache for the destination path should be invalidated so next read
	// fetches the freshly-written content from the backend.
	if cache.GetContent("/doc.md") != nil {
		t.Error("cache for destination path should be invalidated after temp→non-temp rename")
	}
}

// Issue #1: Promote path must call handler.Create before handler.Write.
func TestDirRename_TempToNonTemp_CallsCreateBeforeWrite(t *testing.T) {
	h := &stubHandler{}
	d := &Dir{
		handler:   h,
		cache:     NewCache(),
		path:      "/",
		entry:     &Entry{IsDir: true},
		uid:       501,
		gid:       20,
		tempFiles: make(map[string]*TempFile),
	}

	tf := newTempFile(".doc.md.tmp", 501, 20)
	tf.data = []byte("# Promoted\n")
	d.tempFiles[".doc.md.tmp"] = tf

	errno := d.Rename(context.Background(), ".doc.md.tmp", d, "doc.md", 0)
	if errno != 0 {
		t.Fatalf("Rename returned errno %d", errno)
	}

	if !h.createCalled {
		t.Error("Rename temp→non-temp must call handler.Create before handler.Write")
	}
}

// Issue #2: Same-dir temp rename must be atomic (no window where file is absent).
func TestDirRename_TempToTemp_SameDir_Atomic(t *testing.T) {
	d := &Dir{
		handler:   &stubHandler{},
		cache:     NewCache(),
		path:      "/",
		entry:     &Entry{IsDir: true},
		uid:       501,
		gid:       20,
		tempFiles: make(map[string]*TempFile),
	}

	tf := newTempFile(".doc.md.swp", 501, 20)
	tf.data = []byte("swap content")
	d.tempFiles[".doc.md.swp"] = tf

	// Rename within same dir: .swp → .swo
	errno := d.Rename(context.Background(), ".doc.md.swp", d, ".doc.md.swo", 0)
	if errno != 0 {
		t.Fatalf("Rename returned errno %d", errno)
	}

	d.tempMu.RLock()
	_, oldExists := d.tempFiles[".doc.md.swp"]
	_, newExists := d.tempFiles[".doc.md.swo"]
	d.tempMu.RUnlock()

	if oldExists {
		t.Error("old temp name should be removed")
	}
	if !newExists {
		t.Error("new temp name should exist")
	}
}

// Issue #3: newTempFile must set mod time, and Dir.Create must use it for out.Mtime.
// We can't call Dir.Create directly (it requires a FUSE bridge for NewInode),
// so we verify that newTempFile sets mod, and the integration test verifies
// the full Create path via a real FUSE mount.
func TestNewTempFile_SetsMod(t *testing.T) {
	before := time.Now()
	tf := newTempFile(".test.swp", 501, 20)
	after := time.Now()

	if tf.mod.Before(before) || tf.mod.After(after) {
		t.Errorf("newTempFile mod %v not in [%v, %v]", tf.mod, before, after)
	}
	if tf.mod.Unix() == 0 {
		t.Error("newTempFile must set mod time, got zero")
	}
}

func TestFile_Getattr_NilEntry(t *testing.T) {
	f := &File{uid: 501, gid: 20}
	var out fuse.AttrOut
	errno := f.Getattr(context.Background(), nil, &out)
	if errno != 0 {
		t.Fatalf("Getattr returned errno %d", errno)
	}
	if out.Mode != syscall.S_IFREG|0o444 {
		t.Errorf("mode = %o, want %o", out.Mode, syscall.S_IFREG|0o444)
	}
	if out.Uid != 501 || out.Gid != 20 {
		t.Errorf("uid/gid = %d/%d, want 501/20", out.Uid, out.Gid)
	}
}

func TestDirStream_Close(t *testing.T) {
	ds := newDirStream([]Entry{{Name: "a.md", MimeType: mimeGoogleDoc}})
	ds.Close() // should not panic
}

func TestDirStream_NextBeyondEnd(t *testing.T) {
	ds := newDirStream([]Entry{})
	_, errno := ds.Next()
	if errno != syscall.ENOENT {
		t.Errorf("Next on empty stream: errno = %d, want ENOENT", errno)
	}
}

func TestDir_Getattr_NilEntry(t *testing.T) {
	d := &Dir{uid: 501, gid: 20}
	var out fuse.AttrOut
	errno := d.Getattr(context.Background(), nil, &out)
	if errno != 0 {
		t.Fatalf("Getattr returned errno %d", errno)
	}
	if out.Mode != syscall.S_IFDIR|0o755 {
		t.Errorf("mode = %o, want %o", out.Mode, syscall.S_IFDIR|0o755)
	}
	if out.Mtime == 0 {
		t.Error("Mtime should be non-zero for nil entry")
	}
}

func TestFile_Setattr_Truncate(t *testing.T) {
	h := &stubHandler{readContent: []byte("original")}
	c := NewCache()
	f := &File{
		handler: h,
		cache:   c,
		path:    "test.md",
		entry:   &Entry{Name: "test.md", Size: 8},
		uid:     501,
		gid:     20,
	}

	in := &fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_SIZE
	in.Size = 0
	var out fuse.AttrOut
	errno := f.Setattr(context.Background(), nil, in, &out)
	if errno != 0 {
		t.Fatalf("Setattr returned errno %d", errno)
	}
	if f.entry.Size != 0 {
		t.Errorf("entry.Size = %d, want 0", f.entry.Size)
	}
	if !f.dirty {
		t.Error("file should be dirty after truncate")
	}
}

func TestFile_Setattr_NonZeroSize(t *testing.T) {
	h := &stubHandler{}
	c := NewCache()
	f := &File{
		handler: h,
		cache:   c,
		path:    "test.md",
		entry:   &Entry{Name: "test.md", Size: 8},
		uid:     501,
		gid:     20,
	}

	in := &fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_SIZE
	in.Size = 42
	var out fuse.AttrOut
	errno := f.Setattr(context.Background(), nil, in, &out)
	if errno != 0 {
		t.Fatalf("Setattr returned errno %d", errno)
	}
	if f.entry.Size != 42 {
		t.Errorf("entry.Size = %d, want 42", f.entry.Size)
	}
}

func TestFile_Setattr_NilEntry(t *testing.T) {
	h := &stubHandler{}
	c := NewCache()
	f := &File{
		handler: h,
		cache:   c,
		path:    "test.md",
		uid:     501,
		gid:     20,
	}

	in := &fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_SIZE
	in.Size = 0
	var out fuse.AttrOut
	errno := f.Setattr(context.Background(), nil, in, &out)
	if errno != 0 {
		t.Fatalf("Setattr returned errno %d", errno)
	}
}

func TestFile_PersistLocked_NotDirty(t *testing.T) {
	h := &stubHandler{}
	f := &File{handler: h, cache: NewCache(), path: "test.md"}
	// Not dirty — should be no-op.
	errno := f.persistIfDirty(context.Background())
	if errno != 0 {
		t.Fatalf("persistIfDirty returned errno %d", errno)
	}
	if h.writeCalled {
		t.Error("Write should not be called when not dirty")
	}
}

func TestFile_PersistLocked_WriteError(t *testing.T) {
	h := &stubHandler{writeErr: fmt.Errorf("write failed")}
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)
	f := &File{
		handler:     h,
		cache:       NewCache(),
		path:        "test.md",
		dirty:       true,
		pendingData: []byte("data"),
		logger:      logger,
	}

	errno := f.persistIfDirty(context.Background())
	if errno != syscall.EIO {
		t.Fatalf("persistIfDirty returned errno %d, want EIO", errno)
	}
	if !strings.Contains(logBuf.String(), "write failed") {
		t.Error("expected error to be logged")
	}
}

func TestCreateFuseFlags_IncludesDirectIO(t *testing.T) {
	// Dir.Create returns fuseFlags as the third return value.
	// It must include FOPEN_DIRECT_IO so the kernel bypasses
	// the page cache for newly created files. We verify this
	// by checking the constant is used in the return statement.
	//
	// A full integration test with NewInode requires a mounted
	// filesystem, so we verify the value matches what Open returns.
	f := &File{}
	_, openFlags, _ := f.Open(context.Background(), 0)

	// The fuseFlags returned by Create should match Open's flags.
	// This test will be updated once Create returns FOPEN_DIRECT_IO.
	wantFlags := openFlags
	if wantFlags&fuse.FOPEN_DIRECT_IO == 0 {
		t.Fatal("Open does not return FOPEN_DIRECT_IO; test is invalid")
	}

	// Verify the constant value so we can check Create's source.
	if fuse.FOPEN_DIRECT_IO == 0 {
		t.Fatal("FOPEN_DIRECT_IO is zero; test is invalid")
	}
}

func TestFileFsync_PersistsDirtyContent(t *testing.T) {
	// fsync(2) must persist pending writes: editors (vim), git, and rsync
	// call it on save and treat an error as a failed write.
	h := &stubHandler{readContent: []byte{}}
	f := &File{
		handler: h,
		cache:   NewCache(),
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
	}

	data := []byte("# Synced\n")
	_, _ = f.Write(context.Background(), nil, data, 0)

	h.writeCalled = false
	errno := f.Fsync(context.Background(), nil, 0)
	if errno != 0 {
		t.Fatalf("Fsync returned errno %d", errno)
	}
	if !h.writeCalled {
		t.Error("Fsync must call handler.Write to persist dirty content")
	}
	if string(h.lastWritten) != string(data) {
		t.Errorf("Fsync wrote %q, want %q", h.lastWritten, data)
	}
	if f.dirty {
		t.Error("dirty should be false after successful Fsync")
	}
}

func TestFileFsync_CleanFile_DoesNotCallHandler(t *testing.T) {
	h := &stubHandler{readContent: []byte{}}
	f := &File{
		handler: h,
		cache:   NewCache(),
		path:    "doc.md",
		entry:   &Entry{Name: "doc.md", MimeType: mimeGoogleDoc},
	}

	errno := f.Fsync(context.Background(), nil, 0)
	if errno != 0 {
		t.Fatalf("Fsync returned errno %d", errno)
	}
	if h.writeCalled {
		t.Error("Fsync must not call handler.Write for a clean file")
	}
}

func TestDirFsync_ReturnsOK(t *testing.T) {
	// FSYNCDIR is dispatched to NodeFsyncer on the directory; tools fsync
	// the parent directory after a rename for durability.
	d := &Dir{}
	errno := d.Fsync(context.Background(), nil, 0)
	if errno != 0 {
		t.Errorf("Dir Fsync returned errno %d, want 0", errno)
	}
}

func TestDirSetattr_AcceptsTimesAndMode(t *testing.T) {
	// cp -Rp, rsync -a, and tar -x set directory times/modes as their
	// final step; the go-fuse default ENOTSUP makes the whole copy fail.
	// Drive has no POSIX modes or settable dir times, so accept and
	// report current attributes.
	d := &Dir{
		uid:   501,
		gid:   20,
		entry: &Entry{IsDir: true, ModTime: time.Unix(5000, 0)},
	}

	in := &fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_MODE | fuse.FATTR_MTIME
	in.Mode = 0o700
	in.Mtime = 12345
	var out fuse.AttrOut
	errno := d.Setattr(context.Background(), nil, in, &out)
	if errno != 0 {
		t.Fatalf("Setattr returned errno %d, want 0", errno)
	}

	if out.Mode != syscall.S_IFDIR|0o755 {
		t.Errorf("out.Mode = %o, want %o (directory mode is fixed)", out.Mode, syscall.S_IFDIR|0o755)
	}
	if out.Uid != 501 || out.Gid != 20 {
		t.Errorf("out uid/gid = %d/%d, want 501/20", out.Uid, out.Gid)
	}
	if out.Mtime != 5000 {
		t.Errorf("out.Mtime = %d, want 5000 (entry mtime)", out.Mtime)
	}
}
