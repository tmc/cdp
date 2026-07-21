package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
)

func TestRawCDPNeedsContinuation(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "complete raw cdp",
			line: `Runtime.evaluate {"expression":"document.title"}`,
			want: false,
		},
		{
			name: "incomplete raw cdp",
			line: `Runtime.evaluate {"expression":`,
			want: true,
		},
		{
			name: "nested object",
			line: `Page.printToPDF {"marginTop": 1, "transferMode": {"mode":`,
			want: true,
		},
		{
			name: "brace in string",
			line: `Runtime.evaluate {"expression":"JSON.stringify({ok: true})"}`,
			want: false,
		},
		{
			name: "ordinary command",
			line: `click #submit {ignored`,
			want: false,
		},
		{
			name: "unterminated string",
			line: `Runtime.evaluate {"expression":"document.title}`,
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rawCDPNeedsContinuation(tt.line); got != tt.want {
				t.Fatalf("rawCDPNeedsContinuation(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestInteractiveNavigationContextTimeout(t *testing.T) {
	im := &InteractiveMode{
		ctx: context.Background(),
		cfg: fullCaptureConfig{NavigationTimeout: 20},
	}
	ctx, cancel := im.commandContext(&Command{Category: "Navigation"})
	defer cancel()

	select {
	case <-ctx.Done():
		t.Fatal("navigation context expired immediately")
	case <-time.After(10 * time.Millisecond):
	}

	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("navigation context has no deadline")
	}
	parentDone := im.ctx.Done()
	cancel()
	select {
	case <-parentDone:
		t.Fatal("navigation timeout canceled the browser context")
	default:
	}
}

func TestNavigationProgressWrapError(t *testing.T) {
	var output bytes.Buffer
	nav := newNavigationProgress(newStartupProgress(&output, true), "https://example.test/stall", 3)
	nav.start()
	nav.setStage("response received")
	err := nav.wrapError(context.DeadlineExceeded)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wrapped navigation error does not preserve deadline: %v", err)
	}
	for _, want := range []string{"https://example.test/stall", "timeout 3s", "response received"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("navigation error %q does not contain %q", err, want)
		}
	}
	if !strings.Contains(output.String(), "navigating https://example.test/stall") {
		t.Fatalf("navigation progress missing start: %q", output.String())
	}
}

