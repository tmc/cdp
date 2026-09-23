# Internal chromedp runtime

This directory carries the runtime sources and embedded JavaScript from
chromedp v0.16.0 (commit `7963c203ed5458147d27dc39a5c06d2b12e81664`). The upstream
license is retained in LICENSE. The upstream module checksum is
`h1:rOO4deOm4CbZgBCa8mD9g2rDyIoNs0BkgvNrlbp5ouk=`.

Local changes, all to be re-applied when updating the copy:

- Import paths are rewritten from `github.com/chromedp/chromedp` to
  `github.com/tmc/cdp/internal/chromedp`.
- chromedp.go adds `WithExistingTarget`: canceling a borrowed attachment
  releases its session without closing the browser's target. `WithTargetID`
  and newly allocated targets retain their existing close-on-cancel behavior.
  Invalid borrowed-target options, including conflicting target options and a
  browser-context option on a borrowed target, fail during `Run`, before
  allocation.
- chromedp.go moves the "can not be used before Browser is initialized"
  panics of `WithNewBrowserContext` and `WithExistingBrowserContext` from the
  options into `NewContext`, after option collection, so they apply in either
  option order.
- chromedp.go makes a failed browser allocation sticky: `Run` records the
  error and returns it on later calls instead of allocating again, which
  would close the allocator's one-shot channel twice and panic.
- The `//go:generate go run gen.go` directives are removed from kb/kb.go,
  device/device.go, and the templates in kb/gen.go and device/gen.go, so
  `go generate ./...` does not regenerate those files from live sources and
  drift from the pinned version. Run gen.go by hand only as part of an update.

The MCP/browser implementation imports this package. The public `cdpscripttest`
package and its CLI still use upstream chromedp, preserving their exported
allocator options and action types. Contexts, options and concrete runtime types
from the two implementations must not be exchanged. Protocol data types remain
provided by the shared cdproto dependency. The shared browser feature list is a
string, so each runtime can construct its own allocator option.

The runtime sources, `kb`, `device`, embedded `js`, and license are copied here;
the upstream module file, test image fixtures, and upstream test suite are not.
The local configuration and lifecycle regression tests cover the changed
behavior. Changes to this copy require comparison with the pinned upstream
source and review of the upstream tests affected by the update. Preserve source
and license provenance when updating; do not edit the Go module cache.

An internal import path keeps versioned `go install` working without a module
replacement directive. This is a maintained source copy, not an independently
released module. Remove it when an upstream API can supply the required borrowed
attachment contract, after qualifying the same lifecycle tests.
