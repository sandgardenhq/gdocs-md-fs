package ragfs

import (
	"context"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func TestIsTempFile(t *testing.T) {
	temps := []string{
		"doc.md~",         // vim/emacs backup
		".doc.md.swp",     // vim swap
		".doc.md.swo",     // vim swap overflow
		".doc.md.swn",     // vim swap overflow
		"notes.tmp",       // generic temp
		"#autosave#",      // emacs auto-save
		"~$document.docx", // MS Office lock
		".~lock.file.ods", // LibreOffice lock
		"4913",            // vim writability test
	}
	for _, name := range temps {
		if !isTempFile(name) {
			t.Errorf("isTempFile(%q) = false, want true", name)
		}
	}
}

func TestIsTempFile_NonTemp(t *testing.T) {
	nonTemps := []string{
		"readme.md",
		"notes.txt",
		"photo.png",
		"report.pdf",
		"my-file.md",
		".hidden",
		"backup",
		"49130", // not exactly "4913"
		"a4913", // not exactly "4913"
	}
	for _, name := range nonTemps {
		if isTempFile(name) {
			t.Errorf("isTempFile(%q) = true, want false", name)
		}
	}
}

func TestTempFile_Getattr(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)
	var out fuse.AttrOut
	errno := tf.Getattr(context.Background(), nil, &out)
	if errno != 0 {
		t.Fatalf("Getattr returned errno %d", errno)
	}
	wantMode := uint32(syscall.S_IFREG | 0o644)
	if out.Mode != wantMode {
		t.Errorf("mode: got %o, want %o", out.Mode, wantMode)
	}
	if out.Uid != 501 {
		t.Errorf("uid: got %d, want 501", out.Uid)
	}
	if out.Gid != 20 {
		t.Errorf("gid: got %d, want 20", out.Gid)
	}
}

func TestTempFile_WriteAndRead(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)

	// Write data.
	data := []byte("temporary content")
	written, errno := tf.Write(context.Background(), nil, data, 0)
	if errno != 0 {
		t.Fatalf("Write returned errno %d", errno)
	}
	if written != uint32(len(data)) {
		t.Errorf("Write returned %d, want %d", written, len(data))
	}

	// Read it back.
	dest := make([]byte, 4096)
	result, errno := tf.Read(context.Background(), nil, dest, 0)
	if errno != 0 {
		t.Fatalf("Read returned errno %d", errno)
	}
	got := make([]byte, result.Size())
	buf, _ := result.Bytes(got)
	if string(buf) != string(data) {
		t.Errorf("Read: got %q, want %q", buf, data)
	}
}

func TestTempFile_WriteAtOffset(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)

	// Write initial data.
	_, _ = tf.Write(context.Background(), nil, []byte("hello"), 0)
	// Overwrite at offset.
	_, errno := tf.Write(context.Background(), nil, []byte("WORLD"), 5)
	if errno != 0 {
		t.Fatalf("Write at offset returned errno %d", errno)
	}

	dest := make([]byte, 4096)
	result, _ := tf.Read(context.Background(), nil, dest, 0)
	got := make([]byte, result.Size())
	buf, _ := result.Bytes(got)
	if string(buf) != "helloWORLD" {
		t.Errorf("Read after offset write: got %q, want %q", buf, "helloWORLD")
	}
}

func TestTempFile_ReadAtOffset(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)
	_, _ = tf.Write(context.Background(), nil, []byte("hello world"), 0)

	dest := make([]byte, 4096)
	result, errno := tf.Read(context.Background(), nil, dest, 6)
	if errno != 0 {
		t.Fatalf("Read at offset returned errno %d", errno)
	}
	got := make([]byte, result.Size())
	buf, _ := result.Bytes(got)
	if string(buf) != "world" {
		t.Errorf("Read at offset 6: got %q, want %q", buf, "world")
	}
}

func TestTempFile_ReadBeyondEnd(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)
	_, _ = tf.Write(context.Background(), nil, []byte("short"), 0)

	dest := make([]byte, 4096)
	result, errno := tf.Read(context.Background(), nil, dest, 100)
	if errno != 0 {
		t.Fatalf("Read beyond end returned errno %d", errno)
	}
	if result.Size() != 0 {
		t.Errorf("Read beyond end: got %d bytes, want 0", result.Size())
	}
}

func TestTempFile_Open(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)
	_, flags, errno := tf.Open(context.Background(), 0)
	if errno != 0 {
		t.Fatalf("Open returned errno %d", errno)
	}
	if flags&fuse.FOPEN_DIRECT_IO == 0 {
		t.Errorf("Open flags missing FOPEN_DIRECT_IO")
	}
}

func TestTempFile_Flush(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)
	_, _ = tf.Write(context.Background(), nil, []byte("data"), 0)
	errno := tf.Flush(context.Background(), nil)
	if errno != 0 {
		t.Fatalf("Flush returned errno %d, want OK", errno)
	}
}

