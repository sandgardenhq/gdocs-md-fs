package converter

import (
	"strings"
	"testing"
)

// resolvedFor builds a WikiResolver that returns url for the given target.
func resolvedFor(target, url string) WikiResolver {
	return func(t string) (string, bool) {
		if t == target {
			return url, true
		}
		return "", false
	}
}

func TestFromMarkdown_Wikilink_Resolved(t *testing.T) {
	md := []byte("See [[Page Name]] here.\n")
	reqs, err := FromMarkdown(md, WithWikiResolver(resolvedFor("Page Name", "https://docs.google.com/document/d/abc/edit")))
	if err != nil {
		t.Fatalf("FromMarkdown: %v", err)
	}

	// The displayed text "Page Name" should be inserted (without brackets).
	var hasText bool
	for _, r := range reqs {
		if r.InsertText != nil && strings.Contains(r.InsertText.Text, "Page Name") {
			hasText = true
		}
		if r.InsertText != nil && strings.Contains(r.InsertText.Text, "[[") {
			t.Errorf("displayed text should not contain literal brackets: %q", r.InsertText.Text)
		}
	}
	if !hasText {
		t.Error("expected InsertText for displayed text 'Page Name'")
	}

	// A link to the resolved URL, with no foreground color (not broken).
	var hasLink bool
	for _, r := range reqs {
		if r.UpdateTextStyle == nil || r.UpdateTextStyle.TextStyle == nil {
			continue
		}
		ts := r.UpdateTextStyle.TextStyle
		if ts.Link != nil && ts.Link.Url == "https://docs.google.com/document/d/abc/edit" {
			hasLink = true
			if ts.ForegroundColor != nil {
				t.Error("resolved wikilink should not set a foreground color")
			}
		}
	}
	if !hasLink {
		t.Error("expected a link request to the resolved URL")
	}
}
