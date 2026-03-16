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
	_ fs.InodeEmbedder = (*TempFile)(nil)
	_ fs.NodeGetattrer = (*TempFile)(nil)
	_ fs.NodeSetattrer = (*TempFile)(nil)
	_ fs.NodeOpener    = (*TempFile)(nil)
	_ fs.NodeReader    = (*TempFile)(nil)
	_ fs.NodeWriter    = (*TempFile)(nil)
	_ fs.NodeFlusher   = (*TempFile)(nil)
)

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
	out.Attr.Mode = syscall.S_IFREG | 0o644
	out.Attr.Uid = tf.uid
	out.Attr.Gid = tf.gid
	out.Attr.Size = uint64(len(tf.data))
	out.Attr.Mtime = uint64(tf.mod.Unix())
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
		}
	}
	tf.mod = time.Now()
	out.Attr.Mode = syscall.S_IFREG | 0o644
	out.Attr.Uid = tf.uid
	out.Attr.Gid = tf.gid
	out.Attr.Size = uint64(len(tf.data))
	out.Attr.Mtime = uint64(tf.mod.Unix())
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
	return fuse.ReadResultData(tf.data[off:end]), fs.OK
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
