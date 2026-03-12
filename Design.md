# Design: gdocs-md

A Go CLI tool that mounts Google Drive as a local FUSE filesystem, presenting Google Docs as editable markdown files.

## Architecture Overview

```
+-------------------+     +-------------------+     +-------------------+
|   CLI (cobra)     | --> |   ragfs (FUSE +   | --> |   Google Drive    |
|   gdocs-md mount  |     |   caching layer)  |     |   Handler         |
|   gdocs-md auth   |     |                   |     |                   |
+-------------------+     +-------------------+     +-------------------+
                                                            |
                                    +-------------------+   |   +-------------------+
                                    | Markdown Converter|<--+-->| Google Drive API   |
                                    | (Docs <-> MD)     |       | (v3 + Docs API)    |
                                    +-------------------+       +-------------------+
```

### Package Layout

```
gdocs-md/
  cmd/
    gdocs-md/
      main.go              # Entry point
  internal/
    cli/
      root.go              # Root cobra command
      mount.go             # Mount subcommand
      auth.go              # Auth subcommand
    gdrive/
      handler.go           # ragfs.Handler implementation for Google Drive
      client.go            # Google Drive API v3 client wrapper
      docs.go              # Google Docs API interactions
      oauth.go             # OAuth 2.0 token flow and storage
      types.go             # Drive-specific types
    converter/
      tomarkdown.go        # Google Doc JSON -> Markdown
      frommarkdown.go      # Markdown -> Google Docs API requests
      elements.go          # Structural element mapping
      converter_test.go    # Round-trip fidelity tests
    ragfs/
      ragfs.go             # FUSE filesystem server (bazil.org/fuse)
      handler.go           # Handler interface definition
      cache.go             # In-memory + disk cache with TTL
      node.go              # FUSE node implementations (Dir, File)
      types.go             # Shared types (Entry, Attr)
  scripts/
    build.sh               # Build script
    install.sh             # Install script
  go.mod
  go.sum
```

## Handler Interface

The `ragfs.Handler` interface is the core abstraction. Any cloud storage backend implements this interface, and ragfs provides FUSE mounting and caching on top.

```go
package ragfs

import (
    "context"
    "io"
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

// Handler defines the interface that cloud storage backends must implement.
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
    Stat(ctx context.Context, path string) (*Entry, error)

    // Create creates a new file at the given path.
    Create(ctx context.Context, path string, isDir bool) (*Entry, error)
}
```

## Caching Strategy

The ragfs caching layer sits between FUSE operations and the Handler:

1. **Metadata cache**: Directory listings and file stats cached with 30s TTL
2. **Content cache**: File content cached in-memory (LRU, 100MB default cap) with 60s TTL
3. **Write-through**: Writes update the cache immediately and propagate to backend
4. **Invalidation**: On write/delete/rename, relevant cache entries are invalidated
5. **Target**: Repeated reads of cached content must complete in <200ms

```go
type Cache struct {
    metaTTL    time.Duration  // Default 30s
    contentTTL time.Duration  // Default 60s
    maxSize    int64          // Default 100MB
    mu         sync.RWMutex
    meta       map[string]*cachedMeta
    content    *lru.Cache     // hashicorp/golang-lru
}
```

## Google Drive Integration

### Authentication (OAuth 2.0)

- Uses Google OAuth 2.0 with offline access for refresh tokens
- Scopes: `drive.file`, `drive.readonly`, `documents`
- Token stored in `$XDG_CONFIG_HOME/gdocs-md/token.json` (or `~/.config/gdocs-md/token.json`)
- Client credentials in `$XDG_CONFIG_HOME/gdocs-md/credentials.json`
- `gdocs-md auth` launches browser-based OAuth flow, stores token

### File Type Mapping

| Google Drive MIME Type | Presented As | Behavior |
|------------------------|-------------|----------|
| `application/vnd.google-apps.document` | `filename.md` | Convert to/from markdown |
| `application/vnd.google-apps.folder` | directory | Map to FUSE directory |
| `application/pdf` | `filename.pdf` | Pass-through (read-only download) |
| `image/*` | `filename.png/jpg/etc` | Pass-through (read-only download) |
| Other binary files | original name | Pass-through (read/write) |

