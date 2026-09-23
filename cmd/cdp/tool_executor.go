package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tmc/cdp/cdpscript"
	"github.com/tmc/cdp/internal/scriptbrowser"
)

func runCDPScriptBody(ctx context.Context, scriptBody string, env map[string]string, outputDir string) (stdout, stderr string, err error) {
	if ctx == nil {
		return "", "", fmt.Errorf("browser not ready")
	}
	var out bytes.Buffer
	var errout bytes.Buffer
	opts := []cdpscript.Option{
		cdpscript.WithEnv(os.Environ()...),
		cdpscript.WithEnv(toolEnv(env)...),
		cdpscript.WithStdout(&out),
		cdpscript.WithStderr(&errout),
	}
	if outputDir != "" {
		opts = append(opts, cdpscript.WithOutputDir(outputDir))
	}
	engine := cdpscript.New(opts...)
	err = engine.ExecuteScript(scriptbrowser.Borrow(ctx), "tool.cdp", scriptBody, nil)
	return strings.TrimSpace(out.String()), errout.String(), err
}

func toolEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}
