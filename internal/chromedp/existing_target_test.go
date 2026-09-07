package chromedp

import (
	"context"
	"fmt"
	"testing"

	"github.com/chromedp/cdproto/cdp"
)

func TestExistingTargetInvalidOptions(t *testing.T) {
	for _, tt := range []struct {
		name    string
		options []ContextOption
	}{
		{"empty", []ContextOption{WithExistingTarget("")}},
		{"new context", []ContextOption{WithExistingTarget("target"), WithNewBrowserContext()}},
		{"new context first", []ContextOption{WithNewBrowserContext(), WithExistingTarget("target")}},
		{"existing context", []ContextOption{WithExistingTarget("target"), WithExistingBrowserContext(cdp.BrowserContextID("context"))}},
		{"existing context first", []ContextOption{WithExistingBrowserContext(cdp.BrowserContextID("context")), WithExistingTarget("target")}},
		{"owned then borrowed", []ContextOption{WithTargetID("target"), WithExistingTarget("target")}},
		{"borrowed then owned", []ContextOption{WithExistingTarget("target"), WithTargetID("target")}},
		{"two borrowed", []ContextOption{WithExistingTarget("a"), WithExistingTarget("b")}},
		{"empty then owned", []ContextOption{WithExistingTarget(""), WithTargetID("target")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := NewContext(context.Background(), tt.options...)
			defer cancel()
			if err := Run(ctx); err == nil {
				t.Fatal("invalid options succeeded")
			}
			if c := FromContext(ctx); c.Browser != nil || c.Target != nil {
				t.Fatal("invalid options allocated browser or target")
			}
		})
	}
}
func ExampleWithExistingTarget() {
	ctx, cancel := NewContext(context.Background(), WithExistingTarget(""))
	defer cancel()
	fmt.Println(Run(ctx))
	// Output: existing target ID is empty
}
