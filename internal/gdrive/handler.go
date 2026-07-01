package gdrive

import (
	"context"
	"fmt"
	iofs "io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/sandgardenhq/md-to-gdocs/internal/ragfs"
	"google.golang.org/api/drive/v3"
)

// DriveHandler implements ragfs.Handler backed by Google Drive.
type DriveHandler struct {
	client *Client
	rootID string

	mu        sync.RWMutex
	pathCache map[string]*pathEntry // filesystem path -> Drive file info
}

// Compile-time check that DriveHandler satisfies the Handler interface.
var _ ragfs.Handler = (*DriveHandler)(nil)

// NewDriveHandler returns a DriveHandler rooted at the given Drive folder.
func NewDriveHandler(client *Client, rootFolderID string) *DriveHandler {
	h := &DriveHandler{
		client:    client,
		rootID:    rootFolderID,
		pathCache: make(map[string]*pathEntry),
	}
	// Seed the root entry in the cache.
	h.pathCache["/"] = &pathEntry{
		fileID:   rootFolderID,
		mimeType: MimeFolder,
		name:     "",
	}
	return h
}

// List returns the entries within a directory.
func (h *DriveHandler) List(ctx context.Context, dirPath string) ([]ragfs.Entry, error) {
	folderID, err := h.resolveFileID(ctx, dirPath)
	if err != nil {
		return nil, fmt.Errorf("gdrive: list %q: %w", dirPath, err)
	}

	files, err := h.client.ListFolder(ctx, folderID)
	if err != nil {
		return nil, err
	}

	normDir := normalizePath(dirPath)

	entries := make([]ragfs.Entry, 0, len(files))
	for _, f := range files {
		entry := driveFileToEntry(f)
		childPath := path.Join(normDir, entry.Name)

		// Cache the path mapping.
		h.mu.Lock()
		h.pathCache[childPath] = &pathEntry{
			fileID:   f.Id,
			mimeType: f.MimeType,
			parentID: folderID,
			name:     f.Name,
		}
		h.mu.Unlock()

		entries = append(entries, entry)
	}
	return entries, nil
}

// Read returns the content of a file. Google Docs are exported as markdown.
func (h *DriveHandler) Read(ctx context.Context, filePath string) ([]byte, error) {
	pe, err := h.resolvePathEntry(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("gdrive: read %q: %w", filePath, err)
	}

	// Google Docs get converted to markdown.
	if pe.mimeType == MimeDoc {
		doc, err := h.client.GetDoc(ctx, pe.fileID)
		if err != nil {
			return nil, err
		}
		return docToMarkdown(doc)
	}

	// All other files are downloaded as binary.
	return h.client.DownloadFile(ctx, pe.fileID)
}

// Write writes data to a file. For Google Docs (.md files), the existing
// document body is cleared and replaced with the markdown content converted to
// Docs API requests. Other file types are not currently supported.
func (h *DriveHandler) Write(ctx context.Context, filePath string, data []byte) error {
	pe, err := h.resolvePathEntry(ctx, filePath)
	if err != nil {
		return fmt.Errorf("gdrive: write %q: %w", filePath, err)
	}

	if pe.mimeType == MimeDoc {
		// Fetch the document to determine the body's end index so we
		// can clear existing content before inserting new content.
		doc, err := h.client.GetDoc(ctx, pe.fileID)
		if err != nil {
			return fmt.Errorf("gdrive: write %q: get doc: %w", filePath, err)
		}

		endIndex := docBodyEndIndex(doc)

		requests, err := buildWriteRequests(endIndex, data, h.wikiResolver(ctx))
		if err != nil {
			return err
		}
		return h.client.UpdateDoc(ctx, pe.fileID, requests)
	}

	return fmt.Errorf("gdrive: write %q: writing non-Google-Doc files is not supported", filePath)
}

