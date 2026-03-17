package converter

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	docs "google.golang.org/api/docs/v1"
)

// FromMarkdown parses Markdown content and returns a slice of Google Docs API
// batchUpdate Request objects that insert text and apply formatting starting at
// document index 1.
//
// The caller is responsible for clearing existing document content before
// applying these requests (see gdrive.buildWriteRequests).
func FromMarkdown(md []byte) ([]*docs.Request, error) {
	gm := goldmark.New(goldmark.WithExtensions(extension.Strikethrough))
	reader := text.NewReader(md)
	tree := gm.Parser().Parse(reader)

	b := &requestBuilder{
		source:   md,
		cursor:   1, // Google Docs body starts at index 1.
		requests: nil,
	}

	if err := b.walkNode(tree, md); err != nil {
		return nil, fmt.Errorf("converter: %w", err)
	}

	return reorderRequests(b.requests), nil
}

// reorderRequests sorts requests so that all InsertText requests come first
// (preserving their relative order), followed by UpdateParagraphStyle, then
// UpdateTextStyle. This prevents paragraph style changes (e.g. NORMAL_TEXT)
// from resetting explicitly-applied text formatting (bold, italic, etc.).
func reorderRequests(requests []*docs.Request) []*docs.Request {
	var inserts, paraStyles, textStyles []*docs.Request
	for _, r := range requests {
		switch {
		case r.InsertText != nil:
			inserts = append(inserts, r)
		case r.UpdateParagraphStyle != nil:
			paraStyles = append(paraStyles, r)
		case r.UpdateTextStyle != nil:
			textStyles = append(textStyles, r)
		default:
			inserts = append(inserts, r)
		}
	}
	result := make([]*docs.Request, 0, len(requests))
	result = append(result, inserts...)
	result = append(result, paraStyles...)
	result = append(result, textStyles...)
	return result
}

// requestBuilder accumulates Google Docs API requests while walking a goldmark AST.
type requestBuilder struct {
	source   []byte
	cursor   int64           // current insertion index in the document
	requests []*docs.Request // accumulated requests

	// Style state tracked during tree walk.
	bold          bool
	italic        bool
	strikethrough bool
	code          bool
	linkURL       string // non-empty when inside a link node
}

// insertText creates an InsertText request at the current cursor position and
// advances the cursor. It also applies any pending inline styles to the
// inserted range.
func (b *requestBuilder) insertText(s string) {
	s = sanitizeForDocs(s)
	if s == "" {
		return
	}

	startIdx := b.cursor
	b.requests = append(b.requests, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: startIdx},
			Text:     s,
		},
	})
	b.cursor += int64(utf16CodeUnits(s))
	endIdx := b.cursor

	// Apply accumulated inline styles.
	b.applyTextStyle(startIdx, endIdx)

	// Apply link if present.
	if b.linkURL != "" {
		b.requests = append(b.requests, &docs.Request{
			UpdateTextStyle: &docs.UpdateTextStyleRequest{
				Range: &docs.Range{
					StartIndex: startIdx,
					EndIndex:   endIdx,
				},
				TextStyle: &docs.TextStyle{
					Link: &docs.Link{Url: b.linkURL},
				},
				Fields: "link",
			},
		})
	}
}

// utf16CodeUnits returns the number of UTF-16 code units needed to encode s.
// The Google Docs API uses UTF-16 indices, so characters above U+FFFF (such
// as emoji) require 2 code units (a surrogate pair) instead of 1.
func utf16CodeUnits(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// sanitizeForDocs strips characters that the Google Docs API rejects:
// null bytes, C0/C1 control characters (except tab, newline, carriage return),
// DEL, and replaces invalid UTF-8 sequences with U+FFFD.
func sanitizeForDocs(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t', r == '\n', r == '\r':
			b.WriteRune(r)
		case r < 0x20: // C0 control characters (includes null)
			continue
		case r == 0x7F: // DEL
			continue
		case r >= 0x80 && r <= 0x9F: // C1 control characters
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// applyTextStyle appends UpdateTextStyle requests for any active inline styles.
func (b *requestBuilder) applyTextStyle(startIdx, endIdx int64) {
	if startIdx >= endIdx {
		return
	}

	var fields []string
	ts := &docs.TextStyle{}

	if b.bold {
		ts.Bold = true
		fields = append(fields, "bold")
	}
	if b.italic {
		ts.Italic = true
		fields = append(fields, "italic")
	}
	if b.strikethrough {
		ts.Strikethrough = true
		fields = append(fields, "strikethrough")
	}
	if b.code {
		ts.WeightedFontFamily = &docs.WeightedFontFamily{
			FontFamily: "Courier New",
		}
		fields = append(fields, "weightedFontFamily")
	}

	if len(fields) == 0 {
		return
	}

	b.requests = append(b.requests, &docs.Request{
		UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{
				StartIndex: startIdx,
				EndIndex:   endIdx,
			},
			TextStyle: ts,
			Fields:    strings.Join(fields, ","),
		},
	})
}

// applyParagraphStyle sets the heading style for a paragraph at the given range.
func (b *requestBuilder) applyParagraphStyle(startIdx, endIdx int64, namedStyle string) {
	if namedStyle == "" {
		return
	}
	b.requests = append(b.requests, &docs.Request{
		UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
			Range: &docs.Range{
				StartIndex: startIdx,
				EndIndex:   endIdx,
			},
			ParagraphStyle: &docs.ParagraphStyle{
				NamedStyleType: namedStyle,
			},
			Fields: "namedStyleType",
		},
	})
}

