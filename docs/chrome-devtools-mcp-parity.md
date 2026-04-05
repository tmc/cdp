# Feature Parity Plan: chrome-devtools-mcp Gaps

Reference: https://github.com/ChromeDevTools/chrome-devtools-mcp (28 tools)
Our status: 74 tools in cmd/cdp/

## High Priority: Network/CPU Throttling

**What they have**: Single `emulate` tool with `networkConditions` (enum: Slow 3G, Fast 3G, Slow 4G, Fast 4G, Offline, No emulation) and `cpuThrottlingRate` (1-20x multiplier).

**CDP APIs**:
- `Network.emulateNetworkConditions` — downloadThroughput, uploadThroughput, latency, offline
- `Emulation.setCPUThrottlingRate` — rate float (1 = no throttle, 4 = 4x slower)

**Implementation**: Add to `cmd/cdp/mcp_emulation_tools.go`. We already have `set_offline` there.

```
Tool: set_throttling
Inputs:
  network_preset: "slow-3g" | "fast-3g" | "slow-4g" | "fast-4g" | "offline" | "none"
  cpu_rate: float (1-20, default 1)
  custom_download: int (bytes/sec, optional — overrides preset)
  custom_upload: int (bytes/sec, optional)
  custom_latency: int (ms, optional)
```

Presets (from Puppeteer PredefinedNetworkConditions):
| Preset | Download (KB/s) | Upload (KB/s) | Latency (ms) |
|--------|----------------|---------------|---------------|
| Slow 3G | 50 | 50 | 2000 |
| Fast 3G | 187.5 | 56.25 | 562.5 |
| Slow 4G | 500 | 500 | 400 |
| Fast 4G | 4000 | 3000 | 170 |

**Complexity**: Low (~60 lines). Two CDP calls.
**File**: `cmd/cdp/mcp_emulation_tools.go`

## Medium Priority

### close_page

**What they have**: Close a tab by index.

**Implementation**: Add to `cmd/cdp/mcp_tools.go` tab tools section. Use `target.CloseTarget(targetID)`.

```
Tool: close_tab
Inputs:
  index: int (tab index, optional — defaults to current)
  target_id: string (optional — close by ID)
```

**Complexity**: Very low (~30 lines).
**File**: `cmd/cdp/mcp_tools.go` (near existing tab tools)

### Performance Trace with Core Web Vitals

**What they have**: `performance_record` captures a trace, then extracts CWV scores (LCP, FID/INP, CLS) and structured insights using Chrome DevTools Frontend's TraceEngine.

**Their approach**: They import `chrome-devtools-frontend/mcp/mcp.js` which includes the full TraceEngine, PerformanceTraceFormatter, and PerformanceInsightFormatter. This is a ~massive JS dependency.

**Our approach**: We already have `start_trace`/`stop_trace` (`cmd/cdp/mcp_trace_tools.go`). Add CWV extraction by parsing the raw trace events directly:

```
Tool: analyze_trace (or extend stop_trace output)
Inputs:
  trace_file: string (optional — use last recorded trace)

Output: {
  cwv: { lcp_ms, fid_ms, inp_ms, cls },
  insights: [
    { type: "long-task", duration_ms, stack_url },
    { type: "layout-shift", score, elements },
    { type: "render-blocking", url, duration_ms },
    ...
  ],
  summary: string
}
```

**CWV extraction from trace events** (no external deps):
- **LCP**: Find `largestContentfulPaint::Candidate` trace event → `args.data.size`, `args.data.type`
- **FID/INP**: Find `EventTiming` events → `args.data.interactionId`, `args.data.duration`
- **CLS**: Find `LayoutShift` events where `args.data.had_recent_input == false` → sum `args.data.score`
- **Long tasks**: Find `RunTask` events > 50ms
- **Render-blocking resources**: Find `ResourceSendRequest` with `args.data.renderBlocking == "blocking"`

**Complexity**: Medium (~200 lines). New file `internal/traceinsight/` or add to `mcp_trace_tools.go`.
**File**: `cmd/cdp/mcp_trace_tools.go` + optionally `internal/traceinsight/cwv.go`

### performance_analyze_insight

**What they have**: Takes an insight name (e.g., "LCP", "RenderBlocking", "CLSCulprits") and returns deep analysis.

**Our approach**: Once `analyze_trace` extracts structured data, add:

```
Tool: trace_insight
Inputs:
  insight: "lcp" | "cls" | "inp" | "long-tasks" | "render-blocking" | "network-waterfall"
  trace_file: string (optional)
```

Returns focused analysis for that metric. This is a view into the same trace data, filtered.

**Complexity**: Low-medium (~80 lines, mostly formatting).
**File**: `cmd/cdp/mcp_trace_tools.go`

## Low Priority

### drag

```
Tool: drag (add to mcp_input_tools.go)
Inputs:
  source: string (selector or @ref)
  target: string (selector or @ref)
  steps: int (default 10 — intermediate mouse move steps)
```

Use `Input.dispatchMouseEvent` sequence: mousePressed at source → series of mouseMoved → mouseReleased at target.

**Complexity**: Low (~50 lines).
**File**: `cmd/cdp/mcp_input_tools.go`

### fill_form

```
Tool: fill_form
Inputs:
  fields: [{selector: string, value: string, type?: "text"|"select"|"checkbox"|"radio"}]
  submit: bool (optional — submit form after filling)
```

Batch operation: iterate fields, dispatch appropriate actions per type. Convenience wrapper over existing click/type_text.

**Complexity**: Low (~60 lines).
**File**: `cmd/cdp/mcp_tools.go` or new `mcp_form_tools.go`

### get_network_request / get_console_message (single item detail)

We already have `get_har_entries` and `get_console` for bulk. Add filtering:
- Extend `get_har_entries` with `request_id` param for single entry
- Extend `get_console` with `index` param for single message

**Complexity**: Very low (~10 lines each, add optional params to existing tools).
**Files**: `cmd/cdp/mcp_tools.go` (HAR), `cmd/cdp/mcp_console_tools.go`

## Implementation Order

1. **set_throttling** — highest value, lowest effort
2. **close_tab** — trivial
3. **analyze_trace** (CWV extraction) — medium effort but high differentiation
4. **trace_insight** — builds on analyze_trace
5. **drag** — straightforward
6. **fill_form** — convenience tool
7. **Extend get_har_entries/get_console** — tiny additions

Total: ~7 new tools/extensions, ~500 lines of code.

## Our Advantages (keep and promote)

We have 46 MORE tools than they do, with unique capabilities they can't match:
- **Sourcemap pipeline** (7 tools) — no equivalent anywhere
- **Coverage** (7 tools) — no other MCP browser tool has this
- **WebMCP bridge** (4 tools) — unique in the ecosystem
- **Request/response interception** (4 tools) — they only have network observation
- **DOM diffing, state persistence, custom tool definitions** — all unique
- **Go-native & embeddable** — theirs is TypeScript/Node only
