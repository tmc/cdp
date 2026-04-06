# Reversing Gap Analysis: ndp/cdp Tool Experience

**Context**: Reversed Antigravity (VS Code fork, Electron 39) and Codex (standalone Electron 40) using ndp and cdp MCP tools over remote-debugging-port connections.

---

## 1. What Worked

### Most Valuable Tools

**`evaluate`** — The single most important tool. 90% of the reversing was done through evaluate calls probing `window.*`, `document.*`, DOM structure, and runtime state. The ability to run arbitrary JS in the renderer context is the foundation for everything.

**`list_sources` + `read_source`** — Critical for finding and reading the preload scripts. The preload script is the Rosetta Stone for any Electron app — it defines the entire IPC surface.

**`search_sources`** — Searching across loaded scripts for keywords like "ipcRenderer", "vscode:", "electron" found the preload script immediately. Essential for finding entry points.

**`list_tabs` + `switch_tab`** — Antigravity has multiple windows (editor + Launchpad). Being able to enumerate and switch between them was necessary to understand the dual-workbench architecture.

**`get_page_content`** — Quick way to see what's rendered. The Launchpad's text content immediately revealed the Agent Manager, workspace list, and session history.

**`screenshot`** — Visual confirmation of what we're looking at. The Antigravity screenshot immediately showed it's a VS Code-style IDE with an AI chat panel.

**`set_device`** — Unexpected bonus: viewport changes let us test how the app responds to different screen sizes, revealing responsive layout behaviors.

### Workflows That Clicked

1. **Connect → list_tabs → switch_tab → evaluate probes** — This "connect and orient" workflow worked well.
2. **evaluate to probe globals → find bridge objects → enumerate methods** — Systematic global discovery.
3. **NLS message mining** — Antigravity's `_VSCODE_NLS_MESSAGES` (16,567 messages) was a goldmine. Searching NLS for keywords like "agent", "jetski", "gemini", "credits" revealed feature names, internal codenames, and UX copy that isn't visible in minified code.
4. **Filesystem inspection** — Reading the asar, checking `~/.gemini/`, inspecting the app bundle on disk complemented the runtime analysis well.
5. **ndp detect_electron** — Immediately confirmed Electron version and process type via User-Agent parsing.

---

## 2. What Was Painful

### Session Management

**FIFO/stdin plumbing**: Spent significant time wrestling with MCP server stdin management. Every test required: create FIFO → start process → send init → send notifications/initialized → send tool call → wait → parse output → kill process. This should be a one-liner.

**Session dies on FIFO close**: If the FIFO writer exits (e.g., background `(sleep N)` expires), the MCP server dies and all state is lost. Had to restart coverage, re-switch tabs, etc.

**No interactive mode**: cdp and ndp have `--mcp` mode (JSON-RPC stdio) but no simple CLI mode like `ndp eval --port 9334 "document.title"`. Every test requires full MCP handshake.

### Context Limitations

**AX tree unavailable on attached targets**: When cdp connects to an Electron app via `WithTargetID` (because `Target.createTarget` fails), the accessibility tree is unavailable. This blocked `page_snapshot`, `annotated screenshots`, `dom_diff`, and `stale ref recovery` — all the tools that need the AX tree.

**Only 2 scripts visible via ndp**: Because ndp connects after page load, the Debugger domain only sees scripts parsed during the session. The 24MB workbench bundle was already loaded. Had to use `evaluate` + filesystem access instead of `read_source` for the main code.

**Single target per session**: ndp connects to the first target it finds. Can't switch to a different page target mid-session (unlike cdp's `switch_tab`).

### Output Parsing

**Raw JSON-RPC output**: Had to write Python parsers for every test to extract tool results from raw JSON-RPC lines. No structured output mode.

**No diff between evaluate results**: Wanted to compare `evaluate` output before/after an action — had to manually save and diff.

### Missing Depth

**Can't intercept IPC messages**: Could see the IPC channel names in the preload source, but couldn't actually watch messages flowing between renderer and main process.

**Can't inspect main process**: The Electron main process (where the real app logic lives) is inaccessible via CDP. Only the renderer is exposed. For Codex, the 530KB main process bundle had to be analyzed via filesystem extraction + grep, not live inspection.

