## Project Overview

**gdocs-md**: Mount Google Drive as a local FUSE filesystem where Google Docs appear as editable Markdown files.

### Problem

Editing Google Docs requires a browser or the Google Docs app. There's no way to use your preferred local editor (vim, VS Code, etc.) to work with Google Docs content, and no way to integrate Google Docs into file-based workflows like grep, sed, or version control.

### Approach

A FUSE filesystem that presents a Google Drive folder as a local directory. Google Docs are converted to/from Markdown on read/write. Other file types (PDFs, images) pass through as read-only binary downloads. An in-memory cache keeps reads fast.

## Tech Stack

- **Language**: Go (modules)
- **Runtime**: CLI / FUSE filesystem
- **Package Manager**: Go modules
- **Testing**: `go test` (standard library)
- **Linting**: `golangci-lint`
- **Build**: `go build` / `scripts/build.sh`
- **Key Libraries**: `hanwen/go-fuse/v2`, `google.golang.org/api` (Drive + Docs), `spf13/cobra`, `yuin/goldmark`, `hashicorp/golang-lru/v2`, `golang.org/x/oauth2`

## Commands

```bash
# Run tests
go test ./...

# Run tests with race detector
go test -v -race ./...

# Check coverage
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

# Build
go build -o gdocs-md ./cmd/gdocs-md

# Lint
golangci-lint run ./...
```

## ABSOLUTE RULES - NO EXCEPTIONS

### 1. Test-Driven Development is MANDATORY

**The Iron Law**: NO PRODUCTION CODE WITHOUT A FAILING TEST FIRST

Every single line of production code MUST follow this cycle:
1. **RED**: Write failing test FIRST
2. **Verify RED**: Run test, watch it fail for the RIGHT reason
3. **GREEN**: Write MINIMAL code to pass the test
4. **Verify GREEN**: Run test, confirm it passes
5. **REFACTOR**: Clean up with tests staying green

### 2. Violations = Delete and Start Over

If ANY of these occur, you MUST delete the code and start over:
- Wrote production code before test → DELETE CODE, START OVER
- Test passed immediately → TEST IS WRONG, FIX TEST FIRST
- Can't explain why test failed → NOT TDD, START OVER
- "I'll add tests later" → DELETE CODE NOW
- "Just this once without tests" → NO. DELETE CODE.
- "It's too simple to test" → NO. TEST FIRST.
- "Tests after achieve same goal" → NO. DELETE CODE.

### 3. Test Coverage Requirements

- **Minimum 90%** coverage on ALL metrics:
  - Lines: 90%+
  - Functions: 90%+
  - Branches: 85%+
  - Statements: 90%+
- Coverage below threshold = Implementation incomplete
- Untested code = Code that shouldn't exist

### 4. Before Writing ANY Code

Ask yourself:
1. Did I write a failing test for this?
2. Did I run the test and see it fail?
3. Did it fail for the expected reason?

If ANY answer is "no" → STOP. Write the test first.

### 5. Test File Structure

For every production file, there MUST be a corresponding test file:
- `internal/converter/tomarkdown.go` → `internal/converter/converter_test.go`
- `internal/ragfs/cache.go` → `internal/ragfs/cache_test.go`
- `internal/gdrive/handler.go` → `internal/gdrive/handler_test.go`

### 6. Task Completion Requirements

NO TASK IS COMPLETE until:
- ALL tests pass (100% green)
- Build succeeds with ZERO errors
- NO linter errors or warnings (`golangci-lint run ./...`)
- Coverage meets minimum thresholds (90%+)

A task with failing tests, build errors, or linter warnings is INCOMPLETE.

### 7. Git Commits - Commit Early, Commit Often

- Commit after EACH successful TDD cycle (RED-GREEN-REFACTOR)
- Commit after each test file is created
- Commit after each module implementation
- Commit after fixing bugs or issues
- No more than 30 minutes without a commit
- Never have more than one feature in a single commit

### 8. Red Flags - STOP Immediately

If you catch yourself:
- Opening a code file before a test file
- Writing function implementation before test
- Thinking "I know this works"
- Copying code from examples without tests
- Skipping test runs
- Ignoring failing tests
- Writing multiple features before testing

**STOP. DELETE. START WITH TEST.**

## Git Commit Rules

**COMMIT EARLY, COMMIT OFTEN** - This is mandatory.

- Commit after every successful TDD cycle (RED-GREEN-REFACTOR)
- Commit after completing any discrete unit of work
- Commit before switching context or taking breaks
- Never have more than 30 minutes of uncommitted work
- Each commit should be atomic: one logical change per commit

## Git Workflow

- **Branching**: Feature branches off `main`
- **Branch naming**: `britt/<descriptive-name>` (concise, <30 chars)
- **PRs**: All work merges to `main` via pull request
- **CI**: Tests and golangci-lint run on PRs and block merge

## Verification

See @VERIFICATION_PLAN.md for acceptance testing procedures.
