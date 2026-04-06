package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Action diff tool ---

type ActionDiffInput struct {
	Action string `json:"action"`            // "click", "type", or "navigate"
	Params string `json:"params,omitempty"`   // JSON params for the action: selector, text, url, etc.
	Width  int    `json:"width,omitempty"`    // max width for returned images
}

// actionDiffParams holds parsed action parameters.
type actionDiffParams struct {
	Selector string `json:"selector,omitempty"`
	Text     string `json:"text,omitempty"`
	Submit   bool   `json:"submit,omitempty"`
	URL      string `json:"url,omitempty"`
}

func registerActionDiffTool(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "action_diff",
		Description: `Execute an action and return before/after/diff screenshots with change percentage.
Actions: "click" (needs selector), "type" (needs selector + text), "navigate" (needs url).
Params is a JSON string, e.g. {"selector": "@1"} or {"url": "https://example.com"}.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ActionDiffInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()

		// Parse params.
		var params actionDiffParams
		if input.Params != "" {
			if err := parseJSON(input.Params, &params); err != nil {
				return nil, nil, fmt.Errorf("action_diff: parse params: %w", err)
			}
		}

		// Take "before" screenshot.
		beforePNG, err := captureViewportPNG(actx)
		if err != nil {
			return nil, nil, fmt.Errorf("action_diff: before screenshot: %w", err)
		}

		// Execute the action with a timeout to prevent hanging on
		// hidden/inaccessible elements (e.g. VS Code's shadow DOM).
		if err := executeActionWithTimeout(actx, s, input.Action, params, 10*time.Second); err != nil {
			return nil, nil, fmt.Errorf("action_diff: %s: %w", input.Action, err)
		}

		// Take "after" screenshot.
		afterPNG, err := captureViewportPNG(actx)
		if err != nil {
			return nil, nil, fmt.Errorf("action_diff: after screenshot: %w", err)
		}

		// Compute pixel diff.
		diffPNG, changePct, err := pixelDiff(beforePNG, afterPNG)
		if err != nil {
			return nil, nil, fmt.Errorf("action_diff: diff: %w", err)
		}

		// Optionally downscale.
		mimeType := "image/png"
		if input.Width > 0 {
			beforePNG, _ = downsizeImage(beforePNG, input.Width, mimeType)
			afterPNG, _ = downsizeImage(afterPNG, input.Width, mimeType)
			diffPNG, _ = downsizeImage(diffPNG, input.Width, mimeType)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("action=%s change=%.2f%%", input.Action, changePct)},
				&mcp.ImageContent{Data: beforePNG, MIMEType: mimeType},
				&mcp.ImageContent{Data: afterPNG, MIMEType: mimeType},
				&mcp.ImageContent{Data: diffPNG, MIMEType: mimeType},
			},
		}, nil, nil
	})
}

// captureViewportPNG captures the current viewport as PNG.
func captureViewportPNG(ctx context.Context) ([]byte, error) {
	var buf []byte
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		buf, err = page.CaptureScreenshot().
			WithFormat(page.CaptureScreenshotFormatPng).Do(ctx)
		return err
	})); err != nil {
		return nil, err
	}
	return buf, nil
}

// executeActionWithTimeout wraps executeAction with a deadline.
func executeActionWithTimeout(ctx context.Context, s *mcpSession, action string, params actionDiffParams, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- executeAction(ctx, s, action, params)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timed out after %s", timeout)
	}
}

// executeAction runs the specified action.
func executeAction(ctx context.Context, s *mcpSession, action string, params actionDiffParams) error {
	switch action {
	case "click":
		if params.Selector == "" {
			return fmt.Errorf("click requires selector")
		}
		return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			backendID, err := resolveRefWithRecovery(ctx, s.refs, params.Selector)
			if err != nil {
				return err
			}
			if backendID != 0 {
				return clickByBackendNodeID(ctx, backendID)
			}
			return chromedp.Click(params.Selector, chromedp.ByQuery).Do(ctx)
		}))
	case "type":
		if params.Selector == "" {
			return fmt.Errorf("type requires selector")
		}
		text := params.Text
		if params.Submit {
			text += "\n"
		}
		return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			backendID, err := resolveRefWithRecovery(ctx, s.refs, params.Selector)
			if err != nil {
				return err
			}
			if backendID != 0 {
				return typeByBackendNodeID(ctx, backendID, text)
			}
			return chromedp.SendKeys(params.Selector, text, chromedp.ByQuery).Do(ctx)
		}))
	case "navigate":
		if params.URL == "" {
			return fmt.Errorf("navigate requires url")
		}
		return chromedp.Run(ctx, chromedp.Navigate(params.URL))
	default:
		return fmt.Errorf("unsupported action %q (use click, type, or navigate)", action)
	}
}

// pixelDiff computes a visual diff between two PNG images.
// Returns a diff PNG (changed pixels highlighted in red, unchanged dimmed)
// and the percentage of pixels that changed.
func pixelDiff(beforeData, afterData []byte) ([]byte, float64, error) {
	before, _, err := image.Decode(bytes.NewReader(beforeData))
	if err != nil {
		return nil, 0, fmt.Errorf("decode before: %w", err)
	}
	after, _, err := image.Decode(bytes.NewReader(afterData))
	if err != nil {
		return nil, 0, fmt.Errorf("decode after: %w", err)
	}

	bBounds := before.Bounds()
	aBounds := after.Bounds()

	// Use the larger dimensions.
	w := bBounds.Dx()
	if aBounds.Dx() > w {
		w = aBounds.Dx()
	}
	h := bBounds.Dy()
	if aBounds.Dy() > h {
		h = aBounds.Dy()
	}

	diff := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(diff, diff.Bounds(), image.NewUniform(color.RGBA{0, 0, 0, 255}), image.Point{}, draw.Src)

	changed := 0
	total := w * h
	threshold := uint32(12) // per-channel threshold to ignore anti-aliasing noise

	for y := range h {
		for x := range w {
			var br, bg, bb, ar, ag, ab uint32
			if x < bBounds.Dx() && y < bBounds.Dy() {
				br, bg, bb, _ = before.At(x+bBounds.Min.X, y+bBounds.Min.Y).RGBA()
			}
			if x < aBounds.Dx() && y < aBounds.Dy() {
				ar, ag, ab, _ = after.At(x+aBounds.Min.X, y+aBounds.Min.Y).RGBA()
			}

			dr := absDiff(br>>8, ar>>8)
			dg := absDiff(bg>>8, ag>>8)
			db := absDiff(bb>>8, ab>>8)

			if dr > threshold || dg > threshold || db > threshold {
				changed++
				// Highlight changed pixel in red, intensity proportional to change.
				intensity := uint8(min(255, (dr+dg+db)*2))
				diff.SetRGBA(x, y, color.RGBA{intensity, 0, 0, 255})
			} else {
				// Dim unchanged pixel.
				diff.SetRGBA(x, y, color.RGBA{
					uint8(ar >> 10), // quarter brightness
					uint8(ag >> 10),
					uint8(ab >> 10),
					255,
				})
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, diff); err != nil {
		return nil, 0, fmt.Errorf("encode diff: %w", err)
	}

	pct := 0.0
	if total > 0 {
		pct = float64(changed) / float64(total) * 100
	}
	return buf.Bytes(), math.Round(pct*100) / 100, nil
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// parseJSON is a small helper that decodes JSON from a string.
func parseJSON(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}
