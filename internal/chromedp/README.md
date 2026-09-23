# Internal chromedp runtime

This directory carries the runtime sources and embedded JavaScript from
chromedp v0.16.0 (commit `7963c203ed5458147d27dc39a5c06d2b12e81664`). The upstream
license is retained in LICENSE. The upstream module checksum is
`h1:rOO4deOm4CbZgBCa8mD9g2rDyIoNs0BkgvNrlbp5ouk=`.

The local change adds `WithExistingTarget`: canceling a borrowed attachment
releases its session without closing the browser's target. `WithTargetID` and
newly allocated targets retain their existing close-on-cancel behavior. Invalid
borrowed-target options fail during `Run`, before allocation. Browser-context
option validation runs after option collection to handle both option orders.

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
