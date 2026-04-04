package main

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Scroll tool ---

type ScrollInput struct {
	Direction string `json:"direction,omitempty"`
	Distance  int    `json:"distance,omitempty"`
	Selector  string `json:"selector,omitempty"`
}

func registerScrollTool(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "scroll",
		Description: `Scroll the page or an element. Use direction (up/down/left/right) with optional distance in pixels (default 500). Or provide a selector/@ref to scroll that element into view.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, inp ScrollInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()

		// If selector is provided, scroll that element into view.
		if inp.Selector != "" {
			backendID, err := resolveRef(s.refs, inp.Selector)
			if err != nil {
				return nil, nil, fmt.Errorf("scroll: %w", err)
			}
			if backendID != 0 {
				if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
					return dom.ScrollIntoViewIfNeeded().WithBackendNodeID(backendID).Do(ctx)
				})); err != nil {
					return nil, nil, fmt.Errorf("scroll: %w", err)
				}
			} else {
				js := fmt.Sprintf(`document.querySelector(%q)?.scrollIntoView({behavior:'smooth',block:'center'})`, inp.Selector)
				if err := chromedp.Run(actx, chromedp.Evaluate(js, nil)); err != nil {
					return nil, nil, fmt.Errorf("scroll: %w", err)
				}
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "scrolled " + inp.Selector + " into view"}},
			}, nil, nil
		}

		// Direction-based scroll via mouse wheel events.
		distance := inp.Distance
		if distance == 0 {
			distance = 500
		}

		var deltaX, deltaY float64
		switch inp.Direction {
		case "down", "":
			deltaY = float64(distance)
		case "up":
			deltaY = -float64(distance)
		case "right":
			deltaX = float64(distance)
		case "left":
			deltaX = -float64(distance)
		default:
			return nil, nil, fmt.Errorf("scroll: unknown direction %q (use up/down/left/right)", inp.Direction)
		}

		// Get viewport center for scroll position.
		var result map[string]any
		if err := chromedp.Run(actx, chromedp.Evaluate(`({x: window.innerWidth/2, y: window.innerHeight/2})`, &result)); err != nil {
			return nil, nil, fmt.Errorf("scroll: get viewport: %w", err)
		}
		x, _ := result["x"].(float64)
		y, _ := result["y"].(float64)

		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseWheel, x, y).
				WithDeltaX(deltaX).WithDeltaY(deltaY).Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("scroll: %w", err)
		}

		dir := inp.Direction
		if dir == "" {
			dir = "down"
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("scrolled %s %dpx", dir, distance)}},
		}, nil, nil
	})
}
