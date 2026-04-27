# Plan 01 — Rewrite CDP_SCRIPT_FORMAT.md from the engine

**Status:** Draft v1
**Date:** 2026-04-21
**Depends on:** none
**Blocks:** plans 02, 03

## 1. Problem

`cmd/cdp/CDP_SCRIPT_FORMAT.md` (563 lines) is ~90% aspirational. It advertises
commands that do not exist in `cdpscript/engine.go:310`:

| Doc says | Real status |
|---|---|
| `assert status <code>` | unimplemented — `assert` supports only `exists|text|visible` (`engine.go:830`) |
| `assert no errors`, `assert url contains` | unimplemented |
| `capture network to <file>` | engine has `har <file>` (`engine.go:1137`), not `capture network` |
| `mock api <pattern> with <file>` | unimplemented |
| `throttle <profile>` | unimplemented |
| `save <var> to <file>` | unimplemented |
| `extract <selector> as <var>` | real form is `extract <selector>` → sets `$EXTRACTED` (`engine.go:600`) |
| `include <file>` | unimplemented; real mechanism is `source path` or `source -as name path` (`engine.go:666`) |
| `if { }` / `for { }` control flow | unimplemented |
| `select`, `scroll to` | unimplemented |
| `devtools`, `breakpoint`, `debug` | unimplemented |
| `compare <a> with <b>` | unimplemented |
| `#!/usr/bin/env cdp script` shebang | wrong; real subcommand is `cdp run` (`cmd/cdp/main.go:1069`) or `cdpscript` (`cmd/cdpscript/main.go`) |
| `cdp script test.cdp`, `--var`, `--matrix`, `--watch`, `--parallel`, `--distributed` | none exist |

Agents that read this doc write scripts that fail on the first command.

## 2. Scope

Rewrite `cmd/cdp/CDP_SCRIPT_FORMAT.md` so it describes only what
`cdpscript/engine.go` actually implements today. Move aspirational material
to a separate `FUTURE.md` (or delete; see §6). Leave `skills/writing-cdp-scripts/references/script-format.md` as the agent-facing copy and keep them in sync.

## 3. Out of scope

- Implementing any missing commands. This plan is doc-only.
- Moving the doc out of `cmd/cdp/`. It stays next to the binary that runs it.
- Generating the doc from code. Tempting but premature — the command set is
  small and stable enough that a hand-maintained doc with a one-line
  `go:generate` placeholder is fine for now.

## 4. Canonical command surface (as of `engine.go` @ main)

From `Engine.commands()` at `cdpscript/engine.go:310`:

**Navigation:** `goto <url>`, `back`, `forward`, `reload`.
**Waiting:** `wait <duration|selector>`.
**Interaction:** `click <selector|@ref>`, `fill <selector|@ref> <value>`, `type` (alias for `fill`), `hover <selector>`, `press <key>`.
**JavaScript:** `js <code>`, `jsfile <filename>`.
**Extraction:** `extract <selector>` (sets `$EXTRACTED`), `title` (sets `$TITLE`), `url` (sets `$URL`), `render [--term] [selector]` (sets `$RENDERED`).
**Assertions:** `assert exists <selector>`, `assert text <selector> <expected>`, `assert visible <selector>`.
**Output:** `screenshot <file>`, `pdf <file>`, `log <message>`.
**Network:** `block <pattern>`.
**Scripting:** `source [-x] [-as <name>] <path>`.
**Snapshots & refs:** `snapshot [-i] [--depth N] [--compact] [--selector CSS]` → sets `$SNAPSHOT_REFS`, emits `@e1`-style refs consumed by `click`, `fill`, `type`.
**HAR:** `tag [name]` (sets `$CURRENT_TAG`), `har <file>`, `note <description>`, `capture screenshot|dom [description]`.

Plus whatever `source -as` dynamically registers (sourced-command env vars
`ARG1..ARGN`, `ARGC`; `engine.go:739`).

**Conditions** (`engine.go:371`): `[headless]`, `[has-tab]`.

**Metadata fields honored** (`engine.go:52` — the only fields actually read):
`name`, `description`, `version`, `browser`, `profile`, `headless`,
`timeout`, `env`.

**Metadata fields documented but ignored:** `imports:`. Either delete from
docs or implement. This plan deletes.

## 5. Deliverables

### 5.1 `cmd/cdp/CDP_SCRIPT_FORMAT.md` rewrite

Target ~200 lines, not 563. Sections:

1. What a cdpscript is (txtar with `meta.yaml` + `main.cdp` + optional
   embedded files).
2. Running: `cdp run <path>`, `cdpscript <path>`, or `chmod +x foo.cdpscript`
   with `#!/usr/bin/env cdpscript` (recommended) or `#!/usr/bin/env -S cdp run`.
3. `meta.yaml` — only fields from §4.
4. Command reference — only commands from §4. Each entry: one-line summary,
   args, example, any env var it sets.
5. Refs and snapshots — one short section.
6. HAR recording — tag / har / note / capture pattern.
7. Exit codes — contract from plan 04.
8. A single worked example under 30 lines.

Every command entry cites `engine.go:NNN` so future drift is visible in diff.

### 5.2 `cmd/cdp/CDP_SCRIPT_FORMAT_FUTURE.md` (optional)

If there is appetite to keep the aspirational material, move it here with a
prominent "none of this is implemented; open an issue if you want it" header.
Otherwise delete — git history is the archive.

Recommend **delete**. Aspirational docs in-tree keep getting cited as if real.

### 5.3 `skills/writing-cdp-scripts/references/script-format.md` sync

This file exists and is short (confirm content is already accurate; refresh
if it cites removed commands). Make it the canonical agent-facing reference
and have `CDP_SCRIPT_FORMAT.md` link to it rather than duplicate.

## 6. Rollout

Single PR, single commit, ~half a day. Test plan:

- `go build ./cmd/cdp ./cmd/cdpscript` still passes.
- Every example in the new doc runs end-to-end against the existing engine
  (put them under `cdpscripttest/testdata/` as offline fixtures — overlaps
  with plan 03; keep them as deliverable here if plan 03 slips).
- Grep the rewritten doc for any command not in §4 — must be empty.

## 7. Risks

- **Users had scripts relying on documented-but-never-implemented commands.**
  Near-zero probability; those scripts already don't run. No migration path
  needed.
- **The doc goes stale again.** Mitigation: a `TestScriptFormatDocCommands`
  test that greps every fenced code block for the first token and asserts it
  is a real engine command or a shell snippet. 30 lines. Add it.
