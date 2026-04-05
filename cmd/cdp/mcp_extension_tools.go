package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/extensions"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// extensionInfo describes an installed extension found via CDP target enumeration.
type extensionInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	URL           string `json:"url,omitempty"`
	Type          string `json:"type"` // "service_worker", "page", etc.
	HasBackground bool   `json:"has_background"`
}

// extensionConsoleCollector captures console messages from an extension service worker.
type extensionConsoleCollector struct {
	mu       sync.Mutex
	messages []consoleEntry
	errors   []errorEntry
	ctx      context.Context
	cancel   context.CancelFunc
}

// extractExtensionID extracts the extension ID from a chrome-extension:// URL.
// Returns empty string if the URL is not a chrome-extension URL.
func extractExtensionID(u string) string {
	const prefix = "chrome-extension://"
	if !strings.HasPrefix(u, prefix) {
		return ""
	}
	rest := u[len(prefix):]
	if idx := strings.IndexByte(rest, '/'); idx > 0 {
		return rest[:idx]
	}
	return rest
}

// --- Input types ---

type ListExtensionsInput struct {
	EnabledOnly bool `json:"enabled_only,omitempty"`
}

type ReloadExtensionInput struct {
	ID string `json:"id"`
}

type InstallExtensionInput struct {
	Path string `json:"path"`
}

type ExtensionConsoleInput struct {
	ID    string `json:"id"`
	Clear bool   `json:"clear,omitempty"`
}

type ExtensionEvaluateInput struct {
	ID         string `json:"id"`
	Expression string `json:"expression"`
}

type UninstallExtensionInput struct {
	ID string `json:"id"`
}

type GetExtensionStorageInput struct {
	ID   string   `json:"id"`
	Area string   `json:"area"` // local, sync, session, managed
	Keys []string `json:"keys,omitempty"`
}

type SetExtensionStorageInput struct {
	ID     string         `json:"id"`
	Area   string         `json:"area"`
	Values map[string]any `json:"values"`
}

type ClearExtensionStorageInput struct {
	ID   string `json:"id"`
	Area string `json:"area"`
}

