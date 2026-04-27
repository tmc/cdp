# cdp project improvements

**Status:** Updated after implementation pass
**Date:** 2026-04-24
**Scope:** Concrete improvements for `tmc/cdp` after the browser-harness and
`cdp` tool trials.

This document turns the browser-harness comparison into implementation work.
It is intentionally narrower than the gap analysis. The goal is to make `cdp`
better for agent-driven browser work without copying browser-harness's Python
architecture or adding manager layers.

## 1. Summary

`cdp` has moved from "close to parity" to "has the core parity primitives" for
local, agent-driven browser work. The first implementation pass fixed the most
confusing docs and added the two main live-operation primitives from the
browser-harness comparison: MCP raw CDP and viewport coordinate clicks.

Completed in this pass:

1. Fixed README/script-format/HAR run-mode mismatches.
2. Added MCP `raw_cdp` for arbitrary protocol calls on the active target or
   browser executor.
3. Added `coord:x,y` viewport clicks to MCP `click` and `action_diff`.
4. Documented the screenshot-first live agent loop in the existing
   `operating-cdp-cli` skill.
5. Added/kept offline cdpscript interaction fixtures running through the real
   `cdpscript` runtime via `cdpscripttest.RunCDPScript`.

NotebookLM review added one important correction: before adding more harness
surface, delete or collapse code that violates the Go-native constraints. That
cleanup pass is now complete for the identified items: NDP session-manager
state was collapsed, legacy interactive raw-CDP stubs now execute through a
shared raw-CDP path, and `cdpscript` no longer parses `meta.yaml`.

The remaining work is narrower:

1. Harden attach/recovery UX for already-running browsers and stale targets.
2. Expand offline fixture coverage for mechanics not yet proved locally.
3. Keep existing domain-aligned txtars verified before designing any domain
   catalog.

## 2. Constraints

- Stay Go-native.
- Do not port browser-harness.
- Do not add a manager layer, retry framework, or remote-session router.
- Keep `cmd/ndp/session_manager.go` as a stateless compatibility shim only;
  do not reintroduce session maps, routers, or save/load persistence.
- Do not add a prose `domain-skills/` tree now.
- Domain learning should become executable `txtar` examples or fixtures unless
  a real workflow proves prose is necessary.
- Do not reintroduce `meta.yaml`; txtar comments are human-facing only, while
  runtime configuration belongs in flags, environment, argv, or Go options.
- Prefer files as the index: fixtures, examples, and skills should prove the
  state of support.

## 3. Evidence from the trials

### 3.1 What browser-harness proved

The browser-harness trial successfully ran against a headless Brave CDP
websocket using:

- `new_tab`
- `wait_for_load`
- `page_info`
- screenshot capture
- JavaScript evaluation
- coordinate click
- event draining
- raw `cdp("Runtime.evaluate")`
- `http_get`

The useful lessons are small:

- A raw CDP helper must be present in the normal agent path.
- Coordinate clicks are a first-class live-browser primitive, not a fallback
  hidden behind selectors.
- Stale-session recovery and dialog visibility should be surfaced early.
- A simple doctor/setup command is valuable when browser attachment fails.
- Durable site knowledge is worth keeping, but in `cdp` it should start as
  executable txtar examples, not prose-only skills.

### 3.2 What browser-harness did not prove

The trial also showed browser-harness weaknesses that `cdp` should avoid:

- macOS Brave discovery was incomplete.
- stale `DevToolsActivePort` files caused misleading failure modes.
- several advertised interaction-skill files were stubs.
- README and install docs made `/tmp` installs easy to create accidentally.
- title mutation as a visibility marker is observable by tests and scripts.

Do not copy those behaviors.

### 3.3 What `cdp` proved

The `cdp` trial succeeded with:

- headless CLI navigation, JavaScript, screenshot, render, extract, and HAR
  capture;
- a basic `cdpscript` txtar;
- `go test -tags cdp ./cdpscripttest/`;
- browser discovery for Brave, Chrome, and Chrome for Testing;
- a broad MCP surface across DOM, tabs, dialogs, cookies, PDF, input,
  extension, sourcemap, trace, network, and page-artifact tools.

This validates the existing direction: MCP for live operation, `cdpscript` for
repeatable scripts, and `cdpscripttest` for offline regression fixtures.

### 3.4 What `cdp` did not prove

The trial found these practical gaps; current status is:

- MCP raw CDP is implemented as `raw_cdp`.
- MCP coordinate click is implemented as `coord:x,y`.
- README and script-format examples now match current invocation.
- `cdp --har` no longer silently becomes an implicit shell path for bare HAR
  usage, and docs distinguish bounded capture from interactive shell capture.
