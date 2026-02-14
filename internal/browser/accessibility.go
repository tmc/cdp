package browser

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/chromedp/chromedp"
)

// AccessibilityNode represents a node in the accessibility tree.
type AccessibilityNode struct {
	Role        string               `json:"role"`
	Name        string               `json:"name,omitempty"`
	Description string               `json:"description,omitempty"`
	Level       int                  `json:"level,omitempty"` // For headings
	Disabled    bool                 `json:"disabled,omitempty"`
	Checked     string               `json:"checked,omitempty"` // "true", "false", "mixed"
	Expanded    bool                 `json:"expanded,omitempty"`
	Selected    bool                 `json:"selected,omitempty"`
	Value       string               `json:"value,omitempty"`
	Children    []*AccessibilityNode `json:"children,omitempty"`
}

// RefEntry represents a mapping from a ref ID to element locator info.
type RefEntry struct {
	Selector string `json:"selector"` // e.g., "getByRole('button', { name: \"Submit\" })"
	Role     string `json:"role"`
	Name     string `json:"name,omitempty"`
	Nth      int    `json:"nth,omitempty"` // Disambiguation index when multiple elements have same role+name
}

// RefMap maps ref IDs (e.g., "e1") to RefEntry.
type RefMap struct {
	Refs map[string]*RefEntry `json:"refs"`
}

// EnhancedSnapshot contains the accessibility tree as text and the ref map.
type EnhancedSnapshot struct {
	Tree string  `json:"tree"`
	Refs *RefMap `json:"refs"`
}

// SnapshotOptions configures how the accessibility snapshot is generated.
type SnapshotOptions struct {
	Interactive bool   // Only include interactive elements (buttons, links, inputs, etc.)
	MaxDepth    int    // Maximum depth of tree to include (0 = unlimited)
	Compact     bool   // Remove structural elements without meaningful content
	Selector    string // CSS selector to scope the snapshot
}

// Roles that are interactive and should get refs.
var interactiveRoles = map[string]bool{
	"button":           true,
	"link":             true,
	"textbox":          true,
	"checkbox":         true,
	"radio":            true,
	"combobox":         true,
	"listbox":          true,
	"menuitem":         true,
	"menuitemcheckbox": true,
	"menuitemradio":    true,
	"option":           true,
	"searchbox":        true,
	"slider":           true,
	"spinbutton":       true,
	"switch":           true,
	"tab":              true,
	"treeitem":         true,
}

// Roles that provide structure/context (get refs for text extraction).
var contentRoles = map[string]bool{
	"heading":      true,
	"cell":         true,
	"gridcell":     true,
	"columnheader": true,
	"rowheader":    true,
	"listitem":     true,
	"article":      true,
	"region":       true,
	"main":         true,
	"navigation":   true,
}

// Roles that are purely structural (can be filtered in compact mode).
var structuralRoles = map[string]bool{
	"generic":      true,
	"group":        true,
	"list":         true,
	"table":        true,
	"row":          true,
	"rowgroup":     true,
	"grid":         true,
	"treegrid":     true,
	"menu":         true,
	"menubar":      true,
	"toolbar":      true,
	"tablist":      true,
	"tree":         true,
	"directory":    true,
	"document":     true,
	"application":  true,
	"presentation": true,
	"none":         true,
}

// IsInteractiveRole returns true if the role is interactive.
func IsInteractiveRole(role string) bool {
	return interactiveRoles[strings.ToLower(role)]
}

// IsContentRole returns true if the role provides content.
func IsContentRole(role string) bool {
	return contentRoles[strings.ToLower(role)]
}

// IsStructuralRole returns true if the role is purely structural.
func IsStructuralRole(role string) bool {
	return structuralRoles[strings.ToLower(role)]
}

// roleNameTracker tracks role+name combinations to detect duplicates.
type roleNameTracker struct {
	counts    map[string]int
	refsByKey map[string][]string
}

func newRoleNameTracker() *roleNameTracker {
	return &roleNameTracker{
		counts:    make(map[string]int),
		refsByKey: make(map[string][]string),
	}
}

func (t *roleNameTracker) getKey(role, name string) string {
	return role + ":" + name
}

func (t *roleNameTracker) getNextIndex(role, name string) int {
	key := t.getKey(role, name)
	current := t.counts[key]
	t.counts[key] = current + 1
	return current
}

func (t *roleNameTracker) trackRef(role, name, ref string) {
	key := t.getKey(role, name)
	t.refsByKey[key] = append(t.refsByKey[key], ref)
}

