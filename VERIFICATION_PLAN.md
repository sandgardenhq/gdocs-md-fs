# Verification Plan

## Prerequisites

- **Google Cloud project** with OAuth 2.0 credentials (Desktop application type)
- **Google Drive API** and **Google Docs API** enabled
- Credentials saved to `~/.config/gdocs-md-fs/credentials.json`
- **Test Google Drive folder** containing:
  - At least 2 Google Docs with mixed formatting (headings, bold, italic, lists, tables, code blocks)
  - 1 PDF file
  - 1 image file (PNG or JPG)
  - 1 other binary file (e.g., .zip)
  - 1 subfolder with at least 1 Google Doc inside
- **FUSE support** available on the test machine (macOS native or Linux libfuse)
- `gdocs-md-fs` binary built successfully (`go build -o gdocs-md-fs ./cmd/gdocs-md-fs`)
- `golangci-lint` installed

**Test folder ID**: _[TODO: fill in after creating test folder]_

## Scenarios

### Scenario 1: Authentication

**Context**: No token exists yet (`~/.config/gdocs-md-fs/token.json` does not exist).

**Steps**:
1. Run `gdocs-md-fs auth`
2. Complete the OAuth flow in the browser
3. Check that `~/.config/gdocs-md-fs/token.json` was created

**Success Criteria**:
- [ ] OAuth consent screen opens in browser
- [ ] Token file is created with 0600 permissions
- [ ] Token file contains valid JSON with `access_token` and `refresh_token`

**If Blocked**: If OAuth flow fails, check that credentials.json has the correct client ID/secret and that redirect URI matches.

### Scenario 2: Mount and Read Google Docs

**Context**: Authenticated (token exists). Test Drive folder has Google Docs with mixed formatting.

**Steps**:
1. Create a mountpoint: `mkdir -p /tmp/gdocs-test`
2. Run `gdocs-md-fs mount <test-folder-id> /tmp/gdocs-test`
3. List the mounted directory: `ls /tmp/gdocs-test/`
4. Read a Google Doc: `cat /tmp/gdocs-test/<doc-name>.md`
5. Verify markdown output contains expected formatting

**Success Criteria**:
- [ ] Directory listing shows Google Docs as `.md` files
- [ ] Directory listing shows subfolders as directories
- [ ] Directory listing shows PDFs, images, and other files with original extensions
- [ ] Reading a Google Doc returns valid markdown
- [ ] Headings, bold, italic, lists, links, and tables are correctly converted
- [ ] Code blocks (monospace paragraphs) are wrapped in fenced code blocks

**If Blocked**: If mount fails, check FUSE availability (`ls /dev/fuse` on Linux or check macFUSE on macOS). Check verbose output with `--verbose`.

### Scenario 3: Write Round-Trip

**Context**: Filesystem is mounted. A Google Doc is readable as markdown.

**Steps**:
1. Read the original Doc: `cat /tmp/gdocs-test/<doc-name>.md > /tmp/original.md`
2. Append a new section: `echo -e "\n## Verification Test\n\nThis section was added by gdocs-md-fs." >> /tmp/gdocs-test/<doc-name>.md`
3. Wait a moment, then re-read: `cat /tmp/gdocs-test/<doc-name>.md`
4. Open the Google Doc in a browser and verify the new section appears
5. Remove the test section from the Google Doc in the browser
6. Wait for cache to expire (60s), then re-read the file

**Success Criteria**:
- [ ] Write completes without error
- [ ] Re-reading the file shows the new section
- [ ] Google Doc in browser contains the new "Verification Test" heading and paragraph
- [ ] After removing the section in browser and cache expiry, local read reflects the change

**If Blocked**: If write fails, check error output. Verify the OAuth token has `documents` scope.

### Scenario 4: File Operations (Create, Delete, Rename)

**Context**: Filesystem is mounted.

**Steps**:
1. Create a new Google Doc: `echo "# Test Doc" > /tmp/gdocs-test/test-create.md`
2. Verify it appears in Google Drive (browser)
3. Create a new folder: `mkdir /tmp/gdocs-test/test-folder`
4. Verify the folder appears in Google Drive
5. Rename the doc: `mv /tmp/gdocs-test/test-create.md /tmp/gdocs-test/renamed-doc.md`
6. Verify the rename in Google Drive
7. Delete the doc: `rm /tmp/gdocs-test/renamed-doc.md`
8. Verify it moved to Drive trash
9. Delete the folder: `rmdir /tmp/gdocs-test/test-folder`
10. Verify it moved to Drive trash

**Success Criteria**:
- [ ] New .md file creates a Google Doc in Drive
- [ ] New directory creates a folder in Drive
- [ ] Rename updates the file name in Drive
- [ ] Delete moves file to Drive trash
- [ ] Folder delete moves folder to Drive trash

**If Blocked**: If create fails, check that the OAuth token has `drive.file` scope.

### Scenario 5: Non-Doc File Handling

**Context**: Filesystem is mounted. Test folder contains a PDF, an image, and a binary file.

**Steps**:
1. Read a PDF: `cp /tmp/gdocs-test/<file>.pdf /tmp/test-download.pdf`
2. Verify the PDF is valid: `file /tmp/test-download.pdf`
3. Read an image: `cp /tmp/gdocs-test/<file>.png /tmp/test-download.png`
4. Verify the image is valid: `file /tmp/test-download.png`
5. Attempt to write to the PDF: `echo "test" >> /tmp/gdocs-test/<file>.pdf`

**Success Criteria**:
- [ ] PDF downloads as valid PDF binary
- [ ] Image downloads as valid image binary
- [ ] Write to non-Doc file fails with an I/O error (not silently ignored)

**If Blocked**: If binary downloads are corrupted, check that the Drive API download is not applying any export conversion.

### Scenario 6: Cache Performance

**Context**: Filesystem is mounted. A Google Doc has been read at least once.

**Steps**:
1. Read a file to prime the cache: `cat /tmp/gdocs-test/<doc-name>.md > /dev/null`
2. Time a cached read: `time cat /tmp/gdocs-test/<doc-name>.md > /dev/null`
3. Repeat the timed read 5 times
4. Wait 60s (content cache TTL), then time a read again

**Success Criteria**:
- [ ] Cached reads complete in under 200ms
- [ ] After cache expiry, next read is slower (fetches from Drive)
- [ ] After the slow read, subsequent reads are fast again

**If Blocked**: If reads are consistently slow, check that the cache is being populated (enable `--verbose` logging).

### Scenario 7: Error Handling

**Context**: Filesystem is mounted and working.

**Steps**:
1. Disconnect from the network (turn off Wi-Fi or similar)
2. Attempt to read a file not in cache: `cat /tmp/gdocs-test/<uncached-file>.md`
3. Attempt to read a file that IS in cache: `cat /tmp/gdocs-test/<cached-file>.md`
4. Reconnect to the network
5. Read a file again to confirm recovery

**Success Criteria**:
- [ ] Uncached read returns an I/O error (not a crash or hang)
- [ ] Cached read still succeeds from cache
- [ ] After reconnecting, reads resume normally
- [ ] No data corruption or filesystem crash

**If Blocked**: If the FUSE mount crashes, check signal handling and unmount gracefully before retesting.

## Verification Rules

- Never use mocks or fakes
- All scenarios use real Google Drive with real API calls
- If any success criterion fails, verification fails
- Ask developer for help if blocked, don't guess
