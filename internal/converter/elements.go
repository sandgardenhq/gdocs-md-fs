// Package converter provides bidirectional conversion between Google Docs API
// structured documents and Markdown format.
package converter

import (
	"strings"

	docs "google.golang.org/api/docs/v1"
)

// Named paragraph styles from the Google Docs API.
const (
	StyleNormalText = "NORMAL_TEXT"
	StyleHeading1   = "HEADING_1"
	StyleHeading2   = "HEADING_2"
	StyleHeading3   = "HEADING_3"
	StyleHeading4   = "HEADING_4"
	StyleHeading5   = "HEADING_5"
	StyleHeading6   = "HEADING_6"
	StyleSubtitle   = "SUBTITLE"
	StyleTitle      = "TITLE"
)

// headingPrefix maps named paragraph styles to their Markdown heading prefix.
var headingPrefix = map[string]string{
	StyleTitle:    "# ",
	StyleSubtitle: "## ",
	StyleHeading1: "# ",
	StyleHeading2: "## ",
	StyleHeading3: "### ",
	StyleHeading4: "#### ",
	StyleHeading5: "##### ",
	StyleHeading6: "###### ",
}

// monospaceFonts lists font families that indicate code/monospace text.
var monospaceFonts = map[string]bool{
	"Courier New":      true,
	"Courier":          true,
	"Consolas":         true,
	"Roboto Mono":      true,
	"Source Code Pro":  true,
	"Inconsolata":      true,
	"Fira Code":        true,
	"Ubuntu Mono":      true,
	"Noto Sans Mono":   true,
	"JetBrains Mono":   true,
	"Cascadia Code":    true,
	"Cascadia Mono":    true,
	"Menlo":            true,
	"Monaco":           true,
	"SF Mono":          true,
	"Liberation Mono":  true,
	"DejaVu Sans Mono": true,
}

// isBold returns true if the text style has bold formatting.
func isBold(ts *docs.TextStyle) bool {
	return ts != nil && ts.Bold
}

// isItalic returns true if the text style has italic formatting.
func isItalic(ts *docs.TextStyle) bool {
	return ts != nil && ts.Italic
}

// isStrikethrough returns true if the text style has strikethrough formatting.
func isStrikethrough(ts *docs.TextStyle) bool {
	return ts != nil && ts.Strikethrough
}

// isCode returns true if the text style uses a monospace font family,
// which conventionally represents inline code.
func isCode(ts *docs.TextStyle) bool {
	if ts == nil || ts.WeightedFontFamily == nil {
		return false
	}
	family := ts.WeightedFontFamily.FontFamily
	if monospaceFonts[family] {
		return true
	}
	// Catch common patterns like "Mono" or "Code" in font name.
	lower := strings.ToLower(family)
	return strings.Contains(lower, "mono") || strings.Contains(lower, "code")
}

// isMonospaceParagraph returns true when every text run in a paragraph uses a
// monospace font, indicating the paragraph should be rendered as a code block.
func isMonospaceParagraph(p *docs.Paragraph) bool {
	if p == nil || len(p.Elements) == 0 {
		return false
	}
	for _, elem := range p.Elements {
		if elem.TextRun == nil {
			continue
		}
		// Skip runs that are only whitespace/newlines.
		if strings.TrimSpace(elem.TextRun.Content) == "" {
			continue
		}
		if !isCode(elem.TextRun.TextStyle) {
			return false
		}
	}
	return true
}

// listGlyphType classifies a Google Docs bullet glyph type.
type listGlyphType int

const (
	listUnordered listGlyphType = iota
	listOrdered
)

// classifyListGlyph determines whether a list is ordered or unordered based on
// the bullet's glyph type at the given nesting level.
func classifyListGlyph(lists map[string]docs.List, bullet *docs.Bullet) listGlyphType {
	if bullet == nil || bullet.ListId == "" {
		return listUnordered
	}
	list, ok := lists[bullet.ListId]
	if !ok || list.ListProperties == nil {
		return listUnordered
	}

	level := int(bullet.NestingLevel)
	props := list.ListProperties.NestingLevels
	if level >= len(props) {
		level = 0
	}
	if level >= len(props) {
		return listUnordered
	}

	switch props[level].GlyphType {
	case "DECIMAL", "ZERO_DECIMAL", "ALPHA", "UPPER_ALPHA", "ROMAN", "UPPER_ROMAN":
		return listOrdered
	default:
		return listUnordered
	}
}

// nestingIndent returns the Markdown indentation string for a given nesting
// level. Each level adds 4 spaces.
func nestingIndent(level int64) string {
	if level <= 0 {
		return ""
	}
	return strings.Repeat("    ", int(level))
}

// headingPrefixFor returns the Markdown heading prefix for a named style,
// or an empty string if the style is not a heading.
func headingPrefixFor(style string) string {
	return headingPrefix[style]
}
