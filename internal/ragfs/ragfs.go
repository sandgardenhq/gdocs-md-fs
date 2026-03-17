package ragfs

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Server manages the FUSE mount lifecycle for a ragfs filesystem.
type Server struct {
	handler      Handler
	cache        *Cache
	mountpoint   string
	server       *fuse.Server
	readOnly     bool
	uid          uint32
	gid          uint32
	logger       *log.Logger
	syncInterval time.Duration
	dirtyMu      sync.Mutex
	dirtyFiles   map[*File]struct{}
	stopSync     chan struct{}
	syncDone     chan struct{}
	stopOnce     sync.Once
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithCacheOptions returns a ServerOption that configures the underlying cache.
func WithCacheOptions(opts ...CacheOption) ServerOption {
	return func(s *Server) {
		s.cache = NewCache(opts...)
	}
}

// WithReadOnly returns a ServerOption that mounts the filesystem read-only.
func WithReadOnly(readOnly bool) ServerOption {
	return func(s *Server) {
		s.readOnly = readOnly
	}
}

// WithSyncInterval returns a ServerOption that sets the periodic sync interval.
func WithSyncInterval(d time.Duration) ServerOption {
	return func(s *Server) {
		s.syncInterval = d
	}
}

// NewServer creates a new FUSE server for the given handler and mountpoint.
func NewServer(handler Handler, mountpoint string, opts ...ServerOption) *Server {
	s := &Server{
		handler:      handler,
		mountpoint:   mountpoint,
		uid:          uint32(os.Getuid()),
		gid:          uint32(os.Getgid()),
		syncInterval: time.Second,
		dirtyFiles:   make(map[*File]struct{}),
		stopSync:     make(chan struct{}),
		syncDone:     make(chan struct{}),
	}
	for _, o := range opts {
		o(s)
	}
	if s.cache == nil {
		s.cache = NewCache()
	}
	if s.logger == nil {
		s.logger = log.New(os.Stderr, "ragfs: ", log.LstdFlags)
	}
	return s
}

// Mount mounts the FUSE filesystem and serves requests. It blocks until the
// filesystem is unmounted or an error occurs.
func (s *Server) Mount() error {
	// Ensure the mountpoint directory exists.
	if err := os.MkdirAll(s.mountpoint, 0o755); err != nil {
		return fmt.Errorf("ragfs: create mountpoint: %w", err)
	}

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
		uid:       s.uid,
		gid:       s.gid,
		server:    s,
		logger:    s.logger,
		tempFiles: make(map[string]*TempFile),
	}

	timeout := time.Second
	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			FsName:        "ragfs",
			Name:          "ragfs",
			DisableXAttrs: true,
			Debug:         false,
		},
		AttrTimeout:  &timeout,
		EntryTimeout: &timeout,
	}

	if s.readOnly {
		opts.Options = append(opts.Options, "ro")
	}

	server, err := fs.Mount(s.mountpoint, root, opts)
	if err != nil {
		return fmt.Errorf("ragfs: mount: %w", err)
	}
	s.server = server

	// Start periodic sync of dirty files.
	go s.syncLoop()

	// Serve blocks until unmount.
	server.Wait()

	return nil
}

// registerDirty adds a file to the set of dirty files that the sync loop
// will periodically flush.
func (s *Server) registerDirty(f *File) {
	s.dirtyMu.Lock()
	defer s.dirtyMu.Unlock()
	s.dirtyFiles[f] = struct{}{}
}

// unregisterDirty removes a file from the dirty set.
func (s *Server) unregisterDirty(f *File) {
	s.dirtyMu.Lock()
	defer s.dirtyMu.Unlock()
	delete(s.dirtyFiles, f)
}

// syncLoop runs in a goroutine and periodically flushes dirty files.
func (s *Server) syncLoop() {
	defer close(s.syncDone)
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopSync:
			s.flushAllDirty()
			return
		case <-ticker.C:
			s.flushAllDirty()
		}
	}
}

// flushAllDirty persists all dirty files to the backend.
func (s *Server) flushAllDirty() {
	s.dirtyMu.Lock()
	files := make([]*File, 0, len(s.dirtyFiles))
	for f := range s.dirtyFiles {
		files = append(files, f)
	}
	s.dirtyMu.Unlock()

	for _, f := range files {
		_ = f.persistIfDirty(context.Background())
	}
}

// stopUnmount signals the sync loop to stop and waits for its final flush
// to complete. It is safe to call multiple times.
func (s *Server) stopUnmount() {
	s.stopOnce.Do(func() {
		close(s.stopSync)
	})
	<-s.syncDone
}

// Unmount cleanly unmounts the FUSE filesystem.
func (s *Server) Unmount() error {
	s.stopUnmount()
	if s.server == nil {
		return nil
	}
	return s.server.Unmount()
}
