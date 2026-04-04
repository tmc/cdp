package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// refEntry maps a @ref number to a DOM backend node.
type refEntry struct {
	BackendNodeID cdp.BackendNodeID
	Role          string
	Name          string
}

// refRegistry holds the current @ref→element mapping.
// It is rebuilt on each page_snapshot call.
type refRegistry struct {
	mu      sync.RWMutex
	entries map[int]refEntry
	counter int
}

func newRefRegistry() *refRegistry {
	return &refRegistry{
		entries: make(map[int]refEntry),
	}
}

// reset clears all refs and resets the counter.
func (r *refRegistry) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = make(map[int]refEntry)
	r.counter = 0
}

// add assigns the next @ref number and returns it.
func (r *refRegistry) add(entry refEntry) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counter++
	r.entries[r.counter] = entry
	return r.counter
}

// get looks up a ref by number.
func (r *refRegistry) get(ref int) (refEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[ref]
	return e, ok
}

// interactiveRoles is the set of AX roles that get @ref annotations.
var interactiveRoles = map[string]bool{
	"link":          true,
	"button":        true,
	"textbox":       true,
	"combobox":      true,
	"checkbox":      true,
	"radio":         true,
	"switch":        true,
	"slider":        true,
	"spinbutton":    true,
	"searchbox":     true,
	"menuitem":      true,
	"menuitemradio": true,
	"tab":           true,
	"option":        true,
	"treeitem":      true,
	"listbox":       true,
	"menubar":       true,
	"menu":          true,
}

// axValueString extracts the string value from an AX Value.
func axValueString(v *accessibility.Value) string {
	if v == nil || len(v.Value) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(v.Value, &s); err != nil {
		return string(v.Value)
	}
	return s
}

// axPropertyBool returns the boolean value of a named property, or false.
func axPropertyBool(props []*accessibility.Property, name accessibility.PropertyName) bool {
	for _, p := range props {
		if p.Name == name && p.Value != nil {
			var b bool
			if err := json.Unmarshal(p.Value.Value, &b); err == nil {
				return b
			}
		}
	}
	return false
}

// axPropertyString returns the string value of a named property.
func axPropertyString(props []*accessibility.Property, name accessibility.PropertyName) string {
	for _, p := range props {
		if p.Name == name {
			return axValueString(p.Value)
		}
	}
	return ""
}

// buildAXSnapshot fetches the full AX tree, assigns @refs to interactive
// elements, and returns an indented text representation.
func buildAXSnapshot(ctx context.Context, refs *refRegistry) (string, error) {
	refs.reset()

	nodes, err := accessibility.GetFullAXTree().Do(ctx)
	if err != nil {
		return "", fmt.Errorf("get ax tree: %w", err)
	}

	// Build a lookup from NodeID → *Node and a children map.
	byID := make(map[accessibility.NodeID]*accessibility.Node, len(nodes))
	children := make(map[accessibility.NodeID][]accessibility.NodeID, len(nodes))
	var rootID accessibility.NodeID
	for _, n := range nodes {
		byID[n.NodeID] = n
		if n.ParentID == "" {
			rootID = n.NodeID
		}
	}
	for _, n := range nodes {
		if n.ParentID != "" {
			children[n.ParentID] = append(children[n.ParentID], n.NodeID)
		}
	}

	var b strings.Builder
	var walk func(id accessibility.NodeID, depth int)
	walk = func(id accessibility.NodeID, depth int) {
		n, ok := byID[id]
		if !ok {
			return
		}

		// Skip ignored nodes but still walk children.
		if n.Ignored {
			for _, cid := range children[id] {
				walk(cid, depth)
			}
			return
		}

		role := axValueString(n.Role)
		name := axValueString(n.Name)
		value := axValueString(n.Value)
		desc := axValueString(n.Description)

		// Skip generic/none roles with no name.
		if (role == "generic" || role == "none" || role == "") && name == "" {
			for _, cid := range children[id] {
				walk(cid, depth)
			}
			return
		}

		// Static text nodes — just emit the text.
		if role == "StaticText" || role == "InlineTextBox" {
			if name != "" {
				indent := strings.Repeat("  ", depth)
				fmt.Fprintf(&b, "%s%s\n", indent, name)
			}
			return
		}

		indent := strings.Repeat("  ", depth)

		// Assign @ref for interactive elements.
		var refStr string
		if interactiveRoles[role] && n.BackendDOMNodeID != 0 {
			ref := refs.add(refEntry{
				BackendNodeID: n.BackendDOMNodeID,
				Role:          role,
				Name:          name,
			})
			refStr = fmt.Sprintf(" @%d", ref)
		}

		// Build the line.
		line := indent + role
		if name != "" {
			line += fmt.Sprintf(" %q", name)
		}
		if value != "" {
			line += fmt.Sprintf(" value=%q", value)
		}
		if desc != "" {
			line += fmt.Sprintf(" description=%q", desc)
		}

		// Add useful state properties.
		if axPropertyBool(n.Properties, accessibility.PropertyNameFocused) {
			line += " [focused]"
		}
		if axPropertyBool(n.Properties, accessibility.PropertyNameDisabled) {
			line += " [disabled]"
		}
		if axPropertyBool(n.Properties, accessibility.PropertyNameRequired) {
			line += " [required]"
		}
		checked := axPropertyString(n.Properties, accessibility.PropertyNameChecked)
		if checked == "true" {
			line += " [checked]"
		}
		expanded := axPropertyString(n.Properties, accessibility.PropertyNameExpanded)
		if expanded != "" {
			line += fmt.Sprintf(" [expanded=%s]", expanded)
		}
		if u := axPropertyString(n.Properties, accessibility.PropertyNameURL); u != "" {
			line += fmt.Sprintf(" url=%q", u)
		}

		line += refStr
		fmt.Fprintln(&b, line)

		for _, cid := range children[id] {
			walk(cid, depth+1)
		}
	}

	if rootID != "" {
		walk(rootID, 0)
	}

	return b.String(), nil
}

