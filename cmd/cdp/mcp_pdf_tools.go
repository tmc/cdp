package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- PDF generation tool ---

type SavePDFInput struct {
	Path            string  `json:"path,omitempty"`
	Landscape       bool    `json:"landscape,omitempty"`
	PrintBackground bool    `json:"print_background,omitempty"`
	Scale           float64 `json:"scale,omitempty"`
	PaperWidth      float64 `json:"paper_width,omitempty"`
	PaperHeight     float64 `json:"paper_height,omitempty"`
}

func registerPDFTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "save_pdf",
		Description: "Generate a PDF of the current page. Optionally save to a file path. Returns the PDF as base64 if no path given.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SavePDFInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()

		var data []byte
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			cmd := page.PrintToPDF().WithPrintBackground(true)
			if input.Landscape {
				cmd = cmd.WithLandscape(true)
			}
			if input.PrintBackground {
				cmd = cmd.WithPrintBackground(true)
			}
			if input.Scale > 0 {
				cmd = cmd.WithScale(input.Scale)
			}
			if input.PaperWidth > 0 {
				cmd = cmd.WithPaperWidth(input.PaperWidth)
			}
			if input.PaperHeight > 0 {
				cmd = cmd.WithPaperHeight(input.PaperHeight)
			}
			var err error
			data, _, err = cmd.Do(ctx)
			return err
		})); err != nil {
			return nil, nil, fmt.Errorf("save_pdf: %w", err)
		}

		if input.Path != "" {
			// Resolve relative to output dir if set.
			path := input.Path
			if !filepath.IsAbs(path) && s.outputDir != "" {
				path = filepath.Join(s.outputDir, path)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return nil, nil, fmt.Errorf("save_pdf: create dir: %w", err)
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				return nil, nil, fmt.Errorf("save_pdf: write: %w", err)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("PDF saved to %s (%d bytes)", path, len(data))}},
			}, nil, nil
		}

		// Return as base64.
		encoded := base64.StdEncoding.EncodeToString(data)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("PDF generated (%d bytes, base64 encoded):\n%s", len(data), encoded)}},
		}, nil, nil
	})
}
