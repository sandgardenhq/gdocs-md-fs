package gdrive

import (
	"context"
	"testing"

	"google.golang.org/api/drive/v3"
)

func TestDocURL(t *testing.T) {
	got := docURL("abc123")
	want := "https://docs.google.com/document/d/abc123/edit"
	if got != want {
		t.Errorf("docURL = %q, want %q", got, want)
	}
}

func TestBuildWriteRequests_PassesResolverThrough(t *testing.T) {
	md := []byte("[[Page]]\n")
	resolver := func(target string) (string, bool) {
		if target == "Page" {
			return "https://docs.google.com/document/d/zzz/edit", true
		}
		return "", false
	}

	reqs, err := buildWriteRequests(2, md, resolver)
	if err != nil {
		t.Fatalf("buildWriteRequests: %v", err)
	}

	var hasResolvedLink bool
	for _, r := range reqs {
		if r.UpdateTextStyle != nil && r.UpdateTextStyle.TextStyle != nil &&
			r.UpdateTextStyle.TextStyle.Link != nil &&
			r.UpdateTextStyle.TextStyle.Link.Url == "https://docs.google.com/document/d/zzz/edit" {
			hasResolvedLink = true
		}
	}
	if !hasResolvedLink {
		t.Error("expected resolved wikilink URL in write requests")
	}
}

func TestWikiResolver_BareNameViaTreeWalk(t *testing.T) {
	h := &DriveHandler{
		rootID:    "root",
		pathCache: make(map[string]*pathEntry),
	}
	tree := map[string][]*drive.File{
		"root": {{Id: "doc-bare", Name: "Page", MimeType: MimeDoc}},
	}

	resolve := h.wikiResolverWith(context.Background(), fakeLister(tree))
	url, ok := resolve("Page")
	if !ok || url != "https://docs.google.com/document/d/doc-bare/edit" {
		t.Errorf("resolve = (%q, %v), want resolved doc-bare URL", url, ok)
	}

	if _, ok := resolve("Nope"); ok {
		t.Error("unknown bare name should not resolve")
	}
}

func TestWikiResolver_MemoizesTreeWalk(t *testing.T) {
	h := &DriveHandler{
		rootID:    "root",
		pathCache: make(map[string]*pathEntry),
	}
	var listCalls int
	counting := func(_ context.Context, folderID string) ([]*drive.File, error) {
		listCalls++
		if folderID == "root" {
			return []*drive.File{{Id: "doc-bare", Name: "Page", MimeType: MimeDoc}}, nil
		}
		return nil, nil
	}

	resolve := h.wikiResolverWith(context.Background(), counting)

	// Repeated resolutions of the same hit walk the tree only once.
	if url, ok := resolve("Page"); !ok || url != "https://docs.google.com/document/d/doc-bare/edit" {
		t.Fatalf("resolve = (%q, %v), want resolved doc-bare URL", url, ok)
	}
	resolve("Page")
	if listCalls != 1 {
		t.Errorf("hit walked %d times, want 1 (memoized)", listCalls)
	}

	// Misses are memoized too: the second lookup does not re-walk.
	before := listCalls
	if _, ok := resolve("Missing"); ok {
		t.Fatal("unknown target should not resolve")
	}
	resolve("Missing")
	if listCalls != before+1 {
		t.Errorf("miss walked %d times, want 1 (memoized)", listCalls-before)
	}
}

func TestWikiResolver_PathTargetFromCache(t *testing.T) {
	h := &DriveHandler{
		rootID:    "root",
		pathCache: make(map[string]*pathEntry),
	}
	h.pathCache["/"] = &pathEntry{fileID: "root", mimeType: MimeFolder}
	h.pathCache["/subfolder/Page Name.md"] = &pathEntry{
		fileID:   "doc-7",
		mimeType: MimeDoc,
		parentID: "sub",
		name:     "Page Name",
	}

	resolve := h.wikiResolver(context.Background())
	url, ok := resolve("subfolder/Page Name")
	if !ok {
		t.Fatal("expected path target to resolve")
	}
	if url != "https://docs.google.com/document/d/doc-7/edit" {
		t.Errorf("url = %q", url)
	}
}

func TestWikiResolver_PathTargetNonDoc(t *testing.T) {
	h := &DriveHandler{
		rootID:    "root",
		pathCache: make(map[string]*pathEntry),
	}
	h.pathCache["/"] = &pathEntry{fileID: "root", mimeType: MimeFolder}
	// A path target that resolves to a non-Doc (e.g. a nested folder).
	h.pathCache["/dir/inner.md"] = &pathEntry{
		fileID:   "f1",
		mimeType: MimeFolder,
		name:     "inner",
	}

	resolve := h.wikiResolver(context.Background())
	if _, ok := resolve("dir/inner"); ok {
		t.Error("a path target resolving to a non-Doc must not resolve")
	}
}

func TestFindDocByName_ListerError(t *testing.T) {
	failing := func(_ context.Context, _ string) ([]*drive.File, error) {
		return nil, context.Canceled
	}
	if _, ok := findDocByName(context.Background(), failing, "root", "Page"); ok {
		t.Error("expected no match when the lister errors")
	}
}

// fakeLister returns a folderLister backed by a static folderID -> files map.
func fakeLister(tree map[string][]*drive.File) folderLister {
	return func(_ context.Context, folderID string) ([]*drive.File, error) {
		return tree[folderID], nil
	}
}

func TestFindDocByName_TopLevel(t *testing.T) {
	tree := map[string][]*drive.File{
		"root": {
			{Id: "d1", Name: "Page Name", MimeType: MimeDoc},
			{Id: "d2", Name: "Other", MimeType: MimeDoc},
		},
	}
	id, ok := findDocByName(context.Background(), fakeLister(tree), "root", "Page Name")
	if !ok || id != "d1" {
		t.Errorf("findDocByName = (%q, %v), want (d1, true)", id, ok)
	}
}

func TestFindDocByName_Nested(t *testing.T) {
	tree := map[string][]*drive.File{
		"root": {
			{Id: "sub", Name: "subfolder", MimeType: MimeFolder},
		},
		"sub": {
			{Id: "d9", Name: "Deep Page", MimeType: MimeDoc},
		},
	}
	id, ok := findDocByName(context.Background(), fakeLister(tree), "root", "Deep Page")
	if !ok || id != "d9" {
		t.Errorf("findDocByName = (%q, %v), want (d9, true)", id, ok)
	}
}

func TestFindDocByName_NotFound(t *testing.T) {
	tree := map[string][]*drive.File{
		"root": {{Id: "d1", Name: "Page Name", MimeType: MimeDoc}},
	}
	if _, ok := findDocByName(context.Background(), fakeLister(tree), "root", "Missing"); ok {
		t.Error("expected not found")
	}
}

func TestFindDocByName_IgnoresNonDocs(t *testing.T) {
	// A folder or PDF with the same name must not match.
	tree := map[string][]*drive.File{
		"root": {
			{Id: "f1", Name: "Report", MimeType: MimeFolder},
			{Id: "p1", Name: "Report", MimeType: MimePDF},
		},
	}
	if _, ok := findDocByName(context.Background(), fakeLister(tree), "root", "Report"); ok {
		t.Error("non-Doc match should not resolve")
	}
}
