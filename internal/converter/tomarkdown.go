package converter

import (
	"fmt"
	"strings"

	docs "google.golang.org/api/docs/v1"
)

// ToMarkdown converts a Google Docs API Document to Markdown-formatted bytes.
func ToMarkdown(doc *docs.Document) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("converter: document is nil")
	}
	if doc.Body == nil || len(doc.Body.Content) == 0 {
		return nil, nil
	}

	var buf strings.Builder
	lists := doc.Lists

	// Track whether we are inside a consecutive run of code-block paragraphs
	// so we can wrap them in a single fenced code block.
	inCodeBlock := false

	// Track previous list ID to insert blank lines between different lists.
	prevListID := ""

	for i, elem := range doc.Body.Content {
		if elem.SectionBreak != nil {
			continue
		}
		if elem.TableOfContents != nil {
			continue
		}
		if elem.Table != nil {
			if inCodeBlock {
				buf.WriteString("```\n")
				inCodeBlock = false
			}
			writeTable(&buf, elem.Table)
			prevListID = ""
			continue
		}
		if elem.Paragraph == nil {
			continue
		}

		p := elem.Paragraph

		// Detect horizontal rule: a paragraph containing only an HorizontalRule element.
		if isHorizontalRule(p) {
			if inCodeBlock {
				buf.WriteString("```\n")
				inCodeBlock = false
			}
			buf.WriteString("---\n\n")
			prevListID = ""
			continue
		}

		// Check if this paragraph is a monospace code block.
		isMono := isMonospaceParagraph(p)

		// Determine if this is a list item.
		isList := p.Bullet != nil

		// If we transition out of a list, add a blank line.
		if !isList && prevListID != "" {
			prevListID = ""
		}
		if isList {
			currentListID := p.Bullet.ListId
			if prevListID != "" && prevListID != currentListID {
				buf.WriteString("\n")
			}
			prevListID = currentListID
		} else {
			prevListID = ""
		}

		// Handle code block transitions. Only start a fenced code block
		// when we are already in one or when the next element is also a
		// monospace paragraph (consecutive monospace = code block).
		if isMono && !isList {
			nextIsMono := false
			for j := i + 1; j < len(doc.Body.Content); j++ {
				next := doc.Body.Content[j]
				if next.Paragraph != nil {
					nextIsMono = isMonospaceParagraph(next.Paragraph)
					break
				}
				if next.SectionBreak != nil {
					continue
				}
				break
			}
			if inCodeBlock || nextIsMono {
				if !inCodeBlock {
					buf.WriteString("```\n")
					inCodeBlock = true
				}
				writeCodeBlockContent(&buf, p)
				continue
			}
			// Single monospace paragraph → render with inline code formatting.
		}
		if inCodeBlock {
			buf.WriteString("```\n\n")
			inCodeBlock = false
		}

		// List item.
		if isList {
			writeListItem(&buf, p, lists)
			continue
		}

		// Heading or normal paragraph.
		style := ""
		if p.ParagraphStyle != nil {
			style = p.ParagraphStyle.NamedStyleType
		}
		prefix := headingPrefixFor(style)

		isHeading := prefix != ""
		text := renderTextRuns(p.Elements, isHeading)

		// Skip completely empty paragraphs except as blank lines.
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			// Don't add extra blank lines at the very end.
			if i < len(doc.Body.Content)-1 {
				buf.WriteString("\n")
			}
			continue
		}

		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(trimmed)
			buf.WriteString("\n\n")
		} else {
			buf.WriteString(text)
			// Ensure paragraphs end with a newline.
			if !strings.HasSuffix(text, "\n") {
				buf.WriteString("\n")
			}
		}
	}

	// Close any trailing code block.
	if inCodeBlock {
		buf.WriteString("```\n")
	}

	return []byte(buf.String()), nil
}

// isHorizontalRule returns true if a paragraph contains only a horizontal rule
// element (or a horizontal rule followed by a newline text run).
func isHorizontalRule(p *docs.Paragraph) bool {
	for _, elem := range p.Elements {
		if elem.HorizontalRule != nil {
			return true
		}
	}
	return false
}

