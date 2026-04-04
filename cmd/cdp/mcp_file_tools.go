package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- File upload tool ---

type UploadFileInput struct {
	Selector string   `json:"selector"`
	Files    []string `json:"files"`
}

func registerFileTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "upload_file",
		Description: "Upload file(s) to a file input element by CSS selector or @ref. Provide absolute file paths.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UploadFileInput) (*mcp.CallToolResult, any, error) {
		if len(input.Files) == 0 {
			return nil, nil, fmt.Errorf("upload_file: at least one file path required")
		}

		actx := s.activeCtx()
		backendID, err := resolveRef(s.refs, input.Selector)
		if err != nil {
			return nil, nil, fmt.Errorf("upload_file: %w", err)
		}

		if backendID != 0 {
			if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
				return dom.SetFileInputFiles(input.Files).WithBackendNodeID(backendID).Do(ctx)
			})); err != nil {
				return nil, nil, fmt.Errorf("upload_file: %w", err)
			}
		} else {
			if err := chromedp.Run(actx, chromedp.SetUploadFiles(input.Selector, input.Files, chromedp.ByQuery)); err != nil {
				return nil, nil, fmt.Errorf("upload_file: %w", err)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("uploaded %d file(s) to %s: %s", len(input.Files), input.Selector, strings.Join(input.Files, ", "))}},
		}, nil, nil
	})
}
