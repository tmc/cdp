# Advanced Capabilities Vision

Ideas that build on our unique combination of CDP source access, code coverage, and agentic analysis. These go beyond feature parity with existing tools into genuinely new territory.

## 1. Behavioral Sourcemap Generation

**Problem:** Many production web apps ship bundled JS with no sourcemaps. Debugging, understanding, or modifying their behavior requires reverse-engineering minified bundles — a painful manual process.

**Insight:** Coverage deltas from user actions label chunks of a bundle with semantic meaning. Combined with LLM analysis of the extracted code, we can reconstruct sourcemaps without access to the build system.

**Flow:**
1. Start coverage on a bundled app (no sourcemaps available)
2. Carry out a series of distinct actions, snapshotting coverage after each
3. Each snapshot labels byte ranges in the bundle with the action that triggered them
4. Extract the actual code for each range via `get_source`
5. An LLM analyzes the extracted chunks — identifies function boundaries, variable names, module patterns, infers original file structure
6. Generate synthetic sourcemaps mapping bundle ranges to inferred original files

**Multi-action clustering:**

| Action | Bundle ranges exercised |
|---|---|
| Click "Login" | bundle.js:45000-47200 |
| Type in search box | bundle.js:12000-13500, 89000-89800 |
| Open settings panel | bundle.js:67000-72000 |
| Add item to cart | bundle.js:23000-25100, 89000-89400 |

From this table:
- Ranges cluster into inferred modules (auth, search, settings, cart)
- Shared ranges (89000-89800) suggest utility/shared code
- Function names can be inferred from UI context ("the function at 45000 handles login submission")
- An agent builds a sourcemap: `bundle.js:45000` -> `auth/login.ts:1`

**Output:** Synthetic sourcemap + reconstructed source tree. lcov-compatible, loadable in VS Code, works with standard DevTools.

**Prerequisites:** Sources on disk (`--save-sources`), coverage with deltas (push_context/pop_context auto-snapshots), LLM integration for code analysis.

---

## 2. Coverage-Guided Exploration

**Problem:** Agents automating unknown web apps don't know what they haven't tested. They click around but miss entire features.

**Insight:** Coverage data tells you exactly how much of the app's code has been exercised. An agent can use uncovered code as a signal for unexplored functionality.

**Flow:**
1. Agent explores the app, building coverage over time
2. Periodically check: which large code regions are still uncovered?
3. Read the uncovered source — LLM identifies what it does ("this looks like a settings panel", "this handles file upload")
4. Agent targets those features: finds UI paths that would trigger the uncovered code
5. Repeat until coverage plateau

**Use cases:**
- Automated QA: "explore this app until you've covered 80% of the JS"
- Security auditing: "find and exercise all input handling code"
- Documentation: "discover every feature in this app and describe what it does"

---

## 3. Runtime Dependency Mapping

**Problem:** Understanding how a web app's modules interact at runtime (not just static imports) is hard. Build-time dependency graphs miss dynamic imports, lazy loading, and runtime-wired event handlers.

**Insight:** Coverage deltas from atomic actions show which modules activate together. Co-activation patterns reveal runtime dependencies that static analysis misses.

**Flow:**
1. Instrument with coverage
2. Perform atomic actions (one click, one navigation, one form submit)
3. For each action, record which scripts/ranges activated
4. Build a co-activation graph: modules that fire together are runtime-coupled
5. Overlay on the source tree to show actual runtime dependency flow

**Output:** A graph where nodes are inferred modules and edges are "these activate together when the user does X." Fundamentally different from a static import graph — this shows what actually happens.

---

## 4. Change Impact Analysis

**Problem:** Before deploying a code change, you want to know "what user-visible behavior does this code affect?" Static analysis gives you call graphs but not UI impact.

**Insight:** If you have coverage maps from previous action-labeled sessions, you can reverse the lookup: given a code range, which actions touch it?

**Flow:**
1. Build a coverage database from labeled exploration sessions (action -> code ranges)
2. Given a changed file/function, query: which actions exercise this code?
3. Those actions are the test cases most likely to catch regressions
4. Prioritize automated re-testing on those specific flows

**This inverts the usual testing question** — instead of "did my tests cover the change?" it answers "which user flows does this change affect?"

