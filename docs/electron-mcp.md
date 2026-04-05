# Electron MCP Debugging

Electron apps have two process types that use different protocols:

- **Renderer** (Chromium): Use `cdp --mcp` with `--remote-port`
- **Main process** (Node.js): Use `ndp --mcp` with `--node-port`

## Finding the ports

### Renderer (Chrome DevTools Protocol)

Launch Electron with remote debugging enabled:

```bash
electron --remote-debugging-port=9222 your-app/
```

### Main process (V8 Inspector)

Launch Electron with Node.js inspector enabled:

```bash
electron --inspect=9229 your-app/
# or break on first line:
electron --inspect-brk=9229 your-app/
```

### Both at once

```bash
electron --remote-debugging-port=9222 --inspect=9229 your-app/
```

## MCP configuration

Add both servers to `.mcp.json` for full Electron debugging:

```json
{
  "mcpServers": {
    "electron-renderer": {
      "command": "cdp",
      "args": ["--mcp", "--remote-port", "9222", "--headless=false"]
    },
    "electron-main": {
      "command": "ndp",
      "args": ["--mcp", "--node-port", "9229"]
    }
  }
}
```

## What each server provides

### cdp (renderer)

- Page navigation, screenshots, DOM interaction
- Network interception and HAR recording
- CSS/JS coverage for frontend code
- Extension management
- Sourcemap analysis for bundled frontend code

### ndp (main process)

- JavaScript evaluation in Node.js context
- Source listing and reading (all loaded modules)
- Console and error capture
- CPU profiling and heap snapshots
- Code coverage for backend code
- Sourcemap analysis for bundled server code
- `detect_electron` tool to identify Electron environment

## Detecting Electron

The `detect_electron` tool on the ndp server checks for Electron-specific
globals and reports the Electron version, process type, app name, and path.
This helps agents understand the debugging context.

## Common patterns

### Debug a renderer crash

1. Connect cdp to renderer port
2. Use `screenshot` and `page_snapshot` to see current state
3. Use `get_console` and `get_errors` for error context
4. Use `evaluate` to inspect DOM/JS state

### Debug main process hang

1. Connect ndp to inspector port
2. Use `start_cpu_profile` / `stop_cpu_profile` to find hot code
3. Use `evaluate` to inspect process state
4. Use `get_console` for logged output

### Trace IPC between processes

1. Connect both servers
2. In ndp: `evaluate` with `process.on('message', ...)` or Electron IPC listeners
3. In cdp: `evaluate` with `ipcRenderer.on(...)` watchers
4. Use `get_console` on both sides to see message flow
