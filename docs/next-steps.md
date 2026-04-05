# cdp MCP Server — Next Steps

Status: 74 MCP tools (2026-04-05). Sourcemap pipeline, coverage, DOM diff, @ref system, WebMCP bridge, DevTools coverage extension all in place.

## Done

- ~~End-to-end sourcemap pipeline~~ — validated on react.dev (2026-04-04)
- ~~DevTools live sourcemap integration~~ — serve_sourcemap installs bundle response intercept + Page.reload (2026-04-05)
- ~~WebMCP bridge~~ — 4 tools: enable_webmcp, list_web_tools, invoke_web_tool, web_tool_invocations (2026-04-05)
- ~~DevTools coverage extension~~ — MV3 extension at extension/coverage/ with coverage HTTP API (2026-04-05)

## 1. Node.js & Electron MCP Tools

Add MCP server to ndp for Node.js/Electron V8 debugging (~15 tools).
See [docs/node-electron-mcp.md](node-electron-mcp.md) for full design.

## 2. ToolRegistry Abstraction

Single tool definition auto-generates all three surfaces:

- Define a `ToolDef` struct: name, description, input schema, handler func, readonly flag
- `ToolRegistry` holds all defs, provides: `RegisterMCP(server)`, `RegisterCLI(cmd)`, `RegisterScript(map)`
- Custom .cdp tools (define_tool, tooldef parser) auto-register into all three
- Migrate existing 69 tools to the registry
- This unifies CLI subcommands, cdpscript commands, and MCP tools from one source of truth

## 3. WebMCP Bridge

Wire up Chrome 148's WebMCP domain (observation-only) and build the invocation bridge:

- Enable WebMCP domain, listen for toolsAdded/toolsRemoved events
- Expose `list_web_tools` MCP tool showing discovered browser-side MCP tools
- Build invocation bridge: `invoke_web_tool` uses Runtime.evaluate JS injection to call the tool
- Listen for toolInvoked/toolResponded events for observability
- Handle backendNodeId-based declarative tools (HTML element tools)
- This lets agents discover and use MCP tools that web apps expose natively

## 4. DevTools Coverage Extension

Chrome DevTools extension (~300-500 lines) for tagged coverage profiles:

- Custom panel with context timeline (horizontal bar of push/pop contexts with delta %)
- Click a context → see per-file/function coverage delta for that action
- Compare mode: select two contexts, see code diff between them
- "View in Sources" → navigate DevTools Sources panel to specific file
- Cumulative vs delta toggle
- Export lcov per-context or cumulative
- Communication: read coverage JSON from disk or small HTTP endpoint from cdp
- Reference: xcmcp's axmcp tool has similar panel patterns

## 5. Polish & Harden

- Add `-short` skip guards to heavy browser integration tests
- Fix TestCDP_ListBrowsers expected output format
- Verify external sourcemap .map fetch works on -remote-host path
- Review and fix: navigate tool type mismatch, context/cancel leaks in switch_tab/new_tab
- Add CI-friendly test configuration (SKIP_BROWSER_TESTS env var)
- Document the 69 tools with usage examples
