# Plan 02 — Skills expansion for web-mechanics parity

**Status:** Draft v1
**Date:** 2026-04-21
**Depends on:** plan 01 (script format must be accurate first)

## 1. Goal

Expand `skills/` from the current five skill areas
(`capturing-network-traffic`, `capturing-page-artifacts`,
`debugging-page-javascript`, `operating-cdp-cli`, `writing-cdp-scripts`) to
cover the web mechanics that `browser-use/browser-harness` has doc corpus for
and `tmc/cdp` has MCP tools for but no agent-facing docs. The MCP tools
already exist; the skills are missing docs, not missing features.

## 2. Scope — one new skill per web-mechanics cluster

Each new skill is a directory under `skills/` following the existing shape
(one `SKILL.md` frontmatter + short prose, one or more `references/*.md`).

| New skill | Backing MCP registrations (verified) |
|---|---|
| `handling-dialogs/` | `registerDialogTools` at `cmd/cdp/mcp_dialog_tools.go:86` — `get_dialogs`, `handle_dialog` |
| `working-with-iframes/` | `registerFrameTools` at `cmd/cdp/mcp_frame_tools.go:32` — `list_frames`, `switch_frame` |
| `uploading-files/` | `registerFileTools` at `cmd/cdp/mcp_file_tools.go:20` — `upload_file` |
| `intercepting-requests/` | `registerInterceptTools` at `cmd/cdp/mcp_intercept_tools.go:311` — `intercept_request`, `intercept_response`, `list_intercepts`, `remove_intercept` |
| `managing-storage/` | `registerStorageTools` at `cmd/cdp/mcp_storage_tools.go:30` — `get_storage`, `set_storage`, `clear_storage` (plus `registerCookieTools` via `mcp_tools.go:32` — verify the file/exported names before writing doc) |
| `emulating-device/` | `registerEmulationTools` at `cmd/cdp/mcp_emulation_tools.go:101` — `set_device`, `set_user_agent`, `set_viewport`, `set_geolocation`, `set_offline`, `set_throttling`, `set_extra_headers` |
| `inspecting-dom-changes/` | `registerDomDiffTools` at `cmd/cdp/mcp_domdiff_tools.go:366` — `snapshot_dom`, `dom_diff`, `list_dom_snapshots` |

**Notes on gaps discovered during this plan:**

- `mcp_scroll_tools.go` exists but grep finds no `"*_*"` snake-case tool names
  in it — inspect before writing `scrolling/` skill. May be empty/skeleton.
  **Skip the scrolling skill until the file has real tools or close the gap
  first in a separate PR.**
- Cookies are referenced as `registerCookieTools` from `mcp_tools.go:32`
  but no `mcp_cookie_tools.go` file exists. Find the actual registrar
  (possibly in `mcp_storage_tools.go` or `mcp_state_tools.go`) before the
  storage skill cross-references it.

## 3. What each skill file must contain

Frontmatter (mirror existing skills):

```markdown
---
name: <skill-name>
description: One sentence; agents route on this.
---
```

Body sections:

1. **Quick start** — three-bullet minimum viable flow.
2. **Use this skill for** — bulleted scope.
3. **Use a different skill for** — disambiguate from siblings; avoids MCP
   tool churn during agent routing.
4. **Tool sequence** — the exact MCP tool names (from the verified table in
   §2), in the order they are typically called, with a two-line example
   per tool.
5. **Script form** — the same flow written as a `main.cdp` snippet when the
   `cdpscript` engine supports it. If it does not, say so explicitly; do not
   invent commands (cross-ref plan 01).
6. **Read next** — links to sibling skills and to
   `skills/writing-cdp-scripts/references/script-format.md`.

Every command or tool citation includes `file:line` the first time it
appears so drift shows up in diff review.

## 4. Accretion policy (`skills/domain/`)

Introduce `skills/domain/` as an empty directory with one `TEMPLATE.md` and
one `CONTRIBUTING.md`. Do **not** seed it with ported browser-harness
playbooks.

`TEMPLATE.md` shape:

```markdown
---
name: <site-slug>
last-verified: YYYY-MM-DD
---

# <Site> — durable shape

## URL patterns
## Private APIs
## Stable selectors
## Framework quirks
## Waits and traps
## Recipes (optional, cross-linked to cdpscripttest fixtures)
```

`CONTRIBUTING.md` rules:

- Capture durable shape, not the diary.
- No raw pixel coordinates.
- No secrets, session tokens, user-specific state.
- `last-verified:` frontmatter required.
- A markdown observation is sufficient; a runnable script is nice-to-have.

## 5. Out of scope

- `skills/CONTRIBUTING-SKILLS.md` as a separate file. The three bullets above
  fit inside each skill area's README; a top-level file invites skill-rot
  governance bureaucracy.
- An MCP `list_skills` tool. Grep works; revisit if routing complaints
  arrive.
- Validators, linters, freshness CI jobs.

## 6. Rollout

~half day, pure docs, no code. One PR per skill area is fine; or bundle all
seven + `domain/` into one PR. Bundle preferred — atomic for cross-links.

Test plan: grep each new skill file; every tool name mentioned must appear
in one of the `registerXxxTools` functions listed in §2.

## 7. Risks

- **Skill rot.** `last-verified` in `domain/` mitigates; interaction-skill
  freshness is guarded by plan 03 fixtures (which would break CI when the
  engine changes). No extra mitigation needed.
- **Coverage gaps** — `mcp_scroll_tools.go` looked empty; do a pre-flight
  check before promising a scrolling skill.
