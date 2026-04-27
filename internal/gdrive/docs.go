package gdrive

import (
	"fmt"

	"github.com/brittcrawford/gdocs-md-fs/internal/converter"
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

// docBodyEndIndex returns the end index of the document body. This is needed
// to construct a DeleteContentRange that clears existing content.
func docBodyEndIndex(doc *docs.Document) int64 {
	if doc == nil || doc.Body == nil || len(doc.Body.Content) == 0 {
		return 1
	}
	last := doc.Body.Content[len(doc.Body.Content)-1]
	return last.EndIndex
}

// buildWriteRequests produces the complete set of batch-update requests needed
// to replace a Google Doc's body with new markdown content. If the document has
// existing content (endIndex > 2), a DeleteContentRange request is prepended to
// clear the body before inserting new content. endIndex is the Body's end index
// as reported by the Docs API.
func buildWriteRequests(endIndex int64, md []byte) ([]*docs.Request, error) {
	var reqs []*docs.Request

	// Delete existing body content if present. Index 1 is the start of
	// the body; endIndex-1 avoids deleting the trailing newline that the
	// Docs API requires.
	if endIndex > 2 {
		reqs = append(reqs, &docs.Request{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{
					StartIndex: 1,
					EndIndex:   endIndex - 1,
				},
			},
		})
	}

	if len(md) == 0 {
		return reqs, nil
	}

	insertReqs, err := markdownToDocRequests(md)
	if err != nil {
		return nil, err
	}

	reqs = append(reqs, insertReqs...)
	return reqs, nil
}