// walkNode dispatches on the AST node kind and walks children.
func (b *requestBuilder) walkNode(node ast.Node, source []byte) error {
	switch n := node.(type) {
	case *ast.Document:
		return b.walkChildren(n, source)

	case *ast.Heading:
		return b.handleHeading(n, source)

	case *ast.Paragraph:
		return b.handleParagraph(n, source)

	case *ast.TextBlock:
		return b.handleParagraph(n, source)

	case *ast.Text:
		b.handleText(n, source)
		return nil

	case *ast.String:
		b.insertText(string(n.Value))
		return nil

	case *ast.Emphasis:
		return b.handleEmphasis(n, source)

	case *ast.CodeSpan:
		b.handleCodeSpan(n, source)
		return nil

	case *ast.FencedCodeBlock:
		b.handleFencedCodeBlock(n, source)
		return nil

	case *ast.CodeBlock:
		b.handleFencedCodeBlock(n, source)
		return nil

	case *ast.Link:
		return b.handleLink(n, source)

	case *ast.Image:
		b.handleImage(n, source)
		return nil

	case *ast.List:
		return b.handleList(n, source)

	case *ast.ListItem:
		return b.handleListItem(n, source)

	case *ast.ThematicBreak:
		b.handleThematicBreak()
		return nil

	case *ast.Blockquote:
		return b.handleBlockquote(n, source)

	case *extast.Strikethrough:
		return b.handleStrikethrough(n, source)

	case *ast.AutoLink:
		b.handleAutoLink(n, source)
		return nil

	case *ast.RawHTML:
		// Skip raw HTML.
		return nil

	case *ast.HTMLBlock:
		// Skip HTML blocks.
		return nil

	default:
		// For unknown nodes, try to walk children.
		return b.walkChildren(node, source)
	}
}

// walkChildren walks all children of a node.
func (b *requestBuilder) walkChildren(node ast.Node, source []byte) error {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if err := b.walkNode(child, source); err != nil {
			return err
		}
	}
	return nil
}

// handleHeading processes a heading node (# through ######).
func (b *requestBuilder) handleHeading(n *ast.Heading, source []byte) error {
	startIdx := b.cursor

	// Walk children to insert text content.
	if err := b.walkChildren(n, source); err != nil {
		return err
	}

	// Add a newline after the heading.
	b.insertText("\n")
	endIdx := b.cursor

	// Map heading level to named style.
	var style string
	switch n.Level {
	case 1:
		style = StyleHeading1
	case 2:
		style = StyleHeading2
	case 3:
		style = StyleHeading3
	case 4:
		style = StyleHeading4
	case 5:
		style = StyleHeading5
	case 6:
		style = StyleHeading6
	default:
		style = StyleNormalText
	}

	b.applyParagraphStyle(startIdx, endIdx, style)
	return nil
}

// handleParagraph processes a paragraph node.
func (b *requestBuilder) handleParagraph(node ast.Node, source []byte) error {
	startIdx := b.cursor

	if err := b.walkChildren(node, source); err != nil {
		return err
	}

	// Ensure paragraph ends with a newline.
	b.insertText("\n")
	endIdx := b.cursor

	// Explicitly set NORMAL_TEXT so that any pre-existing paragraph style
	// (e.g. HEADING_1 left over from a previous document body) is cleared.
	b.applyParagraphStyle(startIdx, endIdx, StyleNormalText)
	return nil
}

// handleText processes a text node, including soft/hard line breaks.
func (b *requestBuilder) handleText(n *ast.Text, source []byte) {
	segment := n.Segment
	content := segment.Value(source)
	b.insertText(string(content))

	if n.HardLineBreak() {
		b.insertText("\n")
	} else if n.SoftLineBreak() {
		b.insertText("\n")
	}
}

// handleEmphasis processes emphasis (italic or bold).
func (b *requestBuilder) handleEmphasis(n *ast.Emphasis, source []byte) error {
	if n.Level == 2 {
		prevBold := b.bold
		b.bold = true
		err := b.walkChildren(n, source)
		b.bold = prevBold
		return err
	}
	// Level 1 = italic.
	prevItalic := b.italic
	b.italic = true
	err := b.walkChildren(n, source)
	b.italic = prevItalic
	return err
}

