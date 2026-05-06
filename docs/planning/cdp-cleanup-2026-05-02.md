# cdp repo cleanup — design doc v3 (final)

**Date:** 2026-05-02
**Source:** Two-pass NotebookLM audit + two critique loops
(`e87284ee-86ba-4c0f-9c1a-89b7752b650b`, conv `ab57ab51-3538-4f7f-aa2e-8767e826cbe2`).

**v3 changes vs v2:**
- A6 (strip ActionFunc wrappers) moved before A5 (sourcemap analyzer
  extraction) — A5 churns the same files, so do the mechanical sweep
  first.
- A5 evidence strengthened: `cmd/cdp/interactive.go:48` keeps a
  `*syntheticMapStore` field, which is a type defined in
  `cmd/cdp/mcp_sourcemap_tools.go`. The REPL imports an MCP-transport
  type. Extracting to `internal/sourcemap/analyzer.go` *fixes* this
  cross-cut, not creates one.
- A3.1 stays. The save-sources fix is in the worktree as
  uncommitted modifications (`git status` shows
  `M internal/sources/sources.go` and `M internal/sources/sources_test.go`),
  even though the file content already shows the corrected logic.
  NLM saw the file content and concluded "landed" — it isn't.
- B3 (workspaceContext design doc) **deleted entirely**. The right
  answer is a small mechanical extraction (A8 below) of the two
  shared helpers, with push/pop staying duplicated because the lock
  divergence justifies it.
- New B5: `internal/browser` OOP wrapper architectural question.
  Three of four `cmd/*` binaries still use `browser.Page`; only the
  MCP tool layer in `cmd/cdp` chose raw chromedp. Worth a doc.

NLM citation drift across both critique loops: line numbers were
frequently fabricated, but the *substance* of cited content
matched verifiable text. v3 cites only verified line numbers.

## Goals (in order)

1. Delete dead code with no live callers.
2. Hoist already-implicit invariants into package docs.
3. Reduce duplication only where the duplication is not load-bearing —
   mechanical refactors first, structural extractions after concurrency
   checks.
4. Capture the larger architectural questions as design follow-ups.

## Non-goals

- Rewriting CLI mode dispatch in this pass.
- Unifying the REPL and MCP capability registries (they have different
  contracts).
- Touching `internal/sourcemap` (extractor/generator pair is clean).
- Touching `internal/cdpproxy` or `internal/differential` without an
  explicit maintainer decision (D1, D2).

## Action plan

### A. Code-commit work (atomic, mergeable)

#### A1. Delete dead WebSocket-recorder dual implementation

**Status:** Pass-2 finding B. Dead code.

**Evidence:**
- `internal/recorder/websocket_recorder.go` (544 lines) defines
  `WebSocketRecorder` wrapping `*Recorder`.
- `internal/browser/websocket_har.go` defines `WebSocketHARConverter`.
- Outside-of-itself callers of `WebSocketHARConverter`:
  `internal/recorder/websocket_recorder.go` (lines 20, 55, 503, 529)
  and `internal/browser/websocket_test.go` (lines 362, 574 — i.e.
  tests OF the converter itself).
- Production WS path is `internal/recorder/websocket_capture.go` (added
  in 5a43d3b).
- `grep -rn 'NewWebSocketRecorder\|WebSocketRecorder\b'` outside the
  dead file: zero hits.
- `internal/` import rule: cannot be reached from outside the module.

**Critical nuance:** `internal/browser/websocket_test.go` contains
tests for **both** the dead converter and the live `WebSocketMonitor`
API (used in production via `cmd/churl/main.go:1002`):
- Live (keep): `TestWebSocketMonitoring` (line 42),
  `TestWebSocketWaitConditions` (196),
  `TestWebSocketPerformanceMonitoring` (390),
  `TestWebSocketFiltering` (494),
  `TestWebSocketMultipleConnections` (595).
- Dead (delete): `TestWebSocketHARExport` (line 290).

**Action:**
1. Delete `internal/recorder/websocket_recorder.go`.
2. Delete `internal/browser/websocket_har.go`.
3. Surgically remove only `TestWebSocketHARExport` (and any helpers
   it uniquely uses).
4. `go build ./... && go test -race ./internal/recorder/... ./internal/browser/... ./cmd/churl/...`

**Pre-commit grep:**
```
grep -rln 'WebSocketRecorder\|WebSocketHARConverter\|NewWebSocketHARConverter' .
```

**Estimated diff:** −~1100 / +0.

#### A2. Delete `internal/browser/page_options.go` compat shims

