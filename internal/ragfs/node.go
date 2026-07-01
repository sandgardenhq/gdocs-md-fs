package ragfs

import (
	"bytes"
	"context"
	"log"
	"path"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Dir implements a FUSE directory node backed by a Handler and Cache.
type Dir struct {
	fs.Inode

	handler   Handler
	cache     *Cache
	path      string
	entry     *Entry
	uid       uint32
	gid       uint32
	server    *Server
	logger    *log.Logger
	tempMu    sync.RWMutex
	tempFiles map[string]*TempFile
}

// compile-time interface checks
var (
	_ fs.InodeEmbedder     = (*Dir)(nil)
	_ fs.NodeReaddirer     = (*Dir)(nil)
	_ fs.NodeLookuper      = (*Dir)(nil)
	_ fs.NodeCreater       = (*Dir)(nil)
	_ fs.NodeUnlinker      = (*Dir)(nil)
	_ fs.NodeRenamer       = (*Dir)(nil)
	_ fs.NodeMkdirer       = (*Dir)(nil)
	_ fs.NodeStatfser      = (*Dir)(nil)
	_ fs.NodeSetattrer     = (*Dir)(nil)
	_ fs.NodeFsyncer       = (*Dir)(nil)
	_ fs.NodeSetxattrer    = (*Dir)(nil)
	_ fs.NodeRemovexattrer = (*Dir)(nil)
)

// Setattr accepts chmod/chown/utimens on directories as a no-op and reports
// current attributes. Google Drive has no POSIX modes or settable directory
// times, but recursive copies (cp -Rp, rsync -a, tar -x) set them as their
// final step and fail entirely on the go-fuse default ENOTSUP.
func (d *Dir) Setattr(ctx context.Context, fh fs.FileHandle, _ *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	return d.Getattr(ctx, fh, out)
}

// Fsync on a directory (FSYNCDIR) is a no-op: directory metadata lives in
// Google Drive and every mutation is persisted synchronously, so there is
// nothing to flush. Tools fsync the parent directory after a rename for
// durability and treat the go-fuse default ENOTSUP as a failure.
func (d *Dir) Fsync(_ context.Context, _ fs.FileHandle, _ uint32) syscall.Errno {
	return fs.OK
}

// Setxattr reports that the filesystem does not support extended attributes.
// See File.Setxattr for the rationale; directories are targeted by recursive
// copies (cp -R, rsync) the same way.
func (d *Dir) Setxattr(_ context.Context, _ string, _ []byte, _ uint32) syscall.Errno {
	return syscall.ENOTSUP
}

// Removexattr reports that the filesystem does not support extended attributes.
func (d *Dir) Removexattr(_ context.Context, _ string) syscall.Errno {
	return syscall.ENOTSUP
}

// Readdir returns all entries in this directory. Results are cached.
func (d *Dir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	// Check cache first.
	if cached := d.cache.GetMetaList(d.path); cached != nil {
		return newDirStream(cached), fs.OK
	}

	entries, err := d.handler.List(ctx, d.path)
	if err != nil {
		return nil, syscall.EIO
	}

	d.cache.PutMetaList(d.path, entries)

	return newDirStream(entries), fs.OK
}

// Lookup looks up a child entry by name.
func (d *Dir) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	// Check ephemeral temp files first.
	d.tempMu.RLock()
	tf, ok := d.tempFiles[name]
	d.tempMu.RUnlock()
	if ok {
		tf.mu.Lock()
		out.Mode = syscall.S_IFREG | 0o644
		out.Uid = d.uid
		out.Gid = d.gid
		out.Size = uint64(len(tf.data))
		out.Mtime = uint64(tf.mod.Unix())
		tf.mu.Unlock()
		child := d.NewInode(ctx, tf, fs.StableAttr{Mode: syscall.S_IFREG})
		return child, fs.OK
	}

	childPath := path.Join(d.path, name)

	// Try cached directory listing first.
	if cached := d.cache.GetMetaList(d.path); cached != nil {
		for i := range cached {
			if cached[i].Name == name {
				return d.childInode(ctx, &cached[i], childPath, out), fs.OK
			}
		}
	}

	// Fall back to Stat.
	entry, err := d.handler.Stat(ctx, childPath)
	if err != nil {
		return nil, syscall.ENOENT
	}

	d.cache.PutMeta(childPath, entry)

	return d.childInode(ctx, entry, childPath, out), fs.OK
}

