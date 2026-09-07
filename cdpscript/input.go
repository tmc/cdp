package cdpscript

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tmc/cdp/internal/cdpinput"
	"github.com/tmc/cdp/internal/chromedp"
	"rsc.io/script"
)

// cmdDblclick dispatches a real double click.
//
// A synthetic dblclick from JavaScript exercises the handler but not the input
// path, so it cannot catch a listener attached to the wrong element or one
// shadowed by an overlay. This dispatches the same press/release pairs a mouse
// would, with an increasing click count.
func (e *Engine) cmdDblclick() script.Cmd {
	return simpleCmd("double-click element", "selector|coord:x,y", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("dblclick requires a selector or coord:x,y")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		target := strings.Join(args, " ")
		if strings.HasPrefix(target, "@") {
			return fmt.Errorf("dblclick does not accept @ref targets; use a CSS selector or coord:x,y")
		}
		if e.verbose {
			fmt.Fprintf(e.stderr, "[dblclick] %s\n", target)
		}
		ctx := e.browser.Context()
		p, err := dragPoint(ctx, target)
		if err != nil {
			return fmt.Errorf("dblclick target: %w", err)
		}
		return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			return cdpinput.ClickAtCount(ctx, p, 2)
		}))
	})
}

// cmdSetRange sets the value of an <input type="range"> and fires the events a
// real drag would.
//
// Range inputs cannot be driven by fill or type, which target text entry. This
// sets .value and dispatches input then change, which is the workaround every
// fixture writes by hand. It is deliberately not a pointer gesture: computing
// a thumb position from the track geometry is fragile, and the durable effect
// is the same.
func (e *Engine) cmdSetRange() script.Cmd {
	return simpleCmd("set the value of a range input", "selector value", func(s *script.State, args []string) error {
		if len(args) != 2 {
			return fmt.Errorf("set-range requires a selector and a value")
		}
		selector, value := args[0], args[1]
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("set-range value %q: not a number", value)
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		if e.verbose {
			fmt.Fprintf(e.stderr, "[set-range] %s = %s\n", selector, value)
		}

		var got string
		script := fmt.Sprintf(`(function() {
	const el = document.querySelector(%q);
	if (!el) throw new Error("no element matches " + %q);
	if (el.type !== "range") throw new Error(%q + " is not an input[type=range] (type=" + el.type + ")");
	el.value = %q;
	el.dispatchEvent(new Event("input", {bubbles: true}));
	el.dispatchEvent(new Event("change", {bubbles: true}));
	return el.value;
})()`, selector, selector, selector, value)

		if err := chromedp.Run(e.browser.Context(), chromedp.Evaluate(script, &got)); err != nil {
			return fmt.Errorf("set-range: %w", err)
		}
		// The browser clamps to min/max and snaps to step, so report when the
		// value that landed is not the one asked for.
		if got != value {
			if e.verbose {
				fmt.Fprintf(e.stderr, "[set-range] clamped to %s\n", got)
			}
		}
		return nil
	})
}

// cmdMouse exposes the press, move, and release primitives so a fixture can
// assert in the middle of a gesture — a drag ghost, a live preview — which the
// atomic drag command cannot express. drag remains the common-case wrapper.
func (e *Engine) cmdMouse() script.Cmd {
	return simpleCmd("press, move, or release the mouse", "down|move|up [selector|coord:x,y]", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("mouse requires down, move, or up")
		}
		action := args[0]
		rest := args[1:]

		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		ctx := e.browser.Context()

		// down and move need a target; up defaults to wherever the pointer is.
		var p cdpinput.ViewportPoint
		switch {
		case len(rest) > 0:
			var err error
			target := strings.Join(rest, " ")
			if p, err = dragPoint(ctx, target); err != nil {
				return fmt.Errorf("mouse %s target: %w", action, err)
			}
		case action == "up":
			if !e.mouseTracked {
				return fmt.Errorf("mouse up without a target and no prior mouse position")
			}
			p = e.mousePos
		default:
			return fmt.Errorf("mouse %s requires a selector or coord:x,y", action)
		}

		if e.verbose {
			fmt.Fprintf(e.stderr, "[mouse] %s at %.0f,%.0f\n", action, p.X, p.Y)
		}

		var run func(context.Context) error
		switch action {
		case "down":
			run = func(ctx context.Context) error {
				if err := cdpinput.MouseMoveAt(ctx, p, false); err != nil {
					return err
				}
				return cdpinput.MouseDownAt(ctx, p)
			}
		case "move":
			held := e.mouseDown
			run = func(ctx context.Context) error { return cdpinput.MouseMoveAt(ctx, p, held) }
		case "up":
			run = func(ctx context.Context) error { return cdpinput.MouseUpAt(ctx, p) }
		default:
			return fmt.Errorf("mouse: unknown action %q, want down, move, or up", action)
		}

		if err := chromedp.Run(ctx, chromedp.ActionFunc(run)); err != nil {
			return fmt.Errorf("mouse %s: %w", action, err)
		}

		e.mousePos, e.mouseTracked = p, true
		switch action {
		case "down":
			e.mouseDown = true
		case "up":
			e.mouseDown = false
		}
		return nil
	})
}
