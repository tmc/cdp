package cdpscript

import (
	"context"
	"fmt"
	"strconv"

	"github.com/chromedp/cdproto/input"
	"github.com/tmc/cdp/internal/cdpinput"
	"github.com/tmc/cdp/internal/chromedp"
	"rsc.io/script"
)

type dragSpec struct {
	source string
	target string
	steps  int
}

func (e *Engine) cmdDrag() script.Cmd {
	return simpleCmd("drag from source to target", "source target [steps]", func(s *script.State, args []string) error {
		spec, err := parseDragArgs(args)
		if err != nil {
			return err
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		ctx := e.browser.Context()
		src, err := dragPoint(ctx, spec.source)
		if err != nil {
			return fmt.Errorf("drag source: %w", err)
		}
		dst, err := dragPoint(ctx, spec.target)
		if err != nil {
			return fmt.Errorf("drag target: %w", err)
		}
		if e.verbose {
			fmt.Fprintf(e.stderr, "[drag] %s -> %s (%d steps)\n", spec.source, spec.target, spec.steps)
		}
		return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			return dispatchDrag(ctx, src, dst, spec.steps)
		}))
	})
}

func parseDragArgs(args []string) (dragSpec, error) {
	if len(args) < 2 || len(args) > 3 {
		return dragSpec{}, fmt.Errorf("drag requires source, target, and optional steps")
	}
	steps := 10
	if len(args) == 3 {
		n, err := strconv.Atoi(args[2])
		if err != nil {
			return dragSpec{}, fmt.Errorf("drag steps: %w", err)
		}
		if n <= 0 {
			return dragSpec{}, fmt.Errorf("drag steps must be positive")
		}
		steps = n
	}
	return dragSpec{source: args[0], target: args[1], steps: steps}, nil
}

func dragPoint(ctx context.Context, target string) (cdpinput.ViewportPoint, error) {
	if p, ok, err := cdpinput.ParseCoordSelector(target); ok || err != nil {
		return p, err
	}

	var point cdpinput.ViewportPoint
	if err := chromedp.Run(ctx, chromedp.Evaluate(dragPointScript(target), &point)); err != nil {
		return cdpinput.ViewportPoint{}, err
	}
	return point, nil
}

func dragPointScript(selector string) string {
	return fmt.Sprintf(`(function() {
	const el = document.querySelector(%q);
	if (!el) throw new Error("no element matches " + %q);
	el.scrollIntoView({behavior: "instant", block: "center", inline: "center"});
	const rect = el.getBoundingClientRect();
	return {X: rect.left + rect.width / 2, Y: rect.top + rect.height / 2};
})()`, selector, selector)
}

func dispatchDrag(ctx context.Context, src, dst cdpinput.ViewportPoint, steps int) error {
	if err := input.DispatchMouseEvent(input.MouseMoved, src.X, src.Y).Do(ctx); err != nil {
		return fmt.Errorf("mouse move source: %w", err)
	}
	if err := input.DispatchMouseEvent(input.MousePressed, src.X, src.Y).
		WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
		return fmt.Errorf("mouse down: %w", err)
	}
	for i := 1; i <= steps; i++ {
		frac := float64(i) / float64(steps)
		x := src.X + (dst.X-src.X)*frac
		y := src.Y + (dst.Y-src.Y)*frac
		if err := input.DispatchMouseEvent(input.MouseMoved, x, y).
			WithButton(input.Left).Do(ctx); err != nil {
			return fmt.Errorf("mouse move step %d: %w", i, err)
		}
	}
	if err := input.DispatchMouseEvent(input.MouseReleased, dst.X, dst.Y).
		WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
		return fmt.Errorf("mouse up: %w", err)
	}
	return nil
}
