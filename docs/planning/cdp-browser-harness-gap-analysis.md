# cdp vs browser-harness gap analysis

**Status:** Updated after implementation pass
**Date:** 2026-04-24
**Scope:** `tmc/cdp` compared with `browser-use/browser-harness`

This document asks whether `tmc/cdp` is effectively equivalent to
browser-harness for agent-driven browser work. "Equivalent" means comparable
outcomes for an agent and user, not identical internals.

## 1. Short answer

Closer, but not fully yet.

`tmc/cdp` now has most of the ingredients for a stronger, Go-native harness:
a real CDP CLI, MCP tools, a txtar runtime, a `go test` fixture harness, and
agent-facing skills for several browser mechanics. The first implementation
pass added the two biggest missing live-operation primitives: MCP `raw_cdp`
and viewport coordinate clicks with `coord:x,y`. It also made the
screenshot-first loop explicit in the operating skill and fixed the misleading
HAR/script-format documentation.

Browser-harness is still ahead in the integrated attach/recovery experience
and in the habit of capturing learned site knowledge by default. `cdp` should
close those gaps through diagnostics, fixtures, and executable txtars rather
than by porting browser-harness's Python architecture.

NotebookLM review also identified architecture debt inside the current tree
that conflicts with this direction. That cleanup is now complete:
`cmd/ndp/session_manager.go` is a stateless compatibility shim, legacy
interactive raw-CDP stubs now execute through a shared raw-CDP path, and
`cdpscript` no longer parses `meta.yaml`.

The right goal is not to port browser-harness. The right goal is to make
`cdp` reach the same practical outcomes through Go-native pieces:

- `cmd/cdp` and MCP tools for live operation.
- `cdpscript` for executable txtar automations.
- `cdpscripttest` for offline regression fixtures.
- Agent skills for reusable mechanics.
- Domain-aligned `cdpscript` examples for reusable site workflows, organized
  only after real scripts justify the structure.

## 2. Equivalence criteria

`cdp` should be considered effectively equivalent when an agent can do these
without rediscovering harness details:

1. Attach to the browser the user is already using, recover from stale targets,
   and keep visible tab state understandable.
2. Use screenshots as the default observation loop, then perform visible
   interactions with reliable coordinate or element-ref actions.
3. Fall back to raw CDP for anything not covered by a named helper.
4. Handle common browser mechanics: dialogs, downloads, uploads, cookies,
   storage, scrolling, iframes, cross-origin iframes, shadow DOM, dropdowns,
   tabs, viewport emulation, network inspection, screenshots, and PDFs.
5. Save durable automation as executable `cdpscript` txtar files.
6. Test mechanics offline with `cdpscripttest` fixtures.
7. Accrete domain knowledge in a way that is runnable, reviewable, and easy
   for users to find.
8. Use existing remote-target flags when local user-browser attachment is the
   wrong model.

## 3. Current `cdp` strengths

`cdp` has stronger foundations than browser-harness in several areas:

- Go implementation with a real package boundary story and standard Go tests.
- A broad CLI/MCP surface in `cmd/cdp` for DOM, input, network, dialogs,
  storage, screenshots, PDFs, source maps, tracing, and page artifacts.
- Existing browser discovery and session-detection packages for browser
  installation lookup, DevTools port verification, tab enumeration retries,
  and Brave session-isolation edge cases.
- `cdpscript`, a txtar runtime for repeatable automation, now with positional
  args and runner-supplied environment values.
- `cdpscripttest`, a `go test` harness with offline pages, script assertions,
  artifact capture, and a bridge to run real `cdpscript` fixtures.
- Test-only capabilities that already go beyond browser-harness's core helper
  set, including screenshot comparison, WebRTC inspection, network emulation,
  DOM snapshots/diffs, and source-map tooling.
- Existing skills for operating the CLI, writing scripts, network capture,
  dialogs, DOM changes, storage, screenshots, emulation, iframes, uploads, and
  scrolling.
- Better long-term fit for CI and regression testing than browser-harness's
  intentionally mutable Python-helper model.

These are not small advantages. They mean `cdp` should not copy the
browser-harness architecture.

## 4. Major gaps

### 4.0 Architecture debt that conflicts with the target shape

The plan says not to add a manager layer, retry framework, remote-session
router, or metadata design center. The identified drift has been removed or
quarantined:

- `cmd/ndp/session_manager.go` no longer tracks a session map or persists
  `~/.ndp/sessions` save/load state.
- interactive `Domain.method {json}` handling now uses a shared raw-CDP
  executor instead of hardcoded example stubs.
- `cdpscript` no longer parses `meta.yaml`; checked-in txtars and docs now use
  txtar comments for human-facing headers and runner options for runtime
  configuration.

This cleanup keeps `cdp` from recreating the browser-harness manager model it
is explicitly trying to avoid.

### 4.1 Integrated agent loop

Browser-harness has a single dominant loop:

1. `screenshot()`
2. inspect the visible page
3. `click(x, y)` or type
4. `screenshot()` again
5. use `js(...)` or `cdp(...)` only when pixels are the wrong tool

