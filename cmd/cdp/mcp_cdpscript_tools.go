package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/cdp/cdpscript"
	"github.com/tmc/cdp/internal/scriptbrowser"
)

type cdpscriptInput struct {
	Path   string   `json:"path,omitempty"`
	Script string   `json:"script,omitempty"`
	Args   []string `json:"args,omitempty"`
	Format string   `json:"format,omitempty"` // auto, txtar, or cdp
}

type cdpscriptRunOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
}

func registerCDPScriptTools(server *mcp.Server, s *mcpSession) {
	addMCPTool(server, &mcp.Tool{
		Name:        "run_cdpscript",
		Description: "Run a cdpscript script through the real cdpscript engine. Provide path or script; script may be txtar or plain .cdp.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input cdpscriptInput) (*mcp.CallToolResult, cdpscriptRunOutput, error) {
		actx := s.activeCtx()
		if actx == nil {
			return nil, cdpscriptRunOutput{}, fmt.Errorf("run_cdpscript: browser not ready")
		}
		text, name, err := cdpscriptInputText(input)
		if err != nil {
			return nil, cdpscriptRunOutput{}, err
		}
		runCtx, cancel := requestToolCtx(ctx, actx, 5*time.Minute)
		defer cancel()

		var stdout, stderr bytes.Buffer
		opts := []cdpscript.Option{
			cdpscript.WithEnv(os.Environ()...),
			cdpscript.WithStdout(&stdout),
			cdpscript.WithStderr(&stderr),
		}
		s.mu.Lock()
		outputDir := s.contextOutputDir()
		s.mu.Unlock()
		if dir := outputDir; dir != "" {
			opts = append(opts, cdpscript.WithOutputDir(dir))
		}
		engine := cdpscript.New(opts...)
		err = executeCDPScriptInput(scriptbrowser.Borrow(runCtx), engine, input, name, text)
		out := cdpscriptRunOutput{
			Stdout:   strings.TrimSpace(stdout.String()),
			Stderr:   stderr.String(),
			ExitCode: cdpscript.ExitCode(err),
		}
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, out, nil
		}
		return nil, out, nil
	})

	addMCPTool(server, &mcp.Tool{
		Name:        "validate_script",
		Description: "Validate a cdpscript script without starting a browser or executing actions. Provide path or script; script may be txtar or plain .cdp.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input cdpscriptInput) (*mcp.CallToolResult, map[string]any, error) {
		text, name, err := cdpscriptInputText(input)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, map[string]any{"ok": false, "error": err.Error()}, nil
		}
		err = validateCDPScriptInput(input, name, text)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, map[string]any{"ok": false, "error": err.Error()}, nil
		}
		return nil, map[string]any{"ok": true}, nil
	})
}

func executeCDPScriptInput(ctx context.Context, engine *cdpscript.Engine, input cdpscriptInput, name, text string) error {
	switch scriptFormat(input.Format, name, text) {
	case "txtar":
		return engine.ExecuteReader(ctx, name, strings.NewReader(text), input.Args)
	default:
		return engine.ExecuteScript(ctx, name, text, input.Args)
	}
}

func validateCDPScriptInput(input cdpscriptInput, name, text string) error {
	switch scriptFormat(input.Format, name, text) {
	case "txtar":
		return cdpscript.ValidateReader(name, strings.NewReader(text))
	default:
		return cdpscript.ValidateScript(name, text)
	}
}

func cdpscriptInputText(input cdpscriptInput) (text, name string, err error) {
	switch {
	case input.Path != "" && input.Script != "":
		return "", "", fmt.Errorf("provide path or script, not both")
	case input.Path != "":
		data, err := os.ReadFile(input.Path)
		if err != nil {
			return "", "", fmt.Errorf("read script: %w", err)
		}
		return string(data), filepath.Base(input.Path), nil
	case input.Script != "":
		return input.Script, "inline.cdp", nil
	default:
		return "", "", fmt.Errorf("path or script is required")
	}
}

func scriptFormat(format, name, text string) string {
	switch format {
	case "txtar", "cdp":
		return format
	}
	if hasTxtarMarker(text) || strings.HasSuffix(name, ".txtar") {
		return "txtar"
	}
	return "cdp"
}

// hasTxtarMarker reports whether text has a txtar file marker line,
// "-- name --", on any line including the first.
func hasTxtarMarker(text string) bool {
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, "\r\n")
		if len(line) > len("--  --") && strings.HasPrefix(line, "-- ") && strings.HasSuffix(line, " --") {
			return true
		}
	}
	return false
}
