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
	"enter":      {"Enter", "Enter", 13},
	"tab":        {"Tab", "Tab", 9},
	"escape":     {"Escape", "Escape", 27},
	"backspace":  {"Backspace", "Backspace", 8},
	"delete":     {"Delete", "Delete", 46},
	"space":      {" ", "Space", 32},
	"arrowup":    {"ArrowUp", "ArrowUp", 38},
	"arrowdown":  {"ArrowDown", "ArrowDown", 40},
	"arrowleft":  {"ArrowLeft", "ArrowLeft", 37},
	"arrowright": {"ArrowRight", "ArrowRight", 39},
	"home":       {"Home", "Home", 36},
	"end":        {"End", "End", 35},
	"pageup":     {"PageUp", "PageUp", 33},
	"pagedown":   {"PageDown", "PageDown", 34},
	"f1":         {"F1", "F1", 112},
	"f2":         {"F2", "F2", 113},
	"f3":         {"F3", "F3", 114},
	"f4":         {"F4", "F4", 115},
	"f5":         {"F5", "F5", 116},
	"f6":         {"F6", "F6", 117},
	"f7":         {"F7", "F7", 118},
	"f8":         {"F8", "F8", 119},
	"f9":         {"F9", "F9", 120},
	"f10":        {"F10", "F10", 121},
	"f11":        {"F11", "F11", 122},
	"f12":        {"F12", "F12", 123},
}

type DragInput struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	Steps   int    `json:"steps,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

type FillFormInput struct {
	Fields []FormField `json:"fields"`
	Submit bool        `json:"submit,omitempty"`
}

type FormField struct {
	Selector string `json:"selector"`
	Value    string `json:"value"`
	Type     string `json:"type,omitempty"`
}

func registerInputTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "press_key",
		Description: "Press a keyboard key. Supports: Enter, Tab, Escape, Backspace, Delete, Space, ArrowUp/Down/Left/Right, Home, End, PageUp/PageDown, F1-F12, or any single character. Modifiers: ctrl, alt, shift, meta (comma-separated). Timeout in seconds (default 30).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PressKeyInput) (*mcp.CallToolResult, any, error) {
		actx, cancel := interactionCtx(ctx, s.activeCtx(), input.Timeout)
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
		Description: "Hover over an element by CSS selector or @ref. Triggers mouseover events. Timeout in seconds (default 30).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, inp HoverInput) (*mcp.CallToolResult, any, error) {
		actx, cancel := interactionCtx(ctx, s.activeCtx(), inp.Timeout)
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
		Description: "Focus an element by CSS selector or @ref. Timeout in seconds (default 30).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, inp FocusInput) (*mcp.CallToolResult, any, error) {
		actx, cancel := interactionCtx(ctx, s.activeCtx(), inp.Timeout)
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "drag",
		Description: "Drag from one element to another. Source and target are CSS selectors or @ref. Steps controls intermediate mouse move events (default 10). Timeout in seconds (default 30).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, inp DragInput) (*mcp.CallToolResult, any, error) {
		steps := inp.Steps
		if steps <= 0 {
			steps = 10
		}
		actx, cancel := interactionCtx(ctx, s.activeCtx(), inp.Timeout)
		defer cancel()

		// Get center coordinates of source and target elements.
		srcX, srcY, err := elementCenter(actx, s.refs, inp.Source)
		if err != nil {
			return nil, nil, fmt.Errorf("drag: source: %w", err)
		}
		tgtX, tgtY, err := elementCenter(actx, s.refs, inp.Target)
		if err != nil {
			return nil, nil, fmt.Errorf("drag: target: %w", err)
		}

		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			// Press at source.
			if err := input.DispatchMouseEvent(input.MousePressed, srcX, srcY).
				WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
				return fmt.Errorf("mouse down: %w", err)
			}
			// Move in steps.
			for i := 1; i <= steps; i++ {
				frac := float64(i) / float64(steps)
				x := srcX + (tgtX-srcX)*frac
				y := srcY + (tgtY-srcY)*frac
				if err := input.DispatchMouseEvent(input.MouseMoved, x, y).
					WithButton(input.Left).Do(ctx); err != nil {
					return fmt.Errorf("mouse move step %d: %w", i, err)
				}
			}
			// Release at target.
			return input.DispatchMouseEvent(input.MouseReleased, tgtX, tgtY).
				WithButton(input.Left).WithClickCount(1).Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("drag: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("dragged %s → %s (%d steps)", inp.Source, inp.Target, steps)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fill_form",
		Description: `Fill multiple form fields at once. Each field specifies a CSS selector, value, and optional type (text, select, checkbox, radio). Set submit=true to submit the form after filling.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, inp FillFormInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()
		for i, f := range inp.Fields {
			fieldType := f.Type
			if fieldType == "" {
				fieldType = "text"
			}
			switch fieldType {
			case "text", "textarea", "password", "email", "number", "tel", "url", "search":
				// Clear then type.
				js := fmt.Sprintf(`(function(){
					var el = document.querySelector(%q);
					if (!el) throw new Error("not found: %s");
					el.focus(); el.value = ''; el.dispatchEvent(new Event('input', {bubbles:true}));
				})()`, f.Selector, f.Selector)
				if err := chromedp.Run(actx, chromedp.Evaluate(js, nil)); err != nil {
					return nil, nil, fmt.Errorf("fill_form: field %d clear: %w", i, err)
				}
				if err := chromedp.Run(actx, chromedp.SendKeys(f.Selector, f.Value, chromedp.ByQuery)); err != nil {
					return nil, nil, fmt.Errorf("fill_form: field %d type: %w", i, err)
				}
			case "select":
				if err := chromedp.Run(actx, chromedp.SetValue(f.Selector, f.Value, chromedp.ByQuery)); err != nil {
					return nil, nil, fmt.Errorf("fill_form: field %d select: %w", i, err)
				}
				// Trigger change event.
				js := fmt.Sprintf(`document.querySelector(%q).dispatchEvent(new Event('change', {bubbles:true}))`, f.Selector)
				_ = chromedp.Run(actx, chromedp.Evaluate(js, nil))
			case "checkbox":
				js := fmt.Sprintf(`(function(){
					var el = document.querySelector(%q);
					if (!el) throw new Error("not found: %s");
					var want = %q === 'true' || %q === '1';
					if (el.checked !== want) el.click();
				})()`, f.Selector, f.Selector, f.Value, f.Value)
				if err := chromedp.Run(actx, chromedp.Evaluate(js, nil)); err != nil {
					return nil, nil, fmt.Errorf("fill_form: field %d checkbox: %w", i, err)
				}
			case "radio":
				js := fmt.Sprintf(`(function(){
					var el = document.querySelector(%q);
					if (!el) throw new Error("not found: %s");
					el.click();
				})()`, f.Selector, f.Selector)
				if err := chromedp.Run(actx, chromedp.Evaluate(js, nil)); err != nil {
					return nil, nil, fmt.Errorf("fill_form: field %d radio: %w", i, err)
				}
			default:
				return nil, nil, fmt.Errorf("fill_form: field %d: unknown type %q", i, fieldType)
			}
		}

		if inp.Submit {
			js := `(function(){
				var form = document.querySelector('form');
				if (form) { form.submit(); return 'submitted'; }
				return 'no form found';
			})()`
			var result string
			if err := chromedp.Run(actx, chromedp.Evaluate(js, &result)); err != nil {
				return nil, nil, fmt.Errorf("fill_form: submit: %w", err)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("filled %d field(s)", len(inp.Fields))}},
		}, nil, nil
	})
}

