package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Emulation tools ---

// networkPreset defines a named network throttling configuration.
type networkPreset struct {
	Download float64 // bytes/sec
	Upload   float64 // bytes/sec
	Latency  float64 // ms
	Offline  bool
}

var networkPresets = map[string]networkPreset{
	"slow-3g": {Download: 50 * 1024, Upload: 50 * 1024, Latency: 2000},
	"fast-3g": {Download: 187.5 * 1024, Upload: 56.25 * 1024, Latency: 562.5},
	"slow-4g": {Download: 500 * 1024, Upload: 500 * 1024, Latency: 400},
	"fast-4g": {Download: 4000 * 1024, Upload: 3000 * 1024, Latency: 170},
	"offline": {Offline: true},
	"none":    {Download: -1, Upload: -1, Latency: 0},
}

type SetThrottlingInput struct {
	NetworkPreset  string  `json:"network_preset,omitempty"`
	CPURate        float64 `json:"cpu_rate,omitempty"`
	CustomDownload float64 `json:"custom_download,omitempty"`
	CustomUpload   float64 `json:"custom_upload,omitempty"`
	CustomLatency  float64 `json:"custom_latency,omitempty"`
}

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

// devicePreset defines a mobile/tablet device for emulation.
type devicePreset struct {
	Width       int64
	Height      int64
	ScaleFactor float64
	UserAgent   string
	HasTouch    bool
	IsMobile    bool
}

var devicePresets = map[string]devicePreset{
	"iphone-14":        {Width: 390, Height: 844, ScaleFactor: 3, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1"},
	"iphone-14-pro":    {Width: 393, Height: 852, ScaleFactor: 3, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1"},
	"iphone-15-pro":    {Width: 393, Height: 852, ScaleFactor: 3, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"},
	"iphone-15-pro-max": {Width: 430, Height: 932, ScaleFactor: 3, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"},
	"iphone-se":        {Width: 375, Height: 667, ScaleFactor: 2, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1"},
	"ipad":             {Width: 810, Height: 1080, ScaleFactor: 2, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (iPad; CPU OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1"},
	"ipad-pro-11":      {Width: 834, Height: 1194, ScaleFactor: 2, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"},
	"ipad-pro-12.9":    {Width: 1024, Height: 1366, ScaleFactor: 2, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"},
	"pixel-7":          {Width: 412, Height: 915, ScaleFactor: 2.625, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Mobile Safari/537.36"},
	"pixel-7-pro":      {Width: 412, Height: 892, ScaleFactor: 2.625, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (Linux; Android 13; Pixel 7 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Mobile Safari/537.36"},
	"galaxy-s23":       {Width: 360, Height: 780, ScaleFactor: 3, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (Linux; Android 13; SM-S911B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Mobile Safari/537.36"},
	"galaxy-s23-ultra": {Width: 384, Height: 824, ScaleFactor: 3, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (Linux; Android 13; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Mobile Safari/537.36"},
	"galaxy-tab-s8":    {Width: 800, Height: 1280, ScaleFactor: 2, HasTouch: true, IsMobile: true, UserAgent: "Mozilla/5.0 (Linux; Android 13; SM-X700) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36"},
	"desktop-1080p":    {Width: 1920, Height: 1080, ScaleFactor: 1, HasTouch: false, IsMobile: false, UserAgent: ""},
	"desktop-1440p":    {Width: 2560, Height: 1440, ScaleFactor: 1, HasTouch: false, IsMobile: false, UserAgent: ""},
}

type SetDeviceInput struct {
	Device string `json:"device"` // preset name
}

func registerEmulationTools(server *mcp.Server, s *mcpSession) {
	// Device preset tool.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_device",
		Description: "Emulate a specific device. Sets viewport, scale, user agent, and touch. Presets: iphone-14, iphone-14-pro, iphone-15-pro, iphone-15-pro-max, iphone-se, ipad, ipad-pro-11, ipad-pro-12.9, pixel-7, pixel-7-pro, galaxy-s23, galaxy-s23-ultra, galaxy-tab-s8, desktop-1080p, desktop-1440p.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetDeviceInput) (*mcp.CallToolResult, any, error) {
		preset, ok := devicePresets[input.Device]
		if !ok {
			var names []string
			for k := range devicePresets {
				names = append(names, k)
			}
			sort.Strings(names)
			return nil, nil, fmt.Errorf("set_device: unknown device %q (available: %s)", input.Device, strings.Join(names, ", "))
		}
		actx := s.activeCtx()
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			if err := emulation.SetDeviceMetricsOverride(preset.Width, preset.Height, preset.ScaleFactor, preset.IsMobile).Do(ctx); err != nil {
				return fmt.Errorf("set metrics: %w", err)
			}
			if err := emulation.SetTouchEmulationEnabled(preset.HasTouch).Do(ctx); err != nil {
				return fmt.Errorf("set touch: %w", err)
			}
			if preset.UserAgent != "" {
				if err := emulation.SetUserAgentOverride(preset.UserAgent).Do(ctx); err != nil {
					return fmt.Errorf("set user agent: %w", err)
				}
			}
			return nil
		})); err != nil {
			return nil, nil, fmt.Errorf("set_device: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("device set to %s (%dx%d @%.1fx, mobile=%v, touch=%v)",
				input.Device, preset.Width, preset.Height, preset.ScaleFactor, preset.IsMobile, preset.HasTouch)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_throttling",
		Description: `Set network and CPU throttling. Network presets: slow-3g, fast-3g, slow-4g, fast-4g, offline, none. Custom download/upload in bytes/sec override the preset. CPU rate is a slowdown multiplier (1=normal, 4=4x slower, max 20).`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetThrottlingInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()

		// Resolve network conditions.
		var download, upload, latency float64
		var offline bool
		download, upload = -1, -1 // defaults: no throttling

		if input.NetworkPreset != "" {
			p, ok := networkPresets[input.NetworkPreset]
			if !ok {
				return nil, nil, fmt.Errorf("set_throttling: unknown preset %q", input.NetworkPreset)
			}
			download, upload, latency, offline = p.Download, p.Upload, p.Latency, p.Offline
		}
		if input.CustomDownload > 0 {
			download = input.CustomDownload
		}
		if input.CustomUpload > 0 {
			upload = input.CustomUpload
		}
		if input.CustomLatency > 0 {
			latency = input.CustomLatency
		}

		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return network.OverrideNetworkState(offline, latency, download, upload).Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("set_throttling: network: %w", err)
		}

		// Apply CPU throttling if requested.
		cpuRate := input.CPURate
		if cpuRate > 0 {
			if cpuRate > 20 {
				cpuRate = 20
			}
			if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
				return emulation.SetCPUThrottlingRate(cpuRate).Do(ctx)
			})); err != nil {
				return nil, nil, fmt.Errorf("set_throttling: cpu: %w", err)
			}
		}

		desc := "throttling set"
		if input.NetworkPreset != "" {
			desc += fmt.Sprintf(" (network=%s", input.NetworkPreset)
		} else {
			desc += fmt.Sprintf(" (network: down=%.0f up=%.0f latency=%.0fms", download, upload, latency)
		}
		if cpuRate > 0 {
			desc += fmt.Sprintf(", cpu=%.1fx", cpuRate)
		}
		desc += ")"
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: desc}},
		}, nil, nil
	})

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
