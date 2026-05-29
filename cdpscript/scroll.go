package cdpscript

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"rsc.io/script"
)

type scrollSpec struct {
	direction string
	distance  int
	selector  string
}

func (e *Engine) cmdScroll() script.Cmd {
	return simpleCmd("scroll page or element", "[up|down|left|right [px]|selector]", func(s *script.State, args []string) error {
		spec, err := parseScrollArgs(args)
		if err != nil {
			return err
		}
		ctx := e.browser.Context()
		if spec.selector != "" {
			if e.verbose {
				fmt.Fprintf(os.Stderr, "[scroll] %s\n", spec.selector)
			}
			var ok bool
			if err := chromedp.Run(ctx, chromedp.Evaluate(scrollSelectorScript(spec.selector), &ok)); err != nil {
				return fmt.Errorf("scroll: %w", err)
			}
			if !ok {
				return fmt.Errorf("scroll: no element matches %q", spec.selector)
			}
			return e.setScrollEnv(ctx, s)
		}

		if e.verbose {
			fmt.Fprintf(os.Stderr, "[scroll] %s %d\n", spec.direction, spec.distance)
		}
		if err := e.scrollBy(ctx, spec.direction, spec.distance); err != nil {
			return err
		}
		return e.setScrollEnv(ctx, s)
	})
}

func parseScrollArgs(args []string) (scrollSpec, error) {
	if len(args) == 0 {
		return scrollSpec{direction: "down", distance: 500}, nil
	}
	switch args[0] {
	case "down", "up", "left", "right":
		if len(args) > 2 {
			return scrollSpec{}, fmt.Errorf("scroll %s accepts at most one distance", args[0])
		}
		distance := 500
		if len(args) == 2 {
			n, err := strconv.Atoi(args[1])
			if err != nil {
				return scrollSpec{}, fmt.Errorf("scroll distance: %w", err)
			}
			if n <= 0 {
				return scrollSpec{}, fmt.Errorf("scroll distance must be positive")
			}
			distance = n
		}
		return scrollSpec{direction: args[0], distance: distance}, nil
	default:
		return scrollSpec{selector: strings.Join(args, " ")}, nil
	}
}

func (e *Engine) scrollBy(ctx context.Context, direction string, distance int) error {
	var deltaX, deltaY float64
	switch direction {
	case "down":
		deltaY = float64(distance)
	case "up":
		deltaY = -float64(distance)
	case "right":
		deltaX = float64(distance)
	case "left":
		deltaX = -float64(distance)
	default:
		return fmt.Errorf("scroll: unknown direction %q", direction)
	}

	var point struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`({x: window.innerWidth/2, y: window.innerHeight/2})`, &point)); err != nil {
		return fmt.Errorf("scroll: get viewport: %w", err)
	}
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseWheel, point.X, point.Y).
			WithDeltaX(deltaX).WithDeltaY(deltaY).Do(ctx)
	})); err != nil {
		return fmt.Errorf("scroll: %w", err)
	}
	return nil
}

func scrollSelectorScript(selector string) string {
	return fmt.Sprintf(`(function() {
	const el = document.querySelector(%q);
	if (!el) return false;
	el.scrollIntoView({behavior: "instant", block: "center", inline: "center"});
	return true;
})()`, selector)
}

func (e *Engine) setScrollEnv(ctx context.Context, s *script.State) error {
	var pos struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`({x: Math.round(window.scrollX), y: Math.round(window.scrollY)})`, &pos)); err != nil {
		return fmt.Errorf("scroll: read position: %w", err)
	}
	s.Setenv("SCROLL_X", strconv.Itoa(pos.X))
	s.Setenv("SCROLL_Y", strconv.Itoa(pos.Y))
	return nil
}
