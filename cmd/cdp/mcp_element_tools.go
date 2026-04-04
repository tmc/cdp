package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- find_element tool ---

type FindElementInput struct {
	Role string `json:"role,omitempty"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
	Nth  int    `json:"nth,omitempty"`
}

type foundElement struct {
	Ref  int    `json:"ref"`
	Role string `json:"role"`
	Name string `json:"name"`
}

func registerFindElementTool(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "find_element",
		Description: "Find interactive elements by role, accessible name, or text content. Returns @ref numbers. Run page_snapshot first to populate refs.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input FindElementInput) (*mcp.CallToolResult, any, error) {
		s.refs.mu.RLock()
		entries := make(map[int]refEntry, len(s.refs.entries))
		for k, v := range s.refs.entries {
			entries[k] = v
		}
		s.refs.mu.RUnlock()

		if len(entries) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no refs available — run page_snapshot first"}},
			}, nil, nil
		}

		var matches []foundElement
		for ref, entry := range entries {
			if !matchesFind(entry, input) {
				continue
			}
			matches = append(matches, foundElement{
				Ref:  ref,
				Role: entry.Role,
				Name: entry.Name,
			})
		}

		if input.Nth > 0 && input.Nth <= len(matches) {
			matches = []foundElement{matches[input.Nth-1]}
		}

		if len(matches) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no matching elements found"}},
			}, nil, nil
		}

		data, err := json.Marshal(matches)
		if err != nil {
			return nil, nil, fmt.Errorf("find_element: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}

func matchesFind(entry refEntry, input FindElementInput) bool {
	if input.Role != "" && !strings.EqualFold(entry.Role, input.Role) {
		return false
	}
	if input.Name != "" && !strings.Contains(strings.ToLower(entry.Name), strings.ToLower(input.Name)) {
		return false
	}
	if input.Text != "" && !strings.Contains(strings.ToLower(entry.Name), strings.ToLower(input.Text)) {
		return false
	}
	return true
}

// --- get_element / check_element tools ---

type GetElementInput struct {
	Selector string `json:"selector"`
}

type elementInfo struct {
	TagName     string            `json:"tag_name"`
	Text        string            `json:"text,omitempty"`
	Value       string            `json:"value,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	BoundingBox *boxModel         `json:"bounding_box,omitempty"`
}

type boxModel struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type CheckElementInput struct {
	Selector string `json:"selector"`
}

type elementState struct {
	Exists   bool `json:"exists"`
	Visible  bool `json:"visible,omitempty"`
	Enabled  bool `json:"enabled,omitempty"`
	Checked  bool `json:"checked,omitempty"`
	Selected bool `json:"selected,omitempty"`
	Focused  bool `json:"focused,omitempty"`
}

func registerElementQueryTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_element",
		Description: "Get element details (text, value, attributes, bounding box) by CSS selector or @ref",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetElementInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()
		backendID, err := resolveRef(s.refs, input.Selector)
		if err != nil {
			return nil, nil, fmt.Errorf("get_element: %w", err)
		}

		var info elementInfo
		if backendID != 0 {
			err = chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
				return getElementByBackendID(ctx, backendID, &info)
			}))
		} else {
			err = getElementBySelector(actx, input.Selector, &info)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("get_element: %w", err)
		}

		data, err := json.Marshal(info)
		if err != nil {
			return nil, nil, fmt.Errorf("get_element: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_element",
		Description: "Check element state (visible, enabled, checked, selected, focused) by CSS selector or @ref",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CheckElementInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()
		backendID, err := resolveRef(s.refs, input.Selector)
		if err != nil {
			return nil, nil, fmt.Errorf("check_element: %w", err)
		}

		var state elementState
		if backendID != 0 {
			err = chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
				return checkElementByBackendID(ctx, backendID, &state)
			}))
		} else {
			err = checkElementBySelector(actx, input.Selector, &state)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("check_element: %w", err)
		}

		data, err := json.Marshal(state)
		if err != nil {
			return nil, nil, fmt.Errorf("check_element: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}

func getElementByBackendID(ctx context.Context, backendID cdp.BackendNodeID, info *elementInfo) error {
	node, err := dom.DescribeNode().WithBackendNodeID(backendID).Do(ctx)
	if err != nil {
		return fmt.Errorf("describe node: %w", err)
	}
	info.TagName = node.NodeName

	attrs := make(map[string]string)
	for i := 0; i+1 < len(node.Attributes); i += 2 {
		attrs[node.Attributes[i]] = node.Attributes[i+1]
	}
	info.Attributes = attrs

	obj, err := dom.ResolveNode().WithBackendNodeID(backendID).Do(ctx)
	if err == nil && obj.ObjectID != "" {
		var result map[string]any
		js := `function() { return {text: this.textContent || '', value: this.value || ''}; }`
		if err := callFunctionOn(ctx, obj.ObjectID, js, &result); err == nil {
			if t, ok := result["text"].(string); ok {
				info.Text = strings.TrimSpace(t)
			}
			if v, ok := result["value"].(string); ok {
				info.Value = v
			}
		}
	}

	model, err := dom.GetBoxModel().WithBackendNodeID(backendID).Do(ctx)
	if err == nil && model != nil {
		info.BoundingBox = &boxModel{
			X:      float64(model.Content[0]),
			Y:      float64(model.Content[1]),
			Width:  float64(model.Width),
			Height: float64(model.Height),
		}
	}

	return nil
}

func getElementBySelector(ctx context.Context, selector string, info *elementInfo) error {
	js := fmt.Sprintf(`(function() {
		var el = document.querySelector(%q);
		if (!el) return null;
		var a = {};
		for (var i = 0; i < el.attributes.length; i++) {
			a[el.attributes[i].name] = el.attributes[i].value;
		}
		return {
			tagName: el.tagName,
			text: (el.textContent || '').trim(),
			value: el.value || '',
			attrs: a
		};
	})()`, selector)

	var result any
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("evaluate: %w", err)
	}
	if result == nil {
		return fmt.Errorf("element not found: %s", selector)
	}

	m, ok := result.(map[string]any)
	if !ok {
		return fmt.Errorf("unexpected result type")
	}

	info.TagName, _ = m["tagName"].(string)
	info.Text, _ = m["text"].(string)
	info.Value, _ = m["value"].(string)
	if a, ok := m["attrs"].(map[string]any); ok {
		info.Attributes = make(map[string]string, len(a))
		for k, v := range a {
			info.Attributes[k], _ = v.(string)
		}
	}
	return nil
}

func checkElementByBackendID(ctx context.Context, backendID cdp.BackendNodeID, state *elementState) error {
	axNodes, err := accessibility.GetPartialAXTree().WithBackendNodeID(backendID).WithFetchRelatives(false).Do(ctx)
	if err != nil {
		state.Exists = false
		return nil
	}
	state.Exists = true

	if len(axNodes) > 0 {
		n := axNodes[0]
		state.Focused = axPropertyBool(n.Properties, accessibility.PropertyNameFocused)
		state.Enabled = !axPropertyBool(n.Properties, accessibility.PropertyNameDisabled)
		state.Visible = !n.Ignored
		state.Checked = axPropertyString(n.Properties, accessibility.PropertyNameChecked) == "true"
		state.Selected = axPropertyString(n.Properties, accessibility.PropertyNameSelected) == "true"
	}
	return nil
}

func checkElementBySelector(ctx context.Context, selector string, state *elementState) error {
	js := fmt.Sprintf(`(function() {
		var el = document.querySelector(%q);
		if (!el) return null;
		var rect = el.getBoundingClientRect();
		return {
			visible: rect.width > 0 && rect.height > 0 && getComputedStyle(el).visibility !== 'hidden',
			enabled: !el.disabled,
			checked: !!el.checked,
			selected: !!el.selected,
			focused: document.activeElement === el
		};
	})()`, selector)

	var result any
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("evaluate: %w", err)
	}
	if result == nil {
		state.Exists = false
		return nil
	}
	state.Exists = true
	m, ok := result.(map[string]any)
	if !ok {
		return nil
	}
	state.Visible, _ = m["visible"].(bool)
	state.Enabled, _ = m["enabled"].(bool)
	state.Checked, _ = m["checked"].(bool)
	state.Selected, _ = m["selected"].(bool)
	state.Focused, _ = m["focused"].(bool)
	return nil
}

// callFunctionOn calls a JS function on a remote object and unmarshals the result.
func callFunctionOn(ctx context.Context, objectID runtime.RemoteObjectID, fn string, result any) error {
	obj, _, err := runtime.CallFunctionOn(fn).WithObjectID(objectID).WithReturnByValue(true).Do(ctx)
	if err != nil {
		return err
	}
	if len(obj.Value) == 0 {
		return nil
	}
	return json.Unmarshal(obj.Value, result)
}