**Status:** Pass-1 finding F6.

**Evidence:**
- `internal/browser/page_options.go:171-198`: 5 wrappers, each a
  1-line `return WithFoo(...)`.
- `docs/implementation.md:35`: "Prefer deleting dead compatibility
  layers over preserving parallel implementations."
- Inbound callers are tests in the same package only:
  `page_test.go:29, 82, 98, 134, 278`, `element_test.go:345`,
  `integration_test.go:113, 157, 526, 587, 776 (stale comment)`,
  `testutil_test.go:248`.

**Action:**
1. Mechanical rename in 4 test files
   (`s/NavigateWithTimeout/WithNavigateTimeout/g` etc).
2. Delete the 5 shim functions.
3. Audit `ScreenshotFullPage`, `ScreenshotSelector` in the same file
   for whether they are also dead-shims; extend deletion if so.
4. Update the dangling comment at integration_test.go:776.
5. `go build ./... && go test -race ./internal/browser/...`

**Estimated diff:** −~30 / +0 net (rename is +/−).

#### A3. Save-sources fix + package doc

**Status:** Pass-1 finding F4 + uncommitted worktree fix.

**Pre-existing evidence:**
- `git status` shows `M internal/sources/sources.go` and
  `M internal/sources/sources_test.go` — uncommitted.
- The corrected `Enable()` body at sources.go:108-149 already shows
  the fix (arm `c.fetchCh` and `c.incremental` under `c.mu` BEFORE
  `chromedp.Run(... debugger.Enable() ...)`, with a rollback path on
  failure). Plus regression tests
  `TestDispatchBeforeEnableDoesNotPanic` and
  `TestEnableArmsIncrementalBeforeReplayBurst` in `sources_test.go`.

**A3.1 (prerequisite):** Commit the worktree changes as a single
commit:
```
internal/sources: arm incremental channel before Debugger.enable

The replay burst from Debugger.enable fires synchronously in the
chromedp event loop while Run is blocked. Setting c.incremental and
allocating c.fetchCh AFTER chromedp.Run meant the listener saw
incremental=false during the burst and dropped every replayed
scriptParsed event. Move the channel/flag setup before the Run
call (under c.mu so the listener observes the writes), with
rollback on enable failure so a retry starts clean.

Adds TestDispatchBeforeEnableDoesNotPanic +
TestEnableArmsIncrementalBeforeReplayBurst regression tests.
```

**A3.2:** Write `internal/sources/doc.go` with a 15-25 line overview
of the lifecycle invariants:
1. Register `c.Listener(ctx)` via `chromedp.ListenTarget` BEFORE
   calling `Enable()`.
2. Re-register a per-target listener before `AttachToTarget(ctx)`.
3. The `Listener` closure stamps the per-event session ctx
   (ScriptIDs are per-session, not globally unique).

Cross-reference function-level docs.

**Risk:** A3.1 — already verified passing tests in the previous
conversation. A3.2 — pure docs, zero risk.

#### A6. Strip pointless `chromedp.Run+ActionFunc` wrappers

**Status:** Critique-loop Q4.

**Evidence:**
- 30+ instances across `cmd/cdp/mcp_*.go`. Per-file counts:
  `mcp_emulation_tools.go:8`, `mcp_extension_tools.go:7`,
  `mcp_input_tools.go:7`, `mcp_action_diff.go:3`,
  `mcp_element_tools.go:2`, `mcp_custom_tools.go:2`,
  `mcp_frame_tools.go:2`, plus 1 each in others.
- Example: `cmd/cdp/mcp_frame_tools.go:40`:
  `chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error { tree, err = page.GetFrameTree().Do(ctx); ... }))`
  — equivalent to `tree, err = page.GetFrameTree().Do(actx)`.

**Action:** Per-file commits. Replace
`chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error { x, err = something().Do(ctx); return err }))`
with `x, err = something().Do(ctx)` where the body is a single
`Do(ctx)` call.

**Where to leave alone:** Any `chromedp.Run(ctx, action1, action2, ...)`
with multiple actions — that's the correct idiom.

**Estimated diff per file:** −5 to −20 lines. Total: −~150 / +~80.

#### A5. Extract sourcemap analyzer to `internal/sourcemap/`

**Status:** Pass-2 finding E. Reinstated after critique loop.

