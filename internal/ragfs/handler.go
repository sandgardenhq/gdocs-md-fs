package ragfs

import "context"

// Handler defines the interface that cloud storage backends must implement.
// The ragfs FUSE server calls these methods to serve filesystem operations.
type Handler interface {
	// List returns entries within the given directory path.
	List(ctx context.Context, path string) ([]Entry, error)

	// Read returns the content of the file at the given path.
	// For Google Docs, this returns markdown-converted content.
	Read(ctx context.Context, path string) ([]byte, error)

	// Write writes data to the file at the given path.
	// For Google Docs, this converts markdown back to Doc format.
	Write(ctx context.Context, path string, data []byte) error

	// Delete removes the file or empty directory at the given path.
	Delete(ctx context.Context, path string) error

	// Rename moves or renames a file from oldPath to newPath.
	Rename(ctx context.Context, oldPath, newPath string) error

	// Stat returns metadata for the file or directory at the given path.
	// When the path does not exist, the returned error wraps io/fs.ErrNotExist
	// so callers can distinguish not-found (ENOENT) from I/O failures (EIO).
	Stat(ctx context.Context, path string) (*Entry, error)

	// Create creates a new file at the given path.
	Create(ctx context.Context, path string, isDir bool) (*Entry, error)
}
