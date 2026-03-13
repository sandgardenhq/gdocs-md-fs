package ragfs

import (
	"context"
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// stubHandler is a minimal Handler for unit-testing File methods.
type stubHandler struct {
	readContent []byte
	readErr     error
	writeErr    error
}

func (h *stubHandler) Read(_ context.Context, _ string) ([]byte, error) {
	return h.readContent, h.readErr
}
func (h *stubHandler) Write(_ context.Context, _ string, _ []byte) error {
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
	return nil, fmt.Errorf("not implemented")
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
