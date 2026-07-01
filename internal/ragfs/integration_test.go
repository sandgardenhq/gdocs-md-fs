package ragfs

import (
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"context"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// trackingHandler is a thread-safe Handler for integration tests that stores
// files in memory and tracks every method call.
type trackingHandler struct {
	mu sync.Mutex

	files map[string][]byte // path → content

	createCalls []string
	writeCalls  []writeCall
	readCalls   []string
	deleteCalls []string
	renameCalls [][2]string
}

type writeCall struct {
	path string
	data []byte
}

func newTrackingHandler() *trackingHandler {
	return &trackingHandler{
		files: make(map[string][]byte),
	}
}

func (h *trackingHandler) reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.createCalls = nil
	h.writeCalls = nil
	h.readCalls = nil
	h.deleteCalls = nil
	h.renameCalls = nil
}

func (h *trackingHandler) Create(_ context.Context, path string, _ bool) (*Entry, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.createCalls = append(h.createCalls, path)
	h.files[path] = []byte{}
	return &Entry{
		Name:     filepath.Base(path),
		MimeType: mimeGoogleDoc,
		ModTime:  time.Now(),
	}, nil
}

func (h *trackingHandler) Write(_ context.Context, path string, data []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	h.writeCalls = append(h.writeCalls, writeCall{path: path, data: cp})
	h.files[path] = cp
	return nil
}

func (h *trackingHandler) Read(_ context.Context, path string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readCalls = append(h.readCalls, path)
	if data, ok := h.files[path]; ok {
		cp := make([]byte, len(data))
		copy(cp, data)
		return cp, nil
	}
	return []byte{}, nil
}

func (h *trackingHandler) List(_ context.Context, path string) ([]Entry, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var entries []Entry
	for p := range h.files {
		dir := filepath.Dir(p)
		if dir == "." {
			dir = ""
		}
		if dir == path {
			entries = append(entries, Entry{
				Name:     filepath.Base(p),
				MimeType: mimeGoogleDoc,
				ModTime:  time.Now(),
			})
		}
	}
	return entries, nil
}

func (h *trackingHandler) Delete(_ context.Context, path string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.deleteCalls = append(h.deleteCalls, path)
	delete(h.files, path)
	return nil
}

func (h *trackingHandler) Rename(_ context.Context, oldPath, newPath string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.renameCalls = append(h.renameCalls, [2]string{oldPath, newPath})
	if data, ok := h.files[oldPath]; ok {
		h.files[newPath] = data
		delete(h.files, oldPath)
	}
	return nil
}

func (h *trackingHandler) Stat(_ context.Context, path string) (*Entry, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.files[path]; ok {
		return &Entry{
			Name:     filepath.Base(path),
			MimeType: mimeGoogleDoc,
			ModTime:  time.Now(),
		}, nil
	}
	// Wrap fs.ErrNotExist per the Handler contract so Lookup maps a
	// missing path to ENOENT rather than EIO.
	return nil, fmt.Errorf("not found: %s: %w", path, iofs.ErrNotExist)
}

