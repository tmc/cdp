package main

import (
	"context"
	"fmt"
	"strings"
)

// expandToolVars replaces $name occurrences in s with values from env.
func expandToolVars(s string, env map[string]string) string {
	for k, v := range env {
		s = strings.ReplaceAll(s, "$"+k, v)
	}
	return s
}

// executeToolLines runs each line of a tool script body against a CommandRegistry
// in the given chromedp context. Lines starting with # are comments. Blank lines
// are skipped. Variables ($name) in env are expanded before execution.
//
// The executor func maps a (ctx, line) pair to execution — in the interactive
// shell this calls im.executeCommand; in MCP mode it may call something else.
func executeToolLines(ctx context.Context, script string, env map[string]string, executor func(context.Context, string) error) (string, error) {
	expanded := expandToolVars(script, env)
	lines := strings.Split(expanded, "\n")

	var output strings.Builder
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if err := executor(ctx, line); err != nil {
			return output.String(), fmt.Errorf("line %d (%s): %w", i+1, line, err)
		}
	}
	return output.String(), nil
}