**Evidence:**
- `cmd/cdp/mcp_sourcemap_tools.go` (1239 lines) mixes:
  - Domain types: `inferredResult` (line 59),
    `syntheticMap` (lines 24-27),
    `syntheticMapStore` (line 85),
    `analysisLogEntry` (around 280-340).
  - Disk I/O: `sourcemapDiskPath`, `writeSourcemapToDisk`,
    `loadSourcemapsFromDisk`, `writeStructureSidecar`,
    `analysisLogPath`, `appendAnalysisLog`, `readAnalysisLog`,
    `countAnalysisLogEntries` (lines 118-362).
  - LLM prompt construction: `buildFunctionAnalysisPrompt`,
    `buildAnalysisPrompt` (around lines 413, 762).
  - MCP wiring: `registerSourcemapTools` (line 398) +
    `mcp.AddTool` closures.
  - Bundle/coverage helpers: `extractBundleChunks` (816),
    `extractBundleCoverage` (825), `sampleBundleAnalysis` (874).
- **Cross-cut:** `cmd/cdp/interactive.go:48` declares
  `syntheticMaps *syntheticMapStore` as an `InteractiveMode`
  field — the REPL imports a type defined in an MCP transport file.
  Lines 102-105, 1816-1820, 1848, 1862-1877 confirm REPL usage.
- `docs/implementation.md:33` mandates moving logic out of
  `package main` into focused internal packages.

**Action:**
1. Move domain types + disk I/O + prompt builders + bundle helpers
   into `internal/sourcemap/analyzer.go` (split further if needed:
   `analyzer_log.go` for the analysis log).
2. Update `cmd/cdp/mcp_sourcemap_tools.go` and
   `cmd/cdp/interactive.go` to import from `internal/sourcemap`.
3. `mcp_sourcemap_tools.go` shrinks to the MCP wiring layer plus
   the `mcp.AddTool` closures.
4. The REPL no longer cross-cuts the MCP transport file; both go
   through `internal/sourcemap`.
5. `go build ./... && go test -race ./...`