// TestIntegration uses a single FUSE mount to run all integration subtests,
// avoiding macFUSE mount/unmount churn that causes flaky "result too large" errors.
func TestIntegration(t *testing.T) {
	h := newTrackingHandler()
	// Pre-populate an existing file for the overwrite subtest.
	h.files["existing.md"] = []byte("# Old content\n")
	h.files["readme.md"] = []byte("# README\n\nThis is a test.\n")

	mntDir := t.TempDir()

	root := &Dir{
		handler: h,
		cache:   NewCache(),
		path:    "",
		entry: &Entry{
			IsDir:   true,
			ModTime: time.Now(),
		},
		uid:       uint32(os.Getuid()),
		gid:       uint32(os.Getgid()),
		tempFiles: make(map[string]*TempFile),
	}

	timeout := time.Second
	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			FsName:        "test-ragfs",
			DisableXAttrs: true,
		},
		AttrTimeout:  &timeout,
		EntryTimeout: &timeout,
	}

	server, err := fs.Mount(mntDir, root, opts)
	if err != nil {
		t.Skipf("FUSE mount failed (macFUSE not available?): %v", err)
	}
	t.Cleanup(func() {
		_ = server.Unmount()
	})

	t.Run("CreateAndWriteNewFile_SyncsToHandler", func(t *testing.T) {
		h.reset()

		filePath := filepath.Join(mntDir, "doc.md")
		err := os.WriteFile(filePath, []byte("# Hello World\n"), 0644)
		if err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if len(h.createCalls) == 0 {
			t.Error("handler.Create was never called for new regular file")
		}
		if len(h.writeCalls) == 0 {
			t.Fatal("handler.Write was never called — content did not sync to backend")
		}
		if string(h.writeCalls[len(h.writeCalls)-1].data) != "# Hello World\n" {
			t.Errorf("handler.Write data: got %q, want %q",
				h.writeCalls[len(h.writeCalls)-1].data, "# Hello World\n")
		}
	})

	t.Run("WriteExistingFile_SyncsToHandler", func(t *testing.T) {
		h.reset()

		filePath := filepath.Join(mntDir, "existing.md")
		err := os.WriteFile(filePath, []byte("# New content\n"), 0644)
		if err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if len(h.writeCalls) == 0 {
			t.Fatal("handler.Write was never called — overwrite did not sync to backend")
		}
		lastWrite := h.writeCalls[len(h.writeCalls)-1]
		if string(lastWrite.data) != "# New content\n" {
			t.Errorf("handler.Write data: got %q, want %q", lastWrite.data, "# New content\n")
		}
	})

	t.Run("TempFile_DoesNotSync", func(t *testing.T) {
		h.reset()

		filePath := filepath.Join(mntDir, ".doc.md.swp")
		err := os.WriteFile(filePath, []byte("swap data"), 0644)
		if err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		h.mu.Lock()
		createCalls := len(h.createCalls)
		writeCalls := len(h.writeCalls)
		h.mu.Unlock()

		if createCalls > 0 {
			t.Error("handler.Create should NOT be called for temp file")
		}
		if writeCalls > 0 {
			t.Error("handler.Write should NOT be called for temp file")
		}

		// But the file should be readable from the mount (in-memory).
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("ReadFile temp file: %v", err)
		}
		if string(data) != "swap data" {
			t.Errorf("temp file content: got %q, want %q", data, "swap data")
		}
	})

	t.Run("ReadFile_ReturnsHandlerContent", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(mntDir, "readme.md"))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(data) != "# README\n\nThis is a test.\n" {
			t.Errorf("ReadFile content: got %q, want %q", data, "# README\n\nThis is a test.\n")
		}
	})

	t.Run("AppendToExistingFile_SyncsToHandler", func(t *testing.T) {
		h.reset()
		// Pre-populate so append has existing content.
		h.mu.Lock()
		h.files["appendable.md"] = []byte("# Original\n")
		h.mu.Unlock()

		filePath := filepath.Join(mntDir, "appendable.md")
		f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			t.Fatalf("OpenFile O_APPEND: %v", err)
		}
		_, err = f.WriteString("\nAppended line\n")
		if err != nil {
			t.Fatalf("WriteString: %v", err)
		}
		err = f.Close()
		if err != nil {
			t.Fatalf("Close: %v", err)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if len(h.writeCalls) == 0 {
			t.Fatal("handler.Write was never called — append did not sync to backend")
		}
	})

	t.Run("ShellEchoRedirect_SyncsToHandler", func(t *testing.T) {
		h.reset()
		// Pre-populate to test the > (truncate+write) pattern.
		h.mu.Lock()
		h.files["shell-test.md"] = []byte("# Old\n")
		h.mu.Unlock()

		filePath := filepath.Join(mntDir, "shell-test.md")
		// Mimic: echo "# New" > file.md (O_WRONLY|O_CREATE|O_TRUNC)
		f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			t.Fatalf("OpenFile O_TRUNC: %v", err)
		}
		_, err = f.WriteString("# New\n")
		if err != nil {
			t.Fatalf("WriteString: %v", err)
		}
		err = f.Close()
		if err != nil {
			t.Fatalf("Close: %v", err)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if len(h.writeCalls) == 0 {
			t.Fatal("handler.Write was never called — echo > redirect did not sync to backend")
		}
		lastWrite := h.writeCalls[len(h.writeCalls)-1]
		if string(lastWrite.data) != "# New\n" {
			t.Errorf("handler.Write data: got %q, want %q", lastWrite.data, "# New\n")
		}
	})

	t.Run("ShellEchoAppend_SyncsToHandler", func(t *testing.T) {
		h.reset()
		// Pre-populate for >> pattern.
		h.mu.Lock()
		h.files["append-test.md"] = []byte("# Start\n")
		h.mu.Unlock()

		filePath := filepath.Join(mntDir, "append-test.md")
		// Mimic: echo "extra" >> file.md (O_WRONLY|O_CREATE|O_APPEND)
		f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			t.Fatalf("OpenFile O_APPEND: %v", err)
		}
		_, err = f.WriteString("extra\n")
		if err != nil {
			t.Fatalf("WriteString: %v", err)
		}
		err = f.Close()
		if err != nil {
			t.Fatalf("Close: %v", err)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if len(h.writeCalls) == 0 {
			t.Fatal("handler.Write was never called — echo >> append did not sync to backend")
		}
	})

	t.Run("MultipleWritesSingleOpen_SyncsOnce", func(t *testing.T) {
		h.reset()

		filePath := filepath.Join(mntDir, "multi-write.md")
		f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			t.Fatalf("OpenFile: %v", err)
		}
		// Multiple writes before close.
		_, _ = f.WriteString("# Title\n")
		_, _ = f.WriteString("\nParagraph one.\n")
		_, _ = f.WriteString("\nParagraph two.\n")
		err = f.Close()
		if err != nil {
			t.Fatalf("Close: %v", err)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if len(h.writeCalls) == 0 {
			t.Fatal("handler.Write was never called — multiple writes did not sync")
		}
		lastWrite := h.writeCalls[len(h.writeCalls)-1]
		want := "# Title\n\nParagraph one.\n\nParagraph two.\n"
		if string(lastWrite.data) != want {
			t.Errorf("handler.Write data:\ngot  %q\nwant %q", lastWrite.data, want)
		}
	})

	t.Run("DeleteFile_SyncsToHandler", func(t *testing.T) {
		h.reset()
		// Pre-populate so delete has something to remove.
		h.mu.Lock()
		h.files["to-delete.md"] = []byte("# Delete me\n")
		h.mu.Unlock()

		filePath := filepath.Join(mntDir, "to-delete.md")
		err := os.Remove(filePath)
		if err != nil {
			t.Fatalf("Remove: %v", err)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if len(h.deleteCalls) == 0 {
			t.Fatal("handler.Delete was never called")
		}
	})

	t.Run("RenameFile_SyncsToHandler", func(t *testing.T) {
		h.reset()
		h.mu.Lock()
		h.files["old-name.md"] = []byte("# Rename me\n")
		h.mu.Unlock()

		oldPath := filepath.Join(mntDir, "old-name.md")
		newPath := filepath.Join(mntDir, "new-name.md")
		err := os.Rename(oldPath, newPath)
		if err != nil {
			t.Fatalf("Rename: %v", err)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if len(h.renameCalls) == 0 {
			t.Fatal("handler.Rename was never called")
		}
		last := h.renameCalls[len(h.renameCalls)-1]
		if last[0] != "old-name.md" || last[1] != "new-name.md" {
			t.Errorf("handler.Rename: got %v, want [old-name.md new-name.md]", last)
		}
	})

	t.Run("TempFile_HasMtime", func(t *testing.T) {
		filePath := filepath.Join(mntDir, ".mtime-test.swp")
		err := os.WriteFile(filePath, []byte("mtime data"), 0644)
		if err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		info, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if info.ModTime().Unix() == 0 {
			t.Error("temp file Mtime should be non-zero")
		}
	})

	t.Run("TempFileRenamedToReal_SyncsToHandler", func(t *testing.T) {
		h.reset()

		tmpPath := filepath.Join(mntDir, "atomic.md.tmp")
		realPath := filepath.Join(mntDir, "atomic.md")

		err := os.WriteFile(tmpPath, []byte("# Atomic save\n"), 0644)
		if err != nil {
			t.Fatalf("WriteFile temp: %v", err)
		}

		err = os.Rename(tmpPath, realPath)
		if err != nil {
			t.Fatalf("Rename: %v", err)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if len(h.writeCalls) == 0 {
			t.Fatal("handler.Write was never called — atomic save (temp→real rename) did not sync")
		}
		lastWrite := h.writeCalls[len(h.writeCalls)-1]
		if string(lastWrite.data) != "# Atomic save\n" {
			t.Errorf("handler.Write data: got %q, want %q", lastWrite.data, "# Atomic save\n")
		}
	})
}
