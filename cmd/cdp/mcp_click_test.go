package main

import (
	"context"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/internal/testutil"
)

func TestParseCoordSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		want     viewportPoint
		wantOK   bool
		wantErr  bool
	}{
		{name: "css selector", selector: "button", wantOK: false},
		{name: "ref selector", selector: "@1", wantOK: false},
		{name: "integer coords", selector: "coord:100,200", want: viewportPoint{X: 100, Y: 200}, wantOK: true},
		{name: "decimal coords", selector: "coord:12.5,0.75", want: viewportPoint{X: 12.5, Y: 0.75}, wantOK: true},
		{name: "whitespace", selector: " coord: 12 , 34 ", want: viewportPoint{X: 12, Y: 34}, wantOK: true},
		{name: "missing comma", selector: "coord:12", wantOK: true, wantErr: true},
		{name: "extra comma", selector: "coord:12,34,56", wantOK: true, wantErr: true},
		{name: "non number", selector: "coord:x,34", wantOK: true, wantErr: true},
		{name: "negative x", selector: "coord:-1,34", wantOK: true, wantErr: true},
		{name: "negative y", selector: "coord:1,-34", wantOK: true, wantErr: true},
		{name: "nan", selector: "coord:NaN,34", wantOK: true, wantErr: true},
		{name: "inf", selector: "coord:+Inf,34", wantOK: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := parseCoordSelector(tt.selector)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("point = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestInteractionCtxTimeoutUnits(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want time.Duration
	}{
		{name: "default", in: 0, want: 30 * time.Second},
		{name: "seconds", in: 5, want: 5 * time.Second},
		{name: "milliseconds heuristic", in: 5000, want: 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := interactionCtx(context.Background(), context.Background(), tt.in)
			defer cancel()
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("deadline not set")
			}
			got := time.Until(deadline)
			if got < tt.want-500*time.Millisecond || got > tt.want+500*time.Millisecond {
				t.Fatalf("timeout = %s, want about %s", got, tt.want)
			}
		})
	}
}

func TestValidateRawCDPInput(t *testing.T) {
	tests := []struct {
		name       string
		input      RawCDPInput
		wantMethod string
		wantTarget string
		wantErr    bool
	}{
		{name: "target default", input: RawCDPInput{Method: "Runtime.evaluate"}, wantMethod: "Runtime.evaluate", wantTarget: "target"},
		{name: "browser target", input: RawCDPInput{Method: "Browser.getVersion", Target: "browser"}, wantMethod: "Browser.getVersion", wantTarget: "browser"},
		{name: "trim method", input: RawCDPInput{Method: " Runtime.evaluate "}, wantMethod: "Runtime.evaluate", wantTarget: "target"},
		{name: "missing method", input: RawCDPInput{}, wantErr: true},
		{name: "no domain separator", input: RawCDPInput{Method: "Runtime"}, wantErr: true},
		{name: "too many separators", input: RawCDPInput{Method: "Runtime.evaluate.now"}, wantErr: true},
		{name: "whitespace in method", input: RawCDPInput{Method: "Runtime. evaluate"}, wantErr: true},
		{name: "invalid target", input: RawCDPInput{Method: "Runtime.evaluate", Target: "page"}, wantErr: true},
		{name: "block browser close", input: RawCDPInput{Method: "Browser.close", Target: "browser"}, wantErr: true},
		{name: "block target close", input: RawCDPInput{Method: "Target.closeTarget", Target: "browser"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, target, err := validateRawCDPInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if method != tt.wantMethod || target != tt.wantTarget {
				t.Fatalf("method,target = %q,%q; want %q,%q", method, target, tt.wantMethod, tt.wantTarget)
			}
		})
	}
}

func TestParseRawCDPCommand(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		wantMethod string
		wantParams map[string]any
		wantErr    bool
	}{
		{
			name:       "empty params",
			command:    "Page.reload",
			wantMethod: "Page.reload",
			wantParams: map[string]any{},
		},
		{
			name:       "json params",
			command:    `Runtime.evaluate {"expression":"document.title","returnByValue":true}`,
			wantMethod: "Runtime.evaluate",
			wantParams: map[string]any{"expression": "document.title", "returnByValue": true},
		},
		{
			name:    "invalid method",
			command: "Runtime.evaluate.now {}",
			wantErr: true,
		},
		{
			name:    "invalid json",
			command: `Runtime.evaluate {"expression":}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, params, err := parseRawCDPCommand(tt.command)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseRawCDPCommand(%q) succeeded, want error", tt.command)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRawCDPCommand(%q): %v", tt.command, err)
			}
			if method != tt.wantMethod {
				t.Fatalf("method = %q, want %q", method, tt.wantMethod)
			}
			if !reflect.DeepEqual(params, tt.wantParams) {
				t.Fatalf("params = %#v, want %#v", params, tt.wantParams)
			}
		})
	}
}

func TestIsRawCDPCommandName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "raw method", in: "Runtime.evaluate", want: true},
		{name: "builtin command", in: "click", want: false},
		{name: "empty", in: "", want: false},
		{name: "too many dots", in: "Runtime.evaluate.now", want: false},
		{name: "blocked method", in: "Browser.close", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRawCDPCommandName(tt.in); got != tt.want {
				t.Fatalf("isRawCDPCommandName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestRunRawCDPLiveTargetAndBrowser(t *testing.T) {
	chromePath := testutil.FindChrome()
	if chromePath == "" {
		t.Skip("no Chrome-compatible browser found")
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	)
	allocCtx, cancel := chromedp.NewExecAllocator(t.Context(), opts...)
	t.Cleanup(cancel)

	ctx, cancel := chromedp.NewContext(allocCtx)
	t.Cleanup(cancel)
	ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
	t.Cleanup(cancel)

	page := "data:text/html," + url.PathEscape("<!doctype html><title>raw cdp smoke</title><h1>raw</h1>")
	if err := chromedp.Run(ctx, chromedp.Navigate(page)); err != nil {
		t.Fatalf("navigate: %v", err)
	}

	result, err := runRawCDP(ctx, "Runtime.evaluate", map[string]any{
		"expression":    "document.title",
		"returnByValue": true,
	}, "target")
	if err != nil {
		t.Fatalf("runRawCDP Runtime.evaluate: %v", err)
	}
	remoteObject, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("Runtime.evaluate result = %#v, want result object", result)
	}
	if got := remoteObject["value"]; got != "raw cdp smoke" {
		t.Fatalf("document.title = %#v, want %q", got, "raw cdp smoke")
	}

	result, err = runRawCDP(ctx, "Browser.getVersion", nil, "browser")
	if err != nil {
		t.Fatalf("runRawCDP Browser.getVersion: %v", err)
	}
	if product, ok := result["product"].(string); !ok || product == "" {
		t.Fatalf("Browser.getVersion result = %#v, want product", result)
	}
}
