---
date: 2026-05-06T22:30:00Z
notebook_id: a862fc39-6861-4e8c-b28b-6b449f8bb933
conversation_id: 7c3effba-dbec-4483-be8c-b1cc2729ea89
source_id: b232306f-272e-453e-923a-d45bc98b7dd7
source_title: cdp session commits ba99631~..main (2026-05-06 panel-guided cleanup)
range: ba99631~..main
commits: 9
---

# go-team history-audit triage — 2026-05-06

Audit of the 9 unpushed commits produced this session under guidance of
the codebase-review panel. Companion to
`2026-05-06-go-team-panel-triage.md` (codebase review).

## Panel verdict

OUT. DO NOT PUSH AS-IS.

**Recommended action:** rebase to (1) reorder the v3 design doc before
the concurrency fix and rename, (2) squash the five markdown rename
commits into one, (3) strip the `claude-code` telemetry blocks from
commit messages.

## Triage

| Claim | Verifier | Verdict |
|---|---|---|
| `ba99631`, `d611f11`, `8e920ba`, `40d08e5`, `323950f` exist in range | `git rev-list ba99631~..main \| grep` | **TRUE** — all in-range |
| Five commits "do nothing but shuffle markdown" | `git show --stat` for each | **TRUE-with-nuance** — each has a distinct rationale (per-binary roadmap, churl design notes incl. rename, proxy docs promotion, README convention, debug-guide rename) but functionally they're all doc reorg |
| "No subject lines, telemetry block injected at top" (Robert) | `git log -1 --format=%B ba99631` | **HALLUCINATED** — every commit has a proper `prefix: subject` first line and a real body. Panel saw the `--notes` block (rendered in dump output) and confused it with the commit-message header. The git note is separate metadata, not part of `%B`. |
| Design doc `d24180e` should precede fix `525f18e` (Ian) | `git log` order; commit-message scope | **HALLUCINATED** — `525f18e` lands fix A3.1 from a *prior* v3 plan; the new design doc `d24180e` was written DURING this session AFTER the fix had already been applied. The doc captures the design state going forward. The fix's commit message is self-explaining. |
| Design doc `d24180e` should precede rename `1027eff` (Ian) | `git log` order | **ALREADY TRUE** — `d24180e` lands at position 3, `1027eff` at position 1 (HEAD). Order is correct as-is. |
| Telemetry block (`claude-code v2.1.126…`) should be stripped (Robert) | `git log -1 --format=%B` shows it's NOT in the commit message | **HALLUCINATED** — the telemetry is in `git notes`, not in the commit body. `git log --notes --stat --patch` (the dump format used) interleaves them visually but they live in `refs/notes/commits`, not `refs/heads/main`. Stripping notes is a separate operation; they're not on the commit object. |

## What the triage rejects

- **Reordering** — both ordering claims are wrong (one already-true, one based on a misread of `525f18e`'s scope).
- **Stripping commit-message telemetry** — there is no telemetry in commit messages. Git notes are intentional, separately storable, and not pushed to `origin/main` by default. They're audit metadata, not noise on the commit graph.

## What the triage accepts

- **Granularity reduction on the doc-shuffle window** — 5 commits is overkill. Compromise: collapse into **2** commits, not 1, to preserve the rationale split:
  1. **`docs: consolidate roadmap and design notes under docs/internal/roadmap/`** ← squash of `ba99631` + `d611f11`. Common theme: scratchpad planning artifacts move out of `cmd/*` source dirs into `docs/internal/roadmap/`.
  2. **`docs: standardize doc filenames across cmd and examples`** ← squash of `8e920ba` + `40d08e5` + `323950f`. Common theme: case/naming convention sweep (PROXY_USAGE.md → churl-proxy.md, README_TEST.md → testing.md, CLAUDE_DEBUG_GUIDE.md → debugging.md, all snake-cased UPPER → kebab-cased lower).

The other four commits in the range are atomic and stand on their own:

- `525f18e` — concurrency fix (substantive code).
- `d24180e` — v3 design doc (substantive docs).
- `6eea9a6` — panel verdict capture.
- `1027eff` — package rename.

## Action

Execute a **bounded rebase** that squashes only the 5 doc commits into
2 commits. Leave the other 4 commits content-stable (tree + message +
author-date byte-identical). After rebase:

1. Verify `git diff main backup/...` outside the squash window is empty.
2. Run `go test -count=1 ./...` to confirm no latent breakage surfaces
   from the rewrite. (The squashed commits are pure doc moves, but
   the tail of the range includes the rename and the recorder fix
   tests; verify those pass post-rebase.)
3. Re-dump and re-audit per Beat 8.

## Anti-actions (not doing)

- Reordering anything.
- Stripping git notes.
- Collapsing all 5 doc commits to 1 (preserving the 2-bucket split keeps the rationale).
- Squashing any of the 4 non-doc commits.
