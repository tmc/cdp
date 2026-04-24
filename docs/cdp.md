# CDP - Chrome DevTools Protocol CLI Tool

CDP is a powerful command-line tool for interacting with Chrome using the Chrome DevTools Protocol (CDP). It provides a REPL (Read-Eval-Print Loop) interface to execute CDP commands directly, which is useful for browser automation, debugging, and exploring the CDP API.

## Installation

```bash
go install github.com/tmc/cdp/cmd/cdp@latest
```

Or build from source:

```bash
cd cdp
go build -o cdp ./cmd/cdp
```

## Basic Usage

```bash
# Start an interactive CDP session
cdp

# Start with a specific URL
cdp --url https://example.com

# Use in headless mode
cdp --headless

# Connect to an existing Chrome instance
cdp --debug-port 9222

# Show attachable targets or launch instructions
cdp attach

# Run a script file
cdp run script.txtar

# Save command output with shell redirection
cdp --headless --url https://example.com --js 'document.title' > results.txt
```

## Interactive Mode Commands

In interactive mode, CDP presents a `cdp>` prompt. You can enter CDP commands directly or use predefined aliases:

### Raw CDP Commands

CDP commands follow the format: `Domain.method {"param":"value"}`. For example:

```
cdp> Page.navigate {"url": "https://example.com"}
cdp> Runtime.evaluate {"expression": "document.title"}
cdp> Debugger.setBreakpoint {"location": {"scriptId": "123", "lineNumber": 42}}
```

### Aliases

CDP provides numerous aliases for common operations:

#### Navigation
- `goto https://example.com` - Navigate to URL
- `reload` - Reload current page
- `back` - Go back in history 
- `forward` - Go forward in history

#### DOM Interaction
- `click '#button'` - Click element matching CSS selector
- `focus '#input'` - Focus element matching CSS selector
- `type 'Hello'` - Insert text at current focus

#### Page Info
- `title` - Get page title
- `url` - Get current URL
- `cookies` - Get all cookies
- `html` - Get page HTML

#### Screenshots & PDF
- `screenshot` - Take a screenshot
- `screenshot-full` - Take a full-page screenshot
- `pdf` - Generate PDF of current page

#### Device Emulation
- `mobile` - Emulate mobile device
- `desktop` - Emulate desktop device
- `clear-emulation` - Clear emulation settings

#### Network Conditions
- `offline` - Simulate offline mode
- `online` - Restore normal connection
- `slow-3g` - Simulate slow 3G connection
- `fast-3g` - Simulate fast 3G connection

#### Debugging
- `pause` - Pause JavaScript execution
- `resume` / `cont` - Resume execution
- `step` / `stepinto` - Step into function call
- `next` / `stepover` - Step over function call
- `out` / `stepout` - Step out of current function

#### Coverage
- JavaScript coverage: `covjs_start`, `covjs_take`, `covjs_stop`
- CSS coverage: `covcss_start`, `covcss_take`, `covcss_stop`

#### Browser Management
- `targets` - List browser targets
- `info` - Get browser version info
- `domains` - List available CDP domains

### Help Commands

- `help` - Show general help
- `help aliases` - Show all command aliases
- `help domain Page` - Show help for a specific domain
- `help screenshot` - Show help for a specific command/alias

## Script Mode

Repeatable CDP scripts use Go txtar archives. The archive must contain
`main.cdp`; the txtar comment header is for human-facing usage notes.

```text
# script.txtar
#
# Usage:
#   cdp run script.txtar

-- main.cdp --
goto https://example.com
click '#submit-button'
html
```

Run with:

```bash
cdp run script.txtar
cdpscript script.txtar
```

See `cmd/cdp/CDP_SCRIPT_FORMAT.md` for the canonical script-format reference.

## Output Format

CDP prints responses in pretty-formatted JSON. Event messages are prefixed with `<-- Event:` and special events like Debugger paused/resumed are highlighted.

To save command output to a file, use shell redirection:

```bash
cdp --headless --url https://example.com --js 'document.title' > results.txt
```

## Advanced Features

### Connection to Existing Chrome Instance

Use `cdp attach` first. It probes live DevTools endpoints, prints attachable
page targets, and emits exact launch commands when no browser is listening:

```bash
cdp attach
cdp attach --port 9222 --format json
```

The text output includes commands such as:

```bash
cdp --remote-host localhost --remote-port 9222 --tab <target-id> --shell
```

If no target is found, launch Chrome or Brave with remote debugging enabled:

1. Launch Chrome or Brave with remote debugging enabled:
   ```
   chrome --remote-debugging-port=9222 --user-data-dir="$(mktemp -d)"
   ```

2. Connect CDP to this instance:
   ```
   cdp attach --port 9222
   ```

### Using Chrome Profiles

To use an existing Chrome profile, which includes cookies, extensions, and settings:

```
cdp --use-profile Default
```

### Custom Chrome Path

If Chrome is installed in a non-standard location:

```
cdp --chrome-path /path/to/chrome
```

## Common Use Cases

### Web Page Analysis

```
cdp --url https://example.com
cdp> html
cdp> cookies
```

### JavaScript Debugging

```
cdp --url https://example.com
cdp> Debugger.setBreakpointByUrl {"url": "https://example.com/script.js", "lineNumber": 123}
cdp> step
cdp> Runtime.evaluate {"expression": "someVariable"}
```

### Performance Testing

```
cdp --headless
cdp> goto https://example.com
cdp> Performance.enable {}
cdp> Performance.getMetrics {}
```

### Coverage Analysis

```
cdp> covjs_start
# Interact with page
cdp> covjs_take
```

### Web Scraping

```
cdp --headless
cdp> goto https://example.com
cdp> Runtime.evaluate {"expression": "Array.from(document.querySelectorAll('h1')).map(h => h.textContent)"}
```

## Example Script for Automated Screenshot

```text
# screenshot.txtar
# Usage:
#   cdp run --headless --output screenshots screenshot.txtar

-- main.cdp --
goto https://example.com
wait 1s
screenshot desktop.png
js window.scrollTo(0, document.body.scrollHeight)
screenshot scrolled.png
```

Run with:
```bash
cdp run --headless --output screenshots screenshot.txtar
```