func (t *roleNameTracker) getDuplicateKeys() map[string]bool {
	duplicates := make(map[string]bool)
	for key, refs := range t.refsByKey {
		if len(refs) > 1 {
			duplicates[key] = true
		}
	}
	return duplicates
}

// snapshotGenerator generates accessibility snapshots with refs.
type snapshotGenerator struct {
	refCounter int
	refs       map[string]*RefEntry
	tracker    *roleNameTracker
	options    SnapshotOptions
}

func newSnapshotGenerator(opts SnapshotOptions) *snapshotGenerator {
	return &snapshotGenerator{
		refs:    make(map[string]*RefEntry),
		tracker: newRoleNameTracker(),
		options: opts,
	}
}

func (g *snapshotGenerator) nextRef() string {
	g.refCounter++
	return fmt.Sprintf("e%d", g.refCounter)
}

// buildSelector builds a selector string for storing in ref map.
func buildSelector(role, name string) string {
	if name != "" {
		escapedName := strings.ReplaceAll(name, `"`, `\"`)
		return fmt.Sprintf("getByRole('%s', { name: \"%s\", exact: true })", role, escapedName)
	}
	return fmt.Sprintf("getByRole('%s')", role)
}

// GetAccessibilitySnapshot returns an enhanced accessibility snapshot with refs.
func (p *Page) GetAccessibilitySnapshot(opts SnapshotOptions) (*EnhancedSnapshot, error) {
	gen := newSnapshotGenerator(opts)

	// JavaScript to extract accessibility tree
	js := `
(function() {
	const INTERACTIVE_ROLES = new Set([
		'button', 'link', 'textbox', 'checkbox', 'radio', 'combobox', 'listbox',
		'menuitem', 'menuitemcheckbox', 'menuitemradio', 'option', 'searchbox',
		'slider', 'spinbutton', 'switch', 'tab', 'treeitem'
	]);

	const CONTENT_ROLES = new Set([
		'heading', 'cell', 'gridcell', 'columnheader', 'rowheader', 'listitem',
		'article', 'region', 'main', 'navigation'
	]);

	const STRUCTURAL_ROLES = new Set([
		'generic', 'group', 'list', 'table', 'row', 'rowgroup', 'grid', 'treegrid',
		'menu', 'menubar', 'toolbar', 'tablist', 'tree', 'directory', 'document',
		'application', 'presentation', 'none'
	]);

	// Map HTML elements to implicit ARIA roles
	function getImplicitRole(el) {
		const tag = el.tagName.toLowerCase();
		const type = el.getAttribute('type');

		switch (tag) {
			case 'button': return 'button';
			case 'a': return el.hasAttribute('href') ? 'link' : null;
			case 'input':
				switch (type) {
					case 'button':
					case 'submit':
					case 'reset':
					case 'image': return 'button';
					case 'checkbox': return 'checkbox';
					case 'radio': return 'radio';
					case 'range': return 'slider';
					case 'text':
					case 'email':
					case 'password':
					case 'tel':
					case 'url':
					case 'search':
					case null:
					case undefined: return type === 'search' ? 'searchbox' : 'textbox';
					default: return 'textbox';
				}
			case 'textarea': return 'textbox';
			case 'select': return el.hasAttribute('multiple') ? 'listbox' : 'combobox';
			case 'option': return 'option';
			case 'img': return 'img';
			case 'h1':
			case 'h2':
			case 'h3':
			case 'h4':
			case 'h5':
			case 'h6': return 'heading';
			case 'nav': return 'navigation';
			case 'main': return 'main';
			case 'header': return 'banner';
			case 'footer': return 'contentinfo';
			case 'aside': return 'complementary';
			case 'article': return 'article';
			case 'section': return 'region';
			case 'ul':
			case 'ol': return 'list';
			case 'li': return 'listitem';
			case 'table': return 'table';
			case 'tr': return 'row';
			case 'th': return 'columnheader';
			case 'td': return 'cell';
			case 'form': return 'form';
			default: return null;
		}
	}

	function getAccessibleName(el) {
		// Priority: aria-label > aria-labelledby > visible text > title > placeholder
		if (el.getAttribute('aria-label')) {
			return el.getAttribute('aria-label');
		}

		const labelledBy = el.getAttribute('aria-labelledby');
		if (labelledBy) {
			const labels = labelledBy.split(' ')
				.map(id => document.getElementById(id))
				.filter(Boolean)
				.map(labelEl => labelEl.textContent.trim());
			if (labels.length > 0) {
				return labels.join(' ');
			}
		}

		// For form elements, check associated label
		if (el.id) {
			const label = document.querySelector('label[for="' + el.id + '"]');
			if (label) {
				return label.textContent.trim();
			}
		}

		// For buttons and links, use visible text
		const tag = el.tagName.toLowerCase();
		if (tag === 'button' || tag === 'a') {
			const text = el.textContent.trim();
			if (text) return text.substring(0, 100);
		}

		// For inputs, check placeholder or value
		if (tag === 'input' || tag === 'textarea') {
			if (el.placeholder) return el.placeholder;
			// Don't expose values for security
		}

		// For images, use alt text
		if (tag === 'img') {
			return el.alt || '';
		}

		// Title attribute as fallback
		if (el.title) {
			return el.title;
		}

		return '';
	}

	function getHeadingLevel(el) {
		const tag = el.tagName.toLowerCase();
		const match = tag.match(/^h([1-6])$/);
		if (match) return parseInt(match[1], 10);
		const ariaLevel = el.getAttribute('aria-level');
		if (ariaLevel) return parseInt(ariaLevel, 10);
		return 0;
	}

	function isHidden(el) {
		if (el.getAttribute('aria-hidden') === 'true') return true;
		if (el.hidden) return true;
		const style = window.getComputedStyle(el);
		if (style.display === 'none' || style.visibility === 'hidden') return true;
		return false;
	}

	function walk(el, depth, maxDepth, options) {
		if (!el || isHidden(el)) return null;
		if (maxDepth > 0 && depth > maxDepth) return null;

		const role = el.getAttribute('role') || getImplicitRole(el);
		if (!role) {
			// Element has no accessible role, check children
			const children = [];
			for (const child of el.children) {
				const childNode = walk(child, depth, maxDepth, options);
				if (childNode) children.push(childNode);
			}
			if (children.length === 1) return children[0];
			if (children.length > 1) {
				return { role: 'group', children };
			}
			return null;
		}

		const roleLower = role.toLowerCase();

		// In interactive mode, skip non-interactive elements
		if (options.interactive && !INTERACTIVE_ROLES.has(roleLower)) {
			// But still recurse for children
			const children = [];
			for (const child of el.children) {
				const childNode = walk(child, depth + 1, maxDepth, options);
				if (childNode) children.push(childNode);
			}
			if (children.length === 1) return children[0];
			if (children.length > 0) return { role: 'group', children };
			return null;
		}

		// In compact mode, skip unnamed structural elements
		const name = getAccessibleName(el);
		if (options.compact && STRUCTURAL_ROLES.has(roleLower) && !name) {
			const children = [];
			for (const child of el.children) {
				const childNode = walk(child, depth + 1, maxDepth, options);
				if (childNode) children.push(childNode);
			}
			if (children.length === 1) return children[0];
			if (children.length > 0) return { role: 'group', children };
			return null;
		}

		const node = { role: roleLower };

		if (name) node.name = name;

		// Add specific attributes
		if (roleLower === 'heading') {
			node.level = getHeadingLevel(el);
		}

		if (el.disabled || el.getAttribute('aria-disabled') === 'true') {
			node.disabled = true;
		}

		if (roleLower === 'checkbox' || roleLower === 'radio' || roleLower === 'switch') {
			const checked = el.checked || el.getAttribute('aria-checked');
			if (checked === true || checked === 'true') node.checked = 'true';
			else if (checked === 'mixed') node.checked = 'mixed';
			else node.checked = 'false';
		}

		if (el.getAttribute('aria-expanded') !== null) {
			node.expanded = el.getAttribute('aria-expanded') === 'true';
		}

		if (el.getAttribute('aria-selected') !== null) {
			node.selected = el.getAttribute('aria-selected') === 'true';
		}

		// Mark if this should get a ref
		node.shouldRef = INTERACTIVE_ROLES.has(roleLower) || (CONTENT_ROLES.has(roleLower) && name);

		// Recurse for children
		const children = [];
		for (const child of el.children) {
			const childNode = walk(child, depth + 1, maxDepth, options);
			if (childNode) children.push(childNode);
		}
		if (children.length > 0) {
			node.children = children;
		}

		return node;
	}

	const options = {
		interactive: ` + fmt.Sprintf("%t", opts.Interactive) + `,
		compact: ` + fmt.Sprintf("%t", opts.Compact) + `,
	};

	const maxDepth = ` + fmt.Sprintf("%d", opts.MaxDepth) + `;
	let root = document.body;

	const selector = ` + fmt.Sprintf("%q", opts.Selector) + `;
	if (selector) {
		const el = document.querySelector(selector);
		if (el) root = el;
	}

	return walk(root, 0, maxDepth, options);
})()
`

	var rawTree map[string]interface{}
	if err := chromedp.Run(p.ctx, chromedp.Evaluate(js, &rawTree)); err != nil {
		return nil, fmt.Errorf("extracting accessibility tree: %w", err)
	}

	if rawTree == nil {
		return &EnhancedSnapshot{
			Tree: "(empty)",
			Refs: &RefMap{Refs: make(map[string]*RefEntry)},
		}, nil
	}

	// Convert raw tree to text representation with refs
	tree := gen.formatNode(rawTree, 0)

	// Clean up refs: remove nth from non-duplicates
	duplicates := gen.tracker.getDuplicateKeys()
	for ref, entry := range gen.refs {
		key := gen.tracker.getKey(entry.Role, entry.Name)
		if !duplicates[key] {
			gen.refs[ref].Nth = 0
		}
	}

	return &EnhancedSnapshot{
		Tree: tree,
		Refs: &RefMap{Refs: gen.refs},
	}, nil
}