`cdp` has many equivalent pieces, and the operating skill now documents the
preferred screenshot-first workflow for live browser operation while preserving
element refs and selectors for repeatable scripts.

Status: mostly addressed. Remaining work is examples and fixtures that show
agents following the loop in real tasks.

### 4.2 User-browser bootstrap and recovery

Browser-harness is highly opinionated about connecting to the user's already
running Chrome. It documents setup, the Chrome remote-debugging checkbox,
`DevToolsActivePort`, stale daemon sessions, profile picker behavior, and
tab-visibility traps.

`cdp` already has lower-level discovery and session-detection code for browser
lookup, DevTools probing, tab enumeration retries, and some Brave-specific
isolation cases. The gap is not "invent browser bootstrap." The gap is turning
those pieces into one agent-facing path with the same clarity browser-harness
has. The desired behavior is:

- try the existing browser first;
- explain only the next required user action;
- recover stale targets without making the agent diagnose low-level CDP state;
- keep the visible tab and the controlled target aligned.

Gap: wire the existing discovery/session-detector behavior into a single
documented agent setup and recovery path, with tests where the behavior is not
OS- or browser-UI-dependent.

### 4.3 Compositor-level input

Browser-harness explicitly prefers coordinate clicks because Chrome performs
hit testing through iframes, cross-origin frames, and shadow DOM at the
browser/compositor layer.

`cdp` has input tools and accessibility refs, but its docs still lean toward a
selector/ref mental model. That is useful for scripts, but not always best for
live visual operation.

Status: addressed for the core surfaces. MCP `click`, MCP `action_diff`,
`cdpscript`, and `cdpscripttest` now accept or cover `coord:x,y` viewport
coordinates. Remaining work is more examples showing when to choose
coordinates, accessibility refs, CSS selectors, or JavaScript.

### 4.4 Raw CDP escape hatch

Browser-harness keeps a simple `cdp("Domain.method", **params)` escape hatch
in the normal helper surface. This matters because the harness deliberately
does not wrap every CDP feature.

`cdp` has deep CDP capability, and MCP now exposes a `raw_cdp` escape hatch for
active-target and browser-level methods. The escape story should still become
equally obvious from each user-facing mode:

- live CLI/MCP;
- `cdpscript`;
- `cdpscripttest` fixtures.

Status: mostly addressed. MCP `raw_cdp` exists with validation tests, and
interactive shell raw-CDP commands use the same shared executor. Remaining
work is a live/tool-level smoke test.

### 4.5 Interaction mechanics coverage

Browser-harness ships skills for:

- connection
- cookies
- cross-origin iframes
- dialogs
- downloads
- drag and drop
- dropdowns
- iframes
- network requests
- print as PDF
- profile sync
- screenshots
- scrolling
- shadow DOM
- tabs
- uploads
- viewport

`cdp` has a larger tool surface than browser-harness's small Python helper
set, so browser-harness should not be treated as the ceiling. Current
`cdpscripttest` interaction fixtures cover important basics such as blocking
scripts, form submission, history navigation, keyboard enter, snapshots,
source helpers, and local pages. They do not yet prove the full
browser-harness mechanics list, and they do not make it obvious which
capabilities are intentionally live-only.

Gap: use checked-in fixtures and skill docs as the source of truth. If a
mechanic matters, it should have a local fixture or an explicit live-only note
in the relevant skill. Do not create a separate tracking layer.

### 4.6 Domain knowledge accretion

Browser-harness treats domain skills as the primary compounding mechanism.
Agents are expected to contribute site-specific learnings back under
`domain-skills/<site>/`.

For `cdp`, a plain documentation-only `skills/domain` tree is not the right
next step. The better fit is domain-aligned, executable `cdpscript` examples
or `cdpscripttest` fixtures that can contain:

- `main.cdp`
- helper `.cdp` files
- helper `.js` files
- fixture data
- a short txtar header explaining purpose and inputs
- no required metadata file

Do not design a catalog ahead of repeated use. Existing scripts such as
`examples/extract-gemini-apikey.txtar` and `examples/gdoc-to-markdown.txtar`
already prove the executable domain-script shape. Organize them per domain only
when repeated use makes that organization necessary.

Gap: keep existing domain-aligned txtars from bitrotting by adding a clear
header convention and a CI, nightly, or explicit live-only verification path.

### 4.7 Remote and parallel browsers

Browser-harness supports local and remote browser daemons. Remote browsers are
useful for parallel subagents, headless servers, proxies, and isolated logged
in profiles.

`cdp` should not copy Browser Use cloud semantics or invent a session-routing
layer. The CLI already has remote-target attachment concepts such as remote
host/port and tab selection. Any existing stateful session manager should be
removed or reduced to stateless diagnostics. The first gap is showing agents
how to use the existing flags and when to stop.

Gap: document the existing remote host/port/tab path for agent use. Defer
named sessions, profile isolation, and cleanup policy until a concrete user
workflow requires them.