// Create creates a new file in this directory.
func (d *Dir) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (inode *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if isTempFile(name) {
		tf := newTempFile(name, d.uid, d.gid)
		d.tempMu.Lock()
		if d.tempFiles == nil {
			d.tempFiles = make(map[string]*TempFile)
		}
		d.tempFiles[name] = tf
		d.tempMu.Unlock()

		out.Mode = syscall.S_IFREG | 0o644
		out.Uid = d.uid
		out.Gid = d.gid
		out.Mtime = uint64(tf.mod.Unix())

		child := d.NewInode(ctx, tf, fs.StableAttr{Mode: syscall.S_IFREG})
		return child, nil, fuse.FOPEN_DIRECT_IO, fs.OK
	}

	childPath := path.Join(d.path, name)

	entry, err := d.handler.Create(ctx, childPath, false)
	if err != nil {
		return nil, nil, 0, syscall.EIO
	}

	d.cache.Invalidate(d.path)

	f := &File{
		handler: d.handler,
		cache:   d.cache,
		path:    childPath,
		entry:   entry,
		uid:     d.uid,
		gid:     d.gid,
		server:  d.server,
		logger:  d.logger,
	}

	fillAttrOut(entry, &out.Attr, d.uid, d.gid)

	child := d.NewInode(ctx, f, fs.StableAttr{Mode: syscall.S_IFREG})
	return child, nil, fuse.FOPEN_DIRECT_IO, fs.OK
}

// Unlink deletes a file from this directory.
func (d *Dir) Unlink(ctx context.Context, name string) syscall.Errno {
	// Check ephemeral temp files first.
	d.tempMu.Lock()
	_, isTmp := d.tempFiles[name]
	if isTmp {
		delete(d.tempFiles, name)
	}
	d.tempMu.Unlock()
	if isTmp {
		return fs.OK
	}

	childPath := path.Join(d.path, name)

	if err := d.handler.Delete(ctx, childPath); err != nil {
		return syscall.EIO
	}

	d.cache.Invalidate(d.path)
	d.cache.Invalidate(childPath)

	return fs.OK
}

// Rmdir deletes a subdirectory from this directory.
func (d *Dir) Rmdir(ctx context.Context, name string) syscall.Errno {
	return d.Unlink(ctx, name)
}

