# Plan 05 — Go-native harness design

**Status:** Draft v1
**Date:** 2026-04-23
**Depends on:** none
**Informs:** future cleanup of `cdpscript/`, `cdpscripttest/`, and script metadata

## 1. Problem

The repo has two browser-script systems:

- `cdpscript/` is a runtime-oriented txtar executor.
- `cdpscripttest/` is a `go test` helper built on `rsc.io/script` and
  `rsc.io/script/scripttest`.

They overlap in browser verbs and execution scaffolding, but they do **not**
have identical semantics:

- `cdpscript` treats `goto` as URL navigation and `wait` as
  duration-or-selector.
- `cdpscripttest` treats `navigate` as `BASE_URL`-relative and layers in
  output assertions, `cmp`, `skip`, reporting, and test-only conditions.

That means the right answer is **not** "merge both DSLs into one command
table." The Go-native answer is smaller:

1. share lower layers,
2. keep distinct front-end semantics where they serve different callers,
3. shrink configuration surface until it is obvious what truly belongs in a
   script file.

## 2. Design goals

- Keep package count low and responsibilities obvious.
- Make `cdpscript` the canonical runtime path.
- Keep `cdpscripttest` focused on `go test` integration, not as a second
  browser runtime.
- Prefer small exported APIs over a wide catalog of helper constructors.
- Make script metadata optional and narrow enough that removing it later is
  plausible.

## 3. Recommended architecture

Use three packages only:

### 3.1 `internal/browser`

Owns Chrome lifecycle and page primitives:

- browser discovery / launch / remote tab attach
- page actions and page state
- lower-level helpers for screenshots, PDF, accessibility refs, HAR wiring,
  DOM/text extraction, and timeouts

This package should know nothing about txtar, `rsc.io/script`, or `testing.T`.

### 3.2 `cdpscript`

Owns runtime script execution:

- txtar archive loading
- optional script metadata parsing
- workdir extraction
- runtime command map
- script-scoped help text
- execution entry points used by `cmd/cdpscript` and `cdp run`

`cdpscript` is the canonical runtime package. If a txtar archive is meant to
run outside `go test`, it should flow through this package.

### 3.3 `cdpscripttest`

Owns test harness behavior:

- `go test` entry points
- test-oriented command map and conditions
- stdout/stderr/cmp assertions
- skip/stop/reporting/artifacts
- fixture discovery and execution inside test binaries

It may reuse lower-level helpers from `internal/browser` and may invoke
`cdpscript` for runtime-format fixtures, but it should not become a second
canonical runtime.

## 4. What should be shared vs. separate

### 4.1 Shared

Share only the parts that are genuinely runtime-agnostic:

- browser lifecycle and page primitives in `internal/browser`
- workdir extraction from txtar archives
- common artifact path helpers
- common runtime error classification where semantics match
- a small base set of browser actions only if behavior is actually identical

In practice, "shared" should mean helper functions and unexported files, not a
new framework package.

### 4.2 Intentionally separate

Keep these separate on purpose:

- the public command dialect
- `BASE_URL`-relative test navigation
- stdout/cmp assertion flow
- reporting and artifact streaming
- test-only conditions (`short`, `verbose`, `stdout:...`, etc.)
- runtime-only script help, argv handling, and Unix exit-code behavior

This is the key distinction: **shared execution substrate, separate caller
semantics**.

## 5. Metadata direction

`meta.yaml` is the biggest open design question.

### 5.1 Current reality

Today `cdpscript` reads metadata only from `-- meta.yaml --` inside the txtar
archive. The txtar comment section is not a structured metadata channel.

### 5.2 Options considered

#### Option A — keep `meta.yaml`, but shrink it

Pros:

- already implemented
- works naturally with txtar as "files in an archive"
- no custom parser in the txtar comment preamble
- no migration cliff

Cons:

- another file section to maintain
- invites schema drift if left unconstrained

