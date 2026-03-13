package ragfs

import (
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

	handler Handler
	cache   *Cache
	path    string
	entry   *Entry
	uid     uint32
	gid     uint32
	logger  *log.Logger
}

// compile-time interface checks
var (
	_ fs.InodeEmbedder = (*Dir)(nil)
	_ fs.NodeReaddirer = (*Dir)(nil)
	_ fs.NodeLookuper  = (*Dir)(nil)
	_ fs.NodeCreater   = (*Dir)(nil)
	_ fs.NodeUnlinker  = (*Dir)(nil)
	_ fs.NodeRenamer   = (*Dir)(nil)
	_ fs.NodeMkdirer   = (*Dir)(nil)
)

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
		logger:  d.logger,
	}

	fillAttrOut(entry, &out.Attr, d.uid, d.gid)

	child := d.NewInode(ctx, f, fs.StableAttr{Mode: syscall.S_IFREG})
	return child, nil, fuse.FOPEN_DIRECT_IO, fs.OK
}

// Unlink deletes a file from this directory.
func (d *Dir) Unlink(ctx context.Context, name string) syscall.Errno {
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
		handler: d.handler,
		cache:   d.cache,
		path:    childPath,
		entry:   entry,
		uid:     d.uid,
		gid:     d.gid,
		logger:  d.logger,
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

// childInode creates or retrieves a child inode for the given entry.
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
			logger:  d.logger,
		}, fs.StableAttr{Mode: syscall.S_IFDIR})
	}
	return d.NewInode(ctx, &File{
		handler: d.handler,
		cache:   d.cache,
		path:    childPath,
		entry:   e,
		uid:     d.uid,
		gid:     d.gid,
		logger:  d.logger,
	}, fs.StableAttr{Mode: syscall.S_IFREG})
}

// File implements a FUSE file node backed by a Handler and Cache.
type File struct {
	fs.Inode

	handler Handler
	cache   *Cache
	path    string
	entry   *Entry
	mu      sync.Mutex
	uid     uint32
	gid     uint32
	dirty   bool // true when content has been modified and needs flushing
	logger  *log.Logger
}

// compile-time interface checks
var (
	_ fs.InodeEmbedder = (*File)(nil)
	_ fs.NodeGetattrer = (*File)(nil)
	_ fs.NodeSetattrer = (*File)(nil)
	_ fs.NodeOpener    = (*File)(nil)
	_ fs.NodeReader    = (*File)(nil)
	_ fs.NodeWriter    = (*File)(nil)
	_ fs.NodeFlusher   = (*File)(nil)
)

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
			f.dirty = true
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
		f.cache.PutContent(f.path, []byte{})
		f.dirty = true
		if f.entry != nil {
			f.entry.Size = 0
			f.entry.ModTime = time.Now()
		}
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

	// Read current content.
	var current []byte
	if cached := f.cache.GetContent(f.path); cached != nil {
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

	f.dirty = true

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

	if !f.dirty {
		return fs.OK
	}

	content := f.cache.GetContent(f.path)
	if content == nil {
		content = []byte{}
	}

	if err := f.handler.Write(ctx, f.path, content); err != nil {
		if f.logger != nil {
			f.logger.Printf("flush %q: %v", f.path, err)
		}
		return syscall.EIO
	}

	f.dirty = false
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
