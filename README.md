# cdp

[![Go Reference](https://pkg.go.dev/badge/github.com/tmc/cdp.svg)](https://pkg.go.dev/github.com/tmc/cdp)

`cdp` is a Go module for Chrome DevTools Protocol automation. It includes shared internal packages plus several command-line tools for browser control, traffic capture, script execution, and Node/V8 debugging.

## Commands

- `chrome-to-har`: focused HAR capture CLI
- `cdp`: the main Chrome/CDP CLI
- `churl`: browser-backed fetch and page extraction
- `chdb`: Chrome-focused debugger workflow
- `ndp`: Node/V8 debugger workflow
- `cdpscript`: run CDP scripts
- `cdpscripttest`: test CDP scripts

## Install

The module requires Go 1.26 or newer.

Install the broad CDP CLI:

```bash
go install github.com/tmc/cdp/cmd/cdp@latest
```

Install individual tools:

```bash
go install github.com/tmc/cdp/cmd/chrome-to-har@latest
go install github.com/tmc/cdp/cmd/cdp@latest
go install github.com/tmc/cdp/cmd/churl@latest
go install github.com/tmc/cdp/cmd/chdb@latest
go install github.com/tmc/cdp/cmd/ndp@latest
go install github.com/tmc/cdp/cmd/cdpscript@latest
go install github.com/tmc/cdp/cmd/cdpscripttest@latest
```

## Quick Start

Capture network activity with `chrome-to-har`:

```bash
go install github.com/tmc/cdp/cmd/chrome-to-har@latest
chrome-to-har -url https://example.com -output out.har
```

Capture network activity with `cdp`:

```bash
cdp --url https://example.com --har out.har
```

Evaluate JavaScript in a page:

```bash
cdp --headless --url https://example.com --js 'document.title'
```

Fetch a page with browser execution:

```bash
churl https://example.com
```

Attach to a Node inspector target:

```bash
ndp node attach 9229
```

Run a CDP script:

```bash
cdpscript run script.cdp
```

## Common Tasks

Capture authenticated browser traffic with an existing profile:

```bash
chrome-to-har -profile "Default" \
  -url https://app.example.com \
  -wait-for '#app-root' \
  -output app.har
```

Render a JavaScript-heavy page to text:

```bash
churl --output-format=text --wait-for ".loaded" https://example.com
```

Take a screenshot or extract content from a page:

```bash
cdp -url https://example.com -screenshot full
cdp -url https://example.com -extract 'h1'
cdp -url https://example.com -render body
```

Connect to an existing Chrome instance:

```bash
cdp -debug-port 9222 -list-tabs
cdp -debug-port 9222 -tab <tab-id> -js 'document.title'
```

Connect to a remote browser:

```bash
cdp -remote-host 10.0.0.5 -remote-port 9222 -list-tabs
churl --remote-host 10.0.0.5 --remote-port 9222 https://example.com
```

Use `ndp` against a Node inspector:

```bash
node --inspect=9229 app.js
ndp node attach 9229
```

Run and test CDP scripts:

```bash
cdpscript run login.txtar
cdpscripttest run testdata/login.txtar
```

## Command Summary

`chrome-to-har` is the focused capture entry point. It is useful when you want HAR output, streaming entry logs, profile-based browsing, or differential capture reports.

## What `cdp` Does

The main `cdp` command is the broader general-purpose entry point. It goes beyond `chrome-to-har` and can:

- connect to Chrome or Chromium locally or remotely
- navigate, evaluate JavaScript, and extract page state
- record HAR output and stream capture data
- inject extra capture logic for traffic CDP does not expose directly, including gRPC-Web streams and WebRTC data channel events
- run in interactive and MCP-oriented modes

## What `churl` Does

`churl` is a browser-powered fetch tool for pages that need JavaScript execution. It is useful for:

- SPA-aware page fetches
- extracting rendered HTML or text
- saving HAR alongside fetch output
- mirroring and scripted page interaction

## What `ndp` and `chdb` Do

`ndp` focuses on Node/V8 debugging flows. `chdb` focuses on Chrome-oriented debugging flows. Both are still evolving, but they are intended to expose debugger-oriented workflows rather than generic browser automation.

`cdpscript` and `cdpscripttest` handle repeatable browser scripts and script-driven test runs.

## Documentation

- [docs/usage.md](docs/usage.md)
- [docs/cdp.md](docs/cdp.md)
- [docs/churl.md](docs/churl.md)
- [docs/langmodel.md](docs/langmodel.md)
- [docs/differential-capture.md](docs/differential-capture.md)

For command-level help, use:

```bash
cdp --help
churl --help
chdb --help
ndp --help
cdpscript --help
cdpscripttest --help
```