### 4.8 `cdpscript` format and metadata

The current direction is intentionally Go-native: txtar files, a required
`main.cdp`, optional supporting files, argv, environment, and runner options.

`meta.yaml` has been removed from the runtime. Runner concerns now belong in
Go options, CLI flags, standard environment variables, or script argv. Tiny
scripts stay valid without metadata.

The preferred end state is no `meta.yaml`. Txtar comment/header content is
sufficient for human-facing name, purpose, usage, and inputs. Runtime
configuration belongs outside the archive body or in explicit script commands.

Gap: keep future script-format changes in runner options or explicit commands.
Do not add `flags`, `args`, `stdin`, imports, richer declarative control
blocks, or a replacement metadata file.

### 4.9 Testing and CI confidence

Browser-harness optimizes for live task completion. `cdp` can be stronger by
turning mechanics into offline fixtures, but the current fixture set does not
yet cover enough of the browser-harness interaction surface.

Gap: each high-value mechanic needs at least one local fixture that runs under
`go test -tags cdp`, with no third-party network dependency.

## 5. Recommended implementation order

### P-1: Delete architecture drift

Status: done.

Before expanding the harness, remove code that conflicts with the target
shape:

- delete or collapse `cmd/ndp/session_manager.go`. Done.
- remove legacy interactive raw-CDP stubs from `cmd/cdp/main.go`. Done.
- delete `meta.yaml` parsing from `cdpscript`. Done.

These are cleanup commits, not new abstraction work.

### P0: Turn equivalence claims into fixtures

Status: partially done.

For each high-value browser mechanic, either add a local `cdpscripttest`
fixture or write down why the capability is live-only in the relevant skill
doc. The files are the index.

Current fixture coverage includes blocking scripts, form submission, reload
behavior, keyboard input, snapshot refs, and sourced helper commands. These now
run through the real `cdpscript` runtime via `cdpscripttest.RunCDPScript`.

### P1: Make the live agent loop obvious

Status: done for the operating skill; examples still useful.

Document and, where needed, smooth the default live workflow:

- observe with screenshot/page info;
- act with coordinate input or accessibility refs;
- verify with another screenshot;
- inspect with JS only when necessary;
- fall back to raw CDP.

This is the main browser-harness lesson worth copying.

### P2: Harden browser attach and stale-target recovery

Status: done for the first CLI/operator path.

Use the existing discovery and session-detection packages as the substrate for
the agent path. Add tests where possible and a short operator guide where
Chrome behavior requires user action.

Implemented path: `cdp attach` probes live DevTools endpoints, returns a text
or JSON list of attachable page targets, and prints exact Chrome/Brave launch
instructions when no page target is available. It uses live `/json/version`
and `/json/list` probes rather than trusting stale `DevToolsActivePort`-style
state.

### P3: Expand offline interaction fixtures

Status: open for the remaining mechanics.

Add focused `cdpscripttest` fixtures for missing high-value mechanics:

- dialogs
- downloads
- uploads
- dropdowns
- iframes and cross-origin iframes
- shadow DOM
- tabs
- viewport
- cookies/storage
- PDF output
- coordinate click behavior. Done.

### P4: Keep domain-aligned examples executable

Status: partially done.

Use existing txtars such as `examples/extract-gemini-apikey.txtar` and
`examples/gdoc-to-markdown.txtar` as the first examples. Add clear headers and
a CI, nightly, or explicit live-only verification path before adding new
domain-specific examples. Do not start by importing browser-harness domain
skills or designing a catalog layer.

## 6. Non-goals

- Do not port browser-harness's Python files.
- Do not import the browser-harness domain skill corpus wholesale.
- Do not add a manager layer, retry framework, session framework, or logging
  framework to mimic browser-harness behavior.
- Do not expand `meta.yaml` to solve runtime concerns; delete it unless a real
  compatibility blocker is found.
- Do not make `cdpscripttest` a second canonical runtime.
- Do not require third-party websites for CI fixtures.
- Do not add a remote-session routing layer while remote host/port/tab flags
  cover the need.

## 7. Effective-equivalence milestone

The first credible equivalence milestone is:

- high-value browser mechanics have local fixtures or explicit live-only notes
  in skills. Partial.
- the screenshot-first live loop is documented and works through CLI/MCP.
  Mostly done for MCP.
- raw CDP escape is documented for live MCP use. Partial: MCP exists; CLI
  cleanup is done, but raw CDP in scripts is not part of this milestone unless
  real scripts demand it.
- architecture drift from session-manager, raw-CDP stubs, and `meta.yaml` is
  removed or quarantined. Done.
- user-browser attach/recovery has one documented path. Done for CLI attach.
- missing P3 mechanics have local fixtures or explicit deferrals. Open.
- at least one existing domain-aligned txtar has a clear header and a
  verification path. Open.

At that point, `cdp` can reasonably claim browser-harness-equivalent outcomes
for local agent-driven browser work, while still being more Go-native,
testable, and scriptable.
