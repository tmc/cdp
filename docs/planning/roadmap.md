# NDP Future Roadmap and Planning

This document outlines the strategic direction and planned improvements for the `ndp` (Node Debug Protocol) tool suite.

## 1. Workspace Persistence (Bidirectional Sync)

Currently, `ndp` supports **Hot Reload** (Local -> Debugger). The next major milestone is **Reverse Sync** (Debugger -> Local).

- **Objective**: Allow edits made in connected DevTools (via `ndp proxy` or `devtools` command) to be saved back to the local file system.
- **Challenges**:
    - **Source Mapping**: If a user edits a generated JS file in DevTools, how do we propagate that to the original TS source?
    - **Proxy Interception**: We need to intercept `Debugger.setScriptSource` events flowing *from* the DevTools frontend to the backend.
- **Proposed Architecture**:
    - **Proxy Mode**: Enhanced `ndp proxy` that parses WebSocket frames for `Debugger.setScriptSource`.
    - **Path Mapping**: Robust heuristic to map `file://` URLs in CDP to local disk paths (handling symlinks and remote containers).

## 2. Advanced REPL User Interface (TUI)

The current REPL is a linear CLI. A Text User Interface (TUI) would significantly improve usability.

- **Stack**: `bubbletea` + `lipgloss`.
- **Layout**:
    - **Left Pane**: File/Source Tree (Nodes, Scripts).
    - **Center Pane**: Source Code Viewer (with syntax highlighting and breakpoint indicators).
    - **Right Pane**: Call Stack, Variables (Scope), Watch Expressions.
    - **Bottom Pane**: Console/REPL Input (Logs, Network Activity).
- **Features**:
    - Mouse support for clicking breakpoints/files.
    - Tabbed views for "Network", "Profiler", "Console".

## 3. Deep Domain Integration

### Network Domain
- **Status**: Basic logging implemented.
- **Next Steps**:
    - Full request/response body capture (HAR generation support matches project root goals!).
    - `curl` export command.
    - Request blocking/mocking (CDP `Fetch` domain).

### Profiler Domain
- **Status**: Standalone `ndp profile` command exists.
- **Integration**:
    - **REPL Command**: `profile start` / `profile stop`.
    - **Visualization**: ASCII flamegraphs or export to speedscope.

### Runtime / Console
- **Object Preview**: Better expansion of complex objects in CLI (currently JSON dump).
- **Autocomplete**: IntelliSense-like completion for `exec` commands using `Runtime.globalLexicalScopeNames`.

## 4. Source Map Authorship
- **Goal**: Enable editing TypeScript/Go/Rust directly.
- **Flow**:
    - User edits `.ts` file.
    - `ndp` detects change.
    - `ndp` triggers build (e.g. `npm run build`).
    - `ndp` waits for build.
    - `ndp` hot-reloads generated `.js`.

## 5. Automated Testing & Verification
- **E2E Suite**: Containerized tests spinning up specific Node.js versions.
- **Protocol Fuzzing**: Verify robustness against varied CDP message payloads.

## 6. Remote Debugging Gateway
- **Scenario**: Debugging production/remote containers.
- **Feature**: `ndp gateway` that tunnels CDP over SSH or secure WebSocket types, presenting a local implementation for DevTools to attach to.