// Rename moves or renames an entry from this directory to another.
func (d *Dir) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	// Renaming temp files stays in-memory, unless the destination is a
	// non-temp name (e.g., editor atomic save: write .tmp → rename to .md).
	d.tempMu.RLock()
	tf, isTmp := d.tempFiles[name]
	d.tempMu.RUnlock()
	if isTmp {
		newDir, ok := newParent.(*Dir)
		if !ok {
			return syscall.EIO
		}
		if !isTempFile(newName) {
			// Promote: create the file in the backend, then write content.
			tf.mu.Lock()
			data := make([]byte, len(tf.data))
			copy(data, tf.data)
			tf.mu.Unlock()

			newPath := path.Join(newDir.path, newName)
			if _, err := d.handler.Create(ctx, newPath, false); err != nil {
				return syscall.EIO
			}
			if err := d.handler.Write(ctx, newPath, data); err != nil {
				return syscall.EIO
			}

			d.tempMu.Lock()
			delete(d.tempFiles, name)
			d.tempMu.Unlock()

			d.cache.Invalidate(d.path)
			d.cache.Invalidate(newPath)
			if newDir != d {
				newDir.cache.Invalidate(newDir.path)
			}
			return fs.OK
		}
		// Temp-to-temp rename stays in-memory.
		if d == newDir {
			// Same directory: single lock to avoid a window where the
			// file is absent from the map.
			d.tempMu.Lock()
			delete(d.tempFiles, name)
			tf.name = newName
			d.tempFiles[newName] = tf
			d.tempMu.Unlock()
		} else {
			d.tempMu.Lock()
			delete(d.tempFiles, name)
			d.tempMu.Unlock()
			newDir.tempMu.Lock()
			if newDir.tempFiles == nil {
				newDir.tempFiles = make(map[string]*TempFile)
			}
			tf.name = newName
			newDir.tempFiles[newName] = tf
			newDir.tempMu.Unlock()
		}
		return fs.OK
	}

	oldPath := path.Join(d.path, name)

	newDirNode, ok := newParent.(*Dir)
	if !ok {
		return syscall.EIO
	}
	newPath := path.Join(newDirNode.path, newName)

	if err := d.handler.Rename(ctx, oldPath, newPath); err != nil {
		return syscall.EIO
	}

	d.cache.Invalidate(d.path)
	d.cache.Invalidate(newDirNode.path)
	d.cache.Invalidate(oldPath)
	d.cache.InvalidatePrefix(oldPath + "/")

	return fs.OK
}

// Mkdir creates a new subdirectory in this directory.
func (d *Dir) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	childPath := path.Join(d.path, name)

	entry, err := d.handler.Create(ctx, childPath, true)
	if err != nil {
		return nil, syscall.EIO
	}

	d.cache.Invalidate(d.path)

	fillAttrOut(entry, &out.Attr, d.uid, d.gid)

	child := d.NewInode(ctx, &Dir{
		handler:   d.handler,
		cache:     d.cache,
		path:      childPath,
		entry:     entry,
		uid:       d.uid,
		gid:       d.gid,
		server:    d.server,
		logger:    d.logger,
		tempFiles: make(map[string]*TempFile),
	}, fs.StableAttr{Mode: syscall.S_IFDIR})
	return child, fs.OK
}

// Getattr returns attributes for this directory.
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

// Statfs returns filesystem statistics. Google Drive doesn't map cleanly to
// POSIX filesystem stats, so we return reasonable defaults that satisfy macOS
// and editors checking filesystem capabilities.
func (d *Dir) Statfs(_ context.Context, out *fuse.StatfsOut) syscall.Errno {
	out.Bsize = 4096
	out.Frsize = 4096
	out.Blocks = 1 << 20 // ~4 GB apparent size
	out.Bfree = 1 << 19
	out.Bavail = 1 << 19
	out.NameLen = 255
	return fs.OK
}

// childInode creates or retrieves a child inode for the given entry.
func (d *Dir) childInode(ctx context.Context, e *Entry, childPath string, out *fuse.EntryOut) *fs.Inode {
	fillAttrOut(e, &out.Attr, d.uid, d.gid)

	if e.IsDir {
		return d.NewInode(ctx, &Dir{
			handler:   d.handler,
			cache:     d.cache,
			path:      childPath,
			entry:     e,
			uid:       d.uid,
			gid:       d.gid,
			server:    d.server,
			logger:    d.logger,
			tempFiles: make(map[string]*TempFile),
		}, fs.StableAttr{Mode: syscall.S_IFDIR})
	}
	return d.NewInode(ctx, &File{
		handler: d.handler,
		cache:   d.cache,
		path:    childPath,
		entry:   e,
		uid:     d.uid,
		gid:     d.gid,
		server:  d.server,
		logger:  d.logger,
	}, fs.StableAttr{Mode: syscall.S_IFREG})
}

