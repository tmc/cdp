package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/cdp/internal/chromedp"
	"github.com/tmc/cdp/internal/testutil"
	"github.com/tmc/cdp/internal/tooldef"
)

// scriptToolsClient connects an in-memory client to a server with every
// tool runMCP registers for cfg.
func scriptToolsClient(t *testing.T, s *mcpSession, cfg mcpConfig) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	if err := registerServerTools(t.Context(), server, s, cfg); err != nil {
		t.Fatal(err)
	}
	a, b := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// callText calls a tool and returns its text content, structured content
// and error flag.
func callText(t *testing.T, ctx context.Context, cs *mcp.ClientSession, name string, args map[string]any) (string, any, bool) {
	t.Helper()
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	var b strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), r.StructuredContent, r.IsError
}

func TestValidateScriptTool(t *testing.T) {
	cs := scriptToolsClient(t, &mcpSession{refs: newRefRegistry()}, mcpConfig{})
	tests := []struct {
		name    string
		script  string
		wantErr string
	}{
		{name: "plain", script: "log ok\n"},
		{name: "txtar marker on first line", script: "-- main.cdp --\nlog ok\n"},
		{name: "unknown command", script: "no-such-command\n", wantErr: "unknown command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, _, isErr := callText(t, t.Context(), cs, "validate_script", map[string]any{"script": tt.script})
			if tt.wantErr == "" {
				if isErr {
					t.Fatalf("validate_script(%q) failed: %s", tt.script, text)
				}
				return
			}
			if !isErr || !strings.Contains(text, tt.wantErr) {
				t.Fatalf("validate_script(%q) = %q, isError=%v; want error containing %q", tt.script, text, isErr, tt.wantErr)
			}
		})
	}
}

func TestRunCDPScriptTool(t *testing.T) {
	s := &mcpSession{refs: newRefRegistry(), ctx: context.Background()}
	cs := scriptToolsClient(t, s, mcpConfig{})
	tests := []struct {
		name     string
		script   string
		wantOut  string
		wantExit float64
	}{
		{name: "plain", script: "log hello\n", wantOut: "hello"},
		{name: "txtar marker on first line", script: "-- main.cdp --\nlog from txtar\n", wantOut: "from txtar"},
		{name: "unknown command", script: "no-such-command\n", wantExit: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, out, isErr := callText(t, t.Context(), cs, "run_cdpscript", map[string]any{"script": tt.script})
			m, _ := out.(map[string]any)
			if got := m["exit_code"]; got != tt.wantExit {
				t.Fatalf("exit_code = %v, want %v (output %v)", got, tt.wantExit, out)
			}
			if isErr != (tt.wantExit != 0) {
				t.Fatalf("isError = %v, want %v", isErr, tt.wantExit != 0)
			}
			if got := m["stdout"]; tt.wantOut != "" && got != tt.wantOut {
				t.Fatalf("stdout = %q, want %q", got, tt.wantOut)
			}
		})
	}
}

func TestDefineTool(t *testing.T) {
	dir := t.TempDir()
	s := &mcpSession{refs: newRefRegistry(), ctx: context.Background()}
	cs := scriptToolsClient(t, s, mcpConfig{ToolsDir: dir})
	tests := []struct {
		name    string
		tool    string
		wantErr string
	}{
		{name: "defines tool", tool: "greet"},
		{name: "path traversal", tool: "../x", wantErr: "single path segment"},
		{name: "built-in name", tool: "browser_act", wantErr: "built-in"},
		{name: "define_tool itself", tool: "define_tool", wantErr: "built-in"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, _, isErr := callText(t, t.Context(), cs, "define_tool", map[string]any{
				"name":        tt.tool,
				"description": "greet someone",
				"script":      "log hello $who",
				"inputs":      []string{`who string "who to greet"`},
			})
			if tt.wantErr != "" {
				if !isErr || !strings.Contains(text, tt.wantErr) {
					t.Fatalf("define_tool(%q) = %q, isError=%v; want error containing %q", tt.tool, text, isErr, tt.wantErr)
				}
				if _, err := os.Stat(filepath.Join(dir, tt.tool+".cdp")); err == nil {
					t.Fatalf("define_tool(%q) wrote a tool file", tt.tool)
				}
				return
			}
			if isErr {
				t.Fatalf("define_tool(%q): %s", tt.tool, text)
			}
			if _, err := os.Stat(filepath.Join(dir, tt.tool+".cdp")); err != nil {
				t.Fatal(err)
			}
			text, _, isErr = callText(t, t.Context(), cs, tt.tool, map[string]any{"who": "world"})
			if isErr || text != "hello world" {
				t.Fatalf("%s = %q, isError=%v; want %q", tt.tool, text, isErr, "hello world")
			}
		})
	}
}

