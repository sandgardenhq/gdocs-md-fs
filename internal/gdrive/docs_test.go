package gdrive

import (
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestBuildWriteRequests_PrependsDeleteWhenDocHasContent(t *testing.T) {
	md := []byte("# Hello\n")
	// endIndex > 2 means the doc has existing content to clear.
	reqs, err := buildWriteRequests(50, md, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reqs) == 0 {
		t.Fatal("expected non-empty request list")
	}

	// First request should be DeleteContentRange clearing indices 1..49.
	first := reqs[0]
	if first.DeleteContentRange == nil {
		t.Fatalf("first request should be DeleteContentRange, got %+v", first)
	}
	r := first.DeleteContentRange.Range
	if r.StartIndex != 1 {
		t.Errorf("delete start: got %d, want 1", r.StartIndex)
	}
	if r.EndIndex != 49 {
		t.Errorf("delete end: got %d, want 49", r.EndIndex)
	}

	// Remaining requests should include InsertText from the markdown.
	var hasInsert bool
	for _, req := range reqs[1:] {
		if req.InsertText != nil && strings.Contains(req.InsertText.Text, "Hello") {
			hasInsert = true
			break
		}
	}
	if !hasInsert {
		t.Error("expected InsertText request with 'Hello' after delete")
	}
}

func TestBuildWriteRequests_SkipsDeleteForEmptyDoc(t *testing.T) {
	md := []byte("# Hello\n")
	// endIndex <= 2 means the doc body is empty (just the trailing newline).
	reqs, err := buildWriteRequests(2, md, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No delete request — first request should be InsertText.
	if len(reqs) == 0 {
		t.Fatal("expected non-empty request list")
	}
	if reqs[0].DeleteContentRange != nil {
		t.Error("should not delete content for an empty doc")
	}
}

func TestBuildWriteRequests_NilMarkdownProducesDeleteOnly(t *testing.T) {
	reqs, err := buildWriteRequests(50, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have just the delete request (clearing for truncation).
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].DeleteContentRange == nil {
		t.Fatal("expected DeleteContentRange for nil markdown")
	}
}

func TestBuildWriteRequests_NilMarkdownEmptyDocProducesNoRequests(t *testing.T) {
	reqs, err := buildWriteRequests(2, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("expected 0 requests for nil md + empty doc, got %d", len(reqs))
	}
}

func TestDocBodyEndIndex(t *testing.T) {
	tests := []struct {
		name string
		doc  *docs.Document
		want int64
	}{
		{"nil doc", nil, 1},
		{"nil body", &docs.Document{}, 1},
		{"empty content", &docs.Document{Body: &docs.Body{}}, 1},
		{"single element", &docs.Document{Body: &docs.Body{
			Content: []*docs.StructuralElement{{EndIndex: 42}},
		}}, 42},
		{"multiple elements", &docs.Document{Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{EndIndex: 10},
				{EndIndex: 50},
			},
		}}, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := docBodyEndIndex(tt.doc)
			if got != tt.want {
				t.Errorf("docBodyEndIndex: got %d, want %d", got, tt.want)
			}
		})
	}
}
