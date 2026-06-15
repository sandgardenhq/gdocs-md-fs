package gdrive

import (
	"context"
	"fmt"
	"strings"

	"github.com/sandgardenhq/md-to-gdocs/internal/converter"
	"google.golang.org/api/drive/v3"
)

// docURL builds the canonical Google Docs URL for a Doc file ID.
func docURL(fileID string) string {
	return fmt.Sprintf("https://docs.google.com/document/d/%s/edit", fileID)
}

// folderLister lists the files within a Drive folder. It matches
// Client.ListFolder and is injected so the tree walk can be tested without a
// live Drive connection.
type folderLister func(ctx context.Context, folderID string) ([]*drive.File, error)

// findDocByName searches the folder tree rooted at folderID for a Google Doc
// whose name exactly matches name, returning its file ID. Only Docs match;
// folders and other file types with the same name are ignored. The walk
// descends into subfolders depth-first. Trees are expected to be small.
func findDocByName(ctx context.Context, list folderLister, folderID, name string) (string, bool) {
	files, err := list(ctx, folderID)
	if err != nil {
		return "", false
	}

	var subfolders []string
	for _, f := range files {
		if f.MimeType == MimeDoc && f.Name == name {
			return f.Id, true
		}
		if f.MimeType == MimeFolder {
			subfolders = append(subfolders, f.Id)
		}
	}

	for _, sub := range subfolders {
		if id, ok := findDocByName(ctx, list, sub, name); ok {
			return id, true
		}
	}
	return "", false
}

// wikiResolver returns a converter.WikiResolver that resolves wikilink targets
// against the mounted Drive tree. A target containing a slash is treated as a
// path relative to the mount root (e.g. "subfolder/Page Name"); a bare name is
// searched for across the whole tree. Only Google Docs resolve.
func (h *DriveHandler) wikiResolver(ctx context.Context) converter.WikiResolver {
	return h.wikiResolverWith(ctx, h.client.ListFolder)
}

// wikiResolverWith is wikiResolver with an injectable folder lister, enabling
// tree-walk resolution to be tested without a live Drive connection.
func (h *DriveHandler) wikiResolverWith(ctx context.Context, list folderLister) converter.WikiResolver {
	return func(target string) (string, bool) {
		if strings.Contains(target, "/") {
			pe, err := h.resolvePathEntry(ctx, "/"+target+".md")
			if err != nil || pe.mimeType != MimeDoc {
				return "", false
			}
			return docURL(pe.fileID), true
		}

		id, ok := findDocByName(ctx, list, h.rootID, target)
		if !ok {
			return "", false
		}
		return docURL(id), true
	}
}
