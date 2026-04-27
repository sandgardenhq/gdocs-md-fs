# FUSE File Ownership and Permissions — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make mounted files owned by the current user with MimeType-aware permissions (0644 for Google Docs, 0444 for non-docs, 0755 for dirs).

**Architecture:** Capture UID/GID at mount time in `Server`, propagate through `Dir`/`File` nodes, and teach `fillAttrOut` to set ownership and pick mode based on `Entry.MimeType`.

**Tech Stack:** Go, hanwen/go-fuse/v2, syscall, os

---

### Task 1: Write failing tests for `fillAttrOut` permissions and ownership

**Files:**
- Create: `internal/ragfs/node_test.go`

**Step 1: Write the failing tests**

```go
package ragfs

import (
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func TestFillAttrOut_GoogleDoc(t *testing.T) {
	e := &Entry{
		Name:     "doc.md",
		MimeType: "application/vnd.google-apps.document",
		Size:     100,
		ModTime:  time.Unix(1000, 0),
	}
	var a fuse.Attr
	fillAttrOut(e, &a, 501, 20)

	if a.Mode != 0o644 {
		t.Errorf("mode: got %o, want 0644", a.Mode)
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

	if a.Mode != 0o444 {
		t.Errorf("mode: got %o, want 0444", a.Mode)
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

	if a.Mode != 0o444 {
		t.Errorf("mode: got %o, want 0444", a.Mode)
	}
	if a.Uid != 1000 {
		t.Errorf("uid: got %d, want 1000", a.Uid)
	}
	if a.Gid != 1000 {
		t.Errorf("gid: got %d, want 1000", a.Gid)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/brittcrawford/conductor/workspaces/md-to-gdocs/munich && go test ./internal/ragfs/ -run TestFillAttrOut -v`
Expected: FAIL — `fillAttrOut` currently takes 2 params, not 4. Compilation error: `too many arguments in call to fillAttrOut`.

**Step 3: Commit the failing tests**

```bash
git add internal/ragfs/node_test.go
git commit -m "test: add failing tests for fillAttrOut ownership and permissions"
```

---

### Task 2: Write failing tests for `newDirStream` permissions

**Files:**
- Modify: `internal/ragfs/node_test.go`

**Step 1: Write the failing tests**

Append to `internal/ragfs/node_test.go`:

```go
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
		{"doc.md", 0o644},
		{"report.pdf", 0o444},
		{"subdir", syscall.S_IFDIR | 0o755},
		{"photo.png", 0o444},
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
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/brittcrawford/conductor/workspaces/md-to-gdocs/munich && go test ./internal/ragfs/ -run TestNewDirStream -v`
Expected: FAIL — `newDirStream` currently gives PDF and image files mode `0644` instead of `0444`.

**Step 3: Commit the failing test**

```bash
git add internal/ragfs/node_test.go
git commit -m "test: add failing test for newDirStream MimeType-aware permissions"
```

---

### Task 3: Implement `isWritableFile` helper and update `fillAttrOut`

**Files:**
- Modify: `internal/ragfs/node.go:323-335` (fillAttrOut function)

**Step 1: Add the `isWritableFile` helper**

Add above `fillAttrOut` in `internal/ragfs/node.go`:

```go
// mimeGoogleDoc is the MIME type for Google Docs, defined here to avoid
// importing the gdrive package.
const mimeGoogleDoc = "application/vnd.google-apps.document"

// isWritableFile reports whether a file with the given MIME type should be
// writable (i.e., it's a Google Doc presented as markdown).
func isWritableFile(mimeType string) bool {
	return mimeType == mimeGoogleDoc
}
```

**Step 2: Update `fillAttrOut` to accept uid/gid and use MimeType**

Replace the existing `fillAttrOut` function at line 323:

```go
// fillAttrOut populates a fuse.Attr from an Entry.
func fillAttrOut(e *Entry, a *fuse.Attr, uid, gid uint32) {
	if e == nil {
		return
	}
	a.Uid = uid
	a.Gid = gid
	if e.IsDir {
		a.Mode = syscall.S_IFDIR | 0o755
	} else if isWritableFile(e.MimeType) {
		a.Mode = 0o644
	} else {
		a.Mode = 0o444
	}
	a.Size = e.Size
	a.Mtime = uint64(e.ModTime.Unix())
}
```

