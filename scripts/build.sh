#!/usr/bin/env bash
set -euo pipefail

# build.sh — Build the gdocs-md-fs binary with embedded version information.
#
# Usage:
#   ./scripts/build.sh [VERSION]
#
# Environment variables:
#   GOOS    — Target operating system (e.g. linux, darwin, windows)
#   GOARCH  — Target architecture (e.g. amd64, arm64)
#
# If VERSION is not provided, it is derived from the latest git tag.

# ---------------------------------------------------------------------------
# Navigate to project root (parent of the scripts/ directory)
# ---------------------------------------------------------------------------
cd "$(dirname "$0")/.."

# ---------------------------------------------------------------------------
# Resolve version metadata
# ---------------------------------------------------------------------------
VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "none")"
DATE="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

# ---------------------------------------------------------------------------
# Output binary path
# ---------------------------------------------------------------------------
OUTPUT="bin/gdocs-md-fs"

# ---------------------------------------------------------------------------
# Linker flags — embed version info and strip debug symbols
# ---------------------------------------------------------------------------
MODULE="github.com/sandgardenhq/md-to-gdocs/internal/cli"
LDFLAGS="-s -w"
LDFLAGS="${LDFLAGS} -X ${MODULE}.Version=${VERSION}"
LDFLAGS="${LDFLAGS} -X ${MODULE}.Commit=${COMMIT}"
LDFLAGS="${LDFLAGS} -X ${MODULE}.Date=${DATE}"

# ---------------------------------------------------------------------------
# Ensure the output directory exists
# ---------------------------------------------------------------------------
mkdir -p bin

# ---------------------------------------------------------------------------
# Print build summary
# ---------------------------------------------------------------------------
echo "Building gdocs-md-fs"
echo "  Version : ${VERSION}"
echo "  Commit  : ${COMMIT}"
echo "  Date    : ${DATE}"
echo "  Output  : ${OUTPUT}"
if [[ -n "${GOOS:-}" ]]; then
    echo "  GOOS    : ${GOOS}"
fi
if [[ -n "${GOARCH:-}" ]]; then
    echo "  GOARCH  : ${GOARCH}"
fi
echo ""

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
go build -ldflags "${LDFLAGS}" -o "${OUTPUT}" ./cmd/gdocs-md-fs

echo "Build complete: ${OUTPUT}"