### Markdown Conversion

#### Google Doc -> Markdown (Read Path)

Use Google Docs API `documents.get` to retrieve structured document, then convert:

| Doc Element | Markdown |
|-------------|----------|
| Paragraph (NORMAL) | Plain text with newline |
| Paragraph (HEADING_1..6) | `#` through `######` |
| Bold text run | `**text**` |
| Italic text run | `*text*` |
| Bold+Italic | `***text***` |
| Strikethrough | `~~text~~` |
| Code text run | `` `text` `` |
| Ordered list | `1.` numbered items |
| Unordered list | `-` items |
| Named style SUBTITLE | `## subtitle` |
| Link | `[text](url)` |
| Inline image | `![alt](imageUrl)` |
| Table | GFM table syntax |
| Horizontal rule | `---` |
| Code block (monospace paragraph) | Fenced code block |

#### Markdown -> Google Doc (Write Path)

Parse markdown with goldmark, then generate `documents.batchUpdate` requests:

1. Parse markdown AST
2. Map AST nodes to Google Docs API request objects
3. Clear existing document content (delete all)
4. Insert new content via batchUpdate
5. Apply formatting via batchUpdate

### Error Handling

- Network errors: Return `EIO` to FUSE, log error, retain cache
- Auth errors: Return `EACCES`, prompt re-authentication on next CLI invocation
- Rate limiting: Exponential backoff with jitter, up to 5 retries
- Conflict (concurrent edit): Last-write-wins with warning log

## Phased MVP Scope

### Phase 1: Auth + Read-Only Mount (Core)
- OAuth 2.0 authentication flow (`gdocs-md auth`)
- Mount a single Google Drive folder (`gdocs-md mount <folder-id> <mountpoint>`)
- List files in the folder as FUSE directory entries
- Read Google Docs as markdown files
- Read PDFs and images as pass-through files
- In-memory caching of reads

**Success criteria addressed**: Auth, Mount, Read Docs as MD, PDFs/images pass-through, Caching

### Phase 2: Write Support
- Write/save a `.md` file to update the Google Doc
- Create new files
- Delete files (remove from Drive)
- Move/rename files within the mount

**Success criteria addressed**: Edit/save updates Doc, Delete removes from Drive, Move/rename

### Phase 3: Reliability + Polish
- Round-trip conversion fidelity (>= 95%)
- Write-through cache with proper invalidation
- Comprehensive error handling (no data loss)
- Graceful unmount and signal handling

**Success criteria addressed**: 95% round-trip fidelity, No data loss

## Dependencies

| Package | Purpose |
|---------|---------|
| `bazil.org/fuse` | FUSE filesystem interface |
| `google.golang.org/api/drive/v3` | Google Drive API |
| `google.golang.org/api/docs/v1` | Google Docs API |
| `golang.org/x/oauth2` | OAuth 2.0 client |
| `golang.org/x/oauth2/google` | Google OAuth provider |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/yuin/goldmark` | Markdown parser (for write path) |
| `github.com/hashicorp/golang-lru/v2` | LRU cache |

## CLI Interface

```
gdocs-md - Mount Google Drive as a local filesystem with Docs as Markdown

Usage:
  gdocs-md [command]

Commands:
  auth     Authenticate with Google Drive via OAuth
  mount    Mount a Google Drive folder as a local directory
  version  Print version information

Flags:
  -v, --verbose   Enable verbose logging

Examples:
  gdocs-md auth
  gdocs-md mount <folder-id> ~/drive
  gdocs-md mount --cache-size 200MB <folder-id> ~/drive
```

### Mount Command Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--cache-size` | `100MB` | Maximum in-memory cache size |
| `--cache-ttl` | `60s` | Cache TTL for content |
| `--meta-ttl` | `30s` | Cache TTL for metadata |
| `--foreground` | `false` | Run in foreground (don't daemonize) |
| `--read-only` | `false` | Mount as read-only |
