---
title: Known issues
description: Open bugs and current limitations across the cdp tools, with workarounds where they exist.
icon: triangle-exclamation
---

# Known issues

Open bugs across the repository: the MCP tools, plus `churl`'s recursion and
mirroring flags, which are accepted but do nothing.

## Resolved Issues

### click/type_text/hover/focus/press_key timeout units ambiguous

Resolved by `interactionCtx`: timeout values from 1 through 60 are seconds;
values greater than 60 are treated as milliseconds. This preserves the
documented seconds API while handling common agent inputs such as `5000` for
5 seconds.

**Observed**: 2026-04-05, A998 session. `click(selector: "a[href='/explore']", timeout: 5000)` hung for 6+ minutes on github.com.

**Files**: `cmd/cdp/mcp_tools.go`, `cmd/cdp/mcp_click_test.go`

### Extensions domain pipe requirement for list/install/uninstall

CDP Extensions-domain methods still require `--remote-debugging-pipe` and
`--enable-unsafe-extension-debugging`, but the MCP extension tools no longer
depend on that path for the common operations. `list_extensions`,
`install_extension`, and `uninstall_extension` now fall back through
`chrome.developerPrivate` or service-worker targets when the Extensions domain
is unavailable over a remote-debugging port.

**Files**: `cmd/cdp/mcp_extension_tools.go`

### `-auto-discover=false` made cdp do nothing

With `-auto-discover=false` and no `-chrome-path` or `-remote-host`, every
action was a silent no-op: auto-discovery was what normally filled in
`chromePath` or `remoteHost`, and the block implementing `-js`, `-render`,
`-extract`, and `-har` was guarded on one of them being set.

cdp now resolves an installed executable through `discovery.FindBestBrowser`
and launches it fresh, and exits 3 with `No browser executable found` when
there is none. Verified: `cdp -headless -auto-discover=false -url
https://example.com -js 'document.title'` prints `Example Domain`.

**Files**: `cmd/cdp/main.go`, `cmd/cdp/main_test.go`
(`TestCDP_AutoDiscoverDisabledRunsAction`)

### `-har` was discarded when cdp attached to a running browser

`cdp -url … -har out.har` printed `Recording network traffic to:`, exited 0,
and wrote no file whenever auto-discovery found a browser already listening on
the debug port. The save was guarded on `recorder != nil`, but `-har-mode`
defaults to `enhanced`, which populates `enhancedRecorder` and leaves
`recorder` nil.

Both modes now save on both paths. Verified against a browser confirmed
listening: the default enhanced mode writes the file and reports `Recorded 1
network requests`.

**Files**: `cmd/cdp/main.go` (`saveHAR`), `cmd/cdp/main_test.go`
(`TestCDP_RemoteEnhancedHARWritesFile`)

### Differential capture did not record or compare

`-diff-mode` registered a capture and returned without launching a browser, so
nothing completed it and `-compare-with` had no HAR to read. The recording step
exited 0 throughout, so a pipeline never noticed.

Recording now goes through the normal HAR path into the differential working
directory and calls `CompleteCapture`. Verified: two captures of the same page
into one `-diff-work-dir` both reached `Status: completed` with `Entries: 1`,
and the comparison wrote a report — `0 added, 0 removed, 1 modified requests`.

**Files**: `cmd/chrome-to-har/main.go`, `internal/differential/controller.go`
(`TestCaptureDifferentialCompletesCapture`)

## Active Issues

### 1. churl recursion and mirroring flags do nothing

**Symptom**: `churl -r`, `-m`, `-l`, `-np`, and `-P` are accepted, and churl
exits 0, but no files are written — the page is printed to stdout as though
none of the flags were given.

```
churl -r -np -l 1 -P ./mirror http://localhost:8099/   # exit 0, ./mirror stays empty
```

**Root cause**: the options are parsed into fields that nothing reads;
`cmd/churl/main.go` declares `recursive bool` and never consults it.

**Workaround**: none for mirroring. For a single page, churl works as
documented; for a crawl, drive it from a shell loop over URLs you enumerate
yourself.

**Observed**: 2026-08-04, against this tree.

### 2. click can hang on elements that trigger navigation

**Symptom**: `click` on a link that navigates the page may hang if the navigation changes the DOM before chromedp's click action completes. The element becomes stale mid-action.

**Workaround**: Use `evaluate` with `document.querySelector('a').click()` for navigation-triggering clicks, or use `navigate` directly if the URL is known.

**Files**: the `click` tool in `cmd/cdp/mcp_tools.go`

### 3. Coverage snapshot on minimal-JS pages returns empty

**Symptom**: `get_coverage` after `start_coverage` on server-rendered pages (e.g., Hacker News) returns 0 files because there's little/no JS to profile.

**Not a bug**: Expected behavior — V8 coverage only tracks JavaScript execution. Document this in tool description.

**Observed**: 2026-04-05, A998 session on news.ycombinator.com.

### 4. extension_console/extension_evaluate fail for devtools-only extensions

**Symptom**: "no target found for extension" when calling `extension_console` or `extension_evaluate` on a DevTools panel extension (like our coverage extension).

**Root cause**: DevTools-only extensions (with `devtools_page` but no `background` service worker) don't create CDP-visible targets. There's no `chrome-extension://` target to attach to.

**Workaround**: None currently. DevTools panel extensions run in the DevTools process, not as separate targets.

**Observed**: 2026-04-05, A998 extension test. Extension ID agmhhbefggjmejggmflmppmacnbmhnne.

## Next steps

- [Troubleshooting](/docs/troubleshooting) — symptoms that have a workaround today.
- [How cdp works](/docs/how-cdp-works) — which surfaces are settled and which
  are still moving.
- [Command reference](/docs/commands) — the documented behavior these deviate
  from.
