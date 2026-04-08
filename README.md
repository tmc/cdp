# cdp

[![Go Reference](https://pkg.go.dev/badge/github.com/tmc/cdp.svg)](https://pkg.go.dev/github.com/tmc/cdp)

`cdp` is a Go module for Chrome DevTools Protocol automation. It includes a small library surface plus several command-line tools for browser control, traffic capture, script execution, and Node/V8 debugging.

## Commands

- `cdp`: the main Chrome/CDP CLI
- `churl`: browser-backed fetch and page extraction
- `chdb`: Chrome-focused debugger workflow
- `ndp`: Node/V8 debugger workflow
- `cdpscript`: run CDP scripts
- `cdpscripttest`: test CDP scripts

## Install

The module requires Go 1.26 or newer.

Install the main CLI:

```bash
go install github.com/tmc/cdp@latest
```

Install individual tools:

```bash
go install github.com/tmc/cdp/cmd/cdp@latest
go install github.com/tmc/cdp/cmd/churl@latest
go install github.com/tmc/cdp/cmd/chdb@latest
go install github.com/tmc/cdp/cmd/ndp@latest
go install github.com/tmc/cdp/cmd/cdpscript@latest
go install github.com/tmc/cdp/cmd/cdpscripttest@latest
```

## Quick Start

Capture network activity to HAR:

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

## What `cdp` Does

The main `cdp` command is the general-purpose entry point. It can:

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

## Documentation

- [docs/cdp.md](/Volumes/tmc/go/src/github.com/tmc/cdp/docs/cdp.md)
- [docs/churl.md](/Volumes/tmc/go/src/github.com/tmc/cdp/docs/churl.md)
- [docs/langmodel.md](/Volumes/tmc/go/src/github.com/tmc/cdp/docs/langmodel.md)
- [docs/differential-capture.md](/Volumes/tmc/go/src/github.com/tmc/cdp/docs/differential-capture.md)

For command-level help, use:

```bash
cdp --help
churl --help
chdb --help
ndp --help
cdpscript --help
cdpscripttest --help
```