func (g *snapshotGenerator) formatNode(node map[string]interface{}, depth int) string {
	if node == nil {
		return ""
	}

	roleVal, ok := node["role"]
	if !ok {
		return ""
	}
	role, ok := roleVal.(string)
	if !ok {
		return ""
	}

	var lines []string
	indent := strings.Repeat("  ", depth)

	// Build the line
	line := indent + "- " + role

	// Add name if present
	name := ""
	if nameVal, ok := node["name"]; ok {
		if n, ok := nameVal.(string); ok && n != "" {
			name = n
			line += fmt.Sprintf(" %q", name)
		}
	}

	// Add ref if this node should have one
	if shouldRef, ok := node["shouldRef"].(bool); ok && shouldRef {
		ref := g.nextRef()
		nth := g.tracker.getNextIndex(role, name)
		g.tracker.trackRef(role, name, ref)
		g.refs[ref] = &RefEntry{
			Selector: buildSelector(role, name),
			Role:     role,
			Name:     name,
			Nth:      nth,
		}
		line += fmt.Sprintf(" [ref=%s]", ref)
		if nth > 0 {
			line += fmt.Sprintf(" [nth=%d]", nth)
		}
	}

	// Add special attributes
	if level, ok := node["level"].(float64); ok && level > 0 {
		line += fmt.Sprintf(" [level=%d]", int(level))
	}
	if disabled, ok := node["disabled"].(bool); ok && disabled {
		line += " [disabled]"
	}
	if checked, ok := node["checked"].(string); ok {
		line += fmt.Sprintf(" [checked=%s]", checked)
	}
	if expanded, ok := node["expanded"].(bool); ok {
		if expanded {
			line += " [expanded]"
		} else {
			line += " [collapsed]"
		}
	}
	if selected, ok := node["selected"].(bool); ok && selected {
		line += " [selected]"
	}

	lines = append(lines, line)

	// Process children
	if childrenVal, ok := node["children"]; ok {
		if children, ok := childrenVal.([]interface{}); ok {
			for _, child := range children {
				if childMap, ok := child.(map[string]interface{}); ok {
					childStr := g.formatNode(childMap, depth+1)
					if childStr != "" {
						lines = append(lines, childStr)
					}
				}
			}
		}
	}

	return strings.Join(lines, "\n")
}