func registerExtensionTools(server *mcp.Server, s *mcpSession) {
	// Per-extension console collectors, keyed by extension ID.
	extConsoles := struct {
		mu sync.Mutex
		m  map[string]*extensionConsoleCollector
	}{m: make(map[string]*extensionConsoleCollector)}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_extensions",
		Description: "List installed Chrome extensions. Uses CDP Extensions.getExtensions when available, falls back to target enumeration.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListExtensionsInput) (*mcp.CallToolResult, any, error) {
		// Try CDP Extensions.getExtensions first.
		var exts []*extensions.ExtensionInfo
		cdpErr := chromedp.Run(s.browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			exts, err = extensions.GetExtensions().Do(ctx)
			return err
		}))
		if cdpErr == nil {
			data, err := json.Marshal(exts)
			if err != nil {
				return nil, nil, fmt.Errorf("list_extensions: marshal: %w", err)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
			}, nil, nil
		}
		// Fallback: enumerate CDP targets for chrome-extension:// URLs.
		targets, err := chromedp.Targets(s.browserCtx)
		if err != nil {
			return nil, nil, fmt.Errorf("list_extensions: %w", err)
		}
		seen := make(map[string]*extensionInfo)
		for _, t := range targets {
			extID := extractExtensionID(t.URL)
			if extID == "" {
				continue
			}
			info, ok := seen[extID]
			if !ok {
				info = &extensionInfo{
					ID:   extID,
					Name: t.Title,
					Type: string(t.Type),
					URL:  t.URL,
				}
				seen[extID] = info
			}
			if t.Type == "service_worker" || t.Type == "background_page" {
				info.HasBackground = true
			}
		}
		var result []*extensionInfo
		for _, info := range seen {
			result = append(result, info)
		}
		data, err := json.Marshal(result)
		if err != nil {
			return nil, nil, fmt.Errorf("list_extensions: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reload_extension",
		Description: "Reload an unpacked Chrome extension by ID. Opens chrome://extensions in a temporary tab, calls chrome.developerPrivate.reload(id), then closes the tab.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReloadExtensionInput) (*mcp.CallToolResult, any, error) {
		result, err := runOnExtensionsPage(s, fmt.Sprintf(
			`chrome.developerPrivate.reload(%q, {failQuietly: true})`, input.ID,
		))
		if err != nil {
			return nil, nil, fmt.Errorf("reload_extension: %w", err)
		}
		text := fmt.Sprintf("reloaded extension %s", input.ID)
		if result != "" && result != "undefined" && result != "null" {
			text += ": " + result
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "install_extension",
		Description: "Load an unpacked Chrome extension from a local directory path via CDP Extensions.loadUnpacked.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input InstallExtensionInput) (*mcp.CallToolResult, any, error) {
		// Use the CDP Extensions.loadUnpacked command directly — it accepts a
		// filesystem path and returns the extension ID.
		var extID string
		err := chromedp.Run(s.browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			id, err := extensions.LoadUnpacked(input.Path).Do(ctx)
			if err != nil {
				return err
			}
			extID = id
			return nil
		}))
		if err != nil {
			return nil, nil, fmt.Errorf("install_extension: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("extension loaded: %s", extID)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "extension_console",
		Description: "Get console output and errors from an extension's service worker. Attaches to the service worker target on first call.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExtensionConsoleInput) (*mcp.CallToolResult, any, error) {
		extConsoles.mu.Lock()
		cc, ok := extConsoles.m[input.ID]
		extConsoles.mu.Unlock()

		if !ok {
			// Find the service worker target for this extension.
			tid, err := findExtensionSW(s.browserCtx, input.ID)
			if err != nil {
				return nil, nil, fmt.Errorf("extension_console: %w", err)
			}
			swCtx, swCancel := chromedp.NewContext(s.browserCtx, chromedp.WithTargetID(tid))
			if err := chromedp.Run(swCtx); err != nil {
				swCancel()
				return nil, nil, fmt.Errorf("extension_console: attach: %w", err)
			}
			cc = &extensionConsoleCollector{ctx: swCtx, cancel: swCancel}
			chromedp.ListenTarget(swCtx, func(ev any) {
				switch e := ev.(type) {
				case *runtime.EventConsoleAPICalled:
					var textParts []string
					for _, arg := range e.Args {
						textParts = append(textParts, remoteObjectToString(arg))
					}
					ts := ""
					if e.Timestamp != nil {
						ts = time.Time(*e.Timestamp).Format(time.RFC3339Nano)
					}
					cc.mu.Lock()
					cc.messages = append(cc.messages, consoleEntry{
						Type:      string(e.Type),
						Text:      strings.Join(textParts, " "),
						Timestamp: ts,
					})
					cc.mu.Unlock()
				case *runtime.EventExceptionThrown:
					if e.ExceptionDetails == nil {
						return
					}
					d := e.ExceptionDetails
					text := d.Text
					if d.Exception != nil && d.Exception.Description != "" {
						text = d.Exception.Description
					}
					ts := ""
					if e.Timestamp != nil {
						ts = time.Time(*e.Timestamp).Format(time.RFC3339Nano)
					}
					cc.mu.Lock()
					cc.errors = append(cc.errors, errorEntry{
						Text:      text,
						Timestamp: ts,
						URL:       d.URL,
						Line:      d.LineNumber,
						Column:    d.ColumnNumber,
					})
					cc.mu.Unlock()
				}
			})
			extConsoles.mu.Lock()
			extConsoles.m[input.ID] = cc
			extConsoles.mu.Unlock()
		}

		cc.mu.Lock()
		msgs := make([]consoleEntry, len(cc.messages))
		copy(msgs, cc.messages)
		errs := make([]errorEntry, len(cc.errors))
		copy(errs, cc.errors)
		if input.Clear {
			cc.messages = nil
			cc.errors = nil
		}
		cc.mu.Unlock()

		out := struct {
			Messages []consoleEntry `json:"messages"`
			Errors   []errorEntry   `json:"errors"`
		}{Messages: msgs, Errors: errs}
		data, err := json.Marshal(out)
		if err != nil {
			return nil, nil, fmt.Errorf("extension_console: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "extension_evaluate",
		Description: "Evaluate JavaScript in the context of an extension's service worker. Finds the service worker target and runs Runtime.evaluate there.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExtensionEvaluateInput) (*mcp.CallToolResult, any, error) {
		tid, err := findExtensionSW(s.browserCtx, input.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("extension_evaluate: %w", err)
		}
		swCtx, swCancel := chromedp.NewContext(s.browserCtx, chromedp.WithTargetID(tid))
		defer swCancel()

		var result any
		if err := chromedp.Run(swCtx, chromedp.EvaluateAsDevTools(input.Expression, &result)); err != nil {
			return nil, nil, fmt.Errorf("extension_evaluate: %w", err)
		}
		data, err := json.Marshal(result)
		if err != nil {
			return nil, nil, fmt.Errorf("extension_evaluate: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "uninstall_extension",
		Description: "Uninstall an unpacked extension by ID via CDP Extensions.uninstall.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UninstallExtensionInput) (*mcp.CallToolResult, any, error) {
		err := chromedp.Run(s.browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			return extensions.Uninstall(input.ID).Do(ctx)
		}))
		if err != nil {
			return nil, nil, fmt.Errorf("uninstall_extension: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("uninstalled extension %s", input.ID)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_extension_storage",
		Description: "Get data from extension storage. Area: local, sync, session, or managed. Optionally filter by keys.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetExtensionStorageInput) (*mcp.CallToolResult, any, error) {
		area, err := parseStorageArea(input.Area)
		if err != nil {
			return nil, nil, fmt.Errorf("get_extension_storage: %w", err)
		}
		var data []byte
		err = chromedp.Run(s.browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			params := extensions.GetStorageItems(input.ID, area)
			if len(input.Keys) > 0 {
				params = params.WithKeys(input.Keys)
			}
			val, err := params.Do(ctx)
			if err != nil {
				return err
			}
			data = val
			return nil
		}))
		if err != nil {
			return nil, nil, fmt.Errorf("get_extension_storage: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_extension_storage",
		Description: "Set values in extension storage. Area: local, sync, session, or managed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetExtensionStorageInput) (*mcp.CallToolResult, any, error) {
		area, err := parseStorageArea(input.Area)
		if err != nil {
			return nil, nil, fmt.Errorf("set_extension_storage: %w", err)
		}
		valJSON, err := json.Marshal(input.Values)
		if err != nil {
			return nil, nil, fmt.Errorf("set_extension_storage: marshal: %w", err)
		}
		err = chromedp.Run(s.browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			return extensions.SetStorageItems(input.ID, area, valJSON).Do(ctx)
		}))
		if err != nil {
			return nil, nil, fmt.Errorf("set_extension_storage: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "storage updated"}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "clear_extension_storage",
		Description: "Clear all data in extension storage area. Area: local, sync, session, or managed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ClearExtensionStorageInput) (*mcp.CallToolResult, any, error) {
		area, err := parseStorageArea(input.Area)
		if err != nil {
			return nil, nil, fmt.Errorf("clear_extension_storage: %w", err)
		}
		err = chromedp.Run(s.browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			return extensions.ClearStorageItems(input.ID, area).Do(ctx)
		}))
		if err != nil {
			return nil, nil, fmt.Errorf("clear_extension_storage: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "storage cleared"}},
		}, nil, nil
	})
}

func parseStorageArea(s string) (extensions.StorageArea, error) {
	switch s {
	case "local":
		return extensions.StorageAreaLocal, nil
	case "sync":
		return extensions.StorageAreaSync, nil
	case "session":
		return extensions.StorageAreaSession, nil
	case "managed":
		return extensions.StorageAreaManaged, nil
	default:
		return "", fmt.Errorf("invalid storage area %q (use local, sync, session, or managed)", s)
	}
}

// findExtensionSW finds the service worker target ID for a given extension ID.
func findExtensionSW(browserCtx context.Context, extID string) (target.ID, error) {
	targets, err := chromedp.Targets(browserCtx)
	if err != nil {
		return "", fmt.Errorf("enumerate targets: %w", err)
	}
	prefix := "chrome-extension://" + extID + "/"
	for _, t := range targets {
		if t.Type == "service_worker" && strings.HasPrefix(t.URL, prefix) {
			return t.TargetID, nil
		}
	}
	// Fall back to any target with this extension ID (background page, etc.)
	for _, t := range targets {
		if strings.HasPrefix(t.URL, prefix) {
			return t.TargetID, nil
		}
	}
	return "", fmt.Errorf("no target found for extension %s", extID)
}

// runOnExtensionsPage opens chrome://extensions in a temporary tab,
// evaluates the given JS expression, and closes the tab.
func runOnExtensionsPage(s *mcpSession, expr string) (string, error) {
	tabCtx, tabCancel := chromedp.NewContext(s.browserCtx)
	defer tabCancel()

	if err := chromedp.Run(tabCtx, chromedp.Navigate("chrome://extensions")); err != nil {
		return "", fmt.Errorf("navigate to chrome://extensions: %w", err)
	}
	// Wait briefly for the page to initialize the developerPrivate API.
	time.Sleep(500 * time.Millisecond)

	var result string
	if err := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Use runtime.Evaluate with awaitPromise since developerPrivate methods return promises.
		res, exc, err := runtime.Evaluate(fmt.Sprintf(
			`(async () => { const r = await %s; return JSON.stringify(r); })()`, expr,
		)).WithAwaitPromise(true).Do(ctx)
		if err != nil {
			return err
		}
		if exc != nil {
			return fmt.Errorf("JS exception: %s", exc.Text)
		}
		if res != nil && res.Value != nil {
			json.Unmarshal(res.Value, &result)
		}
		return nil
	})); err != nil {
		return "", err
	}

	// Close the temporary tab.
	tid := chromedp.FromContext(tabCtx).Target.TargetID
	_ = chromedp.Run(s.browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return target.CloseTarget(tid).Do(ctx)
	}))

	return result, nil
}
