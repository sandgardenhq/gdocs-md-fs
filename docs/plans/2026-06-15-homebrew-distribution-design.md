# Homebrew Distribution — Design

**Date:** 2026-06-15
**Status:** Approved, ready for implementation plan

## Goal

Distribute `gdocs-md-fs` through a Homebrew tap so users can install it with:

```bash
brew install sandgardenhq/tap/gdocs-md-fs
```

Releases are fully automated by GoReleaser, triggered by pushing a `v*` git
tag. As a related fix, make `go install` work by correcting the module path.

## Constraints and key decisions

### homebrew-core is not an option

macFUSE is no longer open-source, and Homebrew refuses non-open-source
dependencies in `homebrew-core`. FUSE tools that depend on macFUSE (sshfs,
gocryptfs, encfs) were all evicted from core and now live in third-party taps.
Our only realistic route is a **custom tap we control**, with the formula
handling the macFUSE requirement itself.

### Reuse existing sandgardenhq release infrastructure

The sibling project `sandgardenhq/find-the-gaps` already releases to a tap. We
reuse its conventions:

- **Tap repo `sandgardenhq/homebrew-tap`** — already exists.
- **Actions secret `HOMEBREW_TAP_TOKEN`** — already exists; a cross-repo token
  with write access to the tap (the default `GITHUB_TOKEN` cannot push to a
  separate repo).
- **Build-provenance attestation** and a **post-publish `brew install`
  smoke-test job** — good patterns worth keeping.

We diverge from find-the-gaps in two ways:

- find-the-gaps needs `CGO_ENABLED=1` and therefore builds on native runners
  per target. `gdocs-md-fs` is pure Go (go-fuse v2 has no cgo), so all four
  targets cross-compile with `CGO_ENABLED=0` on a single Linux runner.
  *(Verified: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64 all build.)*
- find-the-gaps uses a hand-rolled workflow with an `.rb.tmpl` template. We use
  GoReleaser's declarative `brews:` block instead — its `dependencies` (with
  `os:` qualifiers) and `caveats:` fields cover everything we need, so no custom
  template is required.

### Platforms

darwin/amd64, darwin/arm64, linux/amd64, linux/arm64.

### macFUSE / libfuse handling

macFUSE is a system extension: installing it needs a sudo password, manual
approval in System Settings, and a reboot — none fully automatable by Homebrew.
So we split by OS in the formula:

- **Linux:** hard dependency — `on_linux { depends_on "libfuse" }`. libfuse is a
  normal package, so this is clean.
- **macOS:** **no hard dependency**; a caveat tells the user to install macFUSE
  themselves. This avoids hijacking the user's install with a reboot-requiring
  kernel extension (the approach used by `gromgit/homebrew-fuse`).

### First release

`v0.1.0` — no git tags exist today.

## Components

### 1. Module-path fix (prerequisite, standalone commit)

Rename the Go module `github.com/brittcrawford/gdocs-md-fs` →
`github.com/sandgardenhq/md-to-gdocs`. The old path does not match the actual
repo (`github.com/sandgardenhq/md-to-gdocs`), so `go install` currently fails.

Scope: 7 references across 6 files — `go.mod`, `internal/gdrive/handler.go`,
`internal/gdrive/docs.go`, `internal/cli/auth.go`, `internal/cli/mount.go`, and
the LDFLAGS `MODULE` var in `scripts/build.sh`.

After the rename:

```bash
go install github.com/sandgardenhq/md-to-gdocs/cmd/gdocs-md-fs@latest
```

works and produces a `gdocs-md-fs` binary (named from the `cmd/` directory).

Verify with `go build ./...`, `go vet ./...`, and the existing test suite green.

### 2. `.goreleaser.yaml` (new, repo root)

- **builds:** one build — `binary: gdocs-md-fs`, `main: ./cmd/gdocs-md-fs`,
  `env: [CGO_ENABLED=0]`, `flags: [-trimpath]`, ldflags stamping
  `internal/cli.Version`, `.Commit`, `.Date` (matching `scripts/build.sh`),
  `goos: [darwin, linux]` × `goarch: [amd64, arm64]`.
- **archives:** `tar.gz`, name `gdocs-md-fs_{{.Tag}}_{{.Os}}-{{.Arch}}`,
  bundling `LICENSE` and `README.md`.
- **checksum:** `checksums.txt`.
- **changelog:** `github-native`.
- **snapshot:** `version_template: "{{ incpatch .Version }}-snapshot"`.
- **brews:** repository `sandgardenhq/homebrew-tap`, `directory: Formula`,
  homepage `https://github.com/sandgardenhq/md-to-gdocs`, description,
  `license: MIT`, `install: bin.install "gdocs-md-fs"`,
  `test: system "#{bin}/gdocs-md-fs", "version"`, `dependencies:` with a
  libfuse entry qualified `os: linux`, and a `caveats:` string for the macOS
  macFUSE instructions.

### 3. `.github/workflows/release.yml` (new)

Triggered on `push: tags: ["v*"]`.

- **goreleaser job** (`ubuntu-latest`): checkout with full history (changelog),
  `setup-go` from `go.mod`, run `goreleaser/goreleaser-action`. Env:
  `GITHUB_TOKEN` (release upload) and `HOMEBREW_TAP_TOKEN` (formula push to the
  tap). Add `actions/attest-build-provenance` over the produced archives.
- **smoke-test job** (`needs: goreleaser`, `macos-latest`): run
  `brew install sandgardenhq/tap/gdocs-md-fs` and
  `gdocs-md-fs version | grep -F "$TAG"`.

### 4. README documentation

Add a **Homebrew** subsection to "Installation" (above "From source") with
`brew install sandgardenhq/tap/gdocs-md-fs` and the macFUSE note, plus a
`go install` one-liner.

## Verification

1. `goreleaser check` passes.
2. `goreleaser release --snapshot --clean` locally produces the four archives,
   `checksums.txt`, and a rendered formula under `dist/` — inspect the formula
   for correct deps and caveats.
3. Push tag `v0.1.0` → release workflow runs, GitHub Release is created, and the
   formula is committed to `sandgardenhq/homebrew-tap`.
4. The macOS smoke-test job goes green (`brew install` + version check).

## Out of scope

- Submitting to homebrew-core (blocked by macFUSE licensing).
- Windows builds (no FUSE story).
- Signing/notarizing the macOS binary (revisit if Gatekeeper friction appears).
