package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Keyboard/input tools ---

type PressKeyInput struct {
	Key       string `json:"key"`
	Modifiers string `json:"modifiers,omitempty"`
	Timeout   int    `json:"timeout,omitempty"`
}

type HoverInput struct {
	Selector string `json:"selector"`
	Timeout  int    `json:"timeout,omitempty"`
}

type FocusInput struct {
	Selector string `json:"selector"`
	Timeout  int    `json:"timeout,omitempty"`
}

// keyDefinitions maps common key names to their CDP key properties.
var keyDefinitions = map[string]struct {
	Key     string
	Code    string
	KeyCode int64
}{
	"enter":     {"Enter", "Enter", 13},
	"tab":       {"Tab", "Tab", 9},
	"escape":    {"Escape", "Escape", 27},
	"backspace": {"Backspace", "Backspace", 8},
	"delete":    {"Delete", "Delete", 46},
	"space":     {" ", "Space", 32},
	"arrowup":   {"ArrowUp", "ArrowUp", 38},
	"arrowdown": {"ArrowDown", "ArrowDown", 40},
	"arrowleft": {"ArrowLeft", "ArrowLeft", 37},
	"arrowright":{"ArrowRight", "ArrowRight", 39},
	"home":      {"Home", "Home", 36},
	"end":       {"End", "End", 35},
	"pageup":    {"PageUp", "PageUp", 33},
	"pagedown":  {"PageDown", "PageDown", 34},
	"f1":        {"F1", "F1", 112},
	"f2":        {"F2", "F2", 113},
	"f3":        {"F3", "F3", 114},
	"f4":        {"F4", "F4", 115},
	"f5":        {"F5", "F5", 116},
	"f6":        {"F6", "F6", 117},
	"f7":        {"F7", "F7", 118},
	"f8":        {"F8", "F8", 119},
	"f9":        {"F9", "F9", 120},
	"f10":       {"F10", "F10", 121},
	"f11":       {"F11", "F11", 122},
	"f12":       {"F12", "F12", 123},
}

func registerInputTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "press_key",
		Description: "Press a keyboard key. Supports: Enter, Tab, Escape, Backspace, Delete, Space, ArrowUp/Down/Left/Right, Home, End, PageUp/PageDown, F1-F12, or any single character. Modifiers: ctrl, alt, shift, meta (comma-separated).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PressKeyInput) (*mcp.CallToolResult, any, error) {
		actx, cancel := interactionCtx(s.activeCtx(), input.Timeout)
		defer cancel()
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return pressKey(ctx, input.Key, input.Modifiers)
		})); err != nil {
			return nil, nil, fmt.Errorf("press_key: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "pressed " + input.Key}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "hover",
		Description: "Hover over an element by CSS selector or @ref. Triggers mouseover events.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, inp HoverInput) (*mcp.CallToolResult, any, error) {
		actx, cancel := interactionCtx(s.activeCtx(), inp.Timeout)
		defer cancel()
		backendID, err := resolveRef(s.refs, inp.Selector)
		if err != nil {
			return nil, nil, fmt.Errorf("hover: %w", err)
		}

		if backendID != 0 {
			if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
				return hoverByBackendNodeID(ctx, backendID)
			})); err != nil {
				return nil, nil, fmt.Errorf("hover: %w", err)
			}
		} else {
			if err := hoverBySelector(actx, inp.Selector); err != nil {
				return nil, nil, fmt.Errorf("hover: %w", err)
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "hovered " + inp.Selector}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "focus",
		Description: "Focus an element by CSS selector or @ref",
	}, func(ctx context.Context, req *mcp.CallToolRequest, inp FocusInput) (*mcp.CallToolResult, any, error) {
		actx, cancel := interactionCtx(s.activeCtx(), inp.Timeout)
		defer cancel()
		backendID, err := resolveRef(s.refs, inp.Selector)
		if err != nil {
			return nil, nil, fmt.Errorf("focus: %w", err)
		}

		if backendID != 0 {
			if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
				return dom.Focus().WithBackendNodeID(backendID).Do(ctx)
			})); err != nil {
				return nil, nil, fmt.Errorf("focus: %w", err)
			}
		} else {
			if err := chromedp.Run(actx, chromedp.Focus(inp.Selector, chromedp.ByQuery)); err != nil {
				return nil, nil, fmt.Errorf("focus: %w", err)
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "focused " + inp.Selector}},
		}, nil, nil
	})
}

