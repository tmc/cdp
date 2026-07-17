package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

// TestMCPStdioToolCallLaunchPath boots the built binary in --mcp mode and
// drives a minimal stdio session: initialize, initialized, then a navigate
// tool call on the launch-browser path — sent immediately, so it races
// browser setup the way a real client does. It guards the runMCP readiness
// signaling that the mcpSession unit tests cannot see: when browserReady was
// only closed by a defer that runs at shutdown, activeCtx blocked every tool
// call forever and this test times out.
func TestMCPStdioToolCallLaunchPath(t *testing.T) {
	skipIfNoBrowser(t)
	cdpPath := buildCDP(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cdpPath, "--mcp", "--headless")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start cdp --mcp: %v", err)
	}
	defer func() {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
	}()

	for _, msg := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"mcp-stdio-test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"navigate","arguments":{"url":"about:blank"}}}`,
	} {
		if _, err := fmt.Fprintln(stdin, msg); err != nil {
			t.Fatalf("write %q: %v", msg, err)
		}
	}

	type response struct {
		ID     *int64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}

	got := make(chan response, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			var resp response
			if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
				continue
			}
			if resp.ID != nil && *resp.ID == 2 {
				got <- resp
				return
			}
		}
	}()

	select {
	case resp := <-got:
		if resp.Error != nil {
			t.Fatalf("navigate returned error: %s", resp.Error)
		}
		if resp.Result == nil {
			t.Fatal("navigate response has neither result nor error")
		}
	case <-ctx.Done():
		t.Fatalf("navigate never answered — browserReady regression? stderr:\n%s", stderr.String())
	}
}
