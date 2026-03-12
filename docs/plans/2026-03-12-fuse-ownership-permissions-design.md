# FUSE File Ownership and Permissions

## Problem

Mounted files are owned by root:wheel with mode 0600. Files should be owned by the current user. Google Docs (.md) should be readable and writable. Non-doc files (PDFs, images) should be readable but not writable.

## Design

### Ownership: capture UID/GID at mount time

`NewServer` calls `os.Getuid()` and `os.Getgid()` once and stores them as `uint32` fields on the `Server` struct. `Mount()` passes these to the root `Dir` node. Both `Dir` and `File` gain unexported `uid` and `gid uint32` fields. Parent nodes propagate their UID/GID when creating children (in `childInode`, `Create`, `Mkdir`).

`fillAttrOut` gains two new parameters (`uid, gid uint32`) and sets `a.Uid` and `a.Gid`. Every call site passes the node's UID/GID through.

### Permissions: MimeType-aware modes

`fillAttrOut` currently picks mode based only on `IsDir`. We add a third case using `MimeType`:

| Type | Mode | Rationale |
|---|---|---|
| Directories | `0755` (rwxr-xr-x) | User can list and traverse |
| Google Docs (MimeDoc) | `0644` (rw-r--r--) | Readable + writable by owner |
| Non-doc files | `0444` (r--r--r--) | Read-only; writes not supported |

The same logic applies in:
- `fillAttrOut` (central function)
- `Dir.Getattr` (already 0755, no change)
- `File.Getattr` fallback path (line 221, needs MimeType check)
- `newDirStream` (directory listing entries — already has access to `Entry.MimeType`)

A helper `isWritableFile(mimeType string) bool` in `node.go` checks for the Google Docs MIME type, avoiding an import of the `gdrive` package from `ragfs`.

### Testing

New file: `internal/ragfs/node_test.go`.

Test cases for `fillAttrOut`:
- Entry with MimeDoc -> mode 0644, correct Uid/Gid
- Entry with PDF MIME -> mode 0444, correct Uid/Gid
- Entry with IsDir=true -> S_IFDIR|0755, correct Uid/Gid
- Nil entry -> no panic

Test cases for `newDirStream`:
- Mixed entries (doc, non-doc, dir) -> correct modes in DirEntry list

## Files to modify

- `internal/ragfs/ragfs.go` — add uid/gid to Server, pass to root Dir
- `internal/ragfs/node.go` — add uid/gid to Dir/File, update fillAttrOut signature, add isWritableFile helper, update all call sites
- `internal/ragfs/node_test.go` — new test file

## Out of scope

- Testing full FUSE mount behavior (covered by VERIFICATION_PLAN.md)
- Changing the `Entry.Mode` field to be used (currently dead code, separate cleanup)