// renderTextRuns converts a slice of ParagraphElements into a Markdown string,
// applying inline formatting for bold, italic, strikethrough, code, and links.
func renderTextRuns(elements []*docs.ParagraphElement, isHeading ...bool) string {
	stripBold := len(isHeading) > 0 && isHeading[0]
	var buf strings.Builder
	for _, elem := range elements {
		if elem.InlineObjectElement != nil {
			writeInlineObject(&buf, elem.InlineObjectElement)
			continue
		}
		if elem.TextRun == nil {
			continue
		}
		tr := elem.TextRun
		content := tr.Content
		if content == "" {
			continue
		}

		ts := tr.TextStyle
		hasLink := ts != nil && ts.Link != nil && ts.Link.Url != ""

		// For code spans, don't apply other formatting.
		if isCode(ts) && !isOnlyNewline(content) {
			text := strings.TrimRight(content, "\n")
			if hasLink {
				buf.WriteString("[`")
				buf.WriteString(text)
				buf.WriteString("`](")
				buf.WriteString(ts.Link.Url)
				buf.WriteString(")")
			} else {
				buf.WriteString("`")
				buf.WriteString(text)
				buf.WriteString("`")
			}
			// Preserve trailing newline if present.
			if strings.HasSuffix(content, "\n") {
				buf.WriteString("\n")
			}
			continue
		}

		// Determine formatting wrappers.
		text := content
		bold := isBold(ts) && !stripBold
		italic := isItalic(ts)
		strike := isStrikethrough(ts)

		// Extract trailing newline so we don't wrap it in formatting markers.
		trailingNewline := ""
		if strings.HasSuffix(text, "\n") {
			trailingNewline = "\n"
			text = strings.TrimRight(text, "\n")
		}

		if text == "" {
			buf.WriteString(trailingNewline)
			continue
		}

		// Build the formatted text.
		formatted := text
		if bold && italic {
			formatted = "***" + formatted + "***"
		} else if bold {
			formatted = "**" + formatted + "**"
		} else if italic {
			formatted = "*" + formatted + "*"
		}
		if strike {
			formatted = "~~" + formatted + "~~"
		}

		if hasLink {
			buf.WriteString("[")
			buf.WriteString(formatted)
			buf.WriteString("](")
			buf.WriteString(ts.Link.Url)
			buf.WriteString(")")
		} else {
			buf.WriteString(formatted)
		}

		buf.WriteString(trailingNewline)
	}
	return buf.String()
}

// isOnlyNewline returns true if the content is only newline characters.
func isOnlyNewline(s string) bool {
	return strings.TrimRight(s, "\n") == ""
}

// writeInlineObject writes an inline image as Markdown.
func writeInlineObject(buf *strings.Builder, obj *docs.InlineObjectElement) {
	if obj == nil {
		return
	}
	// The InlineObjectElement has an InlineObjectId that references the
	// document's InlineObjects map. We write a placeholder with the object ID.
	buf.WriteString("![image](")
	buf.WriteString(obj.InlineObjectId)
	buf.WriteString(")")
}

// writeCodeBlockContent writes the raw text of a monospace paragraph into the
// buffer without any Markdown formatting decorators.
func writeCodeBlockContent(buf *strings.Builder, p *docs.Paragraph) {
	for _, elem := range p.Elements {
		if elem.TextRun == nil {
			continue
		}
		buf.WriteString(elem.TextRun.Content)
	}
}

// writeListItem writes a single list item paragraph as Markdown, handling
// nesting and ordered vs. unordered lists.
func writeListItem(buf *strings.Builder, p *docs.Paragraph, lists map[string]docs.List) {
	bullet := p.Bullet
	indent := nestingIndent(bullet.NestingLevel)

	text := renderTextRuns(p.Elements)
	text = strings.TrimRight(text, "\n")

	// Detect task list checkboxes (Unicode symbols inserted by FromMarkdown).
	if after, found := strings.CutPrefix(text, "☑ "); found {
		buf.WriteString(indent)
		buf.WriteString("- [x] ")
		buf.WriteString(after)
		buf.WriteString("\n")
		return
	}
	if after, found := strings.CutPrefix(text, "☐ "); found {
		buf.WriteString(indent)
		buf.WriteString("- [ ] ")
		buf.WriteString(after)
		buf.WriteString("\n")
		return
	}

	var marker string
	if classifyListGlyph(lists, bullet) == listOrdered {
		marker = "1. "
	} else {
		marker = "- "
	}

	buf.WriteString(indent)
	buf.WriteString(marker)
	buf.WriteString(text)
	buf.WriteString("\n")
}

// writeTable converts a Google Docs Table into GFM table syntax.
func writeTable(buf *strings.Builder, table *docs.Table) {
	if table == nil || len(table.TableRows) == 0 {
		return
	}

	rows := table.TableRows

	// Determine column count from the first row.
	numCols := 0
	if len(rows) > 0 && len(rows[0].TableCells) > 0 {
		numCols = len(rows[0].TableCells)
	}
	if numCols == 0 {
		return
	}

	// Write each row.
	for rowIdx, row := range rows {
		buf.WriteString("|")
		for _, cell := range row.TableCells {
			buf.WriteString(" ")
			buf.WriteString(tableCellText(cell))
			buf.WriteString(" |")
		}
		buf.WriteString("\n")

		// After the header row (first row), write the separator.
		if rowIdx == 0 {
			buf.WriteString("|")
			for range numCols {
				buf.WriteString(" --- |")
			}
			buf.WriteString("\n")
		}
	}
	buf.WriteString("\n")
}

// tableCellText extracts and trims the text content of a table cell.
func tableCellText(cell *docs.TableCell) string {
	var parts []string
	for _, elem := range cell.Content {
		if elem.Paragraph == nil {
			continue
		}
		text := renderTextRuns(elem.Paragraph.Elements)
		text = strings.TrimRight(text, "\n")
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}
