package converter

import (
	"strings"
	"testing"

	docs "google.golang.org/api/docs/v1"
)

// --- Helper factories for building mock Google Docs structures ---

// makeDoc creates a docs.Document with the given structural elements.
func makeDoc(elements ...*docs.StructuralElement) *docs.Document {
	return &docs.Document{
		Body: &docs.Body{
			Content: elements,
		},
		Lists: make(map[string]docs.List),
	}
}

// makeDocWithLists creates a docs.Document with structural elements and list definitions.
func makeDocWithLists(lists map[string]docs.List, elements ...*docs.StructuralElement) *docs.Document {
	d := makeDoc(elements...)
	d.Lists = lists
	return d
}

// makeParagraph creates a StructuralElement containing a paragraph with the given style and text runs.
func makeParagraph(style string, runs ...*docs.TextRun) *docs.StructuralElement {
	var elements []*docs.ParagraphElement
	for _, run := range runs {
		elements = append(elements, &docs.ParagraphElement{TextRun: run})
	}
	return &docs.StructuralElement{
		Paragraph: &docs.Paragraph{
			ParagraphStyle: &docs.ParagraphStyle{
				NamedStyleType: style,
			},
			Elements: elements,
		},
	}
}

// makeListParagraph creates a StructuralElement containing a list item paragraph.
func makeListParagraph(listID string, nestingLevel int64, runs ...*docs.TextRun) *docs.StructuralElement {
	var elements []*docs.ParagraphElement
	for _, run := range runs {
		elements = append(elements, &docs.ParagraphElement{TextRun: run})
	}
	return &docs.StructuralElement{
		Paragraph: &docs.Paragraph{
			ParagraphStyle: &docs.ParagraphStyle{
				NamedStyleType: StyleNormalText,
			},
			Bullet: &docs.Bullet{
				ListId:       listID,
				NestingLevel: nestingLevel,
			},
			Elements: elements,
		},
	}
}

// textRun creates a plain TextRun.
func textRun(content string) *docs.TextRun {
	return &docs.TextRun{
		Content:   content,
		TextStyle: &docs.TextStyle{},
	}
}

// boldRun creates a bold TextRun.
func boldRun(content string) *docs.TextRun {
	return &docs.TextRun{
		Content: content,
		TextStyle: &docs.TextStyle{
			Bold: true,
		},
	}
}

// italicRun creates an italic TextRun.
func italicRun(content string) *docs.TextRun {
	return &docs.TextRun{
		Content: content,
		TextStyle: &docs.TextStyle{
			Italic: true,
		},
	}
}

// boldItalicRun creates a bold+italic TextRun.
func boldItalicRun(content string) *docs.TextRun {
	return &docs.TextRun{
		Content: content,
		TextStyle: &docs.TextStyle{
			Bold:   true,
			Italic: true,
		},
	}
}

// strikethroughRun creates a strikethrough TextRun.
func strikethroughRun(content string) *docs.TextRun {
	return &docs.TextRun{
		Content: content,
		TextStyle: &docs.TextStyle{
			Strikethrough: true,
		},
	}
}

// codeRun creates a monospace (code) TextRun.
func codeRun(content string) *docs.TextRun {
	return &docs.TextRun{
		Content: content,
		TextStyle: &docs.TextStyle{
			WeightedFontFamily: &docs.WeightedFontFamily{
				FontFamily: "Courier New",
			},
		},
	}
}

// linkRun creates a TextRun with a hyperlink.
func linkRun(content, url string) *docs.TextRun {
	return &docs.TextRun{
		Content: content,
		TextStyle: &docs.TextStyle{
			Link: &docs.Link{Url: url},
		},
	}
}