func TestTempFile_Setattr_Truncate(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)
	_, _ = tf.Write(context.Background(), nil, []byte("some content"), 0)

	in := &fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_SIZE
	in.Size = 0
	var out fuse.AttrOut
	errno := tf.Setattr(context.Background(), nil, in, &out)
	if errno != 0 {
		t.Fatalf("Setattr returned errno %d", errno)
	}
	if out.Size != 0 {
		t.Errorf("size after truncate: got %d, want 0", out.Size)
	}

	// Read should return empty.
	dest := make([]byte, 4096)
	result, _ := tf.Read(context.Background(), nil, dest, 0)
	if result.Size() != 0 {
		t.Errorf("Read after truncate: got %d bytes, want 0", result.Size())
	}
}

func TestTempFile_GetattrReflectsSize(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)
	_, _ = tf.Write(context.Background(), nil, []byte("12345"), 0)

	var out fuse.AttrOut
	_ = tf.Getattr(context.Background(), nil, &out)
	if out.Size != 5 {
		t.Errorf("size: got %d, want 5", out.Size)
	}
}

func TestTempFile_Setattr_ShrinkData(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)
	_, _ = tf.Write(context.Background(), nil, []byte("hello world"), 0)

	in := &fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_SIZE
	in.Size = 5
	var out fuse.AttrOut
	errno := tf.Setattr(context.Background(), nil, in, &out)
	if errno != 0 {
		t.Fatalf("Setattr returned errno %d", errno)
	}
	if out.Size != 5 {
		t.Errorf("size after shrink: got %d, want 5", out.Size)
	}
}

func TestTempFile_Setattr_GrowData(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)
	_, _ = tf.Write(context.Background(), nil, []byte("hi"), 0)

	in := &fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_SIZE
	in.Size = 10
	var out fuse.AttrOut
	errno := tf.Setattr(context.Background(), nil, in, &out)
	if errno != 0 {
		t.Fatalf("Setattr returned errno %d", errno)
	}
	if out.Size != 10 {
		t.Errorf("size after grow: got %d, want 10", out.Size)
	}
}

func TestTempFile_Open_WithTrunc(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)
	_, _ = tf.Write(context.Background(), nil, []byte("content"), 0)

	_, flags, errno := tf.Open(context.Background(), syscall.O_TRUNC)
	if errno != 0 {
		t.Fatalf("Open returned errno %d", errno)
	}
	if flags&fuse.FOPEN_DIRECT_IO == 0 {
		t.Error("Open should return FOPEN_DIRECT_IO")
	}
	// After O_TRUNC, data should be nil.
	tf.mu.Lock()
	if tf.data != nil {
		t.Errorf("data after O_TRUNC: got %v, want nil", tf.data)
	}
	tf.mu.Unlock()
}

func TestTempFile_Setattr_NoSize(t *testing.T) {
	tf := newTempFile("doc.md~", 501, 20)
	_, _ = tf.Write(context.Background(), nil, []byte("keep"), 0)

	in := &fuse.SetAttrIn{}
	// No FATTR_SIZE set — data should remain unchanged.
	var out fuse.AttrOut
	errno := tf.Setattr(context.Background(), nil, in, &out)
	if errno != 0 {
		t.Fatalf("Setattr returned errno %d", errno)
	}
	if out.Size != 4 {
		t.Errorf("size: got %d, want 4", out.Size)
	}
}

func TestTempFileSetxattr_ReturnsENOTSUP(t *testing.T) {
	// macOS copyfile aborts a copy when setxattr returns ENOATTR (the
	// go-fuse default); ENOTSUP makes it skip xattrs silently, matching
	// the behavior of File and Dir.
	tf := newTempFile(".doc.md.tmp", 501, 20)
	errno := tf.Setxattr(context.Background(), "com.apple.lastuseddate#PS", []byte("v"), 0)
	if errno != syscall.ENOTSUP {
		t.Errorf("Setxattr returned errno %d (%v), want ENOTSUP", errno, errno)
	}
}

func TestTempFileRemovexattr_ReturnsENOTSUP(t *testing.T) {
	tf := newTempFile(".doc.md.tmp", 501, 20)
	errno := tf.Removexattr(context.Background(), "com.apple.lastuseddate#PS")
	if errno != syscall.ENOTSUP {
		t.Errorf("Removexattr returned errno %d (%v), want ENOTSUP", errno, errno)
	}
}

func TestTempFileFsync_ReturnsOK(t *testing.T) {
	// Temp files are ephemeral and never synced to the backend, but fsync
	// on them must still succeed for editors that fsync before rename.
	tf := newTempFile(".doc.md.tmp", 501, 20)
	errno := tf.Fsync(context.Background(), nil, 0)
	if errno != 0 {
		t.Errorf("Fsync returned errno %d, want 0", errno)
	}
}

func TestTempFileStatfs_MatchesDirStatfs(t *testing.T) {
	var fromDir, fromTemp fuse.StatfsOut
	if errno := (&Dir{}).Statfs(context.Background(), &fromDir); errno != 0 {
		t.Fatalf("Dir Statfs returned errno %d", errno)
	}
	tf := newTempFile(".doc.md.tmp", 501, 20)
	if errno := tf.Statfs(context.Background(), &fromTemp); errno != 0 {
		t.Fatalf("TempFile Statfs returned errno %d", errno)
	}
	if fromTemp != fromDir {
		t.Errorf("TempFile Statfs = %+v, want same as Dir Statfs %+v", fromTemp, fromDir)
	}
}
