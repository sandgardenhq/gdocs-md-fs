# Homebrew Distribution Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Distribute `gdocs-md-fs` via a Homebrew tap (`brew install sandgardenhq/tap/gdocs-md-fs`) with fully automated GoReleaser releases on a `v*` tag, and fix the Go module path so `go install` works.

**Architecture:** A `v*` tag push triggers a GitHub Actions workflow that runs GoReleaser on a single `ubuntu-latest` runner. GoReleaser cross-compiles four targets (darwin/linux × amd64/arm64) with `CGO_ENABLED=0`, builds `tar.gz` archives + checksums, creates a GitHub Release, and pushes an updated formula to the existing `sandgardenhq/homebrew-tap` repo using the existing `HOMEBREW_TAP_TOKEN` secret. The formula declares a hard `libfuse` dependency on Linux and a macFUSE caveat on macOS. A post-publish smoke-test job installs from the tap on macOS.

**Tech Stack:** Go 1.25, GoReleaser v2, GitHub Actions, Homebrew (custom tap), go-fuse/v2 (pure Go, no cgo).

**Design reference:** `docs/plans/2026-06-15-homebrew-distribution-design.md`

**Pre-flight facts (already verified during design):**
- All four targets cross-compile with `CGO_ENABLED=0`.
- Tap repo `sandgardenhq/homebrew-tap` exists.
- Actions secret `HOMEBREW_TAP_TOKEN` exists (cross-repo write token).
- Old module path `github.com/brittcrawford/gdocs-md-fs` appears in 7 places across 6 files.
- Sibling reference implementation: `~/workspace/find-the-gaps` (`.goreleaser.yaml`, `.github/workflows/release.yml`).

**Prerequisites the human must confirm before tagging `v0.1.0`:**
- `goreleaser` CLI installed locally for the snapshot dry-run (`brew install goreleaser`).
- `HOMEBREW_TAP_TOKEN` is present in the `sandgardenhq/md-to-gdocs` repo's Actions secrets (it exists for find-the-gaps; confirm it is also set on this repo, or copy it).

---

## Task 1: Rename the Go module path

Fix the module path so it matches the real repo and `go install` works. This is a standalone commit with no behavior change — the existing test suite is the safety net.

**Files:**
- Modify: `go.mod:1` (module line)
- Modify: `internal/gdrive/handler.go` (import)
- Modify: `internal/gdrive/docs.go` (import)
- Modify: `internal/cli/auth.go` (import)
- Modify: `internal/cli/mount.go` (import)
- Modify: `scripts/build.sh` (LDFLAGS `MODULE` var)

**Step 1: Confirm the current reference set**

Run: `grep -rn "brittcrawford/gdocs-md-fs" .`
Expected: exactly 7 lines across the 6 files listed above. If the count differs, stop and re-scope.

**Step 2: Perform the rename**

Replace every occurrence of `github.com/brittcrawford/gdocs-md-fs` with `github.com/sandgardenhq/md-to-gdocs`.

```bash
grep -rl "brittcrawford/gdocs-md-fs" . \
  | xargs sed -i '' 's|github.com/brittcrawford/gdocs-md-fs|github.com/sandgardenhq/md-to-gdocs|g'
```

(Note: `sed -i ''` is the macOS/BSD form. On Linux use `sed -i`.)

**Step 3: Verify no references remain**

Run: `grep -rn "brittcrawford/gdocs-md-fs" .`
Expected: no output (exit code 1).

**Step 4: Tidy and build**

Run: `go mod tidy && go build ./...`
Expected: no errors. `go.mod` module line now reads `module github.com/sandgardenhq/md-to-gdocs`.

**Step 5: Run the full test suite (the safety net)**

Run: `go test ./...`
Expected: all packages PASS. A pure import-path rename must not change behavior; any failure here means a path was mis-edited.

**Step 6: Verify the version-stamp build path still works**

