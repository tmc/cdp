# CHDB - Chrome Debugger

chdb is a command-line debugger for Chrome and Chromium built on the Chrome DevTools Protocol.

## Features

- Open DevTools for any target
- JavaScript console attached to a page
- Network monitoring
- DOM inspection
- Screenshots
- JavaScript breakpoints
- Target listing, creation, and attach
- Heap snapshots and performance traces

## Installation

```bash
go install github.com/tmc/cdp/cmd/chdb@latest
```

Or build from source:

```bash
cd cmd/chdb
go build .
```

## Prerequisites

- Chrome or Chromium browser running with debugging enabled:
  ```bash
  # Start Chrome with remote debugging
  google-chrome --remote-debugging-port=9222

  # Or with custom port
  google-chrome --remote-debugging-port=9223
  ```

## Usage

### Basic Commands

#### List Available Targets
```bash
# List all Chrome targets on default port (9222)
chdb list

# List targets on specific port
chdb --port 9223 list
```

#### Execute JavaScript
```bash
# Execute JavaScript in first available target
chdb exec "document.title"

# Execute in specific tab
chdb exec "console.log('Hello from CHDB')" --tab <tab-id>
```

#### Navigate to URL
```bash
# Navigate first available target
chdb navigate https://example.com

# Navigate specific tab
chdb navigate https://github.com --tab <tab-id>
```

#### Take Screenshots
```bash
# Screenshot with auto-generated filename
chdb screenshot

# Screenshot with custom filename
chdb screenshot my-screenshot.png

# Screenshot specific tab
chdb screenshot --tab <tab-id> page.png
```

### Debugging Commands

#### Open DevTools Interface
```bash
# Open DevTools for first available target
chdb devtools

# Open DevTools for specific tab
chdb devtools --tab <tab-id>
```

#### Interactive Console Session
```bash
# Start JavaScript console session
chdb console

# Console for specific tab
chdb console --tab <tab-id>
```

#### DOM Inspection
```bash
# Inspect element by CSS selector
chdb inspect "body"
chdb inspect "#main-content"
chdb inspect ".navigation"

# Inspect in specific tab
chdb inspect "form" --tab <tab-id>
```

#### Network Monitoring
```bash
# Monitor network activity indefinitely
chdb monitor

# Monitor for specific duration
chdb monitor --duration 30s
chdb monitor --duration 5m
```

#### Set Breakpoints
```bash
# Set simple breakpoint
chdb break set "script.js:42"

# Set conditional breakpoint
chdb break set "app.js:100" --condition "user.isAdmin"
```

#### Interactive Debugging
```bash
# Start debugging session with DevTools
chdb debug

# Debug specific tab
chdb debug --tab <tab-id>
```

### Target Management

#### Create New Targets
```bash
# Create new blank tab
chdb new

# Create new tab with URL
chdb new https://example.com
```

#### Attach to Running Chrome
```bash
# Attach and keep connection alive
chdb attach

# Attach to specific port
chdb attach 9223
```

### Performance

#### Heap Snapshots
```bash
# Write a heap snapshot to heap.heapsnapshot (default)
chdb heap

# Save to custom file
chdb heap --output app.heapsnapshot
```

#### Performance Traces
```bash
# Record a 5-second trace to trace.json (default)
chdb trace

# Record for a custom duration to a custom file
chdb trace --duration 30s --output perf.json
```

## Examples

### Web Development Workflow
```bash
# 1. Start monitoring network
chdb monitor --duration 60s &

# 2. Navigate to development site
chdb navigate http://localhost:3000

# 3. Take screenshot
chdb screenshot dev-site.png

# 4. Inspect main element
chdb inspect "#app"

# 5. Execute test JavaScript
chdb exec "window.myApp.runTests()"
```

### Debugging Session
```bash
# 1. Set breakpoints
chdb break set "app.js:42" --condition "debug === true"

# 2. Start interactive debugging
chdb debug

# 3. Open console in another terminal
chdb console
```

### Performance Analysis
```bash
# 1. Navigate to target page
chdb navigate https://example.com

# 2. Record a performance trace
chdb trace --duration 30s --output perf.json

# 3. Take heap snapshot
chdb heap --output app.heapsnapshot

# 4. Monitor network during load
chdb monitor --duration 10s
```

## Global Options

- `--verbose, -v`: Enable verbose output
- `--timeout`: Operation timeout in seconds (default: 60)
- `--port`: Chrome debug port (default: 9222)

## Command Reference

Run `chdb help` for the full command list. Common commands:

| Command | Description | Options |
|---------|-------------|---------|
| `list` | List Chrome targets | `--port` |
| `exec <js>` | Execute JavaScript | `--tab` |
| `navigate <url>` | Navigate to URL | `--tab` |
| `screenshot [file]` | Take screenshot | `--tab` |
| `devtools` | Open DevTools GUI | `--tab` |
| `console` | Interactive console | `--tab` |
| `inspect <selector>` | Inspect DOM element | `--tab` |
| `monitor` | Monitor network | `--duration` |
| `break set <location>` | Set breakpoint | `--condition`, `--tab` |
| `debug` | Start debugging | `--tab` |
| `new [url]` | Create new target | |
| `attach [port]` | Attach to Chrome | |
| `heap` | Heap snapshot | `--output`, `--tab` |
| `trace` | Performance trace | `--duration`, `--output`, `--tab` |

## Troubleshooting

### Chrome Not Found
Ensure Chrome is running with debugging enabled:
```bash
google-chrome --remote-debugging-port=9222 --no-first-run --no-default-browser-check
```

### Connection Issues
Check if Chrome is listening:
```bash
curl http://localhost:9222/json/version
```

### Tab Not Found
List available targets:
```bash
chdb list
```
