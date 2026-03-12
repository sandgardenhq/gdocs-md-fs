package gdrive

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	maxRetries     = 5
	baseRetryDelay = 500 * time.Millisecond
)

// Client wraps the Google Drive and Docs API services.
type Client struct {
	drive        *drive.Service
	docs         *docs.Service
	rootFolderID string
}

// NewClient creates a Drive/Docs API client using the provided token source.
// rootFolderID is the Drive folder ID that serves as the filesystem root.
func NewClient(ctx context.Context, tokenSource oauth2.TokenSource, rootFolderID string) (*Client, error) {
	opt := option.WithTokenSource(tokenSource)

	driveSvc, err := drive.NewService(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("gdrive: create drive service: %w", err)
	}

	docsSvc, err := docs.NewService(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("gdrive: create docs service: %w", err)
	}

	return &Client{
		drive:        driveSvc,
		docs:         docsSvc,
		rootFolderID: rootFolderID,
	}, nil
}

// RootFolderID returns the configured root folder ID.
func (c *Client) RootFolderID() string {
	return c.rootFolderID
}

// ListFolder returns all non-trashed files within the given folder.
func (c *Client) ListFolder(ctx context.Context, folderID string) ([]*drive.File, error) {
	var files []*drive.File
	query := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
	pageToken := ""

	for {
		call := c.drive.Files.List().
			Context(ctx).
			Q(query).
			Fields("nextPageToken, files(id, name, mimeType, size, modifiedTime, parents)").
			PageSize(1000)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		var list *drive.FileList
		err := retryWithBackoff(ctx, func() error {
			var err error
			list, err = call.Do()
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("gdrive: list folder %s: %w", folderID, err)
		}

		files = append(files, list.Files...)

		pageToken = list.NextPageToken
		if pageToken == "" {
			break
		}
	}
	return files, nil
}

// GetFile returns metadata for a single file.
func (c *Client) GetFile(ctx context.Context, fileID string) (*drive.File, error) {
	var file *drive.File
	err := retryWithBackoff(ctx, func() error {
		var err error
		file, err = c.drive.Files.Get(fileID).
			Context(ctx).
			Fields("id, name, mimeType, size, modifiedTime, parents").
			Do()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("gdrive: get file %s: %w", fileID, err)
	}
	return file, nil
}

// DownloadFile downloads the binary content of a non-Google-Workspace file.
func (c *Client) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	var data []byte
	err := retryWithBackoff(ctx, func() error {
		resp, err := c.drive.Files.Get(fileID).
			Context(ctx).
			Download()
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		data, err = io.ReadAll(resp.Body)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("gdrive: download file %s: %w", fileID, err)
	}
	return data, nil
}

// GetDoc returns the structured content of a Google Doc.
func (c *Client) GetDoc(ctx context.Context, docID string) (*docs.Document, error) {
	var doc *docs.Document
	err := retryWithBackoff(ctx, func() error {
		var err error
		doc, err = c.docs.Documents.Get(docID).
			Context(ctx).
			Do()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("gdrive: get doc %s: %w", docID, err)
	}
	return doc, nil
}

// UpdateDoc applies batch update requests to a Google Doc.
func (c *Client) UpdateDoc(ctx context.Context, docID string, requests []*docs.Request) error {
	if len(requests) == 0 {
		return nil
	}

	err := retryWithBackoff(ctx, func() error {
		_, err := c.docs.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
			Requests: requests,
		}).Context(ctx).Do()
		return err
	})
	if err != nil {
		return fmt.Errorf("gdrive: update doc %s: %w", docID, err)
	}
	return nil
}

// CreateFile creates a new file with the given name, parent folder, and MIME type.
func (c *Client) CreateFile(ctx context.Context, name, parentID, mimeType string) (*drive.File, error) {
	meta := &drive.File{
		Name:     name,
		MimeType: mimeType,
		Parents:  []string{parentID},
	}

	var file *drive.File
	err := retryWithBackoff(ctx, func() error {
		var err error
		file, err = c.drive.Files.Create(meta).
			Context(ctx).
			Fields("id, name, mimeType, size, modifiedTime, parents").
			Do()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("gdrive: create file %q in %s: %w", name, parentID, err)
	}
	return file, nil
}

// CreateFolder creates a new folder with the given name inside the parent folder.
func (c *Client) CreateFolder(ctx context.Context, name, parentID string) (*drive.File, error) {
	return c.CreateFile(ctx, name, parentID, MimeFolder)
}

// DeleteFile moves a file to the trash.
func (c *Client) DeleteFile(ctx context.Context, fileID string) error {
	err := retryWithBackoff(ctx, func() error {
		_, err := c.drive.Files.Update(fileID, &drive.File{
			Trashed: true,
		}).Context(ctx).Do()
		return err
	})
	if err != nil {
		return fmt.Errorf("gdrive: delete file %s: %w", fileID, err)
	}
	return nil
}

// MoveFile moves a file to a new parent folder and/or renames it.
func (c *Client) MoveFile(ctx context.Context, fileID, newParentID, oldParentID, newName string) (*drive.File, error) {
	call := c.drive.Files.Update(fileID, &drive.File{
		Name: newName,
	}).Context(ctx).
		Fields("id, name, mimeType, size, modifiedTime, parents")

	if newParentID != oldParentID {
		call = call.AddParents(newParentID).RemoveParents(oldParentID)
	}

	var file *drive.File
	err := retryWithBackoff(ctx, func() error {
		var err error
		file, err = call.Do()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("gdrive: move file %s: %w", fileID, err)
	}
	return file, nil
}

// ExportDoc exports a Google Doc (or other Workspace file) to the specified
// MIME type and returns the raw bytes.
func (c *Client) ExportDoc(ctx context.Context, docID string, mimeType string) ([]byte, error) {
	var data []byte
	err := retryWithBackoff(ctx, func() error {
		resp, err := c.drive.Files.Export(docID, mimeType).
			Context(ctx).
			Download()
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		data, err = io.ReadAll(resp.Body)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("gdrive: export doc %s as %s: %w", docID, mimeType, err)
	}
	return data, nil
}

// retryWithBackoff executes fn with exponential backoff and jitter on
// retryable errors (HTTP 429, 500, 502, 503). It retries up to maxRetries
// times.
func retryWithBackoff(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := range maxRetries + 1 {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if !isRetryable(lastErr) {
			return lastErr
		}

		if attempt == maxRetries {
			break
		}

		// Exponential backoff: baseDelay * 2^attempt, with jitter.
		delay := baseRetryDelay * time.Duration(math.Pow(2, float64(attempt)))
		jitter := time.Duration(rand.Int64N(int64(delay / 2)))
		delay = delay + jitter

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return fmt.Errorf("gdrive: max retries exceeded: %w", lastErr)
}

// isRetryable returns true for HTTP status codes that warrant a retry.
func isRetryable(err error) bool {
	if apiErr, ok := err.(*googleapi.Error); ok {
		switch apiErr.Code {
		case 429, 500, 502, 503:
			return true
		}
	}
	return false
}
