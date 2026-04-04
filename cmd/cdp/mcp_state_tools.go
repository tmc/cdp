package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- State save/load tools ---

// browserState represents a serializable browser state snapshot.
type browserState struct {
	Cookies        []*network.CookieParam `json:"cookies,omitempty"`
	LocalStorage   map[string]string      `json:"local_storage,omitempty"`
	SessionStorage map[string]string      `json:"session_storage,omitempty"`
	URL            string                 `json:"url,omitempty"`
}

type SaveStateInput struct {
	Path string `json:"path,omitempty"`
	Name string `json:"name,omitempty"`
}

type LoadStateInput struct {
	Path string `json:"path,omitempty"`
	Name string `json:"name,omitempty"`
}

func registerStateTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "save_state",
		Description: "Save browser state (cookies, localStorage, sessionStorage, URL) to a JSON file. Provide a path or a name (saved to output dir).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SaveStateInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()
		state := browserState{}

		// Capture cookies.
		cookies, err := network.GetCookies().Do(actx)
		if err == nil {
			for _, c := range cookies {
				state.Cookies = append(state.Cookies, cookieToCookieParam(c))
			}
		}

		// Capture localStorage.
		var ls map[string]string
		if err := chromedp.Run(actx, chromedp.Evaluate(`(function() {
			var r = {};
			for (var i = 0; i < localStorage.length; i++) {
				var k = localStorage.key(i);
				r[k] = localStorage.getItem(k);
			}
			return r;
		})()`, &ls)); err == nil {
			state.LocalStorage = ls
		}

		// Capture sessionStorage.
		var ss map[string]string
		if err := chromedp.Run(actx, chromedp.Evaluate(`(function() {
			var r = {};
			for (var i = 0; i < sessionStorage.length; i++) {
				var k = sessionStorage.key(i);
				r[k] = sessionStorage.getItem(k);
			}
			return r;
		})()`, &ss)); err == nil {
			state.SessionStorage = ss
		}

		// Capture current URL.
		var loc string
		if err := chromedp.Run(actx, chromedp.Location(&loc)); err == nil {
			state.URL = loc
		}

		data, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return nil, nil, fmt.Errorf("save_state: marshal: %w", err)
		}

		path := resolvePath(input.Path, input.Name, "state", ".json", s.outputDir)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, nil, fmt.Errorf("save_state: create dir: %w", err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, nil, fmt.Errorf("save_state: write: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("state saved to %s (%d cookies, %d localStorage, %d sessionStorage)", path, len(state.Cookies), len(state.LocalStorage), len(state.SessionStorage))}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "load_state",
		Description: "Load browser state (cookies, localStorage, sessionStorage) from a JSON file previously saved with save_state. Optionally navigate to the saved URL.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input LoadStateInput) (*mcp.CallToolResult, any, error) {
		path := resolvePath(input.Path, input.Name, "state", ".json", s.outputDir)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("load_state: read: %w", err)
		}

		var state browserState
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, nil, fmt.Errorf("load_state: parse: %w", err)
		}

		actx := s.activeCtx()

		// Navigate to saved URL first (so storage operations target the right origin).
		if state.URL != "" {
			if err := chromedp.Run(actx, chromedp.Navigate(state.URL)); err != nil {
				return nil, nil, fmt.Errorf("load_state: navigate: %w", err)
			}
		}

		// Restore cookies.
		if len(state.Cookies) > 0 {
			if err := network.SetCookies(state.Cookies).Do(actx); err != nil {
				return nil, nil, fmt.Errorf("load_state: set cookies: %w", err)
			}
		}

		// Restore localStorage.
		for k, v := range state.LocalStorage {
			js := fmt.Sprintf(`localStorage.setItem(%q, %q)`, k, v)
			if err := chromedp.Run(actx, chromedp.Evaluate(js, nil)); err != nil {
				return nil, nil, fmt.Errorf("load_state: set localStorage: %w", err)
			}
		}

		// Restore sessionStorage.
		for k, v := range state.SessionStorage {
			js := fmt.Sprintf(`sessionStorage.setItem(%q, %q)`, k, v)
			if err := chromedp.Run(actx, chromedp.Evaluate(js, nil)); err != nil {
				return nil, nil, fmt.Errorf("load_state: set sessionStorage: %w", err)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("state loaded from %s (%d cookies, %d localStorage, %d sessionStorage)", path, len(state.Cookies), len(state.LocalStorage), len(state.SessionStorage))}},
		}, nil, nil
	})
}

func cookieToCookieParam(c *network.Cookie) *network.CookieParam {
	p := &network.CookieParam{
		Name:     c.Name,
		Value:    c.Value,
		Domain:   c.Domain,
		Path:     c.Path,
		Secure:   c.Secure,
		HTTPOnly: c.HTTPOnly,
	}
	if c.SameSite != "" {
		p.SameSite = c.SameSite
	}
	if c.Expires > 0 {
		ts := cdp.TimeSinceEpoch(time.Unix(int64(c.Expires), 0))
		p.Expires = &ts
	}
	return p
}

func resolvePath(path, name, prefix, suffix, outputDir string) string {
	if path != "" {
		if !filepath.IsAbs(path) && outputDir != "" {
			return filepath.Join(outputDir, path)
		}
		return path
	}
	n := prefix
	if name != "" {
		n = name
	}
	if outputDir != "" {
		return filepath.Join(outputDir, n+suffix)
	}
	return n + suffix
}
