package gdrive

import (
	"context"
	"testing"
)

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
