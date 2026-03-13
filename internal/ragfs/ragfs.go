package ragfs

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Server manages the FUSE mount lifecycle for a ragfs filesystem.
type Server struct {
	handler    Handler
	cache      *Cache
	mountpoint string
	server     *fuse.Server
	readOnly   bool
	uid        uint32
	gid        uint32
	logger     *log.Logger
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

// NewServer creates a new FUSE server for the given handler and mountpoint.
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
		uid:    s.uid,
		gid:    s.gid,
		logger: s.logger,
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

	// Serve blocks until unmount.
	server.Wait()

	return nil
}

// Unmount cleanly unmounts the FUSE filesystem.
func (s *Server) Unmount() error {
	if s.server == nil {
		return nil
	}
	return s.server.Unmount()
}
