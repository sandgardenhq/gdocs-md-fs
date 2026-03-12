---
flow: |
  "backend-go-developer" -> "go-readability-reviewer"
  "go-readability-reviewer" -> "stpa-analyst"
  "shell-script-coder" -> "shell-script-reviewer"
  "shell-script-reviewer" -> "stpa-analyst"
models:
  "coordinator": "anthropic/claude-opus-4-6 (max)"
  "backend-go-developer": "anthropic/claude-opus-4-6"
  "go-readability-reviewer": "anthropic/claude-opus-4-6"
  "shell-script-coder": "anthropic/claude-opus-4-6"
  "shell-script-reviewer": "anthropic/claude-opus-4-6"
  "stpa-analyst": "anthropic/claude-opus-4-6"
interactive: yes
completionGateScript: go test ./...
---

A Go CLI tool (`gdocs-md`) that mounts Google Drive as a local FUSE filesystem, presenting Google Docs as editable markdown files. Built on the `ragfs` framework which handles FUSE integration and caching.

See `Design.md` for full architecture, handler interface, and phased MVP scope.

## Success Criteria

- [ ] Users can authenticate with Google Drive via OAuth and have tokens stored securely in their config directory
- [ ] A Google Drive folder can be mounted as a local filesystem directory using `gdocs-md mount`
- [ ] Google Docs appear as `.md` files and their content is readable as markdown
- [ ] Editing and saving a `.md` file updates the corresponding Google Doc in Drive
- [ ] Deleting a mounted file removes the corresponding document from Google Drive
- [ ] Files can be moved or renamed within the mounted filesystem
- [ ] Repeated reads of the same document are served from cache with latency under 200ms
- [ ] Round-trip markdown conversion preserves at least 95% of document formatting
- [ ] PDFs and images are accessible as pass-through files without modification
- [ ] No data loss occurs during read/write/sync operations
