# Electron Tooling Improvements

*2026-04-05 — Issues discovered during live reversing of Antigravity, Codex, and Claude Desktop*

Source: `/tmp/reversing-tool-snags.md` (A998 session log)

## cdp Improvements

### 1. Auto-add `--remote-allow-origins=*` when launching Electron apps

**Problem**: Electron apps reject CDP WebSocket connections without matching Origin header. Antigravity rejected ALL origins (localhost, 127.0.0.1, empty). Even with `suppress_origin=True`, apps launched without `--remote-allow-origins=*` refuse connections.

**Fix**: When cdp launches or connects to an Electron target, auto-add `--remote-allow-origins=*` to launch flags. For `--connect-existing`, attempt connection and if 403'd, warn the user to relaunch with the flag.

**Files**: `cmd/cdp/main.go` (launch flags), connection setup

### 2. Reconnect on app restart / debug port loss

**Problem**: Codex restarted itself during UI interaction (possibly auto-update), killing the debug connection. All extracted state was lost. CDP connection silently died.

**Fix**: 
- Detect connection loss (WebSocket close event) and attempt reconnect with backoff
- On reconnect failure, surface a clear error: "target restarted, debug connection lost"
- Consider auto-saving incremental state (console buffer, network log, coverage) to disk so it survives reconnections

**Files**: `cmd/cdp/mcp.go` (session lifecycle)

### 3. Worker targets timeout on evaluate

**Problem**: Codex worker targets (utility processes) timed out on Runtime.evaluate. They're Electron utility processes, not web workers — evaluate doesn't work on them.

**Fix**: When attaching to a target, check target type. For `type: "utility"` or similar non-page targets, skip evaluate-based tools or warn that only limited inspection is available. Don't hang.

**Files**: `cmd/cdp/mcp_tools.go` (target attachment)

### 4. Push context to different port mid-session

**Problem**: cdp MCP server is connected to Brave on 9222 but user wants to inspect Antigravity on 9333. No way to switch targets without restarting the MCP server.

**Fix**: Add `connect` tool that switches the session to a different CDP endpoint mid-session. Tears down existing chromedp context, creates new RemoteAllocator pointing to new port.

**Files**: `cmd/cdp/mcp_tools.go` (new tool)

## ndp Improvements

### 5. Target selection by title or URL

**Problem**: ndp connects to the first target from `/json/list`. Multi-window Electron apps (Antigravity has 3+ windows) need targeted connection.

**Fix**: Add `--target-title` and `--target-url` flags to filter `/json/list` results. Match by substring.

**Files**: `cmd/ndp/main.go`, `cmd/ndp/mcp.go`

### 6. Inspector injection helper

**Problem**: Getting V8 inspector access on hardened Electron apps (Claude Desktop) requires manual fuse flipping + asar extraction + code injection + codesigning. A998 documented the repro but it's 6 steps.

**Fix**: Add `ndp inject --app /Applications/Claude.app --port 9229` command that automates:
1. Copy app to temp location
2. Flip fuses via `@electron/fuses`
3. Extract asar, prepend `require('inspector').open(port)`, repack
4. Ad-hoc codesign
5. Launch and connect

**Caveats**: Requires npm packages (`@electron/fuses`, `asar`). macOS only. Modifies a copy, never the original. Document that this is for authorized analysis only.

**Files**: New `cmd/ndp/inject.go` or separate script

### 7. Save extracted data incrementally

**Problem**: Debug connections can drop at any time (app restart, crash, timeout). Data extracted via evaluate is only in memory.

**Fix**: Add `--output-dir` flag. When set, tools auto-save results:
- `list_sources` → `sources.json`
- `read_source` → `sources/<script-id>.js`
- `evaluate` → `evaluations/<timestamp>.json`
- `get_console` → `console.log`
- Coverage snapshots → `coverage/<name>.json`

**Files**: `cmd/ndp/mcp.go`, `cmd/ndp/mcp_tools.go`

## axmcp Improvements (for xcmcp team)

### 8. Electron app name resolution

**Problem**: `ax_list_windows app="Antigravity"` fails because the process registers as "Electron", not the app name. `ax_list_windows app="Electron"` also fails.