Run: `./scripts/build.sh v0.0.0-test && ./bin/gdocs-md-fs version`
Expected: build succeeds and `version` prints `v0.0.0-test` (confirms the renamed `MODULE` ldflags path in `build.sh` is correct).

**Step 7: Commit**

```bash
git add -A
git commit -m "Rename Go module path to github.com/sandgardenhq/md-to-gdocs

Aligns the module path with the actual repo so \`go install\` works.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: Add `.goreleaser.yaml`

**Files:**
- Create: `.goreleaser.yaml` (repo root)

**Step 1: Write the config**

Create `.goreleaser.yaml` with exactly this content:

```yaml
# goreleaser config for gdocs-md-fs.
# Releases are triggered by pushing a v* tag; see .github/workflows/release.yml.
version: 2

project_name: gdocs-md-fs

before:
  hooks:
    - go mod tidy

builds:
  - id: gdocs-md-fs
    binary: gdocs-md-fs
    main: ./cmd/gdocs-md-fs
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w
      - -X github.com/sandgardenhq/md-to-gdocs/internal/cli.Version={{ .Version }}
      - -X github.com/sandgardenhq/md-to-gdocs/internal/cli.Commit={{ .ShortCommit }}
      - -X github.com/sandgardenhq/md-to-gdocs/internal/cli.Date={{ .Date }}
    goos:
      - darwin
      - linux
    goarch:
      - amd64
      - arm64

archives:
  - id: gdocs-md-fs
    formats: [tar.gz]
    name_template: "gdocs-md-fs_{{ .Tag }}_{{ .Os }}-{{ .Arch }}"
    files:
      - LICENSE
      - README.md

checksum:
  name_template: checksums.txt

snapshot:
  version_template: "{{ incpatch .Version }}-snapshot"

changelog:
  use: github-native

brews:
  - name: gdocs-md-fs
    repository:
      owner: sandgardenhq
      name: homebrew-tap
    directory: Formula
    homepage: https://github.com/sandgardenhq/md-to-gdocs
    description: Mount Google Drive as a local FUSE filesystem with Google Docs as Markdown.
    license: MIT
    install: |
      bin.install "gdocs-md-fs"
    test: |
      system "#{bin}/gdocs-md-fs", "version"
    dependencies:
      - name: libfuse
        os: linux
    caveats: |
      gdocs-md-fs requires FUSE.

      On macOS, install macFUSE separately:
        brew install --cask macfuse
      Then approve the macFUSE system extension in System Settings > Privacy &
      Security and reboot. macFUSE is not installed automatically because it is a
      kernel/system extension that requires manual approval and a restart.
```

**Step 2: Verify the config is valid**

Run: `goreleaser check`
Expected: `1 configuration files validated` / no errors. If `goreleaser` is not installed: `brew install goreleaser` first.

**Step 3: Commit**

```bash
git add .goreleaser.yaml
git commit -m "Add GoReleaser config for cross-platform builds and Homebrew tap

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Validate the full release pipeline locally (snapshot dry-run)

No new files — this is a verification gate that proves Task 2's config produces correct artifacts before any tag is pushed.

**Step 1: Run a snapshot release**

Run: `goreleaser release --snapshot --clean`
Expected: completes successfully; `dist/` is populated.

**Step 2: Verify the four archives and checksums exist**

Run: `ls dist/*.tar.gz dist/checksums.txt`
Expected: four `tar.gz` files (`darwin-amd64`, `darwin-arm64`, `linux-amd64`, `linux-arm64`) plus `checksums.txt`.

**Step 3: Inspect the rendered formula**

Run: `cat dist/homebrew/Formula/gdocs-md-fs.rb` (path may be `dist/.../gdocs-md-fs.rb` — locate with `find dist -name 'gdocs-md-fs.rb'`).
Expected, confirm by eye:
- `class GdocsMdFs < Formula`
- four `url`/`sha256` blocks (or on_macos/on_arm/on_intel/on_linux split) pointing at `github.com/sandgardenhq/md-to-gdocs/releases/download/...`
- `depends_on "libfuse"` guarded under `on_linux`
- the macFUSE `caveats` text present
- `bin.install "gdocs-md-fs"` and the `version` test

