package gdrive

import (
	"fmt"

	"github.com/brittcrawford/gdocs-md/internal/converter"
	"google.golang.org/api/docs/v1"
)

// docToMarkdown converts a Google Doc's structured content to markdown bytes.
func docToMarkdown(doc *docs.Document) ([]byte, error) {
	md, err := converter.ToMarkdown(doc)
	if err != nil {
		return nil, fmt.Errorf("gdrive: convert doc to markdown: %w", err)
	}
	return md, nil
}

// markdownToDocRequests converts markdown bytes into Google Docs API batch
// update requests that can be applied to an existing document.
func markdownToDocRequests(md []byte) ([]*docs.Request, error) {
	reqs, err := converter.FromMarkdown(md)
	if err != nil {
		return nil, fmt.Errorf("gdrive: convert markdown to doc requests: %w", err)
	}
	return reqs, nil
}
