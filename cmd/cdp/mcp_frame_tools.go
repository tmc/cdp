package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Frame/iframe navigation tools ---

type frameInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	URL      string `json:"url"`
	ParentID string `json:"parent_id,omitempty"`
}

type ListFramesOutput struct {
	Frames []frameInfo `json:"frames"`
}

type SwitchFrameInput struct {
	Frame string `json:"frame"`
}

func registerFrameTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_frames",
		Description: "List all frames (including iframes) in the current page. Returns frame ID, name, URL, and parent.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, ListFramesOutput, error) {
		actx := s.activeCtx()
		var tree *page.FrameTree
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			tree, err = page.GetFrameTree().Do(ctx)
			return err
		})); err != nil {
			return nil, ListFramesOutput{}, fmt.Errorf("list_frames: %w", err)
		}

		var frames []frameInfo
		flattenFrameTree(tree, &frames)
		return nil, ListFramesOutput{Frames: frames}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "switch_frame",
		Description: `Switch execution context to a frame. Use "main" for the top frame, a frame name, a numeric index (from list_frames), or a CSS selector for the iframe element.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SwitchFrameInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()

		// "main" returns to the top frame / browser context.
		if input.Frame == "main" || input.Frame == "top" || input.Frame == "" {
			s.setActiveCtx(s.browserCtx, nil)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "switched to main frame"}},
			}, nil, nil
		}

		// Get the frame tree to resolve the target.
		var tree *page.FrameTree
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			tree, err = page.GetFrameTree().Do(ctx)
			return err
		})); err != nil {
			return nil, nil, fmt.Errorf("switch_frame: get frame tree: %w", err)
		}

		var frames []frameInfo
		flattenFrameTree(tree, &frames)

		frameID, err := resolveFrame(frames, input.Frame, actx)
		if err != nil {
			return nil, nil, fmt.Errorf("switch_frame: %w", err)
		}

		// Create a new chromedp context targeting the frame's page target.
		// For iframes, we use the frame's target ID if it's an OOPIF,
		// or navigate within the existing context.
		// chromedp doesn't have direct frame targeting, so we use the
		// iframe's content document via evaluate in the frame's execution context.

		// For same-origin frames, we can use chromedp.WithTargetID if the
		// frame has its own target. Otherwise, we'll set a frame execution
		// context via the Page domain.

		// The simplest reliable approach: find the target for the frame ID.
		targets, err := chromedp.Targets(s.browserCtx)
		if err != nil {
			return nil, nil, fmt.Errorf("switch_frame: list targets: %w", err)
		}

		for _, t := range targets {
			if t.Type == "iframe" && string(t.TargetID) == frameID {
				frameCtx, frameCancel := chromedp.NewContext(s.browserCtx, chromedp.WithTargetID(target.ID(frameID)))
				if err := chromedp.Run(frameCtx); err != nil {
					frameCancel()
					return nil, nil, fmt.Errorf("switch_frame: attach to frame: %w", err)
				}
				s.setActiveCtx(frameCtx, frameCancel)
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("switched to frame %s", input.Frame)}},
				}, nil, nil
			}
		}

		// For same-origin frames that don't have their own target,
		// store the frame ID and use it with subsequent operations.
		s.mu.Lock()
		s.activeFrameID = cdp.FrameID(frameID)
		s.mu.Unlock()

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("switched to frame %s (id: %s)", input.Frame, frameID)}},
		}, nil, nil
	})
}

func flattenFrameTree(tree *page.FrameTree, out *[]frameInfo) {
	if tree == nil || tree.Frame == nil {
		return
	}
	f := tree.Frame
	*out = append(*out, frameInfo{
		ID:       string(f.ID),
		Name:     f.Name,
		URL:      f.URL,
		ParentID: string(f.ParentID),
	})
	for _, child := range tree.ChildFrames {
		flattenFrameTree(child, out)
	}
}

// resolveFrame finds the frame ID from a name, index, or CSS selector.
func resolveFrame(frames []frameInfo, query string, ctx context.Context) (string, error) {
	// Try numeric index (0-based into child frames, skipping main frame).
	if idx, err := strconv.Atoi(query); err == nil {
		childFrames := frames[1:] // skip main frame
		if idx < 0 || idx >= len(childFrames) {
			return "", fmt.Errorf("frame index %d out of range (have %d child frames)", idx, len(childFrames))
		}
		return childFrames[idx].ID, nil
	}

	// Try by name.
	for _, f := range frames {
		if f.Name == query {
			return f.ID, nil
		}
	}

	// Try by frame ID directly.
	for _, f := range frames {
		if f.ID == query {
			return f.ID, nil
		}
	}

	// Try CSS selector — get the iframe element's contentDocument frame ID.
	var frameID string
	js := fmt.Sprintf(`(function() {
		var el = document.querySelector(%q);
		if (!el || !el.contentWindow) return '';
		// Try to get frame ID from the element's src or the content document.
		return el.getAttribute('name') || el.getAttribute('id') || el.src || '';
	})()`, query)
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &frameID)); err == nil && frameID != "" {
		// Match by name, id, or URL from the selector result.
		for _, f := range frames {
			if f.Name == frameID || f.ID == frameID || f.URL == frameID {
				return f.ID, nil
			}
		}
	}

	return "", fmt.Errorf("frame not found: %s", query)
}