**No protobuf decoding**: Antigravity uses `@exa/proto-ts` with Connect/gRPC-Web. Saw the proto type names in the import map but couldn't decode actual messages in transit.

---

## 3. Missing Tools

### Tool 1: `reverse_app` — Automated App Fingerprinting

**Description**: One-shot command that produces a structured app profile.

**Inputs**: `{ port: number }` or `{ target_id: string }`

**Outputs**: Structured JSON with:
- App identity (name, version, electron version, chrome version, from User-Agent)
- Framework detection (React/Preact/Vue/Angular/Svelte — detected via globals, DOM attributes, script patterns)
- Bundler detection (Vite/webpack/esbuild/Rollup — from chunk naming, import map presence, source map references)
- Global objects inventory (non-standard window properties with type and key count)
- Preload/bridge API surface (contextBridge-exposed methods with signatures)
- IPC channel list (from preload source parsing)
- Script inventory (count, total size, by source domain)
- CSP policy (from meta tag)
- Feature flag provider (Statsig/LaunchDarkly/Unleash/Split)
- Error monitoring (Sentry/Bugsnag/Datadog)
- Auth mechanism (OAuth provider, token storage location)

**Why it matters**: The first 30 minutes of every reversing session is this same reconnaissance. Automating it gives instant orientation.

**Complexity**: Medium — mostly combining existing `evaluate` + `list_sources` + `search_sources` calls with pattern matching.

### Tool 2: `intercept_ipc` — Electron IPC Message Sniffer

**Description**: Monkey-patch the preload bridge to log all IPC messages.

**Inputs**: `{ channels?: string[], duration?: number }`

**Outputs**: Stream of IPC messages with: `{ timestamp, direction, channel, args, callstack? }`

**Implementation**: Inject JS that wraps `electronBridge.sendMessageFromView` (or `vscode.ipcRenderer.send/invoke`) with a logging proxy. For incoming messages, add a `window.addEventListener("message", ...)` listener. Store messages in an array, return on demand.

**Why it matters**: IPC is the nervous system of Electron apps. Antigravity routes all AI interactions through IPC. Without seeing actual messages, we only know channel names, not message formats or protocols.

**Complexity**: Low-medium. The injection is straightforward. The hard part is handling async invoke responses and correlating request/response pairs.

### Tool 3: `extract_imports` — Module Dependency Graph

**Description**: Parse JS bundles to extract import/export relationships and build a dependency graph.

**Inputs**: `{ script_id?: string, url?: string, depth?: number }`

**Outputs**: 
```json
{
  "modules": [
    { "id": "chunk-CFjPhJqf", "imports": ["react", "jsx-runtime"], "exports": ["o", "e"], "size": 4500 }
  ],
  "graph": { "index": ["chunk-CFjPhJqf", "message-bus", ...] }
}
```

**Implementation**: Use the V8 `Debugger.getScriptSource` + parse import statements (static `import` and dynamic `import()`). For AMD/CommonJS, parse `define()` and `require()` calls. Build adjacency list.

**Why it matters**: Understanding which chunk does what. Codex had 128 chunks — knowing that `app-server-manager-hooks` imports from `message-bus` which imports from `vscode-api` reveals the communication architecture without reading all the code.

**Complexity**: Medium. Regex-based parsing of import statements covers 80%. Full AST parsing (e.g., via tree-sitter) covers 95%.

### Tool 4: `walk_object` — Deep Object Graph Explorer

**Description**: Recursively explore a JS object, showing its structure with types, sizes, and sampled values.

**Inputs**: `{ expression: string, depth?: number, max_keys?: number, sample_values?: boolean }`

**Outputs**:
```json
{
  "electronBridge": {
    "_type": "object",
    "_keys": 14,
    "sendMessageFromView": { "_type": "function", "_length": 1 },
    "windowType": { "_type": "string", "_value": "electron" },
    "getSentryInitOptions": {
      "_type": "function",
      "_returns": {
        "_type": "object",
        "codexAppSessionId": { "_type": "string", "_sample": "d4e56a24-..." },
        "buildFlavor": { "_type": "string", "_value": "prod" },
        "buildNumber": { "_type": "string", "_value": "1272" }
      }
    }
  }
}
```

