package converter

import (
	"strings"
	"testing"

	docs "google.golang.org/api/docs/v1"
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

// insertedTexts returns all InsertText payloads from the requests.
func insertedTexts(reqs []*docs.Request) []string {
	var out []string
	for _, r := range reqs {
		if r.InsertText != nil {
			out = append(out, r.InsertText.Text)
		}
	}
	return out
}

func TestFromMarkdown_Wikilink_Alias(t *testing.T) {
	md := []byte("[[Page Name|Custom Text]]\n")
	var gotTarget string
	reqs, err := FromMarkdown(md, WithWikiResolver(func(target string) (string, bool) {
		gotTarget = target
		return "https://resolved", true
	}))
	if err != nil {
		t.Fatalf("FromMarkdown: %v", err)
	}

	if gotTarget != "Page Name" {
		t.Errorf("resolver received target %q, want %q", gotTarget, "Page Name")
	}

	// Displayed text is the alias, not the target.
	var hasAlias bool
	for _, txt := range insertedTexts(reqs) {
		if txt == "Custom Text" {
			hasAlias = true
		}
		if strings.Contains(txt, "Page Name") {
			t.Errorf("displayed text should be the alias, got %q", txt)
		}
	}
	if !hasAlias {
		t.Error("expected displayed text 'Custom Text'")
	}
}

func TestFromMarkdown_Wikilink_PathTarget(t *testing.T) {
	md := []byte("[[subfolder/Page Name]]\n")
	var gotTarget string
	reqs, err := FromMarkdown(md, WithWikiResolver(func(target string) (string, bool) {
		gotTarget = target
		return "https://resolved", true
	}))
	if err != nil {
		t.Fatalf("FromMarkdown: %v", err)
	}
	if gotTarget != "subfolder/Page Name" {
		t.Errorf("resolver received target %q, want %q", gotTarget, "subfolder/Page Name")
	}
	// With no alias, the full path is the displayed text.
	var hasDisplay bool
	for _, txt := range insertedTexts(reqs) {
		if txt == "subfolder/Page Name" {
			hasDisplay = true
		}
	}
	if !hasDisplay {
		t.Error("expected displayed text 'subfolder/Page Name'")
	}
}

func TestFromMarkdown_Wikilink_EmptyAliasBehavesLikePlain(t *testing.T) {
	md := []byte("[[Page|]]\n")
	var gotTarget string
	reqs, err := FromMarkdown(md, WithWikiResolver(func(target string) (string, bool) {
		gotTarget = target
		return "https://resolved", true
	}))
	if err != nil {
		t.Fatalf("FromMarkdown: %v", err)
	}
	if gotTarget != "Page" {
		t.Errorf("resolver received target %q, want %q", gotTarget, "Page")
	}
	var hasDisplay bool
	for _, txt := range insertedTexts(reqs) {
		if txt == "Page" {
			hasDisplay = true
		}
	}
	if !hasDisplay {
		t.Error("expected displayed text 'Page'")
	}
}

func TestFromMarkdown_Wikilink_DegenerateRendersLiterally(t *testing.T) {
	cases := []struct {
		name string
		md   string
	}{
		{"empty", "[[]]\n"},
		{"whitespace only", "[[ ]]\n"},
		{"unclosed", "[[Page\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A resolver that would fire if a wikilink were (wrongly) parsed.
			called := false
			reqs, err := FromMarkdown([]byte(tc.md), WithWikiResolver(func(string) (string, bool) {
				called = true
				return "https://x", true
			}))
			if err != nil {
				t.Fatalf("FromMarkdown: %v", err)
			}
			if called {
				t.Error("degenerate input should not be parsed as a wikilink")
			}
			// No sentinel link either.
			if _, _, found := findSentinelLink(reqs); found {
				t.Error("degenerate input should not produce a wikilink")
			}
		})
	}
}

// brokenLinkStyle returns the link style whose URL is the broken sentinel, and
// whether one was found among the requests.
func findSentinelLink(reqs []*docs.Request) (foreground bool, fields string, found bool) {
	for _, r := range reqs {
		if r.UpdateTextStyle == nil || r.UpdateTextStyle.TextStyle == nil {
			continue
		}
		ts := r.UpdateTextStyle.TextStyle
		if ts.Link != nil && ts.Link.Url == brokenWikiLinkURL {
			return ts.ForegroundColor != nil, r.UpdateTextStyle.Fields, true
		}
	}
	return false, "", false
}

func TestFromMarkdown_Wikilink_BrokenViaResolver(t *testing.T) {
	md := []byte("[[Missing Page]]\n")
	reqs, err := FromMarkdown(md, WithWikiResolver(resolvedFor("Other", "https://x")))
	if err != nil {
		t.Fatalf("FromMarkdown: %v", err)
	}

	foreground, fields, found := findSentinelLink(reqs)
	if !found {
		t.Fatal("expected a link to the broken sentinel URL")
	}
	if !foreground {
		t.Error("broken wikilink should set a foreground color")
	}
	if !strings.Contains(fields, "foregroundColor") || !strings.Contains(fields, "link") {
		t.Errorf("Fields = %q, want link and foregroundColor", fields)
	}

	// Displayed text is the target, without brackets.
	var hasText bool
	for _, r := range reqs {
		if r.InsertText != nil && r.InsertText.Text == "Missing Page" {
			hasText = true
		}
	}
	if !hasText {
		t.Error("expected InsertText 'Missing Page'")
	}
}

func TestFromMarkdown_Wikilink_BrokenWithoutResolver(t *testing.T) {
	md := []byte("[[Anything]]\n")
	reqs, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("FromMarkdown: %v", err)
	}
	_, _, found := findSentinelLink(reqs)
	if !found {
		t.Error("with no resolver, wikilink should be broken (sentinel link)")
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