func TestNavigationProgressWaitModes(t *testing.T) {
	for _, mode := range []string{"domcontentloaded", "load", "networkidle"} {
		t.Run(mode, func(t *testing.T) {
			nav := newNavigationProgress(nil, "https://example.test", 1)
			nav.start()
			switch mode {
			case "domcontentloaded":
				nav.domOnce.Do(func() { close(nav.domReady) })
			case "load":
				nav.loadOnce.Do(func() { close(nav.loadReady) })
			case "networkidle":
				nav.idleOnce.Do(func() { close(nav.idleReady) })
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := nav.wait(ctx, mode); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestNavigationProgressNetworkIdleFallback verifies that a page which never
// fires DOMContentLoaded still completes once the network goes idle, so goto
// does not hang on single-page apps that skip the canonical lifecycle events.
func TestNavigationProgressNetworkIdleFallback(t *testing.T) {
	nav := newNavigationProgress(nil, "https://spa.test", 5)
	nav.start()
	// DOM/load never fire; only network idle does.
	nav.idleOnce.Do(func() { close(nav.idleReady) })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := nav.wait(ctx, "domcontentloaded"); err != nil {
		t.Fatalf("wait did not fall back to network idle: %v", err)
	}
}

// TestNavigationProgressSoftSuccessOnResponse verifies that a navigation which
// received a response but never fired a lifecycle event or reached idle
// returns the page on timeout rather than erroring.
func TestNavigationProgressSoftSuccessOnResponse(t *testing.T) {
	nav := newNavigationProgress(nil, "https://slow.test", 1)
	nav.start()
	nav.setStage("response received")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := nav.wait(ctx, "domcontentloaded"); err != nil {
		t.Fatalf("wait should soft-succeed after response received: %v", err)
	}
}

// TestNavigationProgressFailsBeforeResponse verifies that a navigation stuck
// before any response (only "request sent") still fails on timeout: there is
// nothing usable to return.
func TestNavigationProgressFailsBeforeResponse(t *testing.T) {
	nav := newNavigationProgress(nil, "https://stuck.test", 1)
	nav.start()
	nav.setStage("request sent")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := nav.wait(ctx, "domcontentloaded"); err == nil {
		t.Fatal("wait should fail when navigation never received a response")
	}
}

// TestNavigationProgressFollowsRedirect verifies that a top-level navigation
// that redirects cross-origin (e.g. an unauthenticated app bouncing to a login
// page) still advances past "request sent" and credits the response. A
// redirected navigation reuses the same request ID, so tracking by ID rather
// than by the originally requested URL is what makes this work.
func TestNavigationProgressFollowsRedirect(t *testing.T) {
	nav := newNavigationProgress(nil, "https://app.test/page", 5)
	nav.start()

	const reqID = network.RequestID("req-1")

	// Initial document request for the requested URL latches the nav request.
	if !nav.isNavRequest(&network.EventRequestWillBeSent{
		RequestID: reqID,
		Type:      network.ResourceTypeDocument,
		Request:   &network.Request{URL: "https://app.test/page"},
	}) {
		t.Fatal("initial document request not recognized as navigation")
	}

	// Cross-origin redirect: same request ID, RedirectResponse set, new URL
	// that does not match the originally requested URL.
	redirect := &network.EventRequestWillBeSent{
		RequestID:        reqID,
		Type:             network.ResourceTypeDocument,
		Request:          &network.Request{URL: "https://login.test/signin"},
		RedirectResponse: &network.Response{URL: "https://app.test/page"},
	}
	if !nav.isNavRequest(redirect) {
		t.Fatal("redirected request not recognized as navigation")
	}
	if e := redirect; e.RedirectResponse != nil {
		nav.setURL(e.Request.URL)
	}
	if got := nav.currentURL(); got != "https://login.test/signin" {
		t.Fatalf("nav URL not updated to redirect target: %q", got)
	}

	// Response for the redirect target must be credited even though its URL no
	// longer matches the originally requested URL.
	if !nav.isNavResponse(&network.EventResponseReceived{
		RequestID: reqID,
		Response:  &network.Response{URL: "https://login.test/signin"},
	}) {
		t.Fatal("redirect target response not recognized as navigation")
	}

	// An unrelated resource request must not be mistaken for the navigation.
	if nav.isNavResponse(&network.EventResponseReceived{
		RequestID: network.RequestID("req-2"),
		Response:  &network.Response{URL: "https://login.test/style.css"},
	}) {
		t.Fatal("unrelated response wrongly credited to navigation")
	}
}

func TestLongestCommonPrefix(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{
			name:   "empty",
			values: nil,
			want:   "",
		},
		{
			name:   "single",
			values: []string{"screenshot"},
			want:   "screenshot",
		},
		{
			name:   "shared prefix",
			values: []string{"screenshot", "sourcemap", "sources"},
			want:   "s",
		},
		{
			name:   "none",
			values: []string{"click", "navigate"},
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := longestCommonPrefix(tt.values); got != tt.want {
				t.Fatalf("longestCommonPrefix(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

func TestCurrentWord(t *testing.T) {
	line := []rune("click but")
	if got := currentWord(line, len(line)); got != "but" {
		t.Fatalf("currentWord at end = %q, want %q", got, "but")
	}
	if got := currentWord(line, 2); got != "cl" {
		t.Fatalf("currentWord in command = %q, want %q", got, "cl")
	}
}

func TestScannerShellReaderContinuation(t *testing.T) {
	input := strings.NewReader("Runtime.evaluate {\n\"expression\":\"document.title\"\n}\nnext\n")
	var output strings.Builder
	reader := newScannerShellReader(input, &output, true)

	got, err := reader.ReadCommand("cdp> ", rawCDPNeedsContinuation)
	if err != nil {
		t.Fatalf("ReadCommand returned error: %v", err)
	}
	want := "Runtime.evaluate {\n\"expression\":\"document.title\"\n}"
	if got != want {
		t.Fatalf("ReadCommand = %q, want %q", got, want)
	}
	if out := output.String(); out != "cdp> .... .... " {
		t.Fatalf("prompt output = %q, want %q", out, "cdp> .... .... ")
	}

	got, err = reader.ReadCommand("cdp> ", rawCDPNeedsContinuation)
	if err != nil {
		t.Fatalf("second ReadCommand returned error: %v", err)
	}
	if got != "next" {
		t.Fatalf("second ReadCommand = %q, want next", got)
	}
}