// handleStrikethrough processes a strikethrough node (~~text~~).
func (b *requestBuilder) handleStrikethrough(n *extast.Strikethrough, source []byte) error {
	prevStrike := b.strikethrough
	b.strikethrough = true
	err := b.walkChildren(n, source)
	b.strikethrough = prevStrike
	return err
}

// handleCodeSpan processes an inline code span.
func (b *requestBuilder) handleCodeSpan(n *ast.CodeSpan, source []byte) {
	prevCode := b.code
	b.code = true

	// Collect text content from the code span's children.
	var content strings.Builder
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			seg := t.Segment
			content.Write(seg.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				content.WriteString(" ")
			}
		}
	}

	b.insertText(content.String())
	b.code = prevCode
}

// handleFencedCodeBlock processes a fenced code block or indented code block.
func (b *requestBuilder) handleFencedCodeBlock(node ast.Node, source []byte) {
	prevCode := b.code
	b.code = true

	// Collect all lines of the code block.
	var content bytes.Buffer
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		content.Write(line.Value(source))
	}

	text := content.String()
	if text != "" {
		// Ensure text ends with exactly one newline.
		text = strings.TrimRight(text, "\n") + "\n"
		b.insertText(text)
	}

	b.code = prevCode
}

// handleLink processes a link node.
func (b *requestBuilder) handleLink(n *ast.Link, source []byte) error {
	prevURL := b.linkURL
	b.linkURL = string(n.Destination)

	if err := b.walkChildren(n, source); err != nil {
		b.linkURL = prevURL
		return err
	}

	b.linkURL = prevURL
	return nil
}

// handleAutoLink processes an autolink (e.g. <https://example.com>).
func (b *requestBuilder) handleAutoLink(n *ast.AutoLink, source []byte) {
	url := string(n.URL(source))
	label := string(n.Label(source))

	prevURL := b.linkURL
	b.linkURL = url
	b.insertText(label)
	b.linkURL = prevURL
}

// handleImage processes an image node.
func (b *requestBuilder) handleImage(n *ast.Image, source []byte) {
	// Google Docs API doesn't support inline image insertion via InsertText.
	// We insert the alt text as a placeholder and add a link to the image URL.
	var alt string
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			alt += string(t.Value(source))
		}
	}
	if alt == "" {
		alt = "image"
	}
	url := string(n.Destination)

	prevURL := b.linkURL
	b.linkURL = url
	b.insertText(alt)
	b.linkURL = prevURL
}

// handleList processes an ordered or unordered list.
func (b *requestBuilder) handleList(n *ast.List, source []byte) error {
	return b.walkChildren(n, source)
}

// handleListItem processes a single list item.
func (b *requestBuilder) handleListItem(n *ast.ListItem, source []byte) error {
	// Determine the list marker.
	parent := n.Parent()
	list, ok := parent.(*ast.List)

	var marker string
	if ok && list.IsOrdered() {
		marker = fmt.Sprintf("%d. ", list.Start)
	} else {
		marker = "- "
	}

	// Insert the marker text (not styled).
	b.insertText(marker)

	// Walk children — but we need to avoid the extra newline that
	// handleParagraph adds for the first paragraph child, since the list
	// item already provides structure.
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			// Walk the paragraph's children directly to avoid double newlines.
			startIdx := b.cursor
			if err := b.walkChildren(c, source); err != nil {
				return err
			}
			b.insertText("\n")
			b.applyParagraphStyle(startIdx, b.cursor, StyleNormalText)
		default:
			if err := b.walkNode(child, source); err != nil {
				return err
			}
		}
	}

	return nil
}

// handleThematicBreak inserts a horizontal rule.
func (b *requestBuilder) handleThematicBreak() {
	// Insert a horizontal rule. The Google Docs API has an
	// InsertPageBreak but not a native HR. We insert a line of dashes
	// and rely on the caller to handle it, or we can use a special request.
	// For now, insert the text representation.
	b.insertText("---\n")
}

// handleBlockquote processes a blockquote. Google Docs doesn't have native
// blockquote support, so we indent the text with a ">" prefix.
func (b *requestBuilder) handleBlockquote(n *ast.Blockquote, source []byte) error {
	// Walk children normally. In a full implementation we would apply
	// indentation or a custom paragraph style. For now, we prefix with "> ".
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		b.insertText("> ")
		switch c := child.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			startIdx := b.cursor
			if err := b.walkChildren(c, source); err != nil {
				return err
			}
			b.insertText("\n")
			b.applyParagraphStyle(startIdx, b.cursor, StyleNormalText)
		default:
			if err := b.walkNode(child, source); err != nil {
				return err
			}
		}
	}
	return nil
}
