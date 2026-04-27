#!/usr/bin/env bash
set -euo pipefail

# Test script for FUSE write operations.
# Verifies that writes actually change content, not just return code 0.
# Usage: ./scripts/test-write.sh <folder-id> [mountpoint]

FOLDER_ID="${1:?Usage: $0 <folder-id> [mountpoint]}"
MOUNTPOINT="${2:-/tmp/gdocs-test}"
BINARY="$(cd "$(dirname "$0")/.." && pwd)/gdocs-md-fs"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}PASS${NC}: $1"; }
fail() { echo -e "${RED}FAIL${NC}: $1"; FAILURES=$((FAILURES + 1)); }
info() { echo -e "${YELLOW}INFO${NC}: $1"; }

# assert_content FILE NEEDLE DESCRIPTION
# Reads FILE via cat, checks that NEEDLE appears in the output.
assert_content() {
    local file="$1" needle="$2" desc="$3"
    local got
    got=$(cat "$file" 2>&1) || { fail "$desc — cat failed: $got"; return; }
    if echo "$got" | grep -qF "$needle"; then
        pass "$desc"
    else
        fail "$desc — expected to find '$needle' in content, got: ${got:0:300}"
    fi
}

# assert_no_error CMD... DESCRIPTION
# Runs CMD, fails if exit code is non-zero.
assert_no_error() {
    local desc="${!#}"  # last argument
    local cmd=("${@:1:$#-1}")  # all but last
    if "${cmd[@]}" 2>&1; then
        pass "$desc"
    else
        fail "$desc — command failed with rc=$?"
    fi
}

FAILURES=0

cleanup() {
    info "Cleaning up..."
    if mount | grep -q "$MOUNTPOINT"; then
        umount "$MOUNTPOINT" 2>/dev/null || diskutil unmount force "$MOUNTPOINT" 2>/dev/null || true
    fi
    if [[ -n "${MOUNT_PID:-}" ]]; then
        kill "$MOUNT_PID" 2>/dev/null || true
        wait "$MOUNT_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# Build
info "Building binary..."
(cd "$(dirname "$0")/.." && go build -o gdocs-md-fs ./cmd/gdocs-md-fs)

if [[ ! -x "$BINARY" ]]; then
    echo "Binary not found at $BINARY"
    exit 1
fi

# Mount
mkdir -p "$MOUNTPOINT"
info "Mounting $FOLDER_ID at $MOUNTPOINT..."
"$BINARY" mount "$FOLDER_ID" "$MOUNTPOINT" &
MOUNT_PID=$!

info "Waiting for mount..."
for _ in $(seq 1 30); do
    if mount | grep -q "$MOUNTPOINT"; then break; fi
    if ! kill -0 "$MOUNT_PID" 2>/dev/null; then
        echo "Mount process died. Check credentials."
        exit 1
    fi
    sleep 0.5
done

if ! mount | grep -q "$MOUNTPOINT"; then
    echo "Mount did not appear after 15 seconds"
    exit 1
fi
pass "Filesystem mounted"
sleep 1

# List files
info "Listing mounted directory..."
ls -la "$MOUNTPOINT/"
echo ""

# Use a unique token so we can verify OUR write, not stale content.
TOKEN="gdocstest-$$-$(date +%s)"

# --- Test 1: Create new file and verify content ---
info "--- Test 1: Create new file, verify content round-trips ---"
NEW_FILE="$MOUNTPOINT/${TOKEN}-create.md"
EXPECTED_1="# Created $TOKEN"
echo "$EXPECTED_1" > "$NEW_FILE" 2>&1 || fail "Test 1 — echo > failed"
sleep 2
assert_content "$NEW_FILE" "$TOKEN" "Test 1 — new file content matches what was written"
echo ""

# --- Test 2: Overwrite existing file with > (O_TRUNC) ---
info "--- Test 2: Overwrite file with > (verifies O_TRUNC + write + flush) ---"
EXPECTED_2="Overwritten $TOKEN"
echo "$EXPECTED_2" > "$NEW_FILE" 2>&1 || fail "Test 2 — echo > failed"
sleep 2
assert_content "$NEW_FILE" "Overwritten $TOKEN" "Test 2 — overwrite content matches"
# Also verify old content is gone:
GOT2=$(cat "$NEW_FILE" 2>&1) || true
if echo "$GOT2" | grep -qF "# Created"; then
    fail "Test 2 — old content still present after overwrite"
else
    pass "Test 2 — old content gone after overwrite"
fi
echo ""

# --- Test 3: Append with >> ---
info "--- Test 3: Append with >> ---"
APPEND_TOKEN="appended-$TOKEN"
echo "$APPEND_TOKEN" >> "$NEW_FILE" 2>&1 || fail "Test 3 — echo >> failed"
sleep 2
assert_content "$NEW_FILE" "Overwritten $TOKEN" "Test 3 — original content still present after append"
assert_content "$NEW_FILE" "$APPEND_TOKEN" "Test 3 — appended content present"
echo ""

# --- Test 4: Multi-line markdown write ---
info "--- Test 4: Multi-line markdown write ---"
MULTI_FILE="$MOUNTPOINT/${TOKEN}-multi.md"
cat > "$MULTI_FILE" <<EOF
# Multi $TOKEN

Paragraph with **bold** and *italic*.

## Section Two

- item one
- item two
EOF
sleep 2
assert_content "$MULTI_FILE" "Multi $TOKEN" "Test 4 — heading present"
assert_content "$MULTI_FILE" "item one" "Test 4 — list items present"
echo ""

# --- Test 5: Python write with explicit flags ---
info "--- Test 5: Python write with explicit O_WRONLY|O_CREAT|O_TRUNC ---"
PY_FILE="$MOUNTPOINT/${TOKEN}-python.md"
PY_CONTENT="# Python $TOKEN"
python3 -c "
import os
fd = os.open('$PY_FILE', os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)
os.write(fd, b'$PY_CONTENT\n')
os.close(fd)
" 2>&1 || fail "Test 5 — python write failed"
sleep 2
assert_content "$PY_FILE" "Python $TOKEN" "Test 5 — python-written content round-trips"
echo ""

# --- Test 6: Verify no echo write errors ---
info "--- Test 6: echo > must not produce 'write error' ---"
ERR_FILE="$MOUNTPOINT/${TOKEN}-errs.md"
ERR_OUTPUT=$(echo "# Error test $TOKEN" > "$ERR_FILE" 2>&1) || true
if echo "$ERR_OUTPUT" | grep -qi "write error\|input/output error"; then
    fail "Test 6 — echo produced error: $ERR_OUTPUT"
else
    pass "Test 6 — no write errors from echo"
fi
sleep 2
assert_content "$ERR_FILE" "Error test $TOKEN" "Test 6 — content written despite any warnings"
echo ""

# --- Cleanup test files ---
info "Cleaning up test files..."
rm -f "$MOUNTPOINT"/${TOKEN}-*.md 2>/dev/null || true

# --- Summary ---
echo ""
echo "=============================="
if [[ $FAILURES -eq 0 ]]; then
    echo -e "${GREEN}All tests passed!${NC}"
else
    echo -e "${RED}$FAILURES test(s) failed${NC}"
fi
echo "=============================="

exit $FAILURES
