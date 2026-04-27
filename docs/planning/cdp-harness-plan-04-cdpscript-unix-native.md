# Plan 04 — cdpscript as a Unix-native executable

**Status:** Draft v1
**Date:** 2026-04-21
**Depends on:** none (but plan 01 should ideally land first so the README fix
is part of the same truth pass)

## 1. Problem

`cdpscript` is two-thirds a first-class executable:

- Shebang works via `#!/usr/bin/env cdpscript` (`cmd/cdpscript/main.go:34`).
- `cdp run <path>` dispatches to the same engine entry point
  (`cmd/cdp/main.go:1069`).
- `txtar.Parse` tolerates leading non-archive lines (the shebang lands in
  `archive.Comment`).

What is missing:

- **Positional argv after the script path is discarded** — `engine.go:110`
  signature is `ExecuteTxtar(ctx, path)` with no argv parameter.
- **`--help` after a script path prints the CLI flags**, not the script's
  name and description.
- **Exit codes are not a documented contract** — `cmd/cdpscript/main.go:41`
  blanket-returns `os.Exit(1)` on every error including parse failures.
- **`meta.yaml imports:` is documented but never read** (confirmed by grep
  in `cdpscript/engine.go`).
- **Shebang doc in `CDP_SCRIPT_FORMAT.md` points at a nonexistent subcommand**
  (`cdp script`, not `cdp run`) — addressed by plan 01.

## 2. Scope

### 2.1 Positional argv passthrough

Change `ExecuteTxtar` signature:

```go
func (e *Engine) ExecuteTxtar(ctx context.Context, path string, argv []string) error
```

Before calling `script.NewState` at `engine.go:174`, populate env vars:

- `ARG1..ARGN` for each positional (mirrors `source -as` at `engine.go:739`).
- `ARGC` with the count.

**Do not** also populate `$1..$N`. Reviewer caught that `rsc.io/script` has
no native `$N` convention, and the repo already established `ARGn/ARGC` via
sourced commands. Ship one convention.

Update both callers:

- `cmd/cdpscript/main.go:71` — pass `c.fs.Args()[1:]` after stripping the
  script path.
- `cmd/cdp/main.go:1069` — same through its `newScriptCmd`.

### 2.2 Script-scoped `--help`

When `--help` or `-h` appears in argv **after** the script path (i.e., in
what becomes the tail passed to `ExecuteTxtar`):

1. Load the txtar.
2. Print to stdout:
   - `name` (from `meta.yaml`)
   - blank line
   - `description` (if present)
   - blank line
   - `"Usage: <name> [args...]"` — synthesized, no custom schema
3. Exit 0.

No `meta.yaml usage:` / `flags:` / `args:` schema. Reviewer was right:
over-engineered. A script that wants a custom usage writes it in
`description` or adds an `--help` handler in `main.cdp` using `log`.

When `--help` appears **before** the script path, current CLI help is
printed (unchanged).

### 2.3 Exit-code contract

Document and implement in both binaries.

| Code | Meaning |
|---|---|
| 0 | Script ran to completion; all assertions passed |
| 1 | General runtime error |
| 2 | Usage error (bad CLI flags, missing script path, bad argv) |
| 3 | Assertion failed |
| 130 | Interrupted (SIGINT) |

Real work required:

- `cmd/cdpscript/main.go:41` today: `fmt.Fprintf(os.Stderr, ...); os.Exit(1)`.
  Split into:
  - `flag.ErrHelp` → exit 0 (already handled).
  - flag parse errors → exit 2.
  - engine errors tagged as assertion failure → exit 3.
  - everything else → exit 1.
  - `ctx.Err() == context.Canceled` from SIGINT → exit 130.
- `cmd/cdp/main.go:1069` `case "run":` branch calls `exitWithError(...)`
  which already uses typed codes. Add an assertion-failure case mapped to 3.
- Tag engine assertion failures. `engine.go:830` `cmdAssert` returns
  `fmt.Errorf("assertion failed: ...")`. Wrap as a sentinel:

  ```go
  var ErrAssertionFailed = errors.New("assertion failed")
  // in cmdAssert: return fmt.Errorf("%w: no elements found for selector %q", ErrAssertionFailed, selector)
  ```

  Callers `errors.Is(err, cdpscript.ErrAssertionFailed)` to route to exit 3.

### 2.4 Drop `imports:` from docs

`meta.yaml imports:` is documented but `engine.go` never reads it. Two
options:

- Implement it (substantial: need to define resolution semantics, file
  inclusion vs command sourcing, shebang passthrough in imported files).
- Delete from docs.

**Delete.** `source path` already handles composition
(`engine.go:666`). Document `source` in the plan 01 rewrite; drop
`imports:` from `Metadata` struct comment and examples.

### 2.5 Shebang recommendation

Keep `#!/usr/bin/env cdpscript` as the recommended form in docs. Examples
like `examples/aistudio-fc.cdp` that use `#!/usr/bin/env cdp run` require
`env -S` and are macOS < 10.15 hostile. Migrate examples in a follow-up PR;
not part of this plan's hard scope.

## 3. Explicitly not in scope

- `meta.yaml flags:` / `args:` / `stdin:` / `usage:` blocks.
- `cdpscript new` scaffolder.
- `file(1)` magic.
- Subcommand harmonization (`cdpscript run` alias).
- stdin plumbing. `rsc.io/script` runs commands as Go functions inside one
  process; there is no per-command stdin inheritance. `$STDIN` via env var
  breaks on non-trivial input. Defer until a real user need surfaces.

## 4. Rollout

One PR, three files touched: `cdpscript/engine.go`, `cmd/cdpscript/main.go`,
`cmd/cdp/main.go` (the `case "run":` branch). Half a day.

Test plan:

- Add `cdpscript/argv_test.go` with a table-driven test that builds a
  minimal txtar, runs `ExecuteTxtar(ctx, path, []string{"one", "two"})`,
  and asserts `ARG1=one`, `ARG2=two`, `ARGC=2` via `echo $ARG1` in a
  `main.cdp` that uses `log`.
- Golden-test the script-scoped `--help` output against a fixture.
- Exit-code integration test: invoke the binary via `os/exec` with a
  failing assertion, assert exit code 3; with a bad flag, assert exit 2.

## 5. Risks

- **Backwards compatibility.** Existing scripts: `ARGn/ARGC` were not
  previously set at engine level — new env vars, no collisions. `--help`
  behavior change: only triggers when argv contains `--help` **after** a
  script path; invocations like `cdpscript --help` are unchanged.
- **Exit-code change.** Any CI that greps for exit 1 on assertion failures
  will shift to exit 3. Document in release notes. Worth it — pipelines
  deserve the distinction.
- **`ExecuteTxtar` signature change** is a public API break for anything
  importing `cdpscript`. Grep the tree: only `cmd/cdpscript/main.go:71`
  and one expected callsite under `cmd/cdp/main.go:1069` consume it. Fine
  to break in one commit.