// File implements a FUSE file node backed by a Handler and Cache.
type File struct {
	fs.Inode

	handler     Handler
	cache       *Cache
	path        string
	entry       *Entry
	mu          sync.Mutex
	uid         uint32
	gid         uint32
	dirty       bool    // true when content has been modified and needs flushing
	pendingData []byte  // buffered content that survives cache TTL/eviction
	baseContent []byte  // remote content at time of first write; used to detect remote changes
	server      *Server // parent server for dirty file registration
	logger      *log.Logger
}

// compile-time interface checks
var (
	_ fs.InodeEmbedder     = (*File)(nil)
	_ fs.NodeGetattrer     = (*File)(nil)
	_ fs.NodeSetattrer     = (*File)(nil)
	_ fs.NodeOpener        = (*File)(nil)
	_ fs.NodeReader        = (*File)(nil)
	_ fs.NodeWriter        = (*File)(nil)
	_ fs.NodeFlusher       = (*File)(nil)
	_ fs.NodeFsyncer       = (*File)(nil)
	_ fs.NodeSetxattrer    = (*File)(nil)
	_ fs.NodeRemovexattrer = (*File)(nil)
)

// Fsync persists dirty content to the backend. Editors and tools that call
// fsync(2) on save treat an error as a failed write, so this must behave
// like Flush rather than the go-fuse default ENOTSUP.
func (f *File) Fsync(ctx context.Context, _ fs.FileHandle, _ uint32) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.persistLocked(ctx)
}

// Setxattr reports that the filesystem does not support extended attributes.
// macOS cp/copyfile copies xattrs from the source onto the destination via
// setxattr; returning ENOTSUP makes copyfile skip them silently, whereas the
// go-fuse default (ENOATTR) is surfaced as "Attribute not found" and aborts
// the copy.
func (f *File) Setxattr(_ context.Context, _ string, _ []byte, _ uint32) syscall.Errno {
	return syscall.ENOTSUP
}

// Removexattr reports that the filesystem does not support extended attributes.
func (f *File) Removexattr(_ context.Context, _ string) syscall.Errno {
	return syscall.ENOTSUP
}

// Getattr returns file attributes.
func (f *File) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	if f.entry != nil {
		fillAttrOut(f.entry, &out.Attr, f.uid, f.gid)
	} else {
		out.Mode = syscall.S_IFREG | 0o444
		out.Uid = f.uid
		out.Gid = f.gid
		out.Mtime = uint64(time.Now().Unix())
	}
	return fs.OK
}

// Setattr handles attribute changes (primarily truncation).
func (f *File) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if sz, ok := in.GetSize(); ok {
		if sz == 0 {
			f.cache.PutContent(f.path, []byte{})
			f.pendingData = []byte{}
			f.dirty = true
			if f.server != nil {
				f.server.registerDirty(f)
			}
			if f.entry != nil {
				f.entry.Size = 0
				f.entry.ModTime = time.Now()
			}
		} else if f.entry != nil {
			f.entry.Size = sz
		}
	}

	if f.entry != nil {
		fillAttrOut(f.entry, &out.Attr, f.uid, f.gid)
	}
	return fs.OK
}

