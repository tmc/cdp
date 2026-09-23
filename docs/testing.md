---
title: Testing
description: Run the test suites in this repository, including browser-dependent and script fixture tests.
icon: vial
---

# Testing Guide

This repository uses ordinary `go test` workflows. Some packages run entirely in-process. Others require a local Chrome or Chromium binary and skip automatically when the browser is unavailable or when tests run in short mode.

## Quick Start

```bash
# Fast pass
go test -short ./...

# Full repository test run
go test -p 1 ./...

# Focus on the main browser-facing packages
go test -p 1 ./cmd/... ./internal/browser/...

# Browser script fixtures, which are behind a build tag
go test -tags cdp -p 1 -parallel 1 ./cdpscripttest
```

Use `-p 1` when several browser-driving packages run together. For the
`cdpscripttest` fixture package, also use `-parallel 1`: its `t.Parallel`
subtests launch Chrome concurrently, and `-p 1` serializes packages, not
subtests.

## Test Categories

### Pure Go tests

These do not require a local browser. They cover parsers, helpers, filtering, diff logic, and other in-process code.

```bash
go test -short ./...
```

### Browser-dependent tests

These launch or connect to Chrome and exercise real browser flows.

```bash
go test -p 1 ./cmd/churl/... ./internal/browser/...
go test -p 1 ./cmd/cdp/...
```

Browser-backed `cdpscripttest` fixtures are behind the `cdp` build tag:

```bash
go test -tags cdp -p 1 -parallel 1 ./cdpscripttest
```

If Chrome is not installed or discoverable, many of these tests call `testutil.SkipIfNoChrome(t)` and skip.

### Script fixtures

The `cdpscripttest` fixtures are txtar scripts rather than Go tests, and are
behind the `cdp` build tag so a default `go test ./...` does not need a
browser:

```bash
go test -tags cdp -p 1 -parallel 1 ./cdpscripttest
```

See [scripting.md](/docs/scripting) for writing them, and for the `cdpscripttest`
command's `-i` and `-watch` modes, which are a faster way to debug a failing
fixture than `go test` is.

### Documentation tests

Each command has a test asserting that every flag it registers, or every
subcommand it defines, appears in its `doc.go`. These run in-process and need
no browser:

```bash
go test ./cmd/... -run TestDocCovers
```

If you add a flag, this is the test that will tell you to document it.

## Running Specific Tests

```bash
# List tests in a package
go test -list . ./cmd/churl

# Run one test
go test -run TestChurl_ShowHelp ./cmd/churl

# Run with verbose output
go test -v ./cmd/churl

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Chrome Test Helpers

Browser-facing tests share helpers in `internal/testutil/chrome.go`.

Useful helpers:

- `testutil.SkipIfNoChrome(t)`: skip in `-short` mode, when `CI` or `SKIP_BROWSER_TESTS` is set, or when no Chromium-based browser is found
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

- `CHROME_EXECUTABLE_PATH`: explicit path to a Chrome or Chromium executable
- `CI`, `SKIP_BROWSER_TESTS`: skip browser-dependent tests

In practice, the most important control is whether Chrome is installed and discoverable.

## Common Failure Modes

### Chrome not found

Install Chrome, Chromium, or another supported Chromium-based browser, or set `CHROME_EXECUTABLE_PATH`.

```bash
export CHROME_EXECUTABLE_PATH="/path/to/chrome"
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

## Updating cdproto

The DevTools protocol changes with Chrome, and the generated `cdproto`
bindings follow it. Most of that lands safely: a command whose signature
changed, or that was removed, breaks the build. What does not break the build
is a call made by *raw string* rather than through a binding — it compiles,
ships, and fails at run time with `-32601 method not found` against every
browser and every version. Nothing catches it, forever.

`internal/cdpproto` closes that gap. `methods.txt` is a committed list of every
command and event the pinned `cdproto` declares, and two tests use it:

- `TestNoUnknownProtocolMethods` scans every non-test Go file for string
  literals shaped like a method name and fails on any the protocol does not
  have.
- `TestSnapshotIsCurrent` fails when `methods.txt` no longer matches the
  `cdproto` in `go.mod` — which is precisely the moment a dependency bump needs
  a human to look at it.

The update procedure:

```bash
go get -u github.com/chromedp/cdproto github.com/chromedp/chromedp
go generate ./internal/cdpproto
git diff internal/cdpproto/methods.txt
```

That diff is the point of the exercise. It lists exactly which commands and
events appeared and disappeared, and it is what drives the rest of the update:

1. **Removed names** — anything the diff deletes that the tree still calls will
   fail `TestNoUnknownProtocolMethods`. Delete the call; a removed method has no
   working fallback.
2. **Added names** — decide whether the new capability should be exposed as a
   `cdp` command, a `cdpscript` command, or not at all. Most additions are not
   worth surfacing.
3. **Docs and skills** — if step 2 changed a tool's surface, update the page
   under `docs/` and the matching `skills/*/SKILL.md` in the same commit. The
   doc-sync tests already couple these: a flag or command added without its
   documentation fails `internal/docscan`.

Then run the browser suite, because protocol changes surface as behavior
changes rather than compile errors:

```bash
go test -p 1 -parallel 1 -tags cdp ./cdpscripttest -count=1
```

Finally, a name absent from `methods.txt` is not always new. It may never have
existed, or it may have been removed. Either way the call fails at run time
with `-32601` against every browser, so the fix is to delete it, not to wait
for the protocol to catch up.

## Notes

- There is no repository `Makefile` or Docker-based test harness in the current tree.

This page is about running *this repository's* suite. To use `cdpscripttest`
from your own module — import path, what the dependency costs, and the
build-tag arrangement — see
[Adopting cdpscripttest](/docs/cdpscripttest/adopting).

## Next steps

- [cdpscripttest](/docs/cdpscripttest) — the testing package end to end.
- [cdpscript](/docs/scripting) — authoring the fixtures this runs.
- [Troubleshooting](/docs/troubleshooting) — parallelism and browser-discovery
  failures.
