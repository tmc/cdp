package cdpinput

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/chromedp/cdproto/input"
)

// ViewportPoint is a CSS-pixel coordinate in the current viewport.
type ViewportPoint struct {
	X float64
	Y float64
}

// ParseCoordSelector parses coord:x,y viewport coordinates.
func ParseCoordSelector(selector string) (ViewportPoint, bool, error) {
	coord, ok := strings.CutPrefix(strings.TrimSpace(selector), "coord:")
	if !ok {
		return ViewportPoint{}, false, nil
	}
	parts := strings.Split(coord, ",")
	if len(parts) != 2 {
		return ViewportPoint{}, true, fmt.Errorf("invalid coordinate selector %q: want coord:x,y", selector)
	}
	x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return ViewportPoint{}, true, fmt.Errorf("invalid x coordinate %q: %w", parts[0], err)
	}
	y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return ViewportPoint{}, true, fmt.Errorf("invalid y coordinate %q: %w", parts[1], err)
	}
	if x < 0 || y < 0 || math.IsInf(x, 0) || math.IsInf(y, 0) || math.IsNaN(x) || math.IsNaN(y) {
		return ViewportPoint{}, true, fmt.Errorf("invalid coordinate selector %q: coordinates must be finite non-negative numbers", selector)
	}
	return ViewportPoint{X: x, Y: y}, true, nil
}

// ClickAt clicks viewport coordinates using compositor-level mouse events.
func ClickAt(ctx context.Context, p ViewportPoint) error {
	if err := input.DispatchMouseEvent(input.MouseMoved, p.X, p.Y).Do(ctx); err != nil {
		return fmt.Errorf("mouse moved: %w", err)
	}
	if err := input.DispatchMouseEvent(input.MousePressed, p.X, p.Y).
		WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
		return fmt.Errorf("mouse pressed: %w", err)
	}
	if err := input.DispatchMouseEvent(input.MouseReleased, p.X, p.Y).
		WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
		return fmt.Errorf("mouse released: %w", err)
	}
	return nil
}