**Step 4: Verify a built binary runs and is version-stamped**

Run: `find dist -name gdocs-md-fs -type f -path '*darwin*arm64*' -exec {} version \;` (adjust to your host arch; pick the matching dir).
Expected: prints a version string containing the snapshot version.

**Step 5: Clean up the dry-run output**

Run: `rm -rf dist`
(`dist/` is build output; confirm it is gitignored — if not, add it in Task 6.)

No commit (verification only). If anything failed, fix `.goreleaser.yaml` in Task 2 and re-run this task.

---

## Task 4: Add the release workflow

**Files:**
- Create: `.github/workflows/release.yml`

**Step 1: Write the workflow**

Create `.github/workflows/release.yml` with exactly this content:

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions: {}

jobs:
  goreleaser:
    name: Build and publish release
    runs-on: ubuntu-latest
    permissions:
      contents: write       # create the GitHub Release
      id-token: write        # build provenance attestation
      attestations: write    # build provenance attestation
    steps:
      - name: Checkout
        uses: actions/checkout@v6
        with:
          fetch-depth: 0     # full history for the changelog

      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}

      - name: Attest build provenance
        uses: actions/attest-build-provenance@v3
        with:
          subject-path: dist/*.tar.gz

  smoke-test-formula:
    name: Smoke-test published formula
    needs: goreleaser
    runs-on: macos-latest
    steps:
      - name: Install from tap
        run: brew install sandgardenhq/tap/gdocs-md-fs

      - name: Verify gdocs-md-fs
        env:
          VERSION: ${{ github.ref_name }}
        run: |
          gdocs-md-fs version
          gdocs-md-fs version | grep -F "${VERSION}"
```

**Note on the tap token:** GoReleaser's `brews` publisher reads the tap token
from `GITHUB_TOKEN` by default. To make it push to the *separate* tap repo we
must point it at `HOMEBREW_TAP_TOKEN`. GoReleaser reads the env var named by the
`github_token` setting, but the simplest reliable wiring is to set the env var
GoReleaser expects. **During Task 4, verify the exact mechanism** against the
GoReleaser v2 docs for `brews.repository.token` / the `GITHUB_TOKEN` override —
if `HOMEBREW_TAP_TOKEN` is not picked up automatically, add to `.goreleaser.yaml`
under the `brews[0].repository` block:

```yaml
    repository:
      owner: sandgardenhq
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
```

This token override is the canonical GoReleaser pattern for cross-repo tap
pushes; prefer it over relying on `GITHUB_TOKEN`. Add it now if in doubt — it is
harmless when the token is valid.

**Step 2: Lint the workflow YAML**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml')); print('valid yaml')"`
Expected: `valid yaml`.

**Step 3: Re-validate goreleaser config if the token block was added**

Run: `goreleaser check`
Expected: no errors.

**Step 4: Commit**

```bash
git add .github/workflows/release.yml .goreleaser.yaml
git commit -m "Add release workflow: GoReleaser + Homebrew tap publish

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: Update the README

**Files:**
- Modify: `README.md` (the "Installation" section, currently starting at the `## Installation` heading)

**Step 1: Add a Homebrew subsection as the first install option**

In `README.md`, directly under `## Installation` and **above** the existing
`### From source` subsection, insert:

```markdown
### Homebrew (recommended)

```bash
brew install sandgardenhq/tap/gdocs-md-fs
```

On **macOS**, gdocs-md-fs also needs macFUSE, which Homebrew cannot install
automatically (it is a system extension requiring manual approval and a reboot):

```bash
brew install --cask macfuse
```

Then approve the macFUSE system extension under **System Settings > Privacy &
Security** and reboot. On **Linux**, the required `libfuse` is installed
automatically as a formula dependency.

### go install

```bash
go install github.com/sandgardenhq/md-to-gdocs/cmd/gdocs-md-fs@latest
```
```

**Step 2: Reconcile the Requirements section**

The existing `## Requirements` section already mentions macFUSE/libfuse and "Go
1.25+ (for building from source)". Leave it; it is consistent with the new
section. Confirm no contradictory "clone <repo-url>" placeholder remains broken —
the `### From source` block uses `<repo-url>`; replace it with the real URL:

Change `git clone <repo-url>` to `git clone https://github.com/sandgardenhq/md-to-gdocs.git`
and the following `cd gdocs-md-fs` to `cd md-to-gdocs`.

**Step 3: Verify the rendered Markdown**

Run: `grep -n "sandgardenhq/tap/gdocs-md-fs\|go install github.com/sandgardenhq" README.md`
Expected: both the `brew install` line and the `go install` line are present.

**Step 4: Commit**

```bash
git add README.md
git commit -m "Document Homebrew and go install in README

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 6: Final repo hygiene and PR

**Files:**
- Modify (if needed): `.gitignore`

**Step 1: Ensure GoReleaser output is ignored**

Run: `grep -q '^dist/$\|^dist$\|^/dist' .gitignore && echo present || echo missing`
If `missing`, add `dist/` to `.gitignore`:

```bash
printf '\n# GoReleaser build output\ndist/\n' >> .gitignore
git add .gitignore
git commit -m "Ignore GoReleaser dist/ output

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

**Step 2: Full verification sweep**

Run each, expecting success:
- `go build ./...`
- `go test ./...`
- `golangci-lint run ./...`
- `goreleaser check`

**Step 3: Push the branch and open a PR**

```bash
git push -u origin HEAD
gh pr create --base main \
  --title "Package gdocs-md-fs for Homebrew distribution" \
  --body "$(cat <<'EOF'
Adds Homebrew distribution via the sandgardenhq/homebrew-tap, automated with GoReleaser.

## Changes
- Rename Go module path to \`github.com/sandgardenhq/md-to-gdocs\` so \`go install\` works
- Add \`.goreleaser.yaml\` (4 targets, CGO disabled, Homebrew formula with libfuse-on-Linux dep + macFUSE caveat on macOS)
- Add \`.github/workflows/release.yml\` (GoReleaser on \`v*\` tag + build provenance + macOS install smoke test)
- Document Homebrew and \`go install\` in the README

## Notes
- homebrew-core is not viable: macFUSE is not open-source, so the formula lives in our own tap.
- Verified all four targets cross-compile with \`CGO_ENABLED=0\` and that \`goreleaser release --snapshot\` renders a correct formula.

Design: docs/plans/2026-06-15-homebrew-distribution-design.md

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

**Step 4 (human, post-merge — the live release):**

After the PR merges to `main`:
1. Confirm `HOMEBREW_TAP_TOKEN` is set in this repo's Actions secrets.
2. Tag and push the first release:
   ```bash
   git checkout main && git pull
   git tag v0.1.0
   git push origin v0.1.0
   ```
3. Watch the `Release` workflow. Expected: GitHub Release with four archives +
   `checksums.txt`, a new commit on `sandgardenhq/homebrew-tap` adding
   `Formula/gdocs-md-fs.rb`, and the macOS smoke-test job green.
4. Final manual proof on a Mac:
   ```bash
   brew install sandgardenhq/tap/gdocs-md-fs
   gdocs-md-fs version   # prints v0.1.0
   ```

---

## Verification summary

| Gate | Command | When |
|---|---|---|
| Module rename safe | `go test ./...` | Task 1 |
| Config valid | `goreleaser check` | Tasks 2, 4 |
| Artifacts + formula correct | `goreleaser release --snapshot --clean` | Task 3 |
| Workflow YAML valid | `yaml.safe_load` | Task 4 |
| README documents install | `grep` | Task 5 |
| Whole repo green | `go build/test`, `golangci-lint`, `goreleaser check` | Task 6 |
| Live release works | tag `v0.1.0` → Release + tap commit + smoke test | Task 6 (human) |
