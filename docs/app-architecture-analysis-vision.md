# Vision: Automated App Architecture Analysis

*2026-04-05 — Future goal, not currently planned for implementation*

## Concept

Given access to an Electron app (or any CDP/V8 target), automatically produce a structured architecture analysis: OOUX object model, statechart, process map, API surface, feature inventory, and data model. The output should be rich enough to feed into a PRD or serve as onboarding documentation.

## Current Capability

Today's inspect tools (behind `--enable-inspect`) provide the raw data extraction:

| Data | Tool | Status |
|------|------|--------|
| App identity + framework | inspect_fingerprint | Done |
| JS object graph | inspect_walk | Done |
| IPC messages | inspect_ipc_start/log | Done |
| Network traffic | start_network_log | Done |
| V8 sources | list_sources + read_source | Done |
| Runtime state | evaluate | Done |
| Electron detection | detect_electron (ndp) | Done |

An agent can already compose these tools manually to produce a full analysis (validated by A998's reversing of Antigravity, Codex, and Claude Desktop). The question is whether to automate the composition.

## What a Composed Analysis Would Produce

### 1. OOUX Object Model
- Domain objects (conversations, sessions, projects, agents, tools, files)
- Properties, types, relationships between objects
- Extracted from: IPC bridge classes, reactive stores, global state objects

### 2. Statechart / Event Model
- States derived from reactive store values
- Events derived from IPC channel names and method calls
- Transitions derived from observing state changes after IPC calls
- Key flows: conversation lifecycle, agent task execution, tool invocation

### 3. Process Architecture
- Process tree: main → renderers → workers → subprocesses
- IPC channel ownership per process
- State ownership per process
- Mermaid diagram output

### 4. API Surface
- External HTTP endpoints (from network capture)
- IPC method signatures (from bridge object walking)
- MCP server connections and tool inventories
- Auth flow endpoints

### 5. Feature Inventory
- Config flags and feature gates
- User-facing preferences
- Unreleased/gated features
- Route structure (from router walking)

### 6. Data Model
- Local storage (IndexedDB, localStorage, SQLite, config files)
- Directory structure under app data path
- Cache and state file formats

## Architecture Decision: Tool vs Agent Workflow

**Option A: Single `inspect_architecture` tool** (~500 lines)
- Runs all extraction in sequence, returns structured JSON
- Pro: one tool call, consistent output
- Con: rigid, can't adapt to unusual apps, large tool

**Option B: Agent workflow with existing tools** (current approach)
- Agent composes inspect_fingerprint → inspect_walk → evaluate probes → network capture
- Pro: flexible, adapts to each app, uses existing tools
- Con: requires many round-trips, agent must know the workflow

**Option C: Prompt template / skill** (recommended future direction)
- A documented prompt/skill that guides an agent through the analysis using existing tools
- Could be a .cdp script or MCP prompt resource
- Pro: no new code, flexible, agent can adapt
- Con: depends on agent quality

**Recommendation**: Option C for now. The tools exist. What's needed is a well-structured prompt that guides the analysis. If patterns stabilize, consider Option A as a convenience wrapper.

## Validation

A998 manually performed this analysis on three production Electron apps (2026-04-05):
- **Antigravity** (Google): VS Code fork, gRPC-Web, Jetski agent, 100+ brain sessions
- **Codex** (OpenAI): Standalone SPA, Rust CLI, Statsig, SQLite state
- **Claude Desktop** (Anthropic): Webview hybrid, EIPC protocol, ClaudeVM, Operon agent system

Each analysis took 15-30 minutes using evaluate + list_sources + search_sources + walk_object. A composed tool or skill could reduce this to 2-3 minutes.

## Files
- `/tmp/collab-A998-antigravity-reverse.md` — Antigravity analysis
- `/tmp/collab-A998-codex-reverse.md` — Codex analysis
- `/tmp/collab-A998-claude-reverse.md` — Claude Desktop analysis
- `/tmp/collab-A998-reversing-gaps.md` — Tool gap analysis from real usage
- `docs/reversing-tools-design.md` — Full reversing tool proposals
