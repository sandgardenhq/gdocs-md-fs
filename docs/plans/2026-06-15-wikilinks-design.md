# Wikilink Support — Design

Date: 2026-06-15

## Goal

Support Obsidian-style wikilinks in Markdown that resolve to real Google Docs
hyperlinks. Writing `[[Page Name]]` in a mounted Markdown file creates a
clickable link to the Google Doc named "Page Name" within the mounted folder
tree.

## Syntax

- `[[Page Name]]` — link to a Doc named "Page Name".
- `[[subfolder/Page Name]]` — path-qualified target within the mounted tree.
- `[[Page Name|Custom Text]]` — alias; the link targets "Page Name" but displays
  "Custom Text".

Displayed text is the alias when present, otherwise the target string exactly as
written (so `[[subfolder/Page Name]]` displays `subfolder/Page Name`). This keeps
broken links round-tripping cleanly.

Degenerate forms render as literal text, not links: `[[]]`, `[[ ]]`, an unclosed
`[[Page`, and `[[Page|]]` (empty alias behaves like `[[Page]]`).

## Resolution

A wikilink resolves against the mounted folder tree:

- A bare name (`[[Page Name]]`) searches the whole tree for a Doc with that name.
  Trees are expected to be small, so a recursive walk is acceptable.
- A path (`[[subfolder/Page Name]]`) resolves relative to the mount root.
- Only Google Docs are valid targets. A non-Doc file with a matching name does
  not resolve.

Resolved target → real hyperlink `https://docs.google.com/document/d/<id>/edit`.

## Broken links

A wikilink whose target does not exist becomes a real hyperlink to the sentinel
URL `https://example.com/broken-link`, with the displayed text colored maroon
(`RgbColor{Red: 0.8}`). The link is present and visible, marked as broken. Once
the target Doc is created and the file is re-saved, the link upgrades to a real
Doc URL.

## Architecture

The converter package stays pure. Wikilink resolution is injected as a plain
function — not a mock:

```go
// Returns the Doc's URL and true if a Doc matching target exists in the tree;
// "", false if not found (-> broken link).
type WikiResolver func(target string) (url string, ok bool)

func FromMarkdown(md []byte, opts ...Option) ([]*docs.Request, error)
func WithWikiResolver(r WikiResolver) Option
```

Functional options preserve the existing `FromMarkdown(md)` call sites and tests.
With no resolver supplied, every wikilink resolves as broken.

### Write path (Markdown -> Docs)

1. A custom goldmark inline parser, registered at higher priority than the GFM
   link parser, triggers on `[`, peeks for a second `[`, and consumes through the
   closing `]]`. The content splits on the first `|` into `Target` and optional
   `Alias` (both trimmed).
2. A new AST node `WikiLink{ Target, Alias string }` (embedding `ast.BaseInline`)
   carries the data through the walk.
3. `walkNode` gains a `case *WikiLink` -> `handleWikiLink`, which mirrors
   `handleLink`: compute displayed text, set the link URL (resolved or sentinel),
   flag broken links for coloring, insert the text, restore prior state. It reuses
   the existing `insertText` -> `UpdateTextStyleRequest` machinery, so nested
   formatting and UTF-16 indexing already work.

Resolved links emit `Fields: "link"`. Broken links emit
`Fields: "link,foregroundColor"` with the maroon color over the same range.

The resolver implementation lives in the gdrive layer (`docs.go` / `handler.go`),
where `DriveHandler.Write` has the Drive client, current file path, and parent
folder in scope. It builds a closure passed down through `buildWriteRequests` ->
`markdownToDocRequests` -> `FromMarkdown(md, WithWikiResolver(closure))`.

### Read path (Docs -> Markdown)

Deliberately minimal; no Drive access. In `renderTextRuns`, a link run whose URL
equals the sentinel renders as `[[displayed text]]`. Every other link — including
real in-tree Doc links produced by resolved wikilinks — renders as a normal
`[text](url)`.

Consequences:

- Broken wikilinks round-trip exactly (`[[New Page]]` -> red sentinel link ->
  `[[New Page]]`).
- Resolved wikilinks are intentionally lossy on read-back: `[[Existing Page]]`
  reads back as `[Existing Page](https://docs.google.com/document/d/<id>/edit)`
  and re-saves as a working plain link.
- A broken aliased link `[[Page|Alias]]` reads back as `[[Alias]]` (the target is
  not recoverable from a sentinel link) — an accepted edge case.

The sentinel is a single shared constant so write and read agree:

```go
const brokenWikiLinkURL = "https://example.com/broken-link"
```

## Testing (TDD)

RED -> GREEN -> REFACTOR, committing after each cycle. No mocks; the resolver is a
plain function injected in tests.

**Parser / AST (converter):**
1. `[[Page Name]]` -> `WikiLink{Target:"Page Name"}`.
2. `[[a/b/Page]]` -> `Target:"a/b/Page"`.
3. `[[Page|Alias]]` -> `Target:"Page", Alias:"Alias"`.
4. Degenerate cases render literally: `[[]]`, `[[ ]]`, unclosed `[[Page`,
   `[[Page|]]`.

**Write path (`FromMarkdown` + test resolver):**
5. Resolver returns URL -> `InsertText` of displayed text + `UpdateTextStyle` with
   that `Link.Url`, no foreground color.
6. Resolver returns `ok=false` -> sentinel URL + maroon foreground,
   `Fields: "link,foregroundColor"`.
7. No resolver -> treated as broken.
8. Alias -> displayed text = alias, link applied over it.
9. Path target displayed text = full `a/b/Page` when no alias.
10. Resolver receives the raw target string.

**Read path (`ToMarkdown`):**
11. Sentinel URL link -> `[[text]]`.
12. Any other URL -> `[text](url)` unchanged.

**gdrive resolver closure:**
13. Bare name resolves to a Doc via tree walk -> correct Doc URL.
14. `subfolder/Name` resolves relative to mount root.
15. Missing target -> `ok=false`.
16. Non-Doc match -> not resolvable.

**Gates:** 90%+ line / 85%+ branch coverage, `golangci-lint run ./...` clean,
build green.
