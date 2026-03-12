#!/usr/bin/env bash
set -euo pipefail

# install.sh — Install the gdocs-md binary to a directory on $PATH.
#
# Usage:
#   ./scripts/install.sh
#
# The script will:
#   1. Build the binary if it has not been built yet.
#   2. Copy it to $GOPATH/bin (preferred) or /usr/local/bin (fallback).
#   3. Verify the installation by running `gdocs-md version`.

# ---------------------------------------------------------------------------
# Navigate to project root (parent of the scripts/ directory)
# ---------------------------------------------------------------------------
cd "$(dirname "$0")/.."

# ---------------------------------------------------------------------------
# Ensure the binary exists — build it if necessary
# ---------------------------------------------------------------------------
if [[ ! -f "bin/gdocs-md" ]]; then
    echo "Binary not found at bin/gdocs-md, building first..."
    echo ""
    bash scripts/build.sh
    echo ""
fi

# ---------------------------------------------------------------------------
# Determine installation directory
# ---------------------------------------------------------------------------
INSTALL_DIR=""

if [[ -n "${GOPATH:-}" ]] && [[ -d "${GOPATH}/bin" ]]; then
    # Prefer $GOPATH/bin when available
    INSTALL_DIR="${GOPATH}/bin"
elif [[ -n "${GOPATH:-}" ]]; then
    # $GOPATH is set but bin/ doesn't exist yet — try to create it
    if mkdir -p "${GOPATH}/bin" 2>/dev/null; then
        INSTALL_DIR="${GOPATH}/bin"
    fi
fi

# Fallback to /usr/local/bin
if [[ -z "${INSTALL_DIR}" ]]; then
    INSTALL_DIR="/usr/local/bin"
fi

# ---------------------------------------------------------------------------
# Copy binary to the install directory
# ---------------------------------------------------------------------------
echo "Installing gdocs-md to ${INSTALL_DIR} ..."

if [[ -w "${INSTALL_DIR}" ]]; then
    cp bin/gdocs-md "${INSTALL_DIR}/gdocs-md"
else
    echo "(requires elevated privileges)"
    sudo cp bin/gdocs-md "${INSTALL_DIR}/gdocs-md"
fi

# ---------------------------------------------------------------------------
# Verify installation
# ---------------------------------------------------------------------------
if command -v gdocs-md &>/dev/null; then
    echo ""
    echo "Installed version:"
    gdocs-md version
else
    echo ""
    echo "Warning: gdocs-md was copied to ${INSTALL_DIR} but is not on your PATH."
    echo "Add ${INSTALL_DIR} to your PATH and try again."
fi

# ---------------------------------------------------------------------------
# Success
# ---------------------------------------------------------------------------
echo ""
echo "gdocs-md installed successfully to ${INSTALL_DIR}"
echo ""
echo "Quick start:"
echo "  1. gdocs-md auth           # Authenticate with Google Drive"
echo "  2. gdocs-md mount ID ~/drive  # Mount a Drive folder"