**Fix**: When app name lookup fails, try:
1. Search by bundle identifier (com.exa.antigravity)
2. Search by window title substring
3. Search by PID (from `lsof -i :PORT`)
4. Fall back to `ax_apps` and fuzzy match

### 9. Substring match disambiguation

**Problem**: `ax_click contains="Settings…"` matched "Services Settings…" instead of "Settings…". Substring matching is too greedy.

**Fix**: Add `exact=true` param for exact title matching. When multiple matches found, return the list and ask user to disambiguate instead of clicking the first one.

### 10. Element screenshot context

**Problem**: `ax_screenshot contains="Settings"` captures just the tiny button, no surrounding context. Useless for understanding layout.

**Fix**: Add `padding` param (default 50px) that expands the capture rect around the target element. Or add `context=true` that captures the parent container instead.

### 11. Web content in Electron webviews invisible to AX

**Problem**: Web-rendered UI inside Electron webviews has no AX tree elements. Tabs, buttons, and text rendered by React/web frameworks are invisible to axmcp.

**Fix**: This is fundamental — AX sees native UI, not web content. Document the limitation clearly: "For web content inside Electron apps, use cdp tools instead of axmcp." Consider adding a detection: if the target element is a webview, suggest switching to CDP.

### 12. Multi-display coordinate issues

**Problem**: Menu bar items and dialogs on secondary displays show negative coordinates. About dialogs open off-screen.

**Fix**: `ax_list_windows` should report which display each window is on. `ax_screenshot` should handle multi-display coordinates correctly. Add `--display` param to scope operations to a specific monitor.

## Cross-Tool Coordination

### 13. Unified Electron app launcher

**Problem**: Each tool (cdp, ndp, axmcp) needs the app launched with different flags. cdp needs `--remote-debugging-port`, ndp needs `--inspect` or inspector injection, axmcp needs accessibility permissions. Launching is manual and error-prone.

**Future**: A coordinated launcher that starts an Electron app with all debug flags and connects all three MCP servers:
```
electron-debug /Applications/Codex.app \
  --cdp-port 9222 \     # for cdp MCP
  --inspect-port 9229 \ # for ndp MCP  
  --remote-allow-origins=*
```

### 14. Best tool selection guide

Based on A998's experience, document which tool to use when:

| Task | Best tool | Why |
|------|-----------|-----|
| Native UI screenshots | axmcp | Captures chrome, DevTools, browser UI |
| Web content screenshots | cdp | Page.captureScreenshot for renderer content |
| Menu/toolbar interaction | axmcp | AX tree maps native menus |
| Web element interaction | cdp | DOM/JS access for web content |
| Main process inspection | ndp | V8 inspector, require(), IPC routing |
| Renderer JS state | cdp or ndp | Both can evaluate in renderer context |
| Source code reading | ndp | Debugger.getScriptSource |
| Network capture | cdp | Network domain events |
| IPC sniffing | cdp (inspect) | Monkey-patch bridge in renderer |
| Coverage | cdp or ndp | Both have V8 Profiler |
| File system access | ndp (main process) | require('fs') in main process |

### 15. Origin header handling should be automatic

**Problem**: Multiple snags related to WebSocket Origin rejection. This is a solved problem — chromedp handles it, but raw connections don't.

**Fix**: Document that cdp handles this automatically. For manual connections, always use `--remote-allow-origins=*` when launching, or omit the Origin header entirely (raw socket handshake).

## Priority

| # | Improvement | Effort | Impact | Tool |
|---|------------|--------|--------|------|
| 1 | Auto `--remote-allow-origins=*` | Low | High | cdp |
| 4 | Mid-session port switch (`connect`) | Medium | High | cdp |
| 5 | Target selection by title/URL | Low | High | ndp |
| 2 | Reconnect on app restart | Medium | High | cdp |
| 7 | Incremental data saving | Medium | Medium | ndp |
| 3 | Worker target type checking | Low | Medium | cdp |
| 8 | Electron app name resolution | Medium | Medium | axmcp |
| 9 | Exact match disambiguation | Low | Medium | axmcp |
| 14 | Best tool selection docs | Low | Medium | docs |
| 6 | Inspector injection helper | High | Medium | ndp |
| 10-12 | AX screenshot/display fixes | Medium | Low | axmcp |
| 13 | Unified launcher | High | Low (future) | new |
