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

// ensureAccessibility enables the Accessibility domain if not already active.
// Safe to call multiple times — the CDP side is idempotent.
func ensureAccessibility(ctx context.Context) {
	_ = accessibility.Enable().Do(ctx)
}

// buildAXSnapshot fetches the full AX tree, assigns @refs to interactive
// elements, and returns an indented text representation.
// If the CDP Accessibility domain is unavailable (e.g. some Electron targets),
// it falls back to a JS-based extraction.
func buildAXSnapshot(ctx context.Context, refs *refRegistry) (string, error) {
	refs.reset()

	ensureAccessibility(ctx)
	nodes, err := accessibility.GetFullAXTree().Do(ctx)
	if err != nil {
		// Fallback: JS-based snapshot for environments where the AX domain fails.
		result, jsErr := buildJSSnapshot(ctx, refs)
		if jsErr != nil {
			return "", fmt.Errorf("get ax tree: %w (js fallback also failed: %v)", err, jsErr)
		}
		return result, nil
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
// When the stored BackendNodeID is stale (DOM node gone), it attempts
// recovery by re-querying the AX tree for a node matching the stored
// role and name.
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

// resolveRefWithRecovery is like resolveRef but takes a context so it can
// attempt stale-ref recovery when DOM.resolveNode would fail.
func resolveRefWithRecovery(ctx context.Context, refs *refRegistry, selector string) (cdp.BackendNodeID, error) {
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

	// Check if the stored node is still valid by attempting to resolve it.
	_, err := dom.ResolveNode().WithBackendNodeID(entry.BackendNodeID).Do(ctx)
	if err == nil {
		return entry.BackendNodeID, nil
	}

	// Node is stale — try to recover by matching role+name in the AX tree.
	if entry.Role == "" {
		return 0, fmt.Errorf("ref %s is stale and has no role for recovery (run page_snapshot to refresh)", selector)
	}
	recovered, recovErr := recoverRef(ctx, refs, ref, entry)
	if recovErr != nil {
		return 0, fmt.Errorf("ref %s is stale: %w (run page_snapshot to refresh)", selector, recovErr)
	}
	return recovered, nil
}

// recoverRef searches the AX tree for a node matching the given role and name,
// updates the ref entry, and returns the new BackendNodeID.
func recoverRef(ctx context.Context, refs *refRegistry, ref int, entry refEntry) (cdp.BackendNodeID, error) {
	ensureAccessibility(ctx)
	nodes, err := accessibility.GetFullAXTree().Do(ctx)
	if err != nil {
		return 0, fmt.Errorf("recovery ax tree: %w", err)
	}

	// Look for exact role+name match.
	for _, n := range nodes {
		if n.Ignored || n.BackendDOMNodeID == 0 {
			continue
		}
		role := axValueString(n.Role)
		name := axValueString(n.Name)
		if role == entry.Role && name == entry.Name {
			// Update the ref entry with the new BackendNodeID.
			refs.mu.Lock()
			refs.entries[ref] = refEntry{
				BackendNodeID: n.BackendDOMNodeID,
				Role:          role,
				Name:          name,
			}
			refs.mu.Unlock()
			return n.BackendDOMNodeID, nil
		}
	}

	return 0, fmt.Errorf("no matching node with role=%q name=%q", entry.Role, entry.Name)
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

// buildJSSnapshot extracts an accessibility-like snapshot using JavaScript
// when the CDP Accessibility domain is unavailable.
func buildJSSnapshot(ctx context.Context, refs *refRegistry) (string, error) {
	const script = `(function() {
		const interactive = new Set([
			'A', 'BUTTON', 'INPUT', 'SELECT', 'TEXTAREA', 'DETAILS', 'SUMMARY',
			'[role=button]', '[role=link]', '[role=textbox]', '[role=combobox]',
			'[role=checkbox]', '[role=radio]', '[role=tab]', '[role=menuitem]',
			'[role=switch]', '[role=slider]', '[role=option]', '[role=searchbox]'
		]);
		const roleMap = {
			'A': 'link', 'BUTTON': 'button', 'INPUT': 'textbox',
			'SELECT': 'combobox', 'TEXTAREA': 'textbox',
			'DETAILS': 'group', 'SUMMARY': 'button'
		};
		function walk(el, depth) {
			if (!el || el.nodeType !== 1) return '';
			const tag = el.tagName;
			const role = el.getAttribute('role') || roleMap[tag] || tag.toLowerCase();
			const name = el.getAttribute('aria-label') || el.getAttribute('title')
				|| el.getAttribute('alt') || el.getAttribute('placeholder') || '';
			const text = (el.childNodes.length === 1 && el.childNodes[0].nodeType === 3)
				? el.childNodes[0].textContent.trim().slice(0, 100) : '';
			const indent = '  '.repeat(depth);
			let line = indent + role;
			const label = name || text;
			if (label) line += ' "' + label.replace(/"/g, '\\"') + '"';
			const val = el.value;
			if (val !== undefined && val !== '') line += ' value="' + String(val).slice(0, 50) + '"';
			if (el.disabled) line += ' [disabled]';
			if (el.checked) line += ' [checked]';
			// Mark interactive elements
			let isInteractive = false;
			if (interactive.has(tag) || el.getAttribute('role')) {
				const r = el.getBoundingClientRect();
				if (r.width > 0 && r.height > 0) isInteractive = true;
			}
			if (isInteractive) line += ' {{INTERACTIVE:' + role + ':' + (label||'') + '}}';
			let result = line + '\n';
			for (const child of el.children) {
				result += walk(child, depth + 1);
			}
			return result;
		}
		return walk(document.body, 0);
	})()`

	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &result)); err != nil {
		return "", fmt.Errorf("js snapshot: %w", err)
	}

	// Post-process: replace {{INTERACTIVE:role:name}} markers with @ref numbers.
	var b strings.Builder
	for _, line := range strings.Split(result, "\n") {
		if idx := strings.Index(line, " {{INTERACTIVE:"); idx >= 0 {
			marker := line[idx+len(" {{INTERACTIVE:"):]
			line = line[:idx]
			// Parse role:name from marker.
			end := strings.Index(marker, "}}")
			if end > 0 {
				parts := strings.SplitN(marker[:end], ":", 2)
				role := parts[0]
				name := ""
				if len(parts) > 1 {
					name = parts[1]
				}
				// We can't get BackendNodeID from JS, so store with ID=0.
				// The ref is still useful for display but can't be used for interaction.
				ref := refs.add(refEntry{
					BackendNodeID: 0,
					Role:          role,
					Name:          name,
				})
				line += fmt.Sprintf(" @%d", ref)
			}
		}
		if line != "" {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	return "[JS fallback — @refs cannot be used for interaction, use CSS selectors instead]\n" + b.String(), nil
}