// Open opens the file for reading or writing. FOPEN_DIRECT_IO bypasses the
// kernel page cache, which is required because Google Docs report Size=0 and
// the kernel would otherwise short-circuit reads and miscalculate write offsets.
//
// O_TRUNC is handled here because some FUSE implementations (notably macOS)
// may not send a separate SETATTR for truncation after OPEN.
func (f *File) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	if flags&syscall.O_TRUNC != 0 {
		// Read remote content outside the lock (potentially slow I/O)
		// to record as baseline for conflict detection on re-open.
		var base []byte
		if f.handler != nil {
			if content, err := f.handler.Read(ctx, f.path); err == nil {
				base = make([]byte, len(content))
				copy(base, content)
			}
		}

		f.mu.Lock()
		if base != nil {
			f.baseContent = base
		} else {
			f.baseContent = []byte{}
		}
		f.cache.PutContent(f.path, []byte{})
		f.pendingData = []byte{}
		f.dirty = true
		if f.entry != nil {
			f.entry.Size = 0
			f.entry.ModTime = time.Now()
		}
		f.mu.Unlock()
		if f.server != nil {
			f.server.registerDirty(f)
		}
	} else if flags&(syscall.O_WRONLY|syscall.O_RDWR|syscall.O_APPEND) != 0 {
		// Read remote content outside the lock (potentially slow I/O).
		content, err := f.handler.Read(ctx, f.path)

		f.mu.Lock()
		if err != nil {
			if f.pendingData != nil {
				// Cannot verify remote state while dirty; refuse to
				// open with stale data that could corrupt the doc.
				f.mu.Unlock()
				return nil, 0, syscall.EIO
			}
			// Non-dirty open: proceed without refresh; Write will
			// fetch content itself if needed.
		} else if f.pendingData != nil {
			// Dirty file re-opened: check if remote changed since
			// we started writing. If so, discard local edits to
			// avoid writing against a stale base.
			if !bytes.Equal(content, f.baseContent) {
				f.pendingData = nil
				f.dirty = false
				f.baseContent = nil
				f.cache.PutContent(f.path, content)
				if f.entry != nil {
					f.entry.Size = uint64(len(content))
				}
				f.mu.Unlock()
				return nil, 0, syscall.ESTALE
			}
			// Remote unchanged: keep pendingData, update entry.Size
			// to reflect pending content so kernel sends correct offsets.
			if f.entry != nil {
				f.entry.Size = uint64(len(f.pendingData))
			}
		} else {
			// Clean file: cache remote content and record as base
			// for future conflict detection.
			f.baseContent = make([]byte, len(content))
			copy(f.baseContent, content)
			f.cache.PutContent(f.path, content)
			if f.entry != nil {
				f.entry.Size = uint64(len(content))
			}
		}
		f.mu.Unlock()
	}
	return nil, fuse.FOPEN_DIRECT_IO, fs.OK
}

// Read reads file content at the given offset. Results are cached.
func (f *File) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	var data []byte

	if cached := f.cache.GetContent(f.path); cached != nil {
		data = cached
	} else {
		content, err := f.handler.Read(ctx, f.path)
		if err != nil {
			return nil, syscall.EIO
		}
		data = content
		f.cache.PutContent(f.path, data)
	}

	// Update entry size to reflect actual content length. Google Docs
	// report Size=0 from the Drive API; without this the kernel uses
	// the wrong offset for O_APPEND writes.
	if f.entry != nil && len(data) > 0 {
		f.entry.Size = uint64(len(data))
	}

	end := int(off) + len(dest)
	if end > len(data) {
		end = len(data)
	}
	if int(off) >= len(data) {
		return fuse.ReadResultData(nil), fs.OK
	}

	return fuse.ReadResultData(data[off:end]), fs.OK
}

// Write writes data to the file at the given offset.
func (f *File) Write(ctx context.Context, fh fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Read current content, preferring pendingData from prior writes
	// in this session, then cache, then fresh from handler.
	var current []byte
	if f.pendingData != nil {
		current = make([]byte, len(f.pendingData))
		copy(current, f.pendingData)
	} else if cached := f.cache.GetContent(f.path); cached != nil {
		current = make([]byte, len(cached))
		copy(current, cached)
	} else {
		existing, err := f.handler.Read(ctx, f.path)
		if err != nil {
			current = nil
		} else {
			current = existing
		}
	}

	// Clamp offset to actual content length. The kernel may use a stale
	// entry.Size for O_APPEND, producing an offset beyond the real
	// content. Without clamping, the gap fills with zero bytes which
	// corrupt the Google Doc when flushed.
	if int(off) > len(current) {
		off = int64(len(current))
	}

	newEnd := int(off) + len(data)
	if newEnd > len(current) {
		grown := make([]byte, newEnd)
		copy(grown, current)
		current = grown
	}

	copy(current[off:], data)

	// Update the cache with the written content so subsequent reads
	// serve from cache rather than re-fetching from Google.
	// Persistence to the backend is deferred to Flush.
	f.cache.PutContent(f.path, current)

	f.pendingData = make([]byte, len(current))
	copy(f.pendingData, current)
	f.dirty = true
	if f.server != nil {
		f.server.registerDirty(f)
	}

	if f.entry != nil {
		f.entry.Size = uint64(len(current))
		f.entry.ModTime = time.Now()
	}

	return uint32(len(data)), fs.OK
}