**Why it matters**: Currently, exploring a JS object requires writing a custom `evaluate` expression for every level. This is the most common pattern in reversing — "what's in this object? what about that sub-object?" — and it takes 3-5 evaluate calls to get what one walk_object call could provide.

**Complexity**: Medium. Recursive evaluate with depth limiting and cycle detection.

### Tool 5: `decode_protobuf` — Protobuf Message Decoder

**Description**: Given a protobuf binary or base64 blob, decode it using either proto descriptors found in the app or raw field-number decoding.

**Inputs**: `{ data: string, format?: "binary"|"base64", proto_type?: string }`

**Outputs**: Decoded message as JSON, with field numbers if type is unknown.

**Implementation**:
1. Search loaded scripts for protobuf descriptor registrations (`@bufbuild/protobuf` `createDescriptorSet` calls)
2. Extract field definitions from the JS protobuf codegen
3. For unknown types, use raw wire-format decoding (varint, length-delimited, fixed32/64)

**Why it matters**: Antigravity stores annotations as `.pbtxt` files and communicates via gRPC-Web. Codex's worker has thread/model protobufs. Without decoding, these are opaque blobs.

**Complexity**: High. Wire-format decoding is mechanical but extracting type info from JS codegen requires understanding @bufbuild/protobuf's output format.

### Tool 6: `network_log` — Live Network Request Logger

**Description**: Enable CDP Network domain events and stream request/response pairs.

**Inputs**: `{ filter?: string, include_body?: boolean, duration?: number }`

**Outputs**: Array of `{ url, method, status, headers, request_body?, response_body?, timing }`

**Why it matters**: cdp has `get_har_entries` but it only captures completed entries from the HAR recorder. For reversing, we need live capture: "click this button and see what API call it makes." The existing `intercept_request`/`intercept_response` tools modify requests — we need a read-only observer.

**Complexity**: Low. CDP's `Network.enable` + `Network.requestWillBeSent` + `Network.responseReceived` + `Network.getResponseBody` are well-documented.

### Tool 7: `heap_search` — Heap Object Finder

**Description**: Search the V8 heap for objects matching criteria (constructor name, property name, string content).

**Inputs**: `{ constructor?: string, property?: string, string_content?: string, max_results?: number }`

**Outputs**: Array of `{ id, constructor, properties, preview }`

**Implementation**: Use `HeapProfiler.takeHeapSnapshot` + parse the snapshot to find matching nodes. Or use `Runtime.queryObjects` with a prototype reference.

**Why it matters**: Finding runtime state without knowing the global path. "Show me all objects with a `token` property" or "find all instances of `AppServerConnection`" — things you can't discover through globals alone.

**Complexity**: Medium-high. Heap snapshots are large and parsing them is non-trivial. `Runtime.queryObjects` is simpler but requires a prototype reference.

### Tool 8: `coverage_explore` — Coverage-Guided Exploration

**Description**: Combined workflow: start coverage → perform action → snapshot → report which NEW code paths were activated, with the actual source lines.

**Inputs**: `{ action: { type: "click"|"evaluate"|"navigate", params: ... } }`

**Outputs**: 
```json
{
  "action": "click button.settings",
  "new_files_activated": 3,
  "new_lines": [
    { "file": "settings-content-layout.js", "lines": [45, 46, 47, ...], "source_preview": "function openSettings() {..." },
    ...
  ],
  "coverage_delta": { "before": "2.3%", "after": "5.1%", "delta": "+2.8%" }
}
```

**Why it matters**: This is the killer workflow for reversing: "I clicked Settings — which code handles that?" Currently requires: start_coverage → get_coverage(baseline) → do action → get_coverage(after) → get_coverage_delta → manually cross-reference file names → read_source for each. The combined tool would make this instant.

**Complexity**: Medium. Combines existing coverage tools with source reading. The main work is correlating line numbers with source and producing useful previews.

---

## 4. Workflow Improvements

### 1. CLI Mode for Quick Probes

Add `cdp eval --port 9334 "document.title"` and `ndp eval --port 9334 "process.versions"` — single-command evaluate without MCP handshake. Essential for shell scripting and quick probing.

### 2. Persistent Session with Named Handles

