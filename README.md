# cdp

[![Go Reference](https://pkg.go.dev/badge/github.com/tmc/cdp.svg)](https://pkg.go.dev/github.com/tmc/cdp)

`cdp` is a Go module for Chrome DevTools Protocol automation. It includes a live browser CLI, a txtar script runner, and a Go test harness for repeatable browser workflows.

The browser-automation stack has three primary layers:

- `cmd/cdp`: live CDP operation, MCP tools, browser attach, screenshots, HAR, raw CDP, and page artifacts.
- `cdpscript`: executable txtar scripts that turn browser interactions into Unix-style tools.
- `cdpscripttest`: `rsc.io/script`-style browser fixtures for Go tests, including a bridge that runs real `cdpscript` archives.

For MCP actions tied to an inspected page, see [browser observations and actions](docs/browser-observations.md).

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

Attach to a browser the user is already using:

```bash
cdp attach --port 9222
cdp --remote-host localhost --remote-port 9222 --tab <target-id> --shell
cdp --remote-host localhost --remote-port 9222 --tab <target-id> --await --format json --js 'document.title'
```

Run a repeatable browser script:

```bash
cdpscript script.txtar
cdpscript --tab <target-id> --port 9222 script.txtar
cdp run --tab <target-id> --port 9222 script.txtar
```

Test browser behavior from Go:

```go
e := cdpscripttest.NewEngine()
cdpscripttest.Test(t, e, allocCtx, "http://localhost:8080", "testdata/*.txt", nil)
```

For `cdpscript`-format txtars, use `cdpscripttest.RunCDPScript` so tests run the same runtime used by `cdpscript` and `cdp run`.

```go
err := cdpscripttest.RunCDPScript(ctx, "testdata/login.txtar", cdpscripttest.CDPScriptRunOptions{
	Headless: true,
	Env:      []string{"BASE_URL=http://localhost:8080"},
})
```

Capture network activity with `chrome-to-har`:

```bash
go install github.com/tmc/cdp/cmd/chrome-to-har@latest
chrome-to-har --url https://example.com --output out.har
```

Capture network activity with `cdp`:

```bash
cdp --headless --url https://example.com --har out.har --timeout 10
```

Use `--shell` when you want to browse interactively while recording:

```bash
cdp --url https://example.com --har out.har --shell
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

## Common Tasks

Capture authenticated browser traffic with an existing profile:

```bash
chrome-to-har --profile "Default" \
  --url https://app.example.com \
  --wait-for '#app-root' \
  --output app.har
```

Render a JavaScript-heavy page to text:

```bash
churl --output-format=text --wait-for ".loaded" https://example.com
```

Take a screenshot or extract content from a page:

```bash
cdp --url https://example.com --screenshot 'full page.png'
cdp --url https://example.com --extract 'h1'
cdp --url https://example.com --render body
```

Connect to an existing Chrome instance:

```bash
cdp attach --port 9222
cdp --remote-host localhost --remote-port 9222 --tab <tab-id> --js 'document.title'
```

Connect to a remote browser:

```bash
cdp --remote-host 10.0.0.5 --remote-port 9222 --list-tabs
churl --remote-host 10.0.0.5 --remote-port 9222 https://example.com
```

Use `ndp` against a Node inspector:

```bash
node --inspect=9229 app.js
ndp node attach 9229
```

Run and test CDP scripts:

```bash
cdpscript login.txtar
cdpscripttest --url http://localhost:8090 testdata/login.txt
```

## Command Summary

`chrome-to-har` is the focused capture entry point. It is useful when you want HAR output, streaming entry logs, profile-based browsing, or differential capture reports.

## What `cdp` Does

The main `cdp` command is the broader general-purpose entry point. It goes beyond `chrome-to-har` and can:

- connect to Chrome or Chromium locally or remotely
- navigate, evaluate JavaScript, and extract page state
- record HAR output and stream HARL JSONL capture data to a file, or to
  stdout when `--harl-file -` is explicit
- inject extra capture logic for traffic CDP does not expose directly, including gRPC-Web streams and WebRTC SDP, DataChannel, and ICE candidates (under `--full-capture`, select the WebRTC streams with `--webrtc-capture`, e.g. `--webrtc-capture=sdp,datachannel` (default), `=all`, or `=none`)
- run in interactive and MCP-oriented modes

## What `churl` Does

`churl` is a browser-powered fetch tool for pages that need JavaScript execution. It is useful for:

- SPA-aware page fetches
- extracting rendered HTML or text
- saving HAR alongside fetch output
- mirroring and scripted page interaction

## What `ndp` and `chdb` Do

`ndp` focuses on Node/V8 debugging flows. `chdb` focuses on Chrome-oriented debugging flows. Both are still evolving, but they are intended to expose debugger-oriented workflows rather than generic browser automation.

## Automation Stack

`cdp` is the live operator surface. Use it when you need to attach to an existing browser, inspect tabs, observe with screenshots, act with coordinates or element refs, capture HAR/PDF/screenshots, or fall back to raw CDP.

`cdpscript` is the durable automation surface. Scripts are txtar archives with `main.cdp`, optional embedded helper files, argv via `ARG1..ARGN`/`ARGC`, environment variables, output artifacts, and distinct exit codes for usage and assertion failures.

`cdpscripttest` is the Go testing surface. It runs local browser fixtures, screenshot comparisons, network/WebRTC tests, and real `cdpscript` archives under `go test`. Fixtures should avoid third-party network dependencies; authenticated site workflows should live as explicitly live-only examples.

See [docs/planning/cdp-best-in-class-checklist.md](docs/planning/cdp-best-in-class-checklist.md) for the current implementation checklist and verification gates.

## Documentation

- [docs/usage.md](docs/usage.md)
- [docs/plugins.md](docs/plugins.md)
- [docs/cdp.md](docs/cdp.md)
- [docs/churl.md](docs/churl.md)
- [docs/langmodel.md](docs/langmodel.md)
- [docs/differential-capture.md](docs/differential-capture.md)
- [docs/planning/cdp-best-in-class-checklist.md](docs/planning/cdp-best-in-class-checklist.md)
- [docs/planning/cdp-feature-surface-roadmap.md](docs/planning/cdp-feature-surface-roadmap.md)

For command-level help, use:

```bash
cdp --help
churl --help
chdb --help
ndp --help
cdpscript --help
cdpscripttest --help
```
