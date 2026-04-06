# Gap Analysis: cdp/ndp vs agent-browser (Vercel)

*Updated 2026-04-05 — reflects current 110-tool state (91 cdp + 19 ndp)*

## Overview

agent-browser is a Rust CLI (~44K lines, ~100+ commands) designed as a browser automation skill for AI agents. CLI invocation with JSON responses, accessibility-first snapshots, daemon model.

Our cdp/ndp stack is a Go MCP server (~18K lines, 110 tools) with native MCP protocol, multi-target support (browser + Node.js + Electron), and deep V8 instrumentation.

## Architecture Comparison

| | agent-browser | cdp/ndp |
|-|--------------|---------|
| Language | Rust (~44K lines) | Go (~18K lines) |
| Protocol | CLI + JSON stdout | MCP (stdio) |
| Tools/commands | ~100+ | 110 (91 cdp + 19 ndp) |
| Browser lib | Hand-rolled CDP client | chromedp |
| Targets | Browser only | Browser + Node.js + Electron |
| Daemon | Unix socket IPC | MCP persistent connection |
| Distribution | Native binary | Single Go binary, embeddable |

## Where They Lead

### High value, moderate effort to close

#### 1. Annotated screenshots (~200 lines)

`screenshot --annotate` overlays numbered boxes on interactive elements with ref IDs, role names, and bounding boxes. Returns metadata mapping number → ref → role → name → coordinates. Agents say "click element 7" instead of guessing selectors.

**Implementation**: After screenshot PNG, query AX tree for interactive elements, get bounding boxes via DOM.getBoxModel, draw numbered rectangles onto PNG using Go image/draw. Add `annotate` bool param to existing `screenshot` tool. Return metadata array alongside image.

**File**: Extend `cmd/cdp/mcp_tools.go` screenshot handler or new `cmd/cdp/annotate.go`

#### 2. Stale @ref recovery (~100 lines)

When `@ref` backend_node_id becomes invalid after DOM mutation, they re-query AX tree and match by role + accessible name. Fallback to tag + position.

**Implementation**: In `resolveRef()` (mcp_refs.go), when DOM.resolveNode fails: get stored role+name from ref map → re-query AX tree → find matching node → update ref map → retry. Add `role` and `name` fields to stored ref entries during page_snapshot.

**File**: `cmd/cdp/mcp_refs.go`

#### 3. Device emulation presets (~80 lines)

`set device "iPhone 14"` sets viewport + UA + touch + scale in one call. ~20 device presets from Puppeteer's KnownDevices.

**Implementation**: Add `set_device` tool with lookup table. Calls Emulation.SetDeviceMetricsOverride + SetUserAgentOverride + SetTouchEmulationEnabled.

**File**: `cmd/cdp/mcp_emulation_tools.go`

### Medium value

#### 4. Download handling (~100 lines)

`download` clicks element and captures file. `wait --download` waits for completion.

**Implementation**: Browser.setDownloadBehavior + Browser.downloadProgress events. New `download` tool or extend `wait_for` with download param.

#### 5. Clipboard tools (~60 lines)

read/write/copy/paste via navigator.clipboard API.

**Implementation**: Runtime.evaluate with clipboard API + Input.dispatchKeyEvent for copy/paste shortcuts.

#### 6. Visual screenshot diff (~150 lines)

Pixel-level comparison with color threshold, outputs diff image.

**Implementation**: Decode two PNGs, pixel-by-pixel comparison, output diff percentage + highlighted diff image. Go `image` stdlib, no external deps.

#### 7. Batch execution (~80 lines)

Execute multiple tool calls in single invocation, reducing round-trips.

**Implementation**: `batch` tool taking array of {tool, arguments}, sequential execution, bail-on-error option.

### Lower priority (skip or defer)

| Feature | Why skip |
|---------|----------|
| Authentication vault | Our save_state/load_state covers cookies + storage. Full encrypted vault is different product direction. |
| Cloud browser providers | Different product. --connect-existing + --remote-host already supports remote browsers. |
| Video recording | Needs ffmpeg dependency. Agents don't watch video. |
| Live WebSocket streaming | Our coverage HTTP API serves similar purpose for instrumentation. |
| Policy engine | MCP has its own permission modes. |
| WebDriver / iOS | Different protocol entirely. Out of scope. |

## Where We Lead

Capabilities agent-browser lacks entirely:

| Capability | Tools | Notes |
|-----------|-------|-------|
| Sourcemap pipeline | 11 (7 cdp + 4 ndp) | analyze → structure → generate → serve via Fetch intercept. Unique in ecosystem. |
| V8 coverage | 11 (7 cdp + 4 ndp) | Byte-range, snapshots, deltas, LCOV, HTTP API, DevTools extension |
| WebMCP bridge | 4 | Observe/invoke navigator.modelContext tools |
| Request/response interception | 4 | Modify in-flight via Fetch domain (they only block/mock) |
| Node.js + Electron debugging | 19 ndp tools | Full V8 inspector, coverage, profiling, heap snapshots |
| Extension management | 9 | List/install/reload/uninstall/storage with JS fallbacks |
| Runtime tool definition | 1 | define_tool creates MCP tools on the fly |
| DOM structural diffing | 2 | Compare DOM trees, not just AX trees |
| Embedded extension | 1 | go:embed + auto-extract + runtime install |
| State persistence | 2 | save_state/load_state with full page state |

## Rough Parity

Both have equivalent coverage for:
- Navigation (navigate, back, forward, reload)
- Interaction (click, type, hover, focus, press_key, drag, fill_form)
- Screenshots (full page, format options)
- Accessibility tree snapshots (page_snapshot)
- Tab management (list, switch, new, close)
- Cookies + storage (get, set, clear)
- Network tracking + HAR
- Console + error capture
- Emulation (viewport, UA, geolocation, offline, throttling, extra headers)
- Dialog handling
- Frame/iframe switching
- File upload
- PDF export
- Scroll control
- Trace recording

## Implementation Plan

| Phase | Feature | Est. lines | Priority |
|-------|---------|-----------|----------|
| 1 | Annotated screenshots | ~200 | High — biggest agent UX win |
| 2 | Stale @ref recovery | ~100 | High — reliability |
| 3 | Device emulation presets | ~80 | High — convenience |
| 4 | Download handling | ~100 | Medium |
| 5 | Clipboard tools | ~60 | Medium |
| 6 | Visual screenshot diff | ~150 | Medium |
| 7 | Batch execution | ~80 | Medium |
| **Total** | **7 features** | **~770** | |

## Tool Count After Closing Gaps

| State | cdp | ndp | Total |
|-------|-----|-----|-------|
| Current | 91 | 19 | 110 |
| After Phase 1-3 | 93 | 19 | 112 |
| After Phase 4-7 | 97 | 19 | 116 |

Note: We don't need 100+ commands. agent-browser has aliases (goto/navigate/open), granular subcommands (mouse move/down/up/wheel as separate), and admin commands. Our MCP tool interface is more composable — fewer tools with richer parameters.