**Step 3: Update `newDirStream` to use MimeType-aware modes**

Replace the mode logic in `newDirStream` (lines 346-349):

```go
func newDirStream(entries []Entry) *dirStream {
	dirents := make([]fuse.DirEntry, len(entries))
	for i, e := range entries {
		var mode uint32
		if e.IsDir {
			mode = syscall.S_IFDIR | 0o755
		} else if isWritableFile(e.MimeType) {
			mode = 0o644
		} else {
			mode = 0o444
		}
		dirents[i] = fuse.DirEntry{
			Name: e.Name,
			Mode: mode,
		}
	}
	return &dirStream{entries: dirents}
}
```

**Step 4: Run the fillAttrOut and newDirStream tests**

Run: `cd /Users/brittcrawford/conductor/workspaces/md-to-gdocs/munich && go test ./internal/ragfs/ -run "TestFillAttrOut|TestNewDirStream" -v`
Expected: FAIL — compilation errors because call sites of `fillAttrOut` still pass 2 args. That's expected; we fix call sites in Task 4.

**Step 5: Commit the helper and updated functions**

```bash
git add internal/ragfs/node.go
git commit -m "feat: add isWritableFile helper, update fillAttrOut with uid/gid and MimeType-aware modes"
```

---

### Task 4: Add uid/gid to Dir and File, update all call sites

**Files:**
- Modify: `internal/ragfs/node.go` (Dir struct ~line 15, File struct ~line 196, and all `fillAttrOut` call sites)
- Modify: `internal/ragfs/ragfs.go` (Server struct, Mount method)

**Step 1: Add uid/gid fields to Dir and File structs**

In `internal/ragfs/node.go`, update the `Dir` struct (line 15):

```go
type Dir struct {
	fs.Inode

	handler Handler
	cache   *Cache
	path    string
	entry   *Entry
	uid     uint32
	gid     uint32
}
```

Update the `File` struct (line 196):

```go
type File struct {
	fs.Inode

	handler Handler
	cache   *Cache
	path    string
	entry   *Entry
	mu      sync.Mutex
	uid     uint32
	gid     uint32
}
```

**Step 2: Update all `fillAttrOut` call sites in node.go**

There are 5 call sites to update:

1. `Dir.Getattr` (line 164) — add uid/gid to the attr:
```go
func (d *Dir) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFDIR | 0o755
	out.Uid = d.uid
	out.Gid = d.gid
	if d.entry != nil {
		out.Mtime = uint64(d.entry.ModTime.Unix())
		out.Size = d.entry.Size
	} else {
		out.Mtime = uint64(time.Now().Unix())
	}
	return fs.OK
}
```

2. `childInode` (line 177): `fillAttrOut(e, &out.Attr, d.uid, d.gid)`

3. `Create` (line 94): `fillAttrOut(entry, &out.Attr, d.uid, d.gid)`

4. `Mkdir` (line 152): `fillAttrOut(entry, &out.Attr, d.uid, d.gid)`

5. `Setattr` (line 245): `fillAttrOut(f.entry, &out.Attr, f.uid, f.gid)`

**Step 3: Propagate uid/gid when creating child nodes**

Update `childInode` to pass uid/gid to child Dir and File:

```go
func (d *Dir) childInode(ctx context.Context, e *Entry, childPath string, out *fuse.EntryOut) *fs.Inode {
	fillAttrOut(e, &out.Attr, d.uid, d.gid)

	if e.IsDir {
		return d.NewInode(ctx, &Dir{
			handler: d.handler,
			cache:   d.cache,
			path:    childPath,
			entry:   e,
			uid:     d.uid,
			gid:     d.gid,
		}, fs.StableAttr{Mode: syscall.S_IFDIR})
	}
	return d.NewInode(ctx, &File{
		handler: d.handler,
		cache:   d.cache,
		path:    childPath,
		entry:   e,
		uid:     d.uid,
		gid:     d.gid,
	}, fs.StableAttr{})
}
```

