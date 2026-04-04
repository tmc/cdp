package main

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Emulation tools ---

type SetViewportInput struct {
	Width  int64   `json:"width"`
	Height int64   `json:"height"`
	Scale  float64 `json:"device_scale_factor,omitempty"`
	Mobile bool    `json:"mobile,omitempty"`
}

type SetUserAgentInput struct {
	UserAgent      string `json:"user_agent"`
	AcceptLanguage string `json:"accept_language,omitempty"`
	Platform       string `json:"platform,omitempty"`
}

type SetOfflineInput struct {
	Offline bool `json:"offline"`
}

type SetGeolocationInput struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  float64 `json:"accuracy,omitempty"`
}

type SetExtraHeadersInput struct {
	Headers map[string]string `json:"headers"`
}

func registerEmulationTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_viewport",
		Description: "Set the browser viewport size and device metrics. Width and height in pixels.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetViewportInput) (*mcp.CallToolResult, any, error) {
		scale := input.Scale
		if scale == 0 {
			scale = 1
		}
		actx := s.activeCtx()
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetDeviceMetricsOverride(input.Width, input.Height, scale, input.Mobile).Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("set_viewport: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("viewport set to %dx%d (scale=%.1f, mobile=%v)", input.Width, input.Height, scale, input.Mobile)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_user_agent",
		Description: "Override the browser user agent string",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetUserAgentInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()
		cmd := emulation.SetUserAgentOverride(input.UserAgent)
		if input.AcceptLanguage != "" {
			cmd = cmd.WithAcceptLanguage(input.AcceptLanguage)
		}
		if input.Platform != "" {
			cmd = cmd.WithPlatform(input.Platform)
		}
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return cmd.Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("set_user_agent: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "user agent set"}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_offline",
		Description: "Enable or disable network offline emulation",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetOfflineInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return network.OverrideNetworkState(input.Offline, 0, -1, -1).Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("set_offline: %w", err)
		}
		state := "online"
		if input.Offline {
			state = "offline"
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "network set to " + state}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_geolocation",
		Description: "Override the browser geolocation. Latitude and longitude in decimal degrees.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetGeolocationInput) (*mcp.CallToolResult, any, error) {
		accuracy := input.Accuracy
		if accuracy == 0 {
			accuracy = 1
		}
		actx := s.activeCtx()
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetGeolocationOverride().
				WithLatitude(input.Latitude).
				WithLongitude(input.Longitude).
				WithAccuracy(accuracy).Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("set_geolocation: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("geolocation set to %.6f, %.6f", input.Latitude, input.Longitude)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_extra_headers",
		Description: "Set extra HTTP headers that will be sent with every request",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetExtraHeadersInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()
		headers := make(network.Headers)
		for k, v := range input.Headers {
			headers[k] = v
		}
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetExtraHTTPHeaders(headers).Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("set_extra_headers: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("set %d extra header(s)", len(input.Headers))}},
		}, nil, nil
	})
}