// resolveRef resolves a @ref number to a BackendNodeID.
// If the selector starts with "@", it's treated as a ref.
// Otherwise returns 0 (meaning use CSS selector).
func resolveRef(refs *refRegistry, selector string) (cdp.BackendNodeID, error) {
	if !strings.HasPrefix(selector, "@") {
		return 0, nil
	}
	var ref int
	if _, err := fmt.Sscanf(selector, "@%d", &ref); err != nil {
		return 0, fmt.Errorf("invalid ref %q: %w", selector, err)
	}
	entry, ok := refs.get(ref)
	if !ok {
		return 0, fmt.Errorf("ref %s not found (run page_snapshot to refresh refs)", selector)
	}
	return entry.BackendNodeID, nil
}

// clickByBackendNodeID scrolls to the element, gets its center, and clicks it.
func clickByBackendNodeID(ctx context.Context, backendID cdp.BackendNodeID) error {
	// Scroll into view first.
	if err := dom.ScrollIntoViewIfNeeded().WithBackendNodeID(backendID).Do(ctx); err != nil {
		return fmt.Errorf("scroll into view: %w", err)
	}

	// Get content quads to find clickable center.
	quads, err := dom.GetContentQuads().WithBackendNodeID(backendID).Do(ctx)
	if err != nil {
		return fmt.Errorf("get content quads: %w", err)
	}
	if len(quads) == 0 {
		return fmt.Errorf("element has no visible quads")
	}
	// Use the center of the first quad.
	q := quads[0]
	// Quad is [x1,y1, x2,y2, x3,y3, x4,y4].
	x := (q[0] + q[2] + q[4] + q[6]) / 4
	y := (q[1] + q[3] + q[5] + q[7]) / 4

	// Dispatch mousePressed + mouseReleased.
	if err := input.DispatchMouseEvent(input.MousePressed, x, y).
		WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
		return fmt.Errorf("mouse pressed: %w", err)
	}
	if err := input.DispatchMouseEvent(input.MouseReleased, x, y).
		WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
		return fmt.Errorf("mouse released: %w", err)
	}
	return nil
}

// typeByBackendNodeID focuses the element and types text via key events.
func typeByBackendNodeID(ctx context.Context, backendID cdp.BackendNodeID, text string) error {
	if err := dom.Focus().WithBackendNodeID(backendID).Do(ctx); err != nil {
		return fmt.Errorf("focus: %w", err)
	}
	// Use chromedp.KeyEvent for reliable text input.
	if err := chromedp.Run(ctx, chromedp.KeyEvent(text)); err != nil {
		return fmt.Errorf("key event: %w", err)
	}
	return nil
}

// resolveNodeToRemoteObject resolves a BackendNodeID to a runtime.RemoteObjectID.
func resolveNodeToRemoteObject(ctx context.Context, backendID cdp.BackendNodeID) (runtime.RemoteObjectID, error) {
	obj, err := dom.ResolveNode().WithBackendNodeID(backendID).Do(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve node: %w", err)
	}
	return obj.ObjectID, nil
}