// makeTable creates a StructuralElement containing a table.
func makeTable(rows ...[]string) *docs.StructuralElement {
	var tableRows []*docs.TableRow
	for _, row := range rows {
		var cells []*docs.TableCell
		for _, cellText := range row {
			cells = append(cells, &docs.TableCell{
				Content: []*docs.StructuralElement{
					makeParagraph(StyleNormalText, textRun(cellText+"\n")),
				},
			})
		}
		tableRows = append(tableRows, &docs.TableRow{TableCells: cells})
	}
	return &docs.StructuralElement{
		Table: &docs.Table{
			Rows:      int64(len(rows)),
			Columns:   int64(len(rows[0])),
			TableRows: tableRows,
		},
	}
}

// makeHorizontalRule creates a StructuralElement containing a horizontal rule.
func makeHorizontalRule() *docs.StructuralElement {
	return &docs.StructuralElement{
		Paragraph: &docs.Paragraph{
			ParagraphStyle: &docs.ParagraphStyle{
				NamedStyleType: StyleNormalText,
			},
			Elements: []*docs.ParagraphElement{
				{HorizontalRule: &docs.HorizontalRule{}},
				{TextRun: textRun("\n")},
			},
		},
	}
}

// makeUnorderedList creates a list definition for an unordered list.
func makeUnorderedList() *docs.List {
	return &docs.List{
		ListProperties: &docs.ListProperties{
			NestingLevels: []*docs.NestingLevel{
				{GlyphType: "GLYPH_TYPE_UNSPECIFIED"},
			},
		},
	}
}

// makeOrderedList creates a list definition for an ordered list.
func makeOrderedList() *docs.List {
	return &docs.List{
		ListProperties: &docs.ListProperties{
			NestingLevels: []*docs.NestingLevel{
				{GlyphType: "DECIMAL"},
			},
		},
	}
}

// --- ToMarkdown Tests ---

func TestToMarkdown_Headings(t *testing.T) {
	tests := []struct {
		name     string
		style    string
		text     string
		expected string
	}{
		{"Title", StyleTitle, "My Title", "# My Title\n\n"},
		{"Subtitle", StyleSubtitle, "My Subtitle", "## My Subtitle\n\n"},
		{"Heading1", StyleHeading1, "Heading 1", "# Heading 1\n\n"},
		{"Heading2", StyleHeading2, "Heading 2", "## Heading 2\n\n"},
		{"Heading3", StyleHeading3, "Heading 3", "### Heading 3\n\n"},
		{"Heading4", StyleHeading4, "Heading 4", "#### Heading 4\n\n"},
		{"Heading5", StyleHeading5, "Heading 5", "##### Heading 5\n\n"},
		{"Heading6", StyleHeading6, "Heading 6", "###### Heading 6\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := makeDoc(makeParagraph(tt.style, textRun(tt.text+"\n")))
			result, err := ToMarkdown(doc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(result) != tt.expected {
				t.Errorf("got %q, want %q", string(result), tt.expected)
			}
		})
	}
}

