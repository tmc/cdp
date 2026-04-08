# Testing Guide

This repository uses ordinary `go test` workflows. Some packages run entirely in-process. Others require a local Chrome or Chromium binary and skip automatically when the browser is unavailable or when tests run in short mode.

## Quick Start

```bash
# Fast pass
go test -short ./...

# Full repository test run
go test ./...

# Focus on the main browser-facing packages
go test ./cmd/... ./internal/browser/...
```

## Test Categories

### Pure Go tests

These do not require a local browser. They cover parsers, helpers, filtering, diff logic, and other in-process code.

```bash
go test -short ./...
```

### Browser-dependent tests

These launch or connect to Chrome and exercise real browser flows.

```bash
go test ./cmd/churl/... ./internal/browser/...
go test ./cmd/cdp/...
```

If Chrome is not installed or discoverable, many of these tests call `testutil.SkipIfNoChrome(t)` and skip.

## Running Specific Tests

```bash
# List tests in a package
go test -list . ./cmd/churl

# Run one test
go test -run TestBasicRun ./cmd/churl

# Run with verbose output
go test -v ./cmd/churl

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Chrome Test Helpers

Browser-facing tests share helpers in `internal/testutil/chrome.go`.

Useful helpers:

- `testutil.SkipIfNoChrome(t)`: skip when Chrome is unavailable or `go test -short` is in use
- `testutil.MustStartChrome(t, ctx, headless)`: launch Chrome for a test and fail immediately on setup errors
- `testutil.TestServer(t, handler)`: start a local HTTP server for integration tests

Example:

```go
func TestWithChrome(t *testing.T) {
	testutil.SkipIfNoChrome(t)

	ctx := context.Background()
	chromeCtx, cancel := testutil.MustStartChrome(t, ctx, true)
	defer cancel()

	_ = chromeCtx
}
```

## Environment

Recognized environment variables include:

- `CHROME_PATH`: explicit path to a Chrome or Chromium executable
- `CI`: commonly used to signal non-interactive test environments

In practice, the most important control is whether Chrome is installed and discoverable.

## Common Failure Modes

### Chrome not found

Install Chrome, Chromium, or another supported Chromium-based browser, or set `CHROME_PATH`.

```bash
export CHROME_PATH="/path/to/chrome"
go test ./cmd/churl/...
```

### Slow or flaky browser startup

Increase the test timeout for broader package runs:

```bash
go test -timeout 30m ./...
```

### Need a fast pre-commit pass

Use short mode:

```bash
go test -short ./...
```

## Notes

- There is no repository `Makefile` or Docker-based test harness in the current tree.
- The old GitHub Actions-specific instructions that used to live here were stale and have been removed.