// Delete removes a file or empty directory by moving it to the Drive trash.
func (h *DriveHandler) Delete(ctx context.Context, filePath string) error {
	pe, err := h.resolvePathEntry(ctx, filePath)
	if err != nil {
		return fmt.Errorf("gdrive: delete %q: %w", filePath, err)
	}

	if err := h.client.DeleteFile(ctx, pe.fileID); err != nil {
		return err
	}

	// Purge from cache.
	h.mu.Lock()
	delete(h.pathCache, filePath)
	h.mu.Unlock()

	return nil
}

// Rename moves or renames a file from oldPath to newPath.
func (h *DriveHandler) Rename(ctx context.Context, oldPath, newPath string) error {
	oldPE, err := h.resolvePathEntry(ctx, oldPath)
	if err != nil {
		return fmt.Errorf("gdrive: rename %q: %w", oldPath, err)
	}

	newDir := path.Dir(newPath)
	newName := path.Base(newPath)

	// Strip .md extension for Google Docs since Drive stores the name without it.
	if oldPE.mimeType == MimeDoc && strings.HasSuffix(newName, ".md") {
		newName = strings.TrimSuffix(newName, ".md")
	}

	newParentID, err := h.resolveFileID(ctx, newDir)
	if err != nil {
		return fmt.Errorf("gdrive: rename resolve new parent %q: %w", newDir, err)
	}

	oldParentID := oldPE.parentID
	if oldParentID == "" {
		oldDir := path.Dir(oldPath)
		oldParentID, err = h.resolveFileID(ctx, oldDir)
		if err != nil {
			return fmt.Errorf("gdrive: rename resolve old parent %q: %w", oldDir, err)
		}
	}

	file, err := h.client.MoveFile(ctx, oldPE.fileID, newParentID, oldParentID, newName)
	if err != nil {
		return err
	}

	// Update cache.
	h.mu.Lock()
	delete(h.pathCache, oldPath)
	h.pathCache[newPath] = &pathEntry{
		fileID:   file.Id,
		mimeType: file.MimeType,
		parentID: newParentID,
		name:     file.Name,
	}
	h.mu.Unlock()

	return nil
}

// Stat returns metadata for a file or directory.
func (h *DriveHandler) Stat(ctx context.Context, filePath string) (*ragfs.Entry, error) {
	pe, err := h.resolvePathEntry(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("gdrive: stat %q: %w", filePath, err)
	}

	file, err := h.client.GetFile(ctx, pe.fileID)
	if err != nil {
		return nil, err
	}

	entry := driveFileToEntry(file)

	// Update cache with fresh metadata.
	h.mu.Lock()
	h.pathCache[filePath] = &pathEntry{
		fileID:   file.Id,
		mimeType: file.MimeType,
		parentID: pe.parentID,
		name:     file.Name,
	}
	h.mu.Unlock()

	return &entry, nil
}

// Create creates a new file or directory. Files with .md extension are created
// as Google Docs.
func (h *DriveHandler) Create(ctx context.Context, filePath string, isDir bool) (*ragfs.Entry, error) {
	parentDir := path.Dir(filePath)
	name := path.Base(filePath)

	parentID, err := h.resolveFileID(ctx, parentDir)
	if err != nil {
		return nil, fmt.Errorf("gdrive: create resolve parent %q: %w", parentDir, err)
	}

	var file *drive.File

	if isDir {
		file, err = h.client.CreateFolder(ctx, name, parentID)
		if err != nil {
			return nil, err
		}
	} else if strings.HasSuffix(name, ".md") {
		// Create a Google Doc (strip the .md extension for the Drive name).
		docName := strings.TrimSuffix(name, ".md")
		file, err = h.client.CreateFile(ctx, docName, parentID, MimeDoc)
		if err != nil {
			return nil, err
		}
	} else {
		// Create a regular file with a generic MIME type.
		file, err = h.client.CreateFile(ctx, name, parentID, "application/octet-stream")
		if err != nil {
			return nil, err
		}
	}

	entry := driveFileToEntry(file)

	// Cache the new entry.
	h.mu.Lock()
	h.pathCache[filePath] = &pathEntry{
		fileID:   file.Id,
		mimeType: file.MimeType,
		parentID: parentID,
		name:     file.Name,
	}
	h.mu.Unlock()

	return &entry, nil
}

