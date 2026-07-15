package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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
