package ragfs

import (
	"context"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// isTempFile reports whether a filename matches known editor temp file patterns.
// These files are handled as ephemeral in-memory nodes and never synced to the backend.
func isTempFile(name string) bool {
	switch {
	case name == "4913": // vim writability test
		return true
	case strings.HasSuffix(name, "~"): // vim/emacs backup
		return true
	case strings.HasSuffix(name, ".swp"),
		strings.HasSuffix(name, ".swo"),
		strings.HasSuffix(name, ".swn"): // vim swap (may or may not have leading dot)
		return true
	case strings.HasSuffix(name, ".tmp"): // generic temp
		return true
	case strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#"): // emacs auto-save
		return true
	case strings.HasPrefix(name, "~$"): // MS Office lock
		return true
	case strings.HasPrefix(name, ".~lock."): // LibreOffice lock
		return true
	default:
		return false
	}
}

// TempFile is an ephemeral in-memory FUSE file node for editor temp files.
// It never syncs to the backend. Content lives only in memory and is lost on unmount.
type TempFile struct {
	fs.Inode

	mu   sync.Mutex
	data []byte
	name string
	uid  uint32
	gid  uint32
	mod  time.Time
}

// compile-time interface checks
var (
	_ fs.InodeEmbedder     = (*TempFile)(nil)
	_ fs.NodeGetattrer     = (*TempFile)(nil)
	_ fs.NodeSetattrer     = (*TempFile)(nil)
	_ fs.NodeOpener        = (*TempFile)(nil)
	_ fs.NodeReader        = (*TempFile)(nil)
	_ fs.NodeWriter        = (*TempFile)(nil)
	_ fs.NodeFlusher       = (*TempFile)(nil)
	_ fs.NodeFsyncer       = (*TempFile)(nil)
	_ fs.NodeStatfser      = (*TempFile)(nil)
	_ fs.NodeGetlker       = (*TempFile)(nil)
	_ fs.NodeSetlker       = (*TempFile)(nil)
	_ fs.NodeSetlkwer      = (*TempFile)(nil)
	_ fs.NodeSetxattrer    = (*TempFile)(nil)
	_ fs.NodeRemovexattrer = (*TempFile)(nil)
)

// Statfs returns the same filesystem statistics as directories; see fillStatfs.
func (tf *TempFile) Statfs(_ context.Context, out *fuse.StatfsOut) syscall.Errno {
	fillStatfs(out)
	return fs.OK
}

// Getlk reports that no lock conflicts with the request; locks are advisory
// no-ops (see File.Getlk). Office suites lock their temp companion files.
func (tf *TempFile) Getlk(_ context.Context, _ fs.FileHandle, _ uint64, _ *fuse.FileLock, _ uint32, out *fuse.FileLock) syscall.Errno {
	out.Typ = syscall.F_UNLCK
	return fs.OK
}

// Setlk acquires a lock as an advisory no-op; see File.Getlk.
func (tf *TempFile) Setlk(_ context.Context, _ fs.FileHandle, _ uint64, _ *fuse.FileLock, _ uint32) syscall.Errno {
	return fs.OK
}

// Setlkw acquires a lock, waiting if needed, as an advisory no-op; see File.Getlk.
func (tf *TempFile) Setlkw(_ context.Context, _ fs.FileHandle, _ uint64, _ *fuse.FileLock, _ uint32) syscall.Errno {
	return fs.OK
}

// Fsync is a no-op: temp files are ephemeral and in-memory, but editors
// fsync them before renaming and treat an error as a failed save.
func (tf *TempFile) Fsync(_ context.Context, _ fs.FileHandle, _ uint32) syscall.Errno {
	return fs.OK
}

// Setxattr reports that the filesystem does not support extended attributes.
// See File.Setxattr for the rationale; editors write temp files via the same
// macOS copyfile path that aborts on the go-fuse default ENOATTR.
func (tf *TempFile) Setxattr(_ context.Context, _ string, _ []byte, _ uint32) syscall.Errno {
	return syscall.ENOTSUP
}

// Removexattr reports that the filesystem does not support extended attributes.
func (tf *TempFile) Removexattr(_ context.Context, _ string) syscall.Errno {
	return syscall.ENOTSUP
}

func newTempFile(name string, uid, gid uint32) *TempFile {
	return &TempFile{
		name: name,
		uid:  uid,
		gid:  gid,
		mod:  time.Now(),
	}
}

func (tf *TempFile) Getattr(_ context.Context, _ fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	out.Mode = syscall.S_IFREG | 0o644
	out.Uid = tf.uid
	out.Gid = tf.gid
	out.Size = uint64(len(tf.data))
	out.Mtime = uint64(tf.mod.Unix())
	return fs.OK
}

func (tf *TempFile) Setattr(_ context.Context, _ fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	if sz, ok := in.GetSize(); ok {
		if sz == 0 {
			tf.data = nil
		} else if sz < uint64(len(tf.data)) {
			tf.data = tf.data[:sz]
		} else if sz > uint64(len(tf.data)) {
			grown := make([]byte, sz)
			copy(grown, tf.data)
			tf.data = grown
		}
	}
	tf.mod = time.Now()
	out.Mode = syscall.S_IFREG | 0o644
	out.Uid = tf.uid
	out.Gid = tf.gid
	out.Size = uint64(len(tf.data))
	out.Mtime = uint64(tf.mod.Unix())
	return fs.OK
}

func (tf *TempFile) Open(_ context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	tf.mu.Lock()
	if flags&syscall.O_TRUNC != 0 {
		tf.data = nil
	}
	tf.mu.Unlock()
	return nil, fuse.FOPEN_DIRECT_IO, fs.OK
}

func (tf *TempFile) Read(_ context.Context, _ fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	if int(off) >= len(tf.data) {
		return fuse.ReadResultData(nil), fs.OK
	}
	end := int(off) + len(dest)
	if end > len(tf.data) {
		end = len(tf.data)
	}
	result := make([]byte, end-int(off))
	copy(result, tf.data[off:end])
	return fuse.ReadResultData(result), fs.OK
}

func (tf *TempFile) Write(_ context.Context, _ fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	newEnd := int(off) + len(data)
	if newEnd > len(tf.data) {
		grown := make([]byte, newEnd)
		copy(grown, tf.data)
		tf.data = grown
	}
	copy(tf.data[off:], data)
	tf.mod = time.Now()
	return uint32(len(data)), fs.OK
}

func (tf *TempFile) Flush(_ context.Context, _ fs.FileHandle) syscall.Errno {
	return fs.OK
}