func TestToMarkdown_HeadingBoldNotDoubled(t *testing.T) {
	// Google Docs heading styles inherently set Bold=true on text runs.
	// The ToMarkdown converter must NOT render this as **text** since
	// the # prefix already conveys the heading. Without this, headings
	// render as "# **Heading Text**" which is incorrect markdown.
	doc := makeDoc(makeParagraph(StyleHeading1, boldRun("My Heading\n")))
	result, err := ToMarkdown(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(result)
	expected := "# My Heading\n\n"
	if got != expected {
		t.Errorf("heading with bold text style: got %q, want %q", got, expected)
	}
}

func TestToMarkdown_HeadingExplicitItalicPreserved(t *testing.T) {
	// Italic on headings IS explicit formatting and should be preserved.
	// Bold+italic on a heading should render as just italic (bold is from style).
	doc := makeDoc(makeParagraph(StyleHeading1, boldItalicRun("Emphasis Heading\n")))
	result, err := ToMarkdown(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(result)
	expected := "# *Emphasis Heading*\n\n"
	if got != expected {
		t.Errorf("heading with bold+italic: got %q, want %q", got, expected)
	}
}

func TestToMarkdown_TextFormatting(t *testing.T) {
	tests := []struct {
		name     string
		runs     []*docs.TextRun
		expected string
	}{
		{
			name:     "Bold",
			runs:     []*docs.TextRun{boldRun("bold text")},
			expected: "**bold text**\n",
		},
		{
			name:     "Italic",
			runs:     []*docs.TextRun{italicRun("italic text")},
			expected: "*italic text*\n",
		},
		{
			name:     "BoldItalic",
			runs:     []*docs.TextRun{boldItalicRun("bold italic")},
			expected: "***bold italic***\n",
		},
		{
			name:     "Strikethrough",
			runs:     []*docs.TextRun{strikethroughRun("deleted")},
			expected: "~~deleted~~\n",
		},
		{
			name:     "InlineCode",
			runs:     []*docs.TextRun{codeRun("fmt.Println()")},
			expected: "`fmt.Println()`\n",
		},
		{
			name: "MixedFormatting",
			runs: []*docs.TextRun{
				textRun("normal "),
				boldRun("bold"),
				textRun(" and "),
				italicRun("italic"),
				textRun("\n"),
			},
			expected: "normal **bold** and *italic*\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := makeDoc(makeParagraph(StyleNormalText, tt.runs...))
			result, err := ToMarkdown(doc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := string(result)
			if got != tt.expected {
				t.Errorf("got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestToMarkdown_Lists(t *testing.T) {
	t.Run("UnorderedList", func(t *testing.T) {
		lists := map[string]docs.List{
			"list1": *makeUnorderedList(),
		}
		doc := makeDocWithLists(lists,
			makeListParagraph("list1", 0, textRun("Item one\n")),
			makeListParagraph("list1", 0, textRun("Item two\n")),
			makeListParagraph("list1", 0, textRun("Item three\n")),
		)
		result, err := ToMarkdown(doc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "- Item one\n- Item two\n- Item three\n"
		if string(result) != expected {
			t.Errorf("got %q, want %q", string(result), expected)
		}
	})

	t.Run("OrderedList", func(t *testing.T) {
		lists := map[string]docs.List{
			"list2": *makeOrderedList(),
		}
		doc := makeDocWithLists(lists,
			makeListParagraph("list2", 0, textRun("First\n")),
			makeListParagraph("list2", 0, textRun("Second\n")),
			makeListParagraph("list2", 0, textRun("Third\n")),
		)
		result, err := ToMarkdown(doc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "1. First\n1. Second\n1. Third\n"
		if string(result) != expected {
			t.Errorf("got %q, want %q", string(result), expected)
		}
	})

	t.Run("NestedList", func(t *testing.T) {
		lists := map[string]docs.List{
			"list3": {
				ListProperties: &docs.ListProperties{
					NestingLevels: []*docs.NestingLevel{
						{GlyphType: "GLYPH_TYPE_UNSPECIFIED"},
						{GlyphType: "GLYPH_TYPE_UNSPECIFIED"},
					},
				},
			},
		}
		doc := makeDocWithLists(lists,
			makeListParagraph("list3", 0, textRun("Top\n")),
			makeListParagraph("list3", 1, textRun("Nested\n")),
			makeListParagraph("list3", 0, textRun("Back\n")),
		)
		result, err := ToMarkdown(doc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "- Top\n    - Nested\n- Back\n"
		if string(result) != expected {
			t.Errorf("got %q, want %q", string(result), expected)
		}
	})
}

func TestToMarkdown_Links(t *testing.T) {
	doc := makeDoc(makeParagraph(StyleNormalText,
		textRun("Click "),
		linkRun("here", "https://example.com"),
		textRun(" for more.\n"),
	))

	result, err := ToMarkdown(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Click [here](https://example.com) for more.\n"
	if string(result) != expected {
		t.Errorf("got %q, want %q", string(result), expected)
	}
}

func TestToMarkdown_Table(t *testing.T) {
	doc := makeDoc(makeTable(
		[]string{"Name", "Age"},
		[]string{"Alice", "30"},
		[]string{"Bob", "25"},
	))

	result, err := ToMarkdown(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "| Name | Age |\n| --- | --- |\n| Alice | 30 |\n| Bob | 25 |\n\n"
	if string(result) != expected {
		t.Errorf("got:\n%s\nwant:\n%s", string(result), expected)
	}
}

func TestToMarkdown_HorizontalRule(t *testing.T) {
	doc := makeDoc(
		makeParagraph(StyleNormalText, textRun("Above\n")),
		makeHorizontalRule(),
		makeParagraph(StyleNormalText, textRun("Below\n")),
	)

	result, err := ToMarkdown(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Above\n---\n\nBelow\n"
	if string(result) != expected {
		t.Errorf("got %q, want %q", string(result), expected)
	}
}

func TestToMarkdown_CodeBlock(t *testing.T) {
	doc := makeDoc(
		makeParagraph(StyleNormalText, codeRun("func main() {\n")),
		makeParagraph(StyleNormalText, codeRun("    fmt.Println(\"hello\")\n")),
		makeParagraph(StyleNormalText, codeRun("}\n")),
	)

	result, err := ToMarkdown(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "```\nfunc main() {\n    fmt.Println(\"hello\")\n}\n```\n"
	if string(result) != expected {
		t.Errorf("got %q, want %q", string(result), expected)
	}
}

func TestToMarkdown_NilDocument(t *testing.T) {
	_, err := ToMarkdown(nil)
	if err == nil {
		t.Error("expected error for nil document")
	}
}

func TestToMarkdown_EmptyBody(t *testing.T) {
	doc := &docs.Document{Body: &docs.Body{}}
	result, err := ToMarkdown(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %q", string(result))
	}
}

// --- FromMarkdown Tests ---

func TestFromMarkdown_BasicText(t *testing.T) {
	md := []byte("Hello, world!\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Goldmark may split text into multiple nodes. Concatenate all InsertText
	// content and check for the expected string.
	var allText strings.Builder
	for _, req := range requests {
		if req.InsertText != nil {
			allText.WriteString(req.InsertText.Text)
		}
	}
	combined := allText.String()
	if !strings.Contains(combined, "Hello, world!") {
		t.Errorf("expected combined InsertText to contain 'Hello, world!', got %q", combined)
	}
}

func TestFromMarkdown_Headings(t *testing.T) {
	md := []byte("# Heading 1\n\n## Heading 2\n\n### Heading 3\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have UpdateParagraphStyle requests for each heading level.
	styles := make(map[string]bool)
	for _, req := range requests {
		if req.UpdateParagraphStyle != nil {
			styles[req.UpdateParagraphStyle.ParagraphStyle.NamedStyleType] = true
		}
	}

	for _, expected := range []string{StyleHeading1, StyleHeading2, StyleHeading3} {
		if !styles[expected] {
			t.Errorf("missing UpdateParagraphStyle for %s", expected)
		}
	}
}

func TestFromMarkdown_Formatting(t *testing.T) {
	md := []byte("**bold** *italic* ~~strike~~ `code`\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check for UpdateTextStyle requests.
	var hasBold, hasItalic, hasStrike, hasCode bool
	for _, req := range requests {
		if req.UpdateTextStyle == nil {
			continue
		}
		ts := req.UpdateTextStyle.TextStyle
		fields := req.UpdateTextStyle.Fields
		if ts.Bold && strings.Contains(fields, "bold") {
			hasBold = true
		}
		if ts.Italic && strings.Contains(fields, "italic") {
			hasItalic = true
		}
		if ts.Strikethrough && strings.Contains(fields, "strikethrough") {
			hasStrike = true
		}
		if ts.WeightedFontFamily != nil && strings.Contains(fields, "weightedFontFamily") {
			hasCode = true
		}
	}

	if !hasBold {
		t.Error("missing bold UpdateTextStyle request")
	}
	if !hasItalic {
		t.Error("missing italic UpdateTextStyle request")
	}
	if !hasStrike {
		t.Error("missing strikethrough UpdateTextStyle request")
	}
	if !hasCode {
		t.Error("missing code (monospace) UpdateTextStyle request")
	}
}

func TestFromMarkdown_Links(t *testing.T) {
	md := []byte("[click here](https://example.com)\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have an UpdateTextStyle request with a link.
	found := false
	for _, req := range requests {
		if req.UpdateTextStyle != nil && req.UpdateTextStyle.TextStyle.Link != nil {
			if req.UpdateTextStyle.TextStyle.Link.Url == "https://example.com" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("missing link UpdateTextStyle request")
	}

	// Should also have InsertText for "click here".
	hasText := false
	for _, req := range requests {
		if req.InsertText != nil && strings.Contains(req.InsertText.Text, "click here") {
			hasText = true
			break
		}
	}
	if !hasText {
		t.Error("missing InsertText for link text 'click here'")
	}
}

func TestFromMarkdown_List(t *testing.T) {
	md := []byte("- item one\n- item two\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have InsertText requests containing the list items.
	var hasItem1, hasItem2 bool
	for _, req := range requests {
		if req.InsertText == nil {
			continue
		}
		if strings.Contains(req.InsertText.Text, "item one") {
			hasItem1 = true
		}
		if strings.Contains(req.InsertText.Text, "item two") {
			hasItem2 = true
		}
	}
	if !hasItem1 {
		t.Error("missing InsertText for 'item one'")
	}
	if !hasItem2 {
		t.Error("missing InsertText for 'item two'")
	}
}

func TestFromMarkdown_CodeBlock(t *testing.T) {
	md := []byte("```\nfunc main() {}\n```\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have an InsertText with the code content.
	hasCode := false
	for _, req := range requests {
		if req.InsertText != nil && strings.Contains(req.InsertText.Text, "func main()") {
			hasCode = true
			break
		}
	}
	if !hasCode {
		t.Error("missing InsertText for code block content")
	}

	// Should have an UpdateTextStyle with monospace font.
	hasMono := false
	for _, req := range requests {
		if req.UpdateTextStyle != nil &&
			req.UpdateTextStyle.TextStyle.WeightedFontFamily != nil &&
			req.UpdateTextStyle.TextStyle.WeightedFontFamily.FontFamily == "Courier New" {
			hasMono = true
			break
		}
	}
	if !hasMono {
		t.Error("missing monospace UpdateTextStyle for code block")
	}
}

func TestFromMarkdown_ParagraphSetsNormalTextStyle(t *testing.T) {
	md := []byte("Just a plain paragraph.\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A plain paragraph MUST produce an UpdateParagraphStyle request with
	// NORMAL_TEXT. Without this, the paragraph inherits whatever style the
	// Google Doc previously had (e.g. HEADING_1), causing formatting
	// corruption on round-trip writes.
	found := false
	for _, req := range requests {
		if req.UpdateParagraphStyle != nil &&
			req.UpdateParagraphStyle.ParagraphStyle.NamedStyleType == StyleNormalText {
			found = true
			break
		}
	}
	if !found {
		t.Error("plain paragraph must emit UpdateParagraphStyle with NORMAL_TEXT to prevent style bleeding")
	}
}

func TestFromMarkdown_ListItemSetsNormalTextStyle(t *testing.T) {
	md := []byte("- list item\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// List item paragraphs must also emit UpdateParagraphStyle with
	// NORMAL_TEXT to prevent style bleeding from prior headings.
	found := false
	for _, req := range requests {
		if req.UpdateParagraphStyle != nil &&
			req.UpdateParagraphStyle.ParagraphStyle.NamedStyleType == StyleNormalText {
			found = true
			break
		}
	}
	if !found {
		t.Error("list item must emit UpdateParagraphStyle with NORMAL_TEXT to prevent style bleeding")
	}
}

func TestFromMarkdown_BlockquoteSetsNormalTextStyle(t *testing.T) {
	md := []byte("> quoted text\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Blockquote paragraphs must also emit UpdateParagraphStyle with
	// NORMAL_TEXT to prevent style bleeding from prior headings.
	found := false
	for _, req := range requests {
		if req.UpdateParagraphStyle != nil &&
			req.UpdateParagraphStyle.ParagraphStyle.NamedStyleType == StyleNormalText {
			found = true
			break
		}
	}
	if !found {
		t.Error("blockquote must emit UpdateParagraphStyle with NORMAL_TEXT to prevent style bleeding")
	}
}

func TestFromMarkdown_EmojiUTF16Indices(t *testing.T) {
	// Emojis like 🤷 (U+1F937) are 1 Go rune but 2 UTF-16 code units.
	// The Google Docs API uses UTF-16 indices. If the cursor counts runes
	// instead of UTF-16 code units, insertion indices after an emoji will
	// be wrong, causing "insertion index within grapheme cluster" errors.
	md := []byte("Hello 🤷 world\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Collect all InsertText requests and verify indices are sequential
	// using UTF-16 counting. "Hello 🤷 world\n" in UTF-16:
	//   H(1) e(1) l(1) l(1) o(1) (1) 🤷(2) (1) w(1) o(1) r(1) l(1) d(1) \n(1)
	//   = 6 + 2 + 1 + 5 + 1 = 15 UTF-16 code units
	// Starting at index 1, the final cursor should be 1 + 15 = 16.
	var maxEnd int64
	for _, req := range requests {
		if req.InsertText != nil {
			idx := req.InsertText.Location.Index
			textLen := utf16Len(req.InsertText.Text)
			end := idx + int64(textLen)
			if end > maxEnd {
				maxEnd = end
			}
		}
		// Check that style ranges don't split a surrogate pair.
		if req.UpdateTextStyle != nil && req.UpdateTextStyle.Range != nil {
			r := req.UpdateTextStyle.Range
			if r.StartIndex < 0 || r.EndIndex < r.StartIndex {
				t.Errorf("invalid text style range: [%d, %d)", r.StartIndex, r.EndIndex)
			}
		}
		if req.UpdateParagraphStyle != nil && req.UpdateParagraphStyle.Range != nil {
			r := req.UpdateParagraphStyle.Range
			if r.StartIndex < 0 || r.EndIndex < r.StartIndex {
				t.Errorf("invalid paragraph style range: [%d, %d)", r.StartIndex, r.EndIndex)
			}
		}
	}

	// The final cursor position must account for UTF-16 surrogate pairs.
	// With rune counting, cursor would be 1+14=15 (wrong).
	// With UTF-16 counting, cursor would be 1+15=16 (correct).
	if maxEnd != 16 {
		t.Errorf("final cursor position: got %d, want 16 (UTF-16 code units from index 1)", maxEnd)
	}
}

// utf16Len returns the number of UTF-16 code units needed to encode s.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2 // surrogate pair
		} else {
			n++
		}
	}
	return n
}

func TestFromMarkdown_MultipleEmojis(t *testing.T) {
	// Multiple emojis in sequence to verify cursor accumulation.
	md := []byte("🎉🎊🎈\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Each emoji is 2 UTF-16 code units. 3 emojis + \n = 7 UTF-16 units.
	// Starting at index 1, final cursor should be 8.
	var maxEnd int64
	for _, req := range requests {
		if req.InsertText != nil {
			idx := req.InsertText.Location.Index
			textLen := utf16Len(req.InsertText.Text)
			end := idx + int64(textLen)
			if end > maxEnd {
				maxEnd = end
			}
		}
	}
	if maxEnd != 8 {
		t.Errorf("final cursor position: got %d, want 8 (3 emojis * 2 + \\n + start=1)", maxEnd)
	}
}

func TestFromMarkdown_Empty(t *testing.T) {
	requests, err := FromMarkdown([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty input should produce no requests (the AST has no children).
	if len(requests) != 0 {
		t.Errorf("expected 0 requests for empty input, got %d", len(requests))
	}
}

func TestFromMarkdown_ThematicBreak(t *testing.T) {
	md := []byte("above\n\n---\n\nbelow\n")
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have InsertText for "---".
	found := false
	for _, req := range requests {
		if req.InsertText != nil && strings.Contains(req.InsertText.Text, "---") {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing InsertText for thematic break (---)")
	}
}

// --- Round-trip Test ---

func TestRoundTrip_BasicDocument(t *testing.T) {
	// Create a mock Google Doc with various elements.
	doc := makeDoc(
		makeParagraph(StyleHeading1, textRun("My Document\n")),
		makeParagraph(StyleNormalText, textRun("This is a "), boldRun("bold"), textRun(" paragraph.\n")),
		makeParagraph(StyleHeading2, textRun("Section\n")),
		makeParagraph(StyleNormalText, textRun("Some "), italicRun("italic"), textRun(" text.\n")),
	)

	// Convert to Markdown.
	md, err := ToMarkdown(doc)
	if err != nil {
		t.Fatalf("ToMarkdown error: %v", err)
	}

	mdStr := string(md)

	// Verify expected Markdown content.
	if !strings.Contains(mdStr, "# My Document") {
		t.Error("Markdown should contain '# My Document'")
	}
	if !strings.Contains(mdStr, "**bold**") {
		t.Error("Markdown should contain '**bold**'")
	}
	if !strings.Contains(mdStr, "## Section") {
		t.Error("Markdown should contain '## Section'")
	}
	if !strings.Contains(mdStr, "*italic*") {
		t.Error("Markdown should contain '*italic*'")
	}

	// Convert Markdown back to Docs requests.
	requests, err := FromMarkdown(md)
	if err != nil {
		t.Fatalf("FromMarkdown error: %v", err)
	}

	// Verify the round-trip produces valid requests.
	if len(requests) == 0 {
		t.Fatal("expected non-empty request list from round-trip")
	}

	// Check that we have heading style requests.
	var hasH1, hasH2 bool
	for _, req := range requests {
		if req.UpdateParagraphStyle != nil {
			switch req.UpdateParagraphStyle.ParagraphStyle.NamedStyleType {
			case StyleHeading1:
				hasH1 = true
			case StyleHeading2:
				hasH2 = true
			}
		}
	}
	if !hasH1 {
		t.Error("round-trip missing HEADING_1 paragraph style")
	}
	if !hasH2 {
		t.Error("round-trip missing HEADING_2 paragraph style")
	}

	// Check that we have bold text style request.
	hasBold := false
	for _, req := range requests {
		if req.UpdateTextStyle != nil && req.UpdateTextStyle.TextStyle.Bold {
			hasBold = true
			break
		}
	}
	if !hasBold {
		t.Error("round-trip missing bold text style")
	}

	// Check that we have italic text style request.
	hasItalic := false
	for _, req := range requests {
		if req.UpdateTextStyle != nil && req.UpdateTextStyle.TextStyle.Italic {
			hasItalic = true
			break
		}
	}
	if !hasItalic {
		t.Error("round-trip missing italic text style")
	}

	// Check that the inserted text contains the original content.
	var allText strings.Builder
	for _, req := range requests {
		if req.InsertText != nil {
			allText.WriteString(req.InsertText.Text)
		}
	}
	combined := allText.String()
	for _, expected := range []string{"My Document", "bold", "paragraph", "Section", "italic", "text"} {
		if !strings.Contains(combined, expected) {
			t.Errorf("round-trip missing text %q in inserted content", expected)
		}
	}
}

// --- Elements helper tests ---

func TestIsBold(t *testing.T) {
	if isBold(nil) {
		t.Error("isBold(nil) should be false")
	}
	if isBold(&docs.TextStyle{}) {
		t.Error("isBold with no bold should be false")
	}
	if !isBold(&docs.TextStyle{Bold: true}) {
		t.Error("isBold with Bold=true should be true")
	}
}

func TestIsItalic(t *testing.T) {
	if isItalic(nil) {
		t.Error("isItalic(nil) should be false")
	}
	if !isItalic(&docs.TextStyle{Italic: true}) {
		t.Error("isItalic with Italic=true should be true")
	}
}

func TestIsStrikethrough(t *testing.T) {
	if isStrikethrough(nil) {
		t.Error("isStrikethrough(nil) should be false")
	}
	if !isStrikethrough(&docs.TextStyle{Strikethrough: true}) {
		t.Error("isStrikethrough with Strikethrough=true should be true")
	}
}

func TestIsCode(t *testing.T) {
	if isCode(nil) {
		t.Error("isCode(nil) should be false")
	}
	if isCode(&docs.TextStyle{}) {
		t.Error("isCode with no font should be false")
	}
	if !isCode(&docs.TextStyle{
		WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Courier New"},
	}) {
		t.Error("isCode with Courier New should be true")
	}
	if !isCode(&docs.TextStyle{
		WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Roboto Mono"},
	}) {
		t.Error("isCode with Roboto Mono should be true")
	}
	// Test fallback pattern matching.
	if !isCode(&docs.TextStyle{
		WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "My Custom Mono Font"},
	}) {
		t.Error("isCode with 'Mono' in name should be true")
	}
	if isCode(&docs.TextStyle{
		WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Arial"},
	}) {
		t.Error("isCode with Arial should be false")
	}
}

func TestClassifyListGlyph(t *testing.T) {
	unordered := makeUnorderedList()
	ordered := makeOrderedList()

	lists := map[string]docs.List{
		"ul": *unordered,
		"ol": *ordered,
	}

	if classifyListGlyph(lists, &docs.Bullet{ListId: "ul"}) != listUnordered {
		t.Error("expected unordered for 'ul' list")
	}
	if classifyListGlyph(lists, &docs.Bullet{ListId: "ol"}) != listOrdered {
		t.Error("expected ordered for 'ol' list")
	}
	if classifyListGlyph(lists, nil) != listUnordered {
		t.Error("expected unordered for nil bullet")
	}
	if classifyListGlyph(lists, &docs.Bullet{ListId: "nonexistent"}) != listUnordered {
		t.Error("expected unordered for unknown list ID")
	}
}

func TestNestingIndent(t *testing.T) {
	if nestingIndent(0) != "" {
		t.Error("nesting level 0 should produce empty string")
	}
	if nestingIndent(1) != "    " {
		t.Errorf("nesting level 1: got %q, want %q", nestingIndent(1), "    ")
	}
	if nestingIndent(2) != "        " {
		t.Errorf("nesting level 2: got %q, want %q", nestingIndent(2), "        ")
	}
}

func TestHeadingPrefixFor(t *testing.T) {
	tests := map[string]string{
		StyleTitle:      "# ",
		StyleSubtitle:   "## ",
		StyleHeading1:   "# ",
		StyleHeading6:   "###### ",
		StyleNormalText: "",
		"UNKNOWN":       "",
	}
	for style, expected := range tests {
		if got := headingPrefixFor(style); got != expected {
			t.Errorf("headingPrefixFor(%q) = %q, want %q", style, got, expected)
		}
	}
}