---

## 5. Progressive App Understanding

**Problem:** An agent encountering an unknown web app for the first time needs to build a mental model: what are the features, how do they work, what code implements them?

**Insight:** Combine several tools into an autonomous understanding loop:

1. **Accessibility snapshot** — understand the UI structure and available interactions
2. **Action + coverage delta** — understand what code each interaction triggers
3. **Source reading** — understand what the triggered code does
4. **Network observation** — understand what APIs each action calls
5. **HAR analysis** — understand data flow patterns

Each iteration deepens the agent's model. After N iterations, the agent has:
- A feature inventory (from accessibility + interaction exploration)
- A code map (from coverage + source reading)
- An API map (from network observation)
- A behavioral model (from action -> code -> API correlations)

This is essentially automated reverse engineering of a web application, built from tools we already have or are building.

---

## 6. Live Code Annotation

**Problem:** Reading minified/bundled code is useless for humans and hard for LLMs. Even with sourcemaps, understanding unfamiliar code requires context.

**Insight:** Coverage data + behavioral labels can annotate code in-place:

```javascript
// [COVERAGE] This function executes when: "Login" button clicked
// [COVERAGE] Called 3 times during exploration, 0 errors
// [NETWORK] Triggers POST /api/auth/login
function Xr(e, t) {
  // [INFERRED] Parameter e: login form data (email, password)
  // [INFERRED] Parameter t: callback on auth success
  return fetch("/api/auth/login", {
    method: "POST",
    body: JSON.stringify(e)
  }).then(r => r.json()).then(t)
}
```

Generate these annotations automatically from coverage + network + source data. Output as a VS Code extension, an HTML report, or inline comments in the reconstructed source tree.

---

## 7. cdpscripttest: Go Coverage Tools for Web Apps

**Problem:** Go has excellent coverage tooling (`go test -coverprofile`, `go tool cover`), but it only covers Go code. Integration tests that exercise web apps via browser automation produce no frontend coverage data, even though they're testing both the backend and frontend together.

**Insight:** cdpscript tests already drive browser actions. Adding CDP coverage collection to the test harness bridges browser-side JS/CSS coverage into Go's testing ecosystem.

**API sketch:**

```go
func TestLoginFlow(t *testing.T) {
    result := cdpscripttest.Run(t, "login-flow.cdp",
        cdpscripttest.WithCoverage(true),
        cdpscripttest.WithSourcemaps(true),
    )

    // JS/CSS coverage from the browser actions
    t.Logf("JS coverage: %.1f%%", result.Coverage.JSPercent)
    t.Logf("CSS coverage: %.1f%%", result.Coverage.CSSPercent)

    // Per-file breakdown (sourcemapped to original files)
    for _, f := range result.Coverage.Files {
        t.Logf("%s: %.1f%% (%d/%d lines)", f.Path, f.Percent, f.Hit, f.Total)
    }

    // lcov output for CI integration (Codecov, Coveralls, etc.)
    result.Coverage.WriteLCOV("coverage/login-flow.lcov")

    // Per-action deltas (each cdpscript command is a boundary)
    for _, step := range result.Steps {
        t.Logf("  %s: +%.1f%% coverage (%d new lines)",
            step.Command, step.CoverageDelta, step.NewLinesHit)
    }
}
```

**What this enables:**
- `go test -run TestLoginFlow` produces both Go and JS/CSS coverage
- CI pipelines enforce frontend coverage thresholds from integration tests
- Coverage reports merge with Go coverage in unified dashboards
- Per-cdpscript-command deltas show which test steps exercise which code
- Sourcemapped output points to original `.ts`/`.jsx` files, not bundles

**Implementation:**
- `cdpscripttest.Run` wraps script execution with `Profiler.startPreciseCoverage` + `CSS.startRuleUsageTracking`
- Each cdpscript command boundary triggers `Profiler.takePreciseCoverage` for per-step deltas
- On completion, map byte offsets -> line numbers -> sourcemapped originals
- Output lcov format for standard tooling, plus structured Go data for assertions
- `WithSourcemaps(true)` resolves sourcemaps and reports against original files

**The key insight:** This makes `go test` the single command that produces coverage for your entire stack — Go backend + JS/CSS frontend — from the same integration test. No separate JS test runner, no Playwright config, no coverage merging scripts.