Update `Create` to pass uid/gid to the File:

```go
	f := &File{
		handler: d.handler,
		cache:   d.cache,
		path:    childPath,
		entry:   entry,
		uid:     d.uid,
		gid:     d.gid,
	}
```

Update `Mkdir` to pass uid/gid to the Dir:

```go
	child := d.NewInode(ctx, &Dir{
		handler: d.handler,
		cache:   d.cache,
		path:    childPath,
		entry:   entry,
		uid:     d.uid,
		gid:     d.gid,
	}, fs.StableAttr{Mode: syscall.S_IFDIR})
```

**Step 4: Update `File.Getattr` fallback to use MimeType-aware mode**

In `File.Getattr` (line 217), update the fallback when `f.entry` is nil:

```go
func (f *File) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	if f.entry != nil {
		fillAttrOut(f.entry, &out.Attr, f.uid, f.gid)
	} else {
		out.Mode = 0o444
		out.Uid = f.uid
		out.Gid = f.gid
		out.Mtime = uint64(time.Now().Unix())
	}
	return fs.OK
}
```

Note: The fallback uses `0444` because if we have no entry, we don't know the MimeType, so we default to read-only (safe default).

**Step 5: Add uid/gid to Server and pass to root Dir**

In `internal/ragfs/ragfs.go`, update the `Server` struct:

```go
type Server struct {
	handler    Handler
	cache      *Cache
	mountpoint string
	server     *fuse.Server
	readOnly   bool
	uid        uint32
	gid        uint32
}
```

In the `NewServer` function, capture UID/GID. Add `"os"` to the import block:

```go
func NewServer(handler Handler, mountpoint string, opts ...ServerOption) *Server {
	s := &Server{
		handler:    handler,
		mountpoint: mountpoint,
		uid:        uint32(os.Getuid()),
		gid:        uint32(os.Getgid()),
	}
	for _, o := range opts {
		o(s)
	}
	if s.cache == nil {
		s.cache = NewCache()
	}
	return s
}
```

In the `Mount` method, pass uid/gid to the root Dir:

```go
	root := &Dir{
		handler: s.handler,
		cache:   s.cache,
		path:    "",
		entry: &Entry{
			Name:    "",
			IsDir:   true,
			Mode:    os.ModeDir | 0o755,
			ModTime: time.Now(),
		},
		uid: s.uid,
		gid: s.gid,
	}
```

**Step 6: Run all tests**

Run: `cd /Users/brittcrawford/conductor/workspaces/md-to-gdocs/munich && go test ./internal/ragfs/ -v`
Expected: ALL PASS

**Step 7: Run full test suite and linter**

Run: `cd /Users/brittcrawford/conductor/workspaces/md-to-gdocs/munich && go test ./... && golangci-lint run ./...`
Expected: ALL PASS, no lint errors

**Step 8: Commit**

```bash
git add internal/ragfs/node.go internal/ragfs/ragfs.go
git commit -m "feat: set file ownership to current user, apply MimeType-aware permissions"
```

---

### Task 5: Verify and clean up

**Step 1: Run full test suite with race detector**

Run: `cd /Users/brittcrawford/conductor/workspaces/md-to-gdocs/munich && go test -v -race ./...`
Expected: ALL PASS, no race conditions

**Step 2: Check test coverage**

Run: `cd /Users/brittcrawford/conductor/workspaces/md-to-gdocs/munich && go test -coverprofile=coverage.out ./internal/ragfs/ && go tool cover -func=coverage.out`
Expected: `fillAttrOut` and `newDirStream` at 100% coverage

**Step 3: Run linter**

Run: `cd /Users/brittcrawford/conductor/workspaces/md-to-gdocs/munich && golangci-lint run ./...`
Expected: No errors or warnings

**Step 4: Build**

Run: `cd /Users/brittcrawford/conductor/workspaces/md-to-gdocs/munich && go build -o gdocs-md-fs ./cmd/gdocs-md-fs`
Expected: Build succeeds

**Step 5: Final commit if any cleanup was needed**

```bash
git add -A
git commit -m "chore: final cleanup after ownership/permissions implementation"
```
