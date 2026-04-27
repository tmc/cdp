# cdp-harness plan — overview

**Status:** Draft v1
**Date:** 2026-04-21
**Owner:** @tmc

Consolidates prior notebook iterations (harness design v1–v3, Unix-native
cdpscript, converged plan) plus reviewer critique. Supersedes the `/tmp/*.md`
scratch docs. Sibling files in this directory carry the detailed plans per
workstream. This file is the index.

## 1. Why now

`tmc/cdp` already has the tooling surface: one `cdp` binary with ~60 MCP tools
(`cmd/cdp/mcp_*_tools.go`), a txtar script engine (`cdpscript/engine.go`), and
a test-helper library (`cdpscripttest/`). What is missing is:

- A **truthful reference doc** for the script format — the existing
  `cmd/cdp/CDP_SCRIPT_FORMAT.md` is ~90% aspirational and pattern-matches
  agents onto commands that do not exist.
- **Agent-facing skill docs** for web mechanics that already work via MCP
  tools (dialogs, iframes, uploads, file upload, scroll, storage).
- **Runnable offline fixtures** under `cdpscripttest/testdata/` so interaction
  mechanics stay green in CI forever.
- **Unix-citizen ergonomics** for `cdpscript` so `.cdpscript` files can be
  chmod-ed and executed with positional args and a working `--help`.

## 2. Workstream index

| # | File | What it covers |
|---|------|----------------|
| 1 | [01-script-format-truth.md](cdp-harness-plan-01-script-format-truth.md) | Rewrite `CDP_SCRIPT_FORMAT.md` to match `cdpscript/engine.go:310`. |
| 2 | [02-skills-expansion.md](cdp-harness-plan-02-skills-expansion.md) | Fill in `skills/` for web-mechanics parity with browser-harness. |
| 3 | [03-interaction-fixtures.md](cdp-harness-plan-03-interaction-fixtures.md) | Offline txtar fixtures under `cdpscripttest/testdata/`. |
| 4 | [04-cdpscript-unix-native.md](cdp-harness-plan-04-cdpscript-unix-native.md) | Positional argv, script-scoped `--help`, exit-code contract. |
| 5 | [05-go-native-design.md](cdp-harness-plan-05-go-native-design.md) | Package boundaries, shared lower layers, and metadata direction. |

Each file is independently shippable. There is one hard dependency: **plan 01
must land before plan 02 is allowed to cross-link script commands**, because
02's correctness depends on 01's command surface being accurate.

## 3. What was rejected (and why)

Captured here so reviewers do not rediscover these.

- **Porting browser-harness's ~70 playbooks.** Accrete from real flows; do not
  import a corpus built against a different tool.
- **Third-party-network fixtures.** Guarantees flaky CI and invites
  `--tag external` speculative machinery.
- **A new CLI binary.** Extend `cmd/cdpscript` and `cmd/cdp run`.
- **A new file format.** Keep txtar + `meta.yaml` + `main.cdp`.
- **`meta.yaml flags:` / `args:` / `stdin:` blocks.** Over-engineered. Scripts
  parse their own argv; stdin gating is a footgun until a real user asks.
- **MCP `list_skills` tool.** Grep works; revisit only if filesystem walks
  become painful during an agent flow.
- **`cdpscript` subcommand structure.** Current surface is `cdpscript <path>`
  and `cdp run <path>`. Do not introduce `cdpscript run`, `cdpscript test`.

## 4. Non-goals for the whole initiative

- No retries / session / config framework.
- No manager layer; no speculative abstractions.
- No rewrites of `cdpscripttest/` (it is a Go test library, not a fixture
  runner — plan 03 respects that).
- No Python port or cross-runtime compatibility.

## 5. Ship order

Suggested sequencing (each step independently reviewable, each ~half to one
day of work):

1. Plan 01 — script format truth pass. Blocks nothing else strictly, but
   unblocks the skills corpus from compounding fiction.
2. Plan 04 — cdpscript Unix-native. Small, self-contained, touches only
   `cmd/cdpscript/main.go`, `cmd/cdp/main.go`, `cdpscript/engine.go`.
3. Plan 02 — skills expansion. Pure markdown, cross-links to the corrected
   script format from plan 01 and to MCP tool names verified in plan 02.
4. Plan 03 — interaction fixtures. Depends on plans 01 and 04 landing so the
   fixtures use real commands and support positional argv.

## 6. Single biggest risk

Skills and fixtures getting built against aspirational commands. Plan 01 is
the mitigation; it comes first.