// Flush is called when a file descriptor is closed. It persists any dirty
// content to the backend via a single atomic batch update, avoiding race
// conditions between separate truncate and write API calls.
func (f *File) Flush(ctx context.Context, fh fs.FileHandle) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.persistLocked(ctx)
}

// persistIfDirty persists dirty content to the backend. It acquires the mutex
// internally and is safe to call from the sync loop goroutine.
func (f *File) persistIfDirty(ctx context.Context) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.persistLocked(ctx)
}

// persistLocked does the actual persist work. Caller must hold f.mu.
func (f *File) persistLocked(ctx context.Context) syscall.Errno {
	if !f.dirty {
		return fs.OK
	}

	content := f.pendingData
	if content == nil {
		content = []byte{}
	}

	if err := f.handler.Write(ctx, f.path, content); err != nil {
		if f.logger != nil {
			f.logger.Printf("flush %q: %v", f.path, err)
		}
		return syscall.EIO
	}

	f.pendingData = nil
	f.baseContent = nil
	f.dirty = false
	// Invalidate the content cache after a successful write. The
	// markdown→Doc→markdown round-trip may produce different content than
	// what was cached, so stale cache could cause offset mismatches on
	// subsequent appends.
	f.cache.Invalidate(f.path)
	if f.server != nil {
		f.server.unregisterDirty(f)
	}
	return fs.OK
}

// mimeGoogleDoc is the MIME type for Google Docs, defined here to avoid
// importing the gdrive package.
const mimeGoogleDoc = "application/vnd.google-apps.document"

// isWritableFile reports whether a file with the given MIME type should be
// writable (i.e., it's a Google Doc presented as markdown).
func isWritableFile(mimeType string) bool {
	return mimeType == mimeGoogleDoc
}

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
		a.Mode = syscall.S_IFREG | 0o644
	} else {
		a.Mode = syscall.S_IFREG | 0o444
	}
	a.Size = e.Size
	a.Mtime = uint64(e.ModTime.Unix())
}

// dirStream implements fs.DirStream for returning directory entries.
type dirStream struct {
	entries []fuse.DirEntry
	pos     int
}

func newDirStream(entries []Entry) *dirStream {
	dirents := make([]fuse.DirEntry, len(entries))
	for i, e := range entries {
		var mode uint32
		if e.IsDir {
			mode = syscall.S_IFDIR | 0o755
		} else if isWritableFile(e.MimeType) {
			mode = syscall.S_IFREG | 0o644
		} else {
			mode = syscall.S_IFREG | 0o444
		}
		dirents[i] = fuse.DirEntry{
			Name: e.Name,
			Mode: mode,
		}
	}
	return &dirStream{entries: dirents}
}

func (ds *dirStream) HasNext() bool {
	return ds.pos < len(ds.entries)
}

func (ds *dirStream) Next() (fuse.DirEntry, syscall.Errno) {
	if ds.pos >= len(ds.entries) {
		return fuse.DirEntry{}, syscall.ENOENT
	}
	e := ds.entries[ds.pos]
	ds.pos++
	return e, fs.OK
}

func (ds *dirStream) Close() {}