func pressKey(ctx context.Context, key, modifiers string) error {
	mod := parseModifiers(modifiers)

	// Look up named keys.
	kd, ok := keyDefinitions[strings.ToLower(key)]
	if ok {
		down := input.DispatchKeyEvent(input.KeyDown).
			WithKey(kd.Key).
			WithCode(kd.Code).
			WithWindowsVirtualKeyCode(kd.KeyCode).
			WithNativeVirtualKeyCode(kd.KeyCode).
			WithModifiers(mod)
		if err := down.Do(ctx); err != nil {
			return fmt.Errorf("key down: %w", err)
		}
		up := input.DispatchKeyEvent(input.KeyUp).
			WithKey(kd.Key).
			WithCode(kd.Code).
			WithWindowsVirtualKeyCode(kd.KeyCode).
			WithNativeVirtualKeyCode(kd.KeyCode).
			WithModifiers(mod)
		return up.Do(ctx)
	}

	// Single character — send keyDown + char + keyUp.
	if len(key) == 1 {
		char := key
		code := "Key" + strings.ToUpper(char)
		keyCode := int64(strings.ToUpper(char)[0])

		if err := input.DispatchKeyEvent(input.KeyDown).
			WithKey(char).WithCode(code).
			WithWindowsVirtualKeyCode(keyCode).
			WithModifiers(mod).Do(ctx); err != nil {
			return err
		}
		if err := input.DispatchKeyEvent(input.KeyChar).
			WithKey(char).WithText(char).
			WithModifiers(mod).Do(ctx); err != nil {
			return err
		}
		return input.DispatchKeyEvent(input.KeyUp).
			WithKey(char).WithCode(code).
			WithWindowsVirtualKeyCode(keyCode).
			WithModifiers(mod).Do(ctx)
	}

	return fmt.Errorf("unknown key: %s", key)
}

func parseModifiers(s string) input.Modifier {
	var mod input.Modifier
	for _, part := range strings.Split(s, ",") {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "alt":
			mod |= input.ModifierAlt
		case "ctrl", "control":
			mod |= input.ModifierCtrl
		case "meta", "cmd", "command":
			mod |= input.ModifierMeta
		case "shift":
			mod |= input.ModifierShift
		}
	}
	return mod
}

func hoverByBackendNodeID(ctx context.Context, backendID cdp.BackendNodeID) error {
	if err := dom.ScrollIntoViewIfNeeded().WithBackendNodeID(backendID).Do(ctx); err != nil {
		return fmt.Errorf("scroll: %w", err)
	}
	quads, err := dom.GetContentQuads().WithBackendNodeID(backendID).Do(ctx)
	if err != nil {
		return fmt.Errorf("get quads: %w", err)
	}
	if len(quads) == 0 {
		return fmt.Errorf("element has no visible quads")
	}
	q := quads[0]
	x := (q[0] + q[2] + q[4] + q[6]) / 4
	y := (q[1] + q[3] + q[5] + q[7]) / 4
	return input.DispatchMouseEvent(input.MouseMoved, x, y).Do(ctx)
}

func hoverBySelector(ctx context.Context, selector string) error {
	// Use JS to get element center, then dispatch mouseMoved.
	var result map[string]any
	js := fmt.Sprintf(`(function() {
		var el = document.querySelector(%q);
		if (!el) return null;
		var r = el.getBoundingClientRect();
		return {x: r.x + r.width/2, y: r.y + r.height/2};
	})()`, selector)
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("evaluate: %w", err)
	}
	if result == nil {
		return fmt.Errorf("element not found: %s", selector)
	}
	x, _ := result["x"].(float64)
	y, _ := result["y"].(float64)
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseMoved, x, y).Do(ctx)
	}))
}