// resolveFileID returns the Drive file ID for the given filesystem path.
func (h *DriveHandler) resolveFileID(ctx context.Context, fsPath string) (string, error) {
	pe, err := h.resolvePathEntry(ctx, fsPath)
	if err != nil {
		return "", err
	}
	return pe.fileID, nil
}

// normalizePath ensures a filesystem path always has a leading slash and is
// cleaned, so cache keys are consistent regardless of caller conventions.
func normalizePath(p string) string {
	p = path.Clean(p)
	if p == "." || p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// resolvePathEntry returns the cached pathEntry for the given path. If the path
// is not cached, it walks the path from the root, listing folders as needed to
// populate the cache.
func (h *DriveHandler) resolvePathEntry(ctx context.Context, fsPath string) (*pathEntry, error) {
	fsPath = normalizePath(fsPath)

	// Check cache first.
	h.mu.RLock()
	pe, ok := h.pathCache[fsPath]
	h.mu.RUnlock()
	if ok {
		return pe, nil
	}

	// Walk the path from root, resolving each component.
	parts := splitPath(fsPath)
	currentPath := "/"
	currentID := h.rootID

	for _, part := range parts {
		targetPath := path.Join(currentPath, part)

		// Check cache for this intermediate path.
		h.mu.RLock()
		cached, ok := h.pathCache[targetPath]
		h.mu.RUnlock()
		if ok {
			currentPath = targetPath
			currentID = cached.fileID
			continue
		}

		// List the current folder to find the child.
		files, err := h.client.ListFolder(ctx, currentID)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", fsPath, err)
		}

		found := false
		for _, f := range files {
			childName := f.Name
			// Google Docs appear as .md files in the virtual filesystem.
			if f.MimeType == MimeDoc {
				childName = childName + ".md"
			}

			childPath := path.Join(currentPath, childName)

			h.mu.Lock()
			h.pathCache[childPath] = &pathEntry{
				fileID:   f.Id,
				mimeType: f.MimeType,
				parentID: currentID,
				name:     f.Name,
			}
			h.mu.Unlock()

			if childName == part || f.Name == part {
				currentPath = targetPath
				currentID = f.Id
				found = true
			}
		}

		if !found {
			// Wrap fs.ErrNotExist so ragfs can map a genuine missing
			// path to ENOENT while other failures surface as EIO.
			return nil, fmt.Errorf("resolve %q: %q not found in %q: %w", fsPath, part, currentPath, iofs.ErrNotExist)
		}
	}

	h.mu.RLock()
	pe, ok = h.pathCache[fsPath]
	h.mu.RUnlock()
	if !ok {
		// The final component should be cached from the walk.
		return nil, fmt.Errorf("resolve %q: path not found after walk: %w", fsPath, iofs.ErrNotExist)
	}
	return pe, nil
}

// splitPath splits a cleaned path into its non-empty components.
func splitPath(p string) []string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// driveFileToEntry converts a Drive API File to a ragfs.Entry.
func driveFileToEntry(f *drive.File) ragfs.Entry {
	isDir := f.MimeType == MimeFolder
	name := f.Name

	// Google Docs appear as .md files in the virtual filesystem.
	if f.MimeType == MimeDoc {
		name = name + ".md"
	}

	modTime, _ := time.Parse(time.RFC3339, f.ModifiedTime)

	mode := os.FileMode(0644)
	if isDir {
		mode = os.FileMode(0755)
	}

	return ragfs.Entry{
		Name:      name,
		IsDir:     isDir,
		Size:      uint64(f.Size),
		ModTime:   modTime,
		Mode:      mode,
		MimeType:  f.MimeType,
		BackendID: f.Id,
	}
}
