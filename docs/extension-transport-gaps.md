# Extension Tools: Transport Gaps & Research Needed

## Problem

The CDP Extensions domain (`Extensions.loadUnpacked`, `Extensions.getExtensions`, `Extensions.uninstall`, storage APIs) requires Chrome to be launched with `--remote-debugging-pipe` transport. We use `--remote-debugging-port` (WebSocket), so the Extensions domain is unavailable.

Additionally, DevTools-only extensions (like our coverage panel — `devtools_page` manifest key, no `background` service worker) don't create CDP-visible targets, so `extension_console` and `extension_evaluate` can't attach to them.

## Current State (from A998 live testing, 2026-04-05)

| Tool | CDP Domain | JS Fallback | Status |
|------|-----------|-------------|--------|
| list_extensions | Extensions.getExtensions — FAILS (pipe only) | developerPrivate.getExtensionsInfo — WORKS | Needs fallback wired |
| install_extension | Extensions.loadUnpacked — FAILS (pipe only) | developerPrivate.loadUnpacked — untested | --load-extension flag works at launch |
| reload_extension | N/A | developerPrivate.reload — WORKS | Done |
| uninstall_extension | Extensions.uninstall — FAILS (pipe only) | developerPrivate.uninstall — untested | Not built |
| extension_console | Target attach — FAILS (no SW target) | ? | Broken for devtools exts |
| extension_evaluate | Target attach — FAILS (no SW target) | ? | Broken for devtools exts |
| get/set/clear_extension_storage | Extensions.getStorageItems etc — FAILS (pipe only) | Runtime.evaluate on ext page — untested | Not built |

## Research Questions for E794

### 1. Can chromedp use pipe transport?

Chrome supports `--remote-debugging-pipe` which uses stdin/stdout fd 3/4 instead of WebSocket. This enables the full Extensions domain.

- Does chromedp support pipe transport? Check `chromedp.NewAllocator` options.
- If not, can we use a raw CDP client (like ndp's V8InspectorClient) alongside chromedp for just the Extensions domain?
- Could we launch Chrome with BOTH `--remote-debugging-port` (for chromedp) and `--remote-debugging-pipe` (for extensions)? Check if Chrome allows both simultaneously.

### 2. chrome.developerPrivate API surface

`chrome.developerPrivate` is available when evaluating JS on `chrome://extensions`. It's undocumented but stable.

- Map the full API surface: `getExtensionsInfo`, `loadUnpacked`, `uninstall`, `reload`, what else?
- Does `developerPrivate.loadUnpacked` work without a file picker dialog? It may require user gesture.
- Is there a `developerPrivate.getExtensionStorage` or similar?
- Can we call it from any chrome:// page or only chrome://extensions?

### 3. DevTools extension targets

Our coverage extension has `devtools_page` but no `background` service worker. It runs inside the DevTools window, not as a separate extension process.

- When DevTools is open with our panel active, does a new CDP target appear? Check `Target.getTargets()` while DevTools is open.
- Is there a `chrome-devtools://` URL target we could attach to?
- Can we use `chrome.devtools.inspectedWindow.eval()` from outside to interact with the panel?
- Would adding a minimal background service worker to our extension fix the target attach issue? (Even if the SW does nothing, it creates a target.)

### 4. --load-extension runtime behavior

- Can `--load-extension` paths be added after Chrome launch via CDP? Or is it launch-time only?
- Does `chrome.management.installFromLocalPath()` exist? (Probably not in stable.)
- For the embedded extension distribution (go:embed), is extracting to ~/.cdp/extensions/ + --load-extension at launch good enough? Or do we need runtime install?

### 5. Extension storage without pipe transport

- Can we read/write extension storage via `Runtime.evaluate` in the extension's context?
- If we attach to an extension's background SW target, can we evaluate `chrome.storage.local.get()`?
- For devtools extensions without a SW, can we inject a content script that proxies storage access?

## Proposed Architecture After Research

**Tier 1 (JS fallback — works now with port transport):**
- list_extensions → developerPrivate.getExtensionsInfo() on chrome://extensions tab
- install_extension → developerPrivate.loadUnpacked() (if no file picker needed)
- reload_extension → developerPrivate.reload() (already works)
- uninstall_extension → developerPrivate.uninstall()

**Tier 2 (needs investigation):**
- extension_console/evaluate → add background SW to our extensions, or find devtools targets
- extension storage → Runtime.evaluate in SW context, or developerPrivate equivalent

**Tier 3 (pipe transport — optional, full CDP access):**
- If chromedp can do pipe OR we can use both transports, switch to full CDP Extensions domain
- This gives us typed Go APIs instead of JS string injection

## Files

- `cmd/cdp/mcp_extension_tools.go` — current implementation
- `cmd/cdp/mcp_extension_tools_test.go` — tests
- `extension/coverage/manifest.json` — our coverage extension (devtools-only, no SW)
- `docs/extension-dev-tools.md` — original design
- `docs/known-issues.md` — issues #4 and #5