// GetAccessibilitySnapshotJSON returns the accessibility tree as JSON.
func (p *Page) GetAccessibilitySnapshotJSON(opts SnapshotOptions) ([]byte, error) {
	snapshot, err := p.GetAccessibilitySnapshot(opts)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(snapshot, "", "  ")
}

// ParseRef parses a ref from command argument (e.g., "@e1" -> "e1", "ref=e1" -> "e1", "e1" -> "e1").
func ParseRef(arg string) string {
	if strings.HasPrefix(arg, "@") {
		return arg[1:]
	}
	if strings.HasPrefix(arg, "ref=") {
		return arg[4:]
	}
	if matched, _ := regexp.MatchString(`^e\d+$`, arg); matched {
		return arg
	}
	return ""
}

// GetSnapshotStats returns statistics about a snapshot.
func GetSnapshotStats(tree string, refs *RefMap) map[string]int {
	interactive := 0
	if refs != nil {
		for _, entry := range refs.Refs {
			if IsInteractiveRole(entry.Role) {
				interactive++
			}
		}
	}

	lines := strings.Count(tree, "\n") + 1
	if tree == "" {
		lines = 0
	}

	return map[string]int{
		"lines":       lines,
		"chars":       len(tree),
		"tokens":      (len(tree) + 3) / 4, // Rough estimate
		"refs":        len(refs.Refs),
		"interactive": interactive,
	}
}