---

## 8. DevTools Coverage Extension — Tagged Coverage Profiles

**Problem:** Chrome DevTools Coverage tab shows one flat cumulative view. No way to tag coverage by workflow step, compare profiles, or see "this code ran during login but not during checkout." Our push_context/pop_context system produces beautifully labeled coverage deltas, but there's no native way to visualize them in the browser.

**Solution:** A Chrome DevTools extension that adds a custom panel for browsing tagged coverage profiles.

**Extension panel UI:**

```
┌─────────────────────────────────────────────────────────────┐
│  CDP Coverage Explorer                              [⟳] [⚙] │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Context Timeline                                           │
│  ┌──────┐ ┌──────────┐ ┌──────────┐ ┌─────────┐           │
│  │ init │→│  login   │→│dashboard │→│settings │           │
│  │ 12%  │ │  +8%     │ │  +15%    │ │  +5%    │           │
│  └──────┘ └──────────┘ └──────────┘ └─────────┘           │
│           ▲ selected                                        │
│                                                             │
│  Delta: login (45→53% total, +347 lines)                   │
│  ┌──────────────────────────────────────────���──────────┐   │
│  │ File                        Lines   Delta   Funcs   │   │
│  │ src/auth/login.ts           67/120  +67     +4      │   │
│  │ src/auth/session.ts         45/89   +45     +3      │   │
│  │ src/api/client.ts           23/156  +23     +2      │   │
│  │ src/utils/validation.ts     12/34   +12     +1      │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  [View in Sources] [Export lcov] [Compare with...]          │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Key features:**
- **Context timeline** — horizontal bar showing each push/pop context with its coverage delta percentage
- **Click a context** — see which files/functions that context exercised (the delta)
- **Compare mode** — select two contexts, see what code differs between them
- **View in Sources** — click a file to jump to it in the Sources panel with coverage gutter highlights for that specific context
- **Cumulative toggle** — switch between "what did this context add" vs "total coverage through this point"
- **Export** — lcov per-context or cumulative, for CI integration

**Communication with cdp:**
- Extension connects to cdp MCP server via WebSocket (or reads coverage JSON files from disk)
- cdp writes coverage snapshots to a well-known location; extension watches via filesystem or push
- Alternatively, cdp exposes a small HTTP endpoint serving coverage data (simpler than WebSocket for a DevTools extension)

**Implementation:**
- DevTools extension: ~300-500 lines (manifest.json, devtools.html, panel.html, panel.js)
- Uses `chrome.devtools.inspectedWindow` API to interact with the inspected page
- Uses `chrome.devtools.panels.sources.createSidebarPane` or a full custom panel
- Coverage data is JSON — the extension just renders it, all computation happens in cdp
- Could also use `chrome.devtools.panels.openResource` to navigate Sources panel to specific files

**Why an extension vs a web page:**
- Lives inside DevTools — no extra window, integrated workflow
- Can navigate the Sources panel programmatically (openResource)
- Can add sidebar panes to existing panels
- Feels native, not bolted on

**Stretch: synthetic sourcemap serving**
- The extension could coordinate with cdp's Fetch intercept to serve evolving synthetic sourcemaps
- As the agent infers file boundaries, the extension triggers a sourcemap update
- User sees the Sources panel progressively restructure from `bundle.js` into a real source tree
- Coverage re-renders against the new structure — the "live reverse engineering" experience

---

## Implementation Dependencies

| Capability | Requires |
|---|---|
| Behavioral sourcemaps | Sources + Coverage + LLM |
| Coverage-guided exploration | Coverage + Accessibility + Agent loop |
| Runtime dependency mapping | Coverage + Sources |
| Change impact analysis | Coverage database + Sources |
| Progressive app understanding | All tools (accessibility, coverage, sources, network, HAR) |
| Live code annotation | Coverage + Sources + Network + LLM |
| cdpscripttest coverage | Coverage + Sources + cdpscripttest harness |
| DevTools coverage extension | Coverage snapshots + push/pop context data + DevTools extension API |

All build on the Sources (Phase 2, item 7) and Coverage (Phase 2, item 8) foundations from the gap analysis. The tools are the foundation; these capabilities are what you build on top.
