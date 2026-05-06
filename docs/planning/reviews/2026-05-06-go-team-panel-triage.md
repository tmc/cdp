# go-team panel review triage — 2026-05-06

**Date:** 2026-05-06
**Run:** `~/.cdp-go-team-review/run-20260506-144036/`
**Notebook:** `a862fc39-6861-4e8c-b28b-6b449f8bb933`
**Bundle source:** `go-team-review-20260506-144036`
**Synthesis:** `2026-05-06-go-team-panel-meta.md` (sibling)
**HEAD at review time:** `d24180e` (after the doc-cleanup + A3.1 commits earlier this session)

This doc triages the panel's Top-10 against the filesystem and the
project's actual constraints. Every cited path/symbol is checked
before action. Buckets: VERIFIED, HALLUCINATED, OVERRULED, DEFERRED.

## Top-10 triage

| # | Finding | Cost | Bucket | Notes |
|---|---|---|---|---|
| 1 | Recorder I/O mutex (Blocker) | M | **VERIFIED** | `internal/recorder/recorder.go:43,169,609` exist; comment at line 607 explicitly says "caller must hold r.Lock() … to avoid deadlock" — the I/O is in the lock by design. Real concurrency bug. |
| 2 | Collapse cmd/{churl,chdb,ndp,native-host} into cdp subcommands | L | **OVERRULED** | User explicitly disagrees (2026-05-06). Five binaries stay separate. Don't re-litigate. |
| 3 | Unify cdpscript / cdpscripttest dialects via `internal/scriptcmds` | M | **VERIFIED** | Both `cdpscripttest/cmds.go` and `cdpscript/engine.go` exist; the dialect duplication is real. Already on the v3 plan as A1.something — confirm and execute. |
| 4 | De-bloat mcpSession god object | M | **VERIFIED** | `cmd/cdp/mcp.go` `mcpSession` holds 12+ subsystems. Real but invasive — needs its own arc. |
| 5 | Delete internal/secureio | S→M | **VERIFIED-but-bigger** | Panel called this "S" but has 5 callers in `internal/browserprofile` (`SecureWriteFile`, `CreateSecureTempDir`, `SecureRemoveAll`, `SecureDirPerms`). Delete requires touching browserprofile package too. Closer to M. |
| 6 | Rename `chromeprofiles` → `profile`, `cdpinput` → `input`, `cdpproxy` → `proxy` | S | **VERIFIED partially** | Landed as `chromeprofiles → browserprofile` (not bare `profile`: `profile` is too generic and shadowed real loop variables in `cmd/cdp/main.go`; `browserprofile` is explicit per project naming guidelines). `cdpinput → input`: HALLUCINATED — collides with `github.com/chromedp/cdproto/input` already imported in 4+ files. `cdpproxy → proxy`: HALLUCINATED — `cdpproxy` is CDP-protocol-message proxy, not HTTP proxy; the prefix disambiguates. Discard the latter two. |
| 7 | Extract Sourcemap Analyzer from `mcp_sourcemap_tools.go` | M | **VERIFIED** | Already on the v3 plan as A5. |
| 8 | Remove `internal/browser` OOP wrappers | L | **DEFERRED** | Real and significant, but multi-day. Three of four `cmd/*` binaries depend on `browser.Page`. Treat as own arc. |
| 9 | Eliminate `chromedp.Run(ctx, chromedp.ActionFunc(...))` nesting | S | **VERIFIED** | 100 occurrences confirmed via grep. Already on v3 plan as A6. Mechanical sweep, low risk. |
| 10 | Delete page_options.go compat shims, meta.yaml dead code | S | **VERIFIED partially** | `internal/browser/page_options.go` exists (verify it's actual compat shims, not real code). `meta.yaml`: only mention is in `cmd/cdp/script_format_doc_test.go:24` which *forbids* the format doc from mentioning `meta.yaml` — i.e., already handled as a regression test. Discard the meta.yaml part. |

## HALLUCINATIONS

- **#6.b**: `cdpinput → input` ignores existing `github.com/chromedp/cdproto/input` import collision.
- **#6.c**: `cdpproxy → proxy` strips the CDP-vs-HTTP-proxy disambiguation and creates name collisions with HTTP proxy code in `cmd/{ndp,churl}`.
- **#10.b**: meta.yaml is already pinned-not-to-exist by `cmd/cdp/script_format_doc_test.go:24`; the panel didn't see the regression test.

## OVERRULES (user, not panel)

- **#2 binary consolidation** — five `cmd/*` binaries stay as separate top-level commands.
  Reason: not recorded; treat as a project constraint going forward. Re-engage only with
  explicit user request.

## What lands in this session

Cheapest wins that survive triage and don't conflict with the user-overruled item:

1. `internal/chromeprofiles → internal/browserprofile` rename (the only valid #6) — done mechanically (gomvpkg hung at 21min CPU and was killed). `browserprofile` chosen over bare `profile` to avoid shadowing real loop variables in `cmd/cdp/main.go` and to follow CLAUDE.md "explicit and specific" naming.
2. Possibly: delete `internal/secureio` and inline `os.WriteFile`/`os.MkdirTemp`/`os.RemoveAll`
   in the 5 caller sites (#5).

Items 1, 3, 4, 7, 8, 9 are real and worth doing but each is its own arc. They go on the
v3 plan, not this commit chain.

## Single-most-important-next-step

Per panel synthesis: "merge the duplicated `cdpscript` and `cdpscripttest` command
dialects into a single `internal/scriptcmds` package and strip the blocking I/O out
of the chromedp event loop in `internal/recorder`."

After the cheap renames land, the next arc is the recorder I/O fix (#1). It's an
independent unit and unblocks reliable network capture under load.
