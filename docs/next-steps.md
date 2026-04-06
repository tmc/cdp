# cdp / ndp — Next Steps

Status: 98 cdp MCP tools, 20 ndp MCP tools = 118 total (2026-04-05).

## Done (2026-04-04/05)

- ~~Sourcemap pipeline E2E~~ — validated on react.dev
- ~~DevTools live sourcemap~~ — serve_sourcemap installs bundle response intercept + Page.reload
- ~~WebMCP bridge~~ — 4 tools: enable_webmcp, list_web_tools, invoke_web_tool, web_tool_invocations
- ~~DevTools coverage extension~~ — MV3 extension at extension/coverage/ with coverage HTTP API
- ~~Coverage JSON tags~~ — all coverage structs have json tags, Source excluded from API
- ~~coverageProvider interface~~ — coverage API decoupled from mcpSession for ndp reuse
- ~~Extension tools JS fallbacks~~ — list/install/uninstall use developerPrivate + SW target fallbacks (782dc36c)
- ~~Extension storage tools~~ — get/set/clear via evalInExtensionSW(), supports local/sync/session/managed (782dc36c)
- ~~Coverage extension SW~~ — background.js added, creates CDP-visible target (782dc36c)
- ~~ndp MCP server Phase 1~~ — 15 tools: evaluate, sources, coverage, profiler, console, targets (aa6441e6)
- ~~Shared coverage infrastructure~~ — coverage.Store interface + coverage.StartAPI() used by both cdp and ndp (3f9ade5a)
- ~~Embedded extension distribution~~ — go:embed + extract to ~/.cdp/extensions/, auto-loads on MCP startup (da4b55c5)
- ~~Coverage page-reload fix~~ — outerCtx pattern survives navigation (6e7bc33e)
- ~~Inspect tools~~ — inspect_ipc_start/log, inspect_walk, inspect_fingerprint gated behind --enable-inspect
- ~~walk_object tool~~ — deep recursive JS object exploration (9abd5c3f)
- ~~Network capture tools~~ — start_network_log / get_network_log for live capture
- ~~remote-allow-origins flag~~ — auto-include for Electron CDP compatibility
- ~~Worker target type checking~~ — switch_tab warns on non-page targets
- ~~ndp target selection~~ — --target-title / --target-url for multi-window Electron
- ~~connect tool~~ — mid-session CDP port switching

## Workstreams

### 1. chrome-devtools-mcp Parity (~7 tools, ~500 lines)

Close the gaps vs Google's official chrome-devtools-mcp (28 tools). See [/tmp/collab-B140.md] for full analysis.

| Tool | Priority | Effort | File |
|------|----------|--------|------|
| `set_throttling` (network + CPU) | High | ~60 lines | mcp_emulation_tools.go |
| `analyze_trace` (CWV: LCP/INP/CLS) | Medium | ~200 lines | mcp_trace_tools.go + internal/traceinsight/ |
| `trace_insight` (per-metric drill-down) | Medium | ~80 lines | mcp_trace_tools.go |
| `drag` | Low | ~50 lines | mcp_input_tools.go |
| `fill_form` | Low | ~60 lines | mcp_tools.go |
| Extend get_har_entries/get_console | Low | ~20 lines | existing files |

Key decision: extract CWV from raw trace events directly (LargestContentfulPaint::Candidate, EventTiming, LayoutShift) instead of importing Chrome DevTools Frontend's TraceEngine.

### 2. Node.js & Electron MCP Tools

~~**Phase 1**: ndp MCP server — 15 tools done (aa6441e6)~~
~~**Phase 2**: Shared coverage infrastructure via Store interface (3f9ade5a)~~
~~**Phase 3**: Sourcemap pipeline for Node — 4 tools (516ce309)~~
~~**Phase 4**: Electron support — detect_electron tool + docs (f963740e)~~
~~**Phase 5**: Target selection — --target-title/--target-url for multi-window Electron~~

### 3. ToolRegistry Abstraction

Single `ToolDef` struct → auto-generates CLI + cdpscript + MCP surfaces. Architectural cleanup that reduces the tool maintenance surface from 3x duplication to 1x. Not urgent; blocked on getting tool surface stable first.

### 4. Polish & Harden

- `-short` skip guards on browser integration tests
- CI-friendly config (SKIP_BROWSER_TESTS env var)
- Fix: navigate tool type mismatch, context/cancel leaks in switch_tab/new_tab
- Document all tools with usage examples
- Verify external .map fetch on -remote-host path

## Planning Documents Index

| Doc | Scope |
|-----|-------|
| [next-steps.md](next-steps.md) | This file — master task tracker |
| [node-electron-mcp.md](node-electron-mcp.md) | ndp MCP server design (Node/Electron) |
| [advanced-capabilities-vision.md](advanced-capabilities-vision.md) | 8 forward-looking capabilities (sourcemaps, coverage-guided exploration, runtime deps, change impact, progressive understanding, live annotation, cdpscripttest, DevTools extension) |
| [agent-browser-gap-analysis.md](agent-browser-gap-analysis.md) | Feature parity vs vercel-labs/agent-browser (mostly closed) |
| [planning/roadmap.md](planning/roadmap.md) | ndp roadmap: workspace persistence, TUI, domain integration |
| [planning/visionary_roadmap.md](planning/visionary_roadmap.md) | ndp vision: AI co-pilot, time travel, multiplayer debugging, DAP bridge |
| [chrome-devtools-mcp-parity.md](chrome-devtools-mcp-parity.md) | chrome-devtools-mcp parity plan (7 gap tools) |
