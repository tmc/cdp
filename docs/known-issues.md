# Known Issues — cdp MCP Tools

## Resolved Issues

### click/type_text/hover/focus/press_key timeout units ambiguous

Resolved by `interactionCtx`: timeout values from 1 through 60 are seconds;
values greater than 60 are treated as milliseconds. This preserves the
documented seconds API while handling common agent inputs such as `5000` for
5 seconds.

**Observed**: 2026-04-05. `click(selector: "a[href='/explore']", timeout: 5000)` hung for 6+ minutes on github.com.

**Files**: `cmd/cdp/mcp_tools.go`, `cmd/cdp/mcp_click_test.go`

### Extensions domain pipe requirement for list/install/uninstall

CDP Extensions-domain methods still require `--remote-debugging-pipe` and
`--enable-unsafe-extension-debugging`, but the MCP extension tools no longer
depend on that path for the common operations. `list_extensions`,
`install_extension`, and `uninstall_extension` now fall back through
`chrome.developerPrivate` or service-worker targets when the Extensions domain
is unavailable over a remote-debugging port.

**Files**: `cmd/cdp/mcp_extension_tools.go`

## Active Issues

### 1. click can hang on elements that trigger navigation

**Symptom**: `click` on a link that navigates the page may hang if the navigation changes the DOM before chromedp's click action completes. The element becomes stale mid-action.

**Workaround**: Use `evaluate` with `document.querySelector('a').click()` for navigation-triggering clicks, or use `navigate` directly if the URL is known.

**Files**: `cmd/cdp/mcp_tools.go`

### 2. Coverage snapshot on minimal-JS pages returns empty

**Symptom**: `get_coverage` after `start_coverage` on server-rendered pages (e.g., Hacker News) returns 0 files because there's little/no JS to profile.

**Not a bug**: V8 coverage only tracks JavaScript execution.

**Observed**: 2026-04-05 on news.ycombinator.com.

### 3. extension_console/extension_evaluate fail for devtools-only extensions

**Symptom**: "no target found for extension" when calling `extension_console` or `extension_evaluate` on a DevTools panel extension (such as `extension/coverage`).

**Root cause**: DevTools-only extensions (with `devtools_page` but no `background` service worker) don't create CDP-visible targets. There's no `chrome-extension://` target to attach to.

**Workaround**: None currently. DevTools panel extensions run in the DevTools process, not as separate targets.

**Observed**: 2026-04-05.
