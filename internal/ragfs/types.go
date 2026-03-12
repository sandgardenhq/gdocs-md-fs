// Package ragfs provides a FUSE filesystem server with caching and a pluggable
// backend handler interface.
package ragfs

import (
	"os"
	"time"
)

// Entry represents a file or directory in the virtual filesystem.
type Entry struct {
	Name    string
	IsDir   bool
	Size    uint64
	ModTime time.Time
	Mode    os.FileMode
	// MimeType is the original MIME type from the backend.
	// Used to determine how to present the file (e.g., Google Doc -> .md).
	MimeType string
	// BackendID is the opaque identifier used by the backend (e.g., Drive file ID).
	BackendID string
}
