# Known Issues — cdp MCP Tools

## Resolved Issues

### click/type_text/hover/focus/press_key timeout units ambiguous

Resolved by `interactionCtx`: timeout values from 1 through 60 are seconds;
values greater than 60 are treated as milliseconds. This preserves the
documented seconds API while handling common agent inputs such as `5000` for
5 seconds.

**Observed**: 2026-04-05, A998 session. `click(selector: "a[href='/explore']", timeout: 5000)` hung for 6+ minutes on github.com.

**Files**: `cmd/cdp/mcp_tools.go`, `cmd/cdp/mcp_click_test.go`

## Active Issues

### 1. click can hang on elements that trigger navigation

**Symptom**: `click` on a link that navigates the page may hang if the navigation changes the DOM before chromedp's click action completes. The element becomes stale mid-action.

**Workaround**: Use `evaluate` with `document.querySelector('a').click()` for navigation-triggering clicks, or use `navigate` directly if the URL is known.

**Files**: `cmd/cdp/mcp_tools.go:248-270`

### 2. Coverage snapshot on minimal-JS pages returns empty

**Symptom**: `get_coverage` after `start_coverage` on server-rendered pages (e.g., Hacker News) returns 0 files because there's little/no JS to profile.

**Not a bug**: Expected behavior — V8 coverage only tracks JavaScript execution. Document this in tool description.

**Observed**: 2026-04-05, A998 session on news.ycombinator.com.

### 3. CDP Extensions domain requires --remote-debugging-pipe, not --remote-debugging-port

**Symptom**: `list_extensions` returns null, `install_extension` via `Extensions.loadUnpacked` fails with "Method not available". CDP Extensions domain commands only work with pipe transport.

**Root cause**: Chrome's Extensions CDP domain is gated behind `--remote-debugging-pipe` + `--enable-unsafe-extension-debugging`. We connect via `--remote-debugging-port` (WebSocket), which doesn't expose the Extensions domain.

**Workaround**: JS injection via `chrome.developerPrivate` API on `chrome://extensions` page works for most operations. `reload_extension` already uses this successfully. `--load-extension` CLI flag works for loading at launch.

**Fix**: Add JS fallback to `list_extensions` using `developerPrivate.getExtensionsInfo()`. Consider supporting pipe transport as an option for full Extensions domain access.

**Observed**: 2026-04-05, A998 extension test. Brave 146.1.88.138.

### 4. extension_console/extension_evaluate fail for devtools-only extensions

**Symptom**: "no target found for extension" when calling `extension_console` or `extension_evaluate` on a DevTools panel extension (like our coverage extension).

**Root cause**: DevTools-only extensions (with `devtools_page` but no `background` service worker) don't create CDP-visible targets. There's no `chrome-extension://` target to attach to.

**Workaround**: None currently. DevTools panel extensions run in the DevTools process, not as separate targets.

**Observed**: 2026-04-05, A998 extension test. Extension ID agmhhbefggjmejggmflmppmacnbmhnne.