func TestCustomToolsDirSkipsBuiltinNames(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"close_tab", "browser_observe", "greet"} {
		def := &tooldef.ToolDef{Name: name, Description: "custom " + name, Script: "log custom"}
		if err := os.WriteFile(filepath.Join(dir, name+".cdp"), tooldef.Generate(def), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	cs := scriptToolsClient(t, &mcpSession{refs: newRefRegistry()}, mcpConfig{ToolsDir: dir})
	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	desc := make(map[string]string)
	for _, tool := range res.Tools {
		desc[tool.Name] = tool.Description
	}
	for _, name := range []string{"close_tab", "browser_observe"} {
		if d := desc[name]; strings.HasPrefix(d, "custom ") {
			t.Errorf("custom tool replaced built-in %s", name)
		}
		if _, ok := desc["custom_"+name]; ok {
			t.Errorf("custom tool registered as custom_%s, want it skipped", name)
		}
	}
	if desc["greet"] != "custom greet" {
		t.Errorf("greet description = %q, want custom greet", desc["greet"])
	}
}

func TestCustomToolWaitsForBrowser(t *testing.T) {
	dir := t.TempDir()
	def := &tooldef.ToolDef{Name: "greet", Script: "log hello"}
	if err := os.WriteFile(filepath.Join(dir, "greet.cdp"), tooldef.Generate(def), 0o666); err != nil {
		t.Fatal(err)
	}

	t.Run("request canceled before ready", func(t *testing.T) {
		s := &mcpSession{refs: newRefRegistry(), browserReady: make(chan struct{})}
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := executeToolScript(ctx, s, "log hello", nil)
			done <- err
		}()
		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("executeToolScript = %v, want deadline exceeded", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("executeToolScript did not return after request cancellation")
		}
	})

	t.Run("waits for ready", func(t *testing.T) {
		s := &mcpSession{refs: newRefRegistry(), browserReady: make(chan struct{})}
		cs := scriptToolsClient(t, s, mcpConfig{ToolsDir: dir})
		go func() {
			time.Sleep(100 * time.Millisecond)
			s.mu.Lock()
			s.ctx = context.Background()
			s.mu.Unlock()
			s.signalBrowserReady()
		}()
		text, _, isErr := callText(t, t.Context(), cs, "greet", nil)
		if isErr || text != "hello" {
			t.Fatalf("greet = %q, isError=%v; want hello", text, isErr)
		}
	})

	t.Run("setup failed", func(t *testing.T) {
		s := &mcpSession{refs: newRefRegistry(), browserReady: make(chan struct{})}
		s.setupErr = os.ErrNotExist
		s.signalBrowserReady()
		cs := scriptToolsClient(t, s, mcpConfig{ToolsDir: dir})
		text, _, isErr := callText(t, t.Context(), cs, "greet", nil)
		if !isErr || !strings.Contains(text, "browser setup failed") {
			t.Fatalf("greet = %q, isError=%v; want browser setup failed", text, isErr)
		}
	})
}

func TestScriptToolsInBrowser(t *testing.T) {
	skipIfNoBrowser(t)
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(testutil.FindChrome()))
	alloc, stop := chromedp.NewExecAllocator(t.Context(), opts...)
	t.Cleanup(stop)
	ctx, stop := chromedp.NewContext(alloc)
	t.Cleanup(stop)
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html,<title>fixture</title>")); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	s := &mcpSession{refs: newRefRegistry(), ctx: ctx, browserCtx: ctx}
	cs := scriptToolsClient(t, s, mcpConfig{ToolsDir: dir})

	_, out, isErr := callText(t, t.Context(), cs, "run_cdpscript", map[string]any{"script": "title\n"})
	if m, _ := out.(map[string]any); isErr || m["stdout"] != "fixture" {
		t.Fatalf("run_cdpscript title = %v, isError=%v; want fixture", out, isErr)
	}

	text, _, isErr := callText(t, t.Context(), cs, "define_tool", map[string]any{"name": "page_title", "description": "page title", "script": "title"})
	if isErr {
		t.Fatalf("define_tool: %s", text)
	}
	text, _, isErr = callText(t, t.Context(), cs, "page_title", nil)
	if isErr || text != "fixture" {
		t.Fatalf("page_title = %q, isError=%v; want fixture", text, isErr)
	}
}
