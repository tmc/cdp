// Package scriptbrowser lets commands in this module run a cdpscript.Engine
// against a browser they already own.
//
// The browser must be attached to the context through this module's
// internal/chromedp copy, which code outside the module cannot import. That
// is why the hook is internal: external callers share a browser with
// cdpscript.WithRemoteTab instead.
package scriptbrowser

import "context"

type borrowKey struct{}

// Borrow returns a copy of ctx marked so that a cdpscript.Engine executing
// with it runs against the browser attached to ctx instead of launching its
// own. The engine does not close a borrowed browser. If ctx carries no
// browser, the engine launches its own as usual.
func Borrow(ctx context.Context) context.Context {
	return context.WithValue(ctx, borrowKey{}, true)
}

// Borrowed reports whether ctx was marked by Borrow.
func Borrowed(ctx context.Context) bool {
	b, _ := ctx.Value(borrowKey{}).(bool)
	return b
}
