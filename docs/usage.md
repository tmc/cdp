# Usage Guide

This repository currently has two top-level browser entry points:

- `chrome-to-har` at the module root, focused on HAR capture and differential capture.
- `cdp` in `cmd/cdp`, focused on direct Chrome DevTools Protocol automation.

Use `chrome-to-har` when you want a capture-oriented workflow. Use `cdp` when you want a general-purpose Chrome/CDP CLI.

## `chrome-to-har`

The root command launches Chrome, navigates to a page, and writes HAR or differential capture output.

```bash
chrome-to-har -url https://example.com -output output.har
```

Common flags:

- `-url`: starting URL to navigate to
- `-output`: output HAR path
- `-stream`: stream entries as NDJSON while capturing
- `-filter`: apply a jq filter to streamed entries
- `-template`: render streamed entries with a Go template
- `-profile`: use a Chrome profile directory
- `-wait-for`: wait for a CSS selector before capture completes
- `-wait-stable`: wait for network and DOM stability
- `-headless`: run Chrome headless
- `-list-profiles`: list available Chrome profiles

Examples:

```bash
# Basic capture
chrome-to-har -url https://example.com -output example.har

# Stream only API traffic
chrome-to-har -url https://example.com \
  -stream \
  -urls='api\.example\.com'

# Capture with a profile
chrome-to-har -profile "Default" \
  -url https://example.com \
  -output session.har

# Wait for an application shell before finishing
chrome-to-har -url https://app.example.com \
  -wait-for '#app-root' \
  -wait-stable \
  -output app.har
```

## `cdp`

The `cdp` command is the general Chrome/CDP tool in this repository.

```bash
cdp -url https://example.com -js 'document.title'
```

Common flags:

- `-url`: starting URL
- `-js`: JavaScript to evaluate
- `-har`: write HAR output
- `-screenshot`: capture a screenshot
- `-extract`: extract text or HTML from a selector
- `-interactive` or `-shell`: keep the browser open for manual or REPL-driven work
- `-headless`: run Chrome headless
- `-use-profile`: use an existing Chrome profile
- `-remote-host` and `-remote-port`: connect to a remote Chrome instance

Examples:

```bash
# Evaluate JavaScript
cdp -headless -url https://example.com -js 'document.title'

# Capture HAR and screenshot in one run
cdp -url https://example.com \
  -har capture.har \
  -screenshot full

# Render page content
cdp -url https://example.com -render body

# Connect to an existing Chrome debug port
cdp -debug-port 9222 -list-tabs
```

## Streaming and Filtering

`chrome-to-har` supports streaming entry output as NDJSON:

```jsonl
{"startedDateTime":"2024-01-01T00:00:00Z","request":{"method":"GET","url":"https://example.com"},"response":{"status":200}}
{"startedDateTime":"2024-01-01T00:00:01Z","request":{"method":"POST","url":"https://api.example.com"},"response":{"status":201}}
```

Examples:

```bash
# Only errors
chrome-to-har -stream -filter='select(.response.status >= 400)'

# Format a CSV-like summary
chrome-to-har -stream \
  -template='{{.request.method}},{{.request.url}},{{.response.status}},{{.time}}'

# Focus on authentication traffic
chrome-to-har -stream \
  -urls='auth|login|token'
```

## Differential Capture

The root command also exposes differential capture mode:

```bash
# Create a baseline capture
chrome-to-har -diff-mode -url https://example.com -capture-name baseline

# Compare two captures
chrome-to-har -baseline baseline -compare-with candidate -diff-output report.html
```

See [differential-capture.md](/Volumes/tmc/go/src/github.com/tmc/cdp/docs/differential-capture.md) for more detail.
