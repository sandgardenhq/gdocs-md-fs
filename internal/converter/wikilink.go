package converter

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// brokenWikiLinkURL is the sentinel URL applied to wikilinks whose target does
// not resolve to a Doc in the mounted tree. The read path recognizes this URL
// and renders such links back as [[...]] wikilinks.
const brokenWikiLinkURL = "https://example.com/broken-link"

// brokenLinkRed is the red component of the maroon foreground color used to
// visually mark broken wikilinks.
const brokenLinkRed = 0.8

// WikiResolver maps a wikilink target (the text inside [[...]], before any
// alias) to a Doc URL. It returns the URL and true when a matching Doc exists in
// the mounted tree, or "", false when the target does not resolve.
type WikiResolver func(target string) (url string, ok bool)

// Option configures FromMarkdown.
type Option func(*config)

// config holds optional FromMarkdown settings.
type config struct {
	resolver WikiResolver
}

// WithWikiResolver supplies a resolver used to turn wikilink targets into Doc
// URLs. Without it, every wikilink is treated as broken.
func WithWikiResolver(r WikiResolver) Option {
	return func(c *config) {
		c.resolver = r
	}
}

// KindWikiLink is the AST node kind for wikilinks.
var KindWikiLink = ast.NewNodeKind("WikiLink")

// WikiLink is an inline AST node representing an Obsidian-style [[Target]] or
// [[Target|Alias]] wikilink.
type WikiLink struct {
	ast.BaseInline
	Target string // the link target (page name or path)
	Alias  string // optional display text; empty when absent
}

// Kind returns the node kind.
func (n *WikiLink) Kind() ast.NodeKind { return KindWikiLink }

// Dump implements ast.Node.
func (n *WikiLink) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Target": n.Target,
		"Alias":  n.Alias,
	}, nil)
}

// wikiLinkParser is a goldmark inline parser for [[...]] syntax.
type wikiLinkParser struct{}

// Trigger returns the byte that starts a wikilink.
func (p *wikiLinkParser) Trigger() []byte { return []byte{'['} }

// Parse recognizes a wikilink at the current position. It returns nil (letting
// other parsers, such as the standard link parser, try) for anything that is
// not a well-formed, non-empty [[...]].
func (p *wikiLinkParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	// Need at least "[[x]]".
	if len(line) < 5 || line[0] != '[' || line[1] != '[' {
		return nil
	}

	closing := bytes.Index(line[2:], []byte("]]"))
	if closing < 0 {
		return nil
	}
	inner := line[2 : 2+closing]

	target, alias, _ := bytes.Cut(inner, []byte{'|'})

	targetStr := strings.TrimSpace(string(target))
	if targetStr == "" {
		return nil
	}
	aliasStr := strings.TrimSpace(string(alias))

	block.Advance(2 + closing + 2)
	return &WikiLink{Target: targetStr, Alias: aliasStr}
}

// wikiLinkInlineParser is the prioritized parser registered with goldmark. It
// runs before the standard link parser (priority 200) so that [[ is handled as
// a wikilink rather than a nested link.
var wikiLinkInlineParser = util.Prioritized(&wikiLinkParser{}, 199)

// handleWikiLink emits requests for a wikilink node, resolving its target to a
// Doc URL or, when unresolved, a red sentinel-linked placeholder.
func (b *requestBuilder) handleWikiLink(n *WikiLink) {
	display := n.Target
	if n.Alias != "" {
		display = n.Alias
	}

	var url string
	var ok bool
	if b.resolver != nil {
		url, ok = b.resolver(n.Target)
	}

	prevURL, prevBroken := b.linkURL, b.linkBroken
	if ok {
		b.linkURL = url
		b.linkBroken = false
	} else {
		b.linkURL = brokenWikiLinkURL
		b.linkBroken = true
	}
	b.insertText(display)
	b.linkURL, b.linkBroken = prevURL, prevBroken
}