Currently: create FIFO → background process → manual lifecycle management.
Needed: `cdp connect --port 9334 --session codex-re` that creates a persistent named session, then `cdp call codex-re evaluate '{"expression":"..."}'` to send commands to it. The session survives across calls.

### 3. Batch Evaluate

`evaluate_batch` that takes an array of expressions and returns all results in one call. Currently each evaluate is a full round-trip with 2-3 seconds of FIFO plumbing overhead. 10 probes = 30+ seconds. A batch call would take 3 seconds.

### 4. Structured Evaluate Output

`evaluate` should have a `format: "json"` option that tries to JSON.parse the result and returns structured data instead of a string-escaped JSON string. Currently every result is double-escaped (`\"{\n  \"keys\": [...]\""`), requiring manual unescaping.

### 5. Source Map Auto-Resolution

When `read_source` returns minified code and the script has a `//# sourceMappingURL`, automatically try to fetch and apply the source map. Many Electron apps include source maps (Antigravity's jetskiAgent.js pointed to `https://main.vscode-cdn.net/sourcemaps/...`).

### 6. Target Selection for ndp

ndp should support `--target-id` or `--target-title` to connect to a specific page target instead of always picking the first one. Critical for multi-window Electron apps.

### 7. auto-reconnect on Target Change

When the connected target navigates or reloads, the session should auto-reconnect instead of dying. This happened multiple times during testing.

---

## 5. Antigravity/Codex-Specific Gaps

### Things Our Tools Couldn't Reach

1. **Electron main process**: The main process (where BrowserWindow management, app lifecycle, IPC routing, and the actual Codex CLI spawning happen) is completely invisible through CDP. The 530KB main process bundle had to be extracted from the asar and analyzed with grep — no live inspection possible. Would need `--inspect` on a dev build to access this.

2. **gRPC-Web traffic (Antigravity)**: Antigravity's agent communicates via Connect/gRPC-Web to Google Cloud. These are binary protobuf payloads over HTTP/2 — even if we captured the network requests, we can't decode them without the proto descriptors. The proto types exist in the `@exa/proto-ts` npm package but are bundled inside the asar.

3. **IPC message content**: We found all the channel names (`vscode:*`, `codex_desktop:*`) but never saw an actual message payload. The preload scripts show the API surface but not the data format. Would need the `intercept_ipc` tool proposed above.

4. **Codex CLI protocol**: Codex Desktop spawns `codex app-server` over stdio. The message protocol between the Electron app and the CLI is unknown — it could be JSON-RPC, protobuf, or a custom format. Would need to intercept the stdio pipe or attach to the CLI process.

5. **IndexedDB/SQLite state**: Codex uses better-sqlite3 for persistence. We couldn't inspect the database contents through CDP. Would need `evaluate` to access IndexedDB or a tool that extracts SQLite databases from the app's userData directory.

6. **Worker thread internals**: Codex has a dedicated worker process handling threads, realtime audio, and telemetry. Workers are separate V8 contexts — we could see the worker as a CDP target but didn't probe it deeply.

7. **Protobuf annotation files**: Antigravity stores 130+ `.pbtxt` files in `~/.gemini/antigravity/annotations/`. We could read the raw text (`last_user_view_time:{seconds:1769156142}`) but without the proto schema, we don't know the full message structure.

8. **Extension host**: Antigravity runs extensions in a separate Node.js process (extension host). Extensions like the Gemini agent plugin run there — not accessible via the renderer CDP connection.

### What Would Unlock the Deepest Reversing

In priority order:
1. **`intercept_ipc`** — See actual IPC messages flowing (low effort, high value)
2. **`network_log`** — Live network capture with body content (low effort, high value)  
3. **`reverse_app`** — Automated fingerprinting (medium effort, saves 30min per app)
4. **`walk_object`** — Deep object explorer (medium effort, replaces most evaluate gymnastics)
5. **`coverage_explore`** — "Click and see what code runs" (medium effort, transforms reversing workflow)
6. **`extract_imports`** — Module graph for understanding architecture (medium effort)
7. **`decode_protobuf`** — Proto message decoding (high effort, critical for gRPC apps)
8. **`heap_search`** — Find hidden state in memory (high effort, essential for deep reversing)