#### Option B — move metadata into the txtar comment/header preamble

Pros:

- fewer moving pieces in the archive
- may look simpler for tiny scripts

Cons:

- requires a custom parser in a region txtar treats as free-form comment
- competes with the shebang use case
- creates a second language inside the archive
- makes `rsc.io/script` / txtar usage less conventional, not more

#### Option C — support both indefinitely

Pros:

- easiest migration on paper

Cons:

- two sources of truth
- more parser and docs complexity
- permanent maintenance cost for a problem we do not fully understand yet

### 5.3 Recommendation

Short term: **keep `meta.yaml`, but aggressively shrink it and stop adding
knobs**.

That means:

- treat `meta.yaml` as optional
- move execution concerns toward Go options / CLI flags rather than file
  metadata
- keep only fields that are clearly script-local defaults

Recommended direction for fields:

- Keep for now: `name`, `description`, `env`
- Keep only if still justified after cleanup: `headless`, `timeout`
- Move out of script metadata over time: `browser`, `profile`
- Do not add: `flags`, `args`, `stdin`, `imports`, or richer declarative
  control blocks

The design pressure should run in one direction only: **smaller and more
optional**.

This keeps the implementation conventional today while preserving the ability
to delete `meta.yaml` later if it becomes nearly empty. If that deletion ever
happens, it should happen by subtraction, not by introducing a second metadata
format first.

## 6. Exported API direction

The public API should shrink.

### 6.1 `cdpscript`

Keep a minimal execution-oriented surface:

- `New(opts ...Option) *Engine`
- `(*Engine).ExecuteTxtar(ctx, path, argv)`
- `HelpText(path)`
- a small option set for output dir, remote tab, verbosity, and other true
  runtime options

Avoid exporting parsing helpers and metadata types unless outside callers
actually need them.

### 6.2 `cdpscripttest`

Keep the harness API centered on test entry points:

- `NewEngine`
- `NewCLIEngine`
- `DefaultCmds`
- `DefaultConds`
- `Run`
- `Test`
- runtime-fixture bridge functions if still needed

Avoid exporting a large catalog of command constructor helpers as the primary
API. Callers who need custom commands can write `script.Cmd` directly.

## 7. Migration plan

No flag day.

### Step 1 — make `cdpscript` the canonical runtime

- keep `cmd/cdpscript` and `cdp run` on `cdpscript`
- allow `cdpscripttest` runtime-format fixtures to call into `cdpscript`
  instead of duplicating archive rewriting and execution setup

### Step 2 — share only lower layers

- pull genuinely shared helpers downward into `internal/browser` or unexported
  helpers inside `cdpscript`
- do **not** merge the public command dialects

### Step 3 — shrink metadata

- stop documenting speculative `meta.yaml` fields
- de-emphasize execution concerns inside script metadata
- keep scripts without `meta.yaml` first-class

### Step 4 — tighten exported APIs

- keep small engine/test entry points
- stop treating per-command constructors as the main public surface unless a
  real external use case exists

### Step 5 — revisit `meta.yaml`

After the runtime/test boundary is cleaner and the remaining `meta.yaml` fields
are truly minimal, re-evaluate whether the file still earns its keep.

That is the right time to answer "do we need it?" Not before.

## 8. Non-goals

- No new top-level framework package.
- No permanent dual-format metadata support.
- No attempt to force `cdpscript` and `cdpscripttest` into one public DSL.
- No YAML schema growth in the name of convenience.

## 9. Why this is more Go-native

- Small number of packages with clear ownership.
- Runtime concerns stay separate from test harness concerns.
- Text archive remains a normal txtar archive, not a custom container format.
- Public APIs stay small and execution-oriented.
- Migration is incremental and mostly subtractive.

This design is intentionally conservative. The biggest improvement is not a new
abstraction; it is removing ambiguity about what is shared, what is separate,
and how little metadata a script should need.