- The attach/recovery workflow now has a `cdp attach` path that probes live
  DevTools endpoints, lists attachable page targets, and prints launch
  instructions when no target is available.
- Several mechanics still need local fixtures or explicit live-only notes.

## 4. Priority work

### P0: Fix doc and existing-command mismatches

Status: done.

Fix documentation and command behavior that caused the `cdp` trial to fail or
detour before adding new capabilities:

- README examples must show current `cdpscript` invocation accurately.
- Script examples should use txtar archives with `main.cdp`.
- Plain `.cdp` execution should not be documented as a `cdpscript` input.
- HAR examples should be explicit about one-shot capture versus interactive
  shell behavior.
- `cdp --har` one-shot examples should include enough context, such as `--url`,
  to exit cleanly instead of dropping agents into an interactive shell.

Why first: agents copy README examples. Broken examples waste runs and hide
the capabilities that already work.

### P1: Add raw CDP over MCP

Status: done for MCP and interactive shell.

Add a single MCP tool that sends arbitrary CDP methods against the active
target.

Suggested shape:

```json
{
  "method": "Runtime.evaluate",
  "params": {
    "expression": "document.title",
    "returnByValue": true
  }
}
```

Design rules:

- Keep the tool small.
- Do not add typed wrappers as part of this work.
- Reuse existing active-target/session plumbing.
- Return the raw result object as structured JSON where possible.
- Report CDP errors without panicking or hiding the original method.

Implemented name: `raw_cdp`.

Implementation note: keep this with the existing MCP custom/tool execution
surface rather than creating a new package or control layer. It is a thin
built-in MCP tool, not a typed CDP wrapper framework.

Tests:

- Unit-test argument validation. Done.
- Add an MCP-level or tool-executor test for `Runtime.evaluate`. Still open.
- If the test harness allows it, add a local page test for `Page.navigate`
  followed by `Runtime.evaluate`. Still open.

Remaining raw-CDP work:

- `cdpscript` does not yet have a raw CDP command. Add one only if real scripts
  need it; otherwise keep raw CDP as a live MCP/shell escape hatch.

Why high priority: this unblocks every CDP feature not yet wrapped by
`cmd/cdp` tools and matches the most important browser-harness primitive.

### P2: Add coordinate input to MCP `click`

Status: done for MCP `click`, `action_diff`, `cdpscript`, and fixture coverage.

Extend the existing MCP `click` tool without widening `ClickInput` into
unrelated optional fields. Keep the single string selector/ref input and add an
explicit coordinate syntax parsed before normal selector handling.

Suggested syntax:

```text
coord:100,200
```

The existing `selector` field would then accept:

- CSS selectors;
- `@ref` values from `page_snapshot`;
- `coord:x,y` viewport coordinates.

Behavior:

- reject malformed coordinate strings;
- use the existing `input.DispatchMouseEvent` pattern for coordinate clicks;
- preserve current selector/ref behavior;
- document viewport-coordinate semantics.

Tests:

- Existing selector/ref click tests must still pass. Done.
- Parser validation tests are present. Done.
- If a test-only `Click` helper remains in `cdpscripttest`, align its
  `coord:x,y` behavior with MCP or delete the duplicate path. Done.
- Add a local fixture that clicks a visible element by coordinate. Done.
- Add at least one iframe-oriented fixture if it is reliable in headless CI.
  Still open.

Why second: coordinate clicks are the main live-operation advantage
browser-harness demonstrated, and they are especially useful across iframes,
shadow DOM, and visually obvious UI.

### P3: Document the live agent loop

Status: done.

Update the existing operating-CLI skill documentation to make the live browser
loop obvious:

1. observe with screenshot and page info;
2. choose coordinate click, `@ref`, selector, or JavaScript deliberately;
3. act;
4. screenshot again;
5. fall back to raw CDP when no helper exists.

The skill should say when not to use coordinates:

- repeatable scripts that need stable selectors;
- hidden inputs or zero-size elements;
- flows sensitive to viewport, zoom, or layout;
- assertions that should use DOM state instead of pixels.

This should link to existing screenshot, iframe, upload, dialog, storage, and
script-format skills rather than duplicating them. Do not create a separate
skill unless the operating-CLI skill becomes too large to navigate.

### P4: Add missing interaction fixtures

Status: partially done.

Add focused `cdpscripttest` fixtures for mechanics that matter and can run
offline:

- dialogs;
- downloads;
- uploads;
- dropdowns;
- same-origin iframes;
- shadow DOM;
- tabs;
- viewport;
- cookies/storage;
- PDF output;
- coordinate click.

Rules:

- Use local fixture pages only.
- No third-party network.
- Keep each fixture small.
- If a mechanic is live-only or not reliable in CI, document that in the
  relevant skill instead of pretending there is fixture coverage.

Current fixture coverage includes blocking scripts, form submission, reload
behavior, keyboard input, snapshot refs, sourced helper commands, and
coordinate clicks. Those now run through the real `cdpscript` runtime instead
of a parallel test-only command table. Remaining fixture priorities are
dialogs, uploads, same-origin iframe switching, storage/cookies, and PDF
output.

### P5: Improve attach and recovery UX

Status: done for the first diagnostic/operator path.

Do not create a session manager. Use the existing discovery and
session-detection code. `cmd/ndp/session_manager.go` is now a stateless
compatibility shim; remote endpoints belong in flags, MCP connection payloads,
or stateless diagnostics, not a persisted daemon-like state layer.

Improvements:

- add a single `cdp attach` path, or equivalent documented command sequence,
  that returns an agent-readable list of attachable targets or exact shell
  instructions for launching Chrome/Brave with remote debugging. Done.
- return concrete diagnostics from the existing MCP connection flow when
  attach fails. Partially done through CLI attach diagnostics; MCP-specific
  wording can still improve.
- document the shortest path for attaching to an already-debuggable browser.
  Done in `docs/cdp.md`.
- add next-step hints when no debug port is enabled. Done.
- verify stale `DevToolsActivePort`-style failures with actual port probes.
  Done by probing `/json/version` and `/json/list`; no state file is trusted.
- prefer existing remote host/port/tab flags for remote targets. Done.
- document how to list, choose, and switch tabs in agent workflows. Done for
  `cdp attach`; broader MCP operator docs can still improve.

This is important, but it should stay diagnostic and connective. It must not
become a session manager.

### P6: Continue domain-aligned examples

Status: existing examples prove the shape; durability is still open.

The repository already has useful examples such as Google-cookie, Gemini-key,
Google Docs, NotebookLM, `examples/extract-gemini-apikey.txtar`, and
`examples/gdoc-to-markdown.txtar`. The next action is not to invent a catalog
or add speculative examples. It is to keep improving existing txtars and wire a
small representative subset into CI, nightly checks, or an explicit live-only
verification workflow so they do not bitrot.

Rules:

- improve or add executable examples, not prose-only domain skills;
- prefer single-file txtars until repeated use justifies organization;
- include a short txtar header that explains purpose, inputs, and safety;
- do not store secrets, cookies, or user-specific state;
- add local tests only when the workflow can be made offline.

Do not create a catalog until examples become hard to find.

## 5. Suggested atomic commits

Completed review units:

1. `cmd/cdp: fix HAR run mode docs`
2. `cmd/cdp: add MCP raw CDP controls`
3. `skills: document live cdp agent loop`
4. `cdpscripttest: run cdpscript fixtures directly`
5. `docs: point cdp script format to canonical skill`
6. `cmd/cdp: execute raw CDP shell commands`
7. `cdpscript: drop meta yaml runtime config`
8. `cmd/ndp: collapse session manager state`
9. `cdpscript: support coordinate clicks`
10. `cmd/cdp: add attach diagnostics`
11. `cmd/cdp: harden attach diagnostics`
12. `docs: refresh cdp usage examples`
13. `cmd/cdp: test interaction timeout units`
14. `docs: sync known cdp issues`
15. `docs: make readme har example headless`

Next suggested atomic commits:

1. dialogs/uploads/storage/PDF fixture batch, split if any fixture is noisy.
2. target-switch operator notes beyond the current `cdp attach` path.
3. CI/nightly coverage for one or two existing domain-aligned txtar examples.

Planning docs should remain uncommitted unless explicitly requested.

## 6. Effective improvement milestone

The project can claim the next practical milestone when:

- README examples match real commands. Done.
- MCP has raw CDP and coordinate click. Done.
- the live-agent-loop skill exists. Done.
- speculative session-manager, raw-CDP stub, and `meta.yaml` design centers are
  removed or explicitly quarantined. Done.
- high-value mechanics have fixtures or explicit live-only notes. Partial.
- attach failure tells the agent the next action. Done for CLI attach.
- at least one existing domain-aligned txtar has a clear header and a
  verification path. Open.

At that point, the remaining browser-harness advantage is mostly cultural:
whether agents consistently capture durable site learnings after real tasks.
