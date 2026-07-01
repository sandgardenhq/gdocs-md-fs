package gdrive

import (
	"context"
	"errors"
	iofs "io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestNormalizePath_EmptyString(t *testing.T) {
	if got := normalizePath(""); got != "/" {
		t.Errorf("normalizePath(%q) = %q, want %q", "", got, "/")
	}
}

func TestNormalizePath_NoLeadingSlash(t *testing.T) {
	if got := normalizePath("docs/file.md"); got != "/docs/file.md" {
		t.Errorf("normalizePath(%q) = %q, want %q", "docs/file.md", got, "/docs/file.md")
	}
}

func TestNormalizePath_WithLeadingSlash(t *testing.T) {
	if got := normalizePath("/docs/file.md"); got != "/docs/file.md" {
		t.Errorf("normalizePath(%q) = %q, want %q", "/docs/file.md", got, "/docs/file.md")
	}
}

func TestNormalizePath_Dot(t *testing.T) {
	if got := normalizePath("."); got != "/" {
		t.Errorf("normalizePath(%q) = %q, want %q", ".", got, "/")
	}
}

func TestNormalizePath_RootSlash(t *testing.T) {
	if got := normalizePath("/"); got != "/" {
		t.Errorf("normalizePath(%q) = %q, want %q", "/", got, "/")
	}
}

func TestNormalizePath_TrailingSlash(t *testing.T) {
	if got := normalizePath("docs/"); got != "/docs" {
		t.Errorf("normalizePath(%q) = %q, want %q", "docs/", got, "/docs")
	}
}

func TestResolvePathEntry_WithoutLeadingSlash(t *testing.T) {
	// Bug: FUSE root Dir has path="" so Lookup passes "Hello World.md"
	// (no leading slash) to handler.Stat -> resolvePathEntry. But the
	// walk caches entries at "/Hello World.md" (with slash). The final
	// cache lookup for "Hello World.md" fails with ENOENT.
	h := &DriveHandler{
		rootID:    "root-id",
		pathCache: make(map[string]*pathEntry),
	}
	// Seed root and a file entry as the walk would create them.
	h.pathCache["/"] = &pathEntry{fileID: "root-id", mimeType: MimeFolder}
	h.pathCache["/Hello World.md"] = &pathEntry{
		fileID:   "doc-123",
		mimeType: MimeDoc,
		parentID: "root-id",
		name:     "Hello World",
	}

	// This is how FUSE Lookup calls it: no leading slash.
	pe, err := h.resolvePathEntry(context.Background(), "Hello World.md")
	if err != nil {
		t.Fatalf("resolvePathEntry(%q) failed: %v", "Hello World.md", err)
	}
	if pe.fileID != "doc-123" {
		t.Errorf("fileID: got %q, want %q", pe.fileID, "doc-123")
	}
}

func TestResolvePathEntry_WithLeadingSlash(t *testing.T) {
	// Paths with leading slash should also work (used by List's cache).
	h := &DriveHandler{
		rootID:    "root-id",
		pathCache: make(map[string]*pathEntry),
	}
	h.pathCache["/"] = &pathEntry{fileID: "root-id", mimeType: MimeFolder}
	h.pathCache["/doc.md"] = &pathEntry{
		fileID:   "doc-456",
		mimeType: MimeDoc,
		parentID: "root-id",
		name:     "doc",
	}

	pe, err := h.resolvePathEntry(context.Background(), "/doc.md")
	if err != nil {
		t.Fatalf("resolvePathEntry(%q) failed: %v", "/doc.md", err)
	}
	if pe.fileID != "doc-456" {
		t.Errorf("fileID: got %q, want %q", pe.fileID, "doc-456")
	}
}

func TestResolvePathEntry_NestedPathWithoutLeadingSlash(t *testing.T) {
	// Nested paths from subdirs with path="subfolder" (no leading slash).
	h := &DriveHandler{
		rootID:    "root-id",
		pathCache: make(map[string]*pathEntry),
	}
	h.pathCache["/"] = &pathEntry{fileID: "root-id", mimeType: MimeFolder}
	h.pathCache["/subfolder"] = &pathEntry{
		fileID:   "folder-id",
		mimeType: MimeFolder,
		parentID: "root-id",
		name:     "subfolder",
	}
	h.pathCache["/subfolder/doc.md"] = &pathEntry{
		fileID:   "nested-doc",
		mimeType: MimeDoc,
		parentID: "folder-id",
		name:     "doc",
	}

	pe, err := h.resolvePathEntry(context.Background(), "subfolder/doc.md")
	if err != nil {
		t.Fatalf("resolvePathEntry(%q) failed: %v", "subfolder/doc.md", err)
	}
	if pe.fileID != "nested-doc" {
		t.Errorf("fileID: got %q, want %q", pe.fileID, "nested-doc")
	}
}

// newEmptyDriveHandler returns a DriveHandler whose Drive API always reports
// an empty folder, so any path resolution walks and finds nothing.
func newEmptyDriveHandler(t *testing.T) *DriveHandler {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"files": []}`))
	}))
	t.Cleanup(srv.Close)

	svc, err := drive.NewService(context.Background(),
		option.WithEndpoint(srv.URL), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	return NewDriveHandler(&Client{drive: svc, rootFolderID: "root-id"}, "root-id")
}

func TestStat_NotFound_WrapsErrNotExist(t *testing.T) {
	// The ragfs layer distinguishes ENOENT from EIO via io/fs.ErrNotExist;
	// a plain error would make Lookup report every missing file as an
	// I/O failure (or, before the fix, every I/O failure as missing).
	h := newEmptyDriveHandler(t)

	_, err := h.Stat(context.Background(), "/missing.md")
	if err == nil {
		t.Fatal("Stat on a missing path must return an error")
	}
	if !errors.Is(err, iofs.ErrNotExist) {
		t.Errorf("Stat error = %v, want errors.Is(err, fs.ErrNotExist)", err)
	}
}