// elementCenter returns the center coordinates of an element by selector or @ref.
func elementCenter(ctx context.Context, refs *refRegistry, selector string) (float64, float64, error) {
	backendID, err := resolveRef(refs, selector)
	if err != nil {
		return 0, 0, err
	}
	if backendID != 0 {
		if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			return dom.ScrollIntoViewIfNeeded().WithBackendNodeID(backendID).Do(ctx)
		})); err != nil {
			return 0, 0, fmt.Errorf("scroll: %w", err)
		}
		var quads []dom.Quad
		if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			var e error
			quads, e = dom.GetContentQuads().WithBackendNodeID(backendID).Do(ctx)
			return e
		})); err != nil {
			return 0, 0, fmt.Errorf("get quads: %w", err)
		}
		if len(quads) == 0 {
			return 0, 0, fmt.Errorf("element has no visible quads")
		}
		q := quads[0]
		x := (q[0] + q[2] + q[4] + q[6]) / 4
		y := (q[1] + q[3] + q[5] + q[7]) / 4
		return x, y, nil
	}

	// CSS selector path.
	var result map[string]any
	js := fmt.Sprintf(`(function() {
		var el = document.querySelector(%q);
		if (!el) return null;
		el.scrollIntoView({block:'center'});
		var r = el.getBoundingClientRect();
		return {x: r.x + r.width/2, y: r.y + r.height/2};
	})()`, selector)
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return 0, 0, fmt.Errorf("evaluate: %w", err)
	}
	if result == nil {
		return 0, 0, fmt.Errorf("element not found: %s", selector)
	}
	x, _ := result["x"].(float64)
	y, _ := result["y"].(float64)
	return x, y, nil
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