**Risk:** Medium. The split is along a real seam, and the
cross-cut from interactive.go *forces* the move (the REPL
shouldn't be reaching into mcp_sourcemap_tools.go anyway). Validate
by reading every `*syntheticMapStore` use chain before splitting.

**Estimated diff:** ~1000 lines moved. Lines reduced via
`mcp_sourcemap_tools.go` shrinkage: ~600. Net: cmd/cdp drops by
~600 lines, internal/sourcemap grows by ~700 (some new
package-doc glue).

#### A7. (Optional) Consolidate small mcp_*.go helpers

**Status:** Critique-loop Q1. Verified, smaller than NLM framed.

**Evidence:** 3 named helpers
(`parseJSON` at `mcp_action_diff.go:259`,
 `parseArguments` at `mcp_custom_tools.go:107`,
 `splitFirstArg` at `mcp_custom_tools.go:426`).

**Probe first:**
```
grep -n '^func [a-z]' cmd/cdp/mcp_*.go | grep -v '^cmd/cdp/mcp_tools' | head -30
```

If the count of small unexported helpers exceeds ~5-7, consolidate
into `cmd/cdp/mcp_util.go`. If just the 3 named, leave alone
(Russ-Cox rule: three similar lines beat a premature abstraction).

**Action contingent on probe.**

#### A8. Extract `writeCoverageLcov` + `contextOutputDir` helpers

**Status:** Was A4 in v1, reclassified to B3 in v2, demoted to a
small extraction in v3 after critique-loop pushback.

**Evidence:**
- `cmd/cdp/interactive.go:1276+` `writeCoverageLcov` (~60 lines)
- `cmd/cdp/mcp.go:152+` `writeCoverageLcov` (similar size)
- `interactive.go` `contextOutputDir()` and the equivalent on
  `mcpSession` are 5-line helpers that join `baseOutputDir` with
  `contextStack`.

**What we are NOT extracting:** push/pop themselves, because
`mcpSession` carefully drops `s.mu` before disk I/O
(`mcp.go:137`) and `InteractiveMode` is single-threaded with no
mutex at all. Sharing the full struct would either reintroduce a
stall in the MCP path or force the REPL to acquire/release a
lock it doesn't need.

**Action:**
1. New file `cmd/cdp/workspace_util.go` exporting:
   - `func contextOutputDir(base string, stack []string) string`
   - `func writeCoverageLcov(contextDir, name string, endSnap *coverage.Snapshot, snapshots []*coverage.Snapshot) error`
2. `interactive.go` and `mcp.go` keep their own push/pop, but call
   the shared helpers.

**Risk:** Low. Pure extraction of stateless helpers.

**Estimated diff:** −~120 / +~70.

### B. Design-doc work (open questions, no commit yet)

#### B1. cdp binary scope (was: CLI subcommand migration)

**Status:** Pass-1 finding F1. **Prerequisite question added.**

Before "how do we migrate `cdp -shell -harl ...` flags to subcommands,"
answer: **does `cmd/cdp` belong in the runtime path at all for those
modes?**

Plan-05 directives (lines 35-37):
> Make `cdpscript` the canonical runtime path. Keep `cdpscripttest`
> focused on `go test` integration, not as a second browser runtime.

If Plan-05 is the source of truth, the right answer for `cdp -harl`
might be **delegate to `chrome-to-har` and delete the flag from `cdp`**.

**Doc:** `docs/planning/cdp-binary-scope.md`. Per-mode question:
keep / migrate to subcommand / delegate / delete.

#### B2. Recorder lock-granularity follow-up

**Status:** Pass-1 finding F5.

**Doc:** `docs/planning/recorder-lock-granularity.md`. Document
current single-mutex design, why it was chosen (deadlock avoidance
from streamEntry under HandleNetworkEvent — see comment at
`internal/recorder/recorder.go:606-609`), failure modes, candidate
fixes (split lock, buffered writer, async-write channel), and a
**measurement plan** before any refactor.

#### B4. cdpscript ↔ cdpscripttest internal dialect duplication

**Status:** Critique-loop Q3.

**Hard precondition (maintainer-stated, 2026-05-02):** Both
`cdpscript` and `cdpscripttest` are first-class deliverables in this
repo. This finding is **only** about reducing internal duplication
between them — not about deleting either, extracting either to a
sibling repo, or shrinking either out of existence. Plan-05's
"cdpscripttest is not a second browser runtime" reads as "don't grow
parallel runtime functionality," not as license to retire it.

**Evidence:**
- `cdpscript/engine.go:345` `(*Engine).commands()` returns
  `map[string]script.Cmd`: `goto`, `wait`, `click`, `eval`, etc.
- `cdpscripttest/cmds.go:30+` `DefaultCmds()` returns its own
  `map[string]script.Cmd`: `Navigate()`, `WaitVisible()`, `Click()`,
  `SendKeys()`, etc. Comments mark "matches cmd/cdp shell"
  (lines ~38, 41, 49-51, 57-58).
- Plan-05 lines 27-32 ("share lower layers") is the design rule
  this duplication violates.

**Doc:** `docs/planning/cdpscript-cdpscripttest-share-layer.md`.
Two in-scope options: (a) `cdpscripttest` re-imports the engine's
`commands()` map and wraps each entry with a `script.Cmd`-compatible
adapter; (b) lift the per-action implementations into a new
`internal/scriptcmds/` shared by both. Both packages stay; the goal
is to delete the *duplicated implementations*, not the packages
themselves. (c) "accept duplication" is also valid if the test
harness genuinely needs different timeout/state behavior than the
runtime engine — that case must be made explicitly, not by default.

#### B5. `internal/browser` OOP wrapper deprecation question

**Status:** Critique-loop critique 6.

**Evidence:**
- `internal/browser/page.go` (883 lines), `browser.go` (1384),
  `element.go` (328) implement a Playwright-style OOP surface.
- Heavy `cmd/*` callers: `cmd/churl/main.go` (full `browser.Page`
  use), `cmd/chrome-to-har/main.go` (full `browser.Page` use),
  `cmd/cdp/main.go:904` (`enhancedCommands` is
  `map[string]func(*browser.Page, []string) error`).
- Light/no caller: the MCP tool layer
  (`cmd/cdp/mcp_*_tools.go`) — these use raw `chromedp.Run(...)`
  directly. No `browser.Page` references in any `mcp_*_tools.go`.

**The architectural question:** if Plan-05's directive is followed
and `cdp` shrinks to delegate to `chrome-to-har` and `cdpscript`,
does `internal/browser` survive as the chrome-to-har/churl
foundation, or does it get rewritten in `cdpscript`-style raw-chromedp
patterns? B1 and B5 are entangled and should be written together.

**Doc:** `docs/planning/internal-browser-future.md`. Inputs: which
binaries depend on the OOP surface, what features the OOP wrapper
provides over raw chromedp (timeouts, retry helpers, error wrapping
— enumerate), and a per-binary recommendation.

### C. Roadmap entries

#### C1. REPL ↔ MCP capability surface drift

(Unchanged from v1.) Single-line entry in `docs/planning/roadmap.md`
or `next-steps.md` (verify canonical location): "REPL/MCP capability
registries have drifted; unification pass should be scoped before
either grows further. See conv ab57ab51."

### D. Explicit decisions (maintainer call)

#### D1. `internal/cdpproxy/proxy.go`

983-line MITM CDP proxy with embedded HTML UI; one live caller in
`cmd/cdp/main.go:1481, 1676`. Decisions: promote to `cmd/cdpproxy`,
keep, delete.

#### D2. `internal/differential/` controller demotion

~3000 lines. Demote `DifferentialController` to a stateless
`Compare(a, b *har.HAR) *DiffResult`? Real downstream caller:
`cmd/chrome-to-har/main.go`. Substantial refactor; needs its own
design doc if pursued.

## Findings explicitly dropped after triage

- **NLM Pass-2 A** (`cdpscripttest` is "a product, not test helper"):
  scope-mismatch. `cdpscript` and `cdpscripttest` are first-class
  packages in this repo and are not subject to extraction or removal
  (maintainer requirement, 2026-05-02). The report generator and
  `cmd/cdpscripttest` CLI are real deliverables; webrtc fixtures are
  part of the contract. The only in-scope follow-up is the *internal
  dialect duplication* between the two packages, captured in B4 — and
  B4 is explicitly framed as "share the layer" not "delete the layer."
- **NLM Pass-2 C** (sourcemap extractor/generator share state):
  wrong. Boundary is clean. Drop.
- **Critique-loop Q2** (`monitorAllTabs` flag orphaned): NLM
  hallucinated the orphan status. The flag is wired:
  - declared at `cmd/cdp/main.go:72` (`cliRunMode` field) and 1211
  - bound at `main.go:1271` `flag.BoolVar`
  - propagated at `main.go:1425` (passed into `cliRunMode{}`)
  - read at `main.go:1882` (inside the `harMode == "enhanced"` path)
  Drop.
- **v2's B3** (workspaceContext extraction-as-design-doc):
  reclassified again to A8 (small extraction of `writeCoverageLcov`
  + `contextOutputDir` helpers). Push/pop stays per-mode because the
  lock divergence between `mcpSession` (drops mu before I/O) and
  `InteractiveMode` (no mu) justifies it.

## Suggested merge order

1. **A1** — delete dead WS recorder + surgical test removal.
   (−~1100 lines, low risk.)
2. **A2** — delete compat shims.
3. **A3.1** — commit worktree save-sources fix + regression tests.
4. **A3.2** — write `internal/sources/doc.go`.
5. **A6** — strip `chromedp.Run+ActionFunc` wrappers (per-file commits).
6. **A8** — extract `writeCoverageLcov` + `contextOutputDir` helpers.
7. **A5** — extract sourcemap analyzer to `internal/sourcemap`.
   (Last, because A6 churned the same files.)
8. **A7** (optional, contingent on probe).
9. **B1, B2, B4, B5, C1** — design-doc/roadmap PR. Land after the code
   commits.
10. **D1, D2** — wait for maintainer decision.

## Risks and mitigations

- **A1:** `internal/` rule guarantees no out-of-tree callers. Safe.
- **A3.1:** already verified by tests in the previous conversation.
- **A5:** the cross-cut from `interactive.go` to
  `*syntheticMapStore` actually argues *for* the extraction. Watch
  for nested types (`inferredResult` may need to stay exported).
- **A6:** `Do(ctx)` swap can change error wrapping if the original
  closure constructed a custom error. Per-file review catches this.
- **A8:** stateless helpers, no concurrency concerns.

## Verification step

After each commit:
```
go build ./...
go test -race ./...
```
Plus per-commit `grep` checks listed under each item.

## Validation against project rules

- **Russ-Cox style:** A1, A2, A6, A8 are deletions or simplifications.
  A5 splits a 1239-line file along a real seam. None introduce new
  abstractions.
- **No backwards-compat hacks:** A1 + A2 delete dead code outright.
- **Minimal interfaces:** A8 exports two function-level helpers, not
  a struct. Matches "minimal interfaces" rule.
- **No external deps:** All actions are intra-repo refactors.
- **`docs/implementation.md:33`:** A5 directly executes the
  "move command logic out of `package main` into focused internal
  packages" directive.
- **`docs/implementation.md:35`:** A2 directly executes the
  "delete dead compatibility layers" directive.
- **Plan-05 lines 27-37:** B1, B4, B5 are framed around the Plan-05
  directives ("share lower layers", "make cdpscript canonical",
  "cdpscripttest is not a second runtime").
