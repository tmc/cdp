package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- DOM snapshot / diff tools ---

// domNode is a simplified tree node for diffing.
type domNode struct {
	Role     string            `json:"role"`
	Name     string            `json:"name,omitempty"`
	Value    string            `json:"value,omitempty"`
	Props    map[string]string `json:"props,omitempty"`
	Children []*domNode        `json:"children,omitempty"`
}

// domSnapshotStore holds named DOM snapshots.
type domSnapshotStore struct {
	mu        sync.Mutex
	snapshots map[string]*domNode
}

func newDomSnapshotStore() *domSnapshotStore {
	return &domSnapshotStore{
		snapshots: make(map[string]*domNode),
	}
}

func (ds *domSnapshotStore) save(name string, root *domNode) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.snapshots[name] = root
}

func (ds *domSnapshotStore) get(name string) *domNode {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.snapshots[name]
}

func (ds *domSnapshotStore) list() []string {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	var names []string
	for k := range ds.snapshots {
		names = append(names, k)
	}
	return names
}

// captureAXTree fetches the accessibility tree and builds a simplified domNode tree.
func captureAXTree(ctx context.Context) (*domNode, error) {
	var nodes []*accessibility.Node
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		nodes, err = accessibility.GetFullAXTree().Do(ctx)
		return err
	})); err != nil {
		return nil, fmt.Errorf("get ax tree: %w", err)
	}

	byID := make(map[accessibility.NodeID]*accessibility.Node, len(nodes))
	children := make(map[accessibility.NodeID][]accessibility.NodeID, len(nodes))
	var rootID accessibility.NodeID
	for _, n := range nodes {
		byID[n.NodeID] = n
		if n.ParentID == "" {
			rootID = n.NodeID
		} else {
			children[n.ParentID] = append(children[n.ParentID], n.NodeID)
		}
	}

	var build func(id accessibility.NodeID) *domNode
	build = func(id accessibility.NodeID) *domNode {
		n, ok := byID[id]
		if !ok {
			return nil
		}
		if n.Ignored {
			// Flatten ignored nodes — promote their children.
			var merged []*domNode
			for _, cid := range children[id] {
				if child := build(cid); child != nil {
					merged = append(merged, child)
				}
			}
			if len(merged) == 1 {
				return merged[0]
			}
			if len(merged) > 1 {
				return &domNode{Role: "group", Children: merged}
			}
			return nil
		}

		role := axValueString(n.Role)
		name := axValueString(n.Name)
		value := axValueString(n.Value)

		// Skip noise.
		if (role == "generic" || role == "none" || role == "") && name == "" {
			var merged []*domNode
			for _, cid := range children[id] {
				if child := build(cid); child != nil {
					merged = append(merged, child)
				}
			}
			if len(merged) == 1 {
				return merged[0]
			}
			if len(merged) > 1 {
				return &domNode{Role: "group", Children: merged}
			}
			return nil
		}

		// Collapse StaticText.
		if role == "StaticText" || role == "InlineTextBox" {
			if name == "" {
				return nil
			}
			return &domNode{Role: "text", Name: name}
		}

		dn := &domNode{
			Role:  role,
			Name:  name,
			Value: value,
		}

		// Capture useful properties.
		props := make(map[string]string)
		for _, p := range n.Properties {
			v := axValueString(p.Value)
			if v != "" && v != "false" {
				props[string(p.Name)] = v
			}
		}
		if len(props) > 0 {
			dn.Props = props
		}

		for _, cid := range children[id] {
			if child := build(cid); child != nil {
				dn.Children = append(dn.Children, child)
			}
		}
		return dn
	}

	root := build(rootID)
	if root == nil {
		root = &domNode{Role: "document"}
	}
	return root, nil
}

// --- Diff algorithm ---

// renderTree renders a domNode tree to indented lines.
func renderTree(n *domNode, depth int) []string {
	if n == nil {
		return nil
	}
	var lines []string
	lines = append(lines, strings.Repeat("  ", depth)+formatNode(n))
	for _, c := range n.Children {
		lines = append(lines, renderTree(c, depth+1)...)
	}
	return lines
}

// diffDom produces a diff between two DOM trees using LCS on rendered lines.
// Modified lines show inline changes with [-removed-]{+added+} markers.
func diffDom(before, after *domNode) string {
	beforeLines := renderTree(before, 0)
	afterLines := renderTree(after, 0)
	return lineDiff(beforeLines, afterLines)
}

// lineDiff produces a unified-style diff with [-removed-]{+added+} for inline changes.
func lineDiff(a, b []string) string {
	// LCS table.
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var out strings.Builder
	var removed, added []string
	flush := func() {
		// Try to pair removed/added lines for inline word diff.
		pairs := len(removed)
		if len(added) < pairs {
			pairs = len(added)
		}
		for k := 0; k < pairs; k++ {
			wd := wordDiff(removed[k], added[k])
			fmt.Fprintf(&out, "  %s\n", wd)
		}
		for k := pairs; k < len(removed); k++ {
			fmt.Fprintf(&out, "- %s\n", removed[k])
		}
		for k := pairs; k < len(added); k++ {
			fmt.Fprintf(&out, "+ %s\n", added[k])
		}
		removed = removed[:0]
		added = added[:0]
	}

	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			flush()
			fmt.Fprintf(&out, "  %s\n", a[i])
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			removed = append(removed, a[i])
			i++
		} else {
			added = append(added, b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		removed = append(removed, a[i])
	}
	for ; j < len(b); j++ {
		added = append(added, b[j])
	}
	flush()
	return out.String()
}

// wordDiff produces an inline diff between two lines using [-removed-]{+added+} markers.
func wordDiff(a, b string) string {
	if a == b {
		return a
	}
	aToks := wordTokens(a)
	bToks := wordTokens(b)

	// LCS on tokens.
	dp := make([][]int, len(aToks)+1)
	for i := range dp {
		dp[i] = make([]int, len(bToks)+1)
	}
	for i := len(aToks) - 1; i >= 0; i-- {
		for j := len(bToks) - 1; j >= 0; j-- {
			if aToks[i] == bToks[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var buf strings.Builder
	var rem, add []string
	flushWd := func() {
		if len(rem) > 0 {
			fmt.Fprintf(&buf, "[-%s-]", strings.Join(rem, ""))
			rem = rem[:0]
		}
		if len(add) > 0 {
			fmt.Fprintf(&buf, "{+%s+}", strings.Join(add, ""))
			add = add[:0]
		}
	}

	i, j := 0, 0
	for i < len(aToks) && j < len(bToks) {
		if aToks[i] == bToks[j] {
			flushWd()
			buf.WriteString(aToks[i])
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			rem = append(rem, aToks[i])
			i++
		} else {
			add = append(add, bToks[j])
			j++
		}
	}
	for ; i < len(aToks); i++ {
		rem = append(rem, aToks[i])
	}
	for ; j < len(bToks); j++ {
		add = append(add, bToks[j])
	}
	flushWd()
	return buf.String()
}

// wordTokens splits a string into tokens preserving whitespace.
func wordTokens(s string) []string {
	var tokens []string
	var cur []byte
	inSpace := false
	for i := 0; i < len(s); i++ {
		isSpace := s[i] == ' ' || s[i] == '\t'
		if i > 0 && isSpace != inSpace {
			tokens = append(tokens, string(cur))
			cur = cur[:0]
		}
		cur = append(cur, s[i])
		inSpace = isSpace
	}
	if len(cur) > 0 {
		tokens = append(tokens, string(cur))
	}
	return tokens
}

func formatNode(n *domNode) string {
	var parts []string
	parts = append(parts, n.Role)
	if n.Name != "" {
		parts = append(parts, fmt.Sprintf("%q", n.Name))
	}
	if n.Value != "" {
		parts = append(parts, fmt.Sprintf("value=%q", n.Value))
	}
	for k, v := range n.Props {
		parts = append(parts, fmt.Sprintf("[%s=%s]", k, v))
	}
	return strings.Join(parts, " ")
}

// --- MCP tool registration ---

type SnapshotDomInput struct {
	Name string `json:"name"`
}

type DomDiffInput struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

func registerDomDiffTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "snapshot_dom",
		Description: "Capture a named DOM snapshot (simplified accessibility tree) for later comparison with dom_diff. Returns a summary of the captured tree.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SnapshotDomInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" {
			return nil, nil, fmt.Errorf("snapshot_dom: name is required")
		}

		actx := s.activeCtx()
		root, err := captureAXTree(actx)
		if err != nil {
			return nil, nil, fmt.Errorf("snapshot_dom: %w", err)
		}

		if s.domSnapshots == nil {
			s.domSnapshots = newDomSnapshotStore()
		}
		s.domSnapshots.save(input.Name, root)

		nodeCount := countNodes(root)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("snapshot %q saved (%d nodes)", input.Name, nodeCount)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "dom_diff",
		Description: "Compare two named DOM snapshots and show a git-style diff of what changed. Use snapshot_dom to capture before/after states.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DomDiffInput) (*mcp.CallToolResult, any, error) {
		if s.domSnapshots == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no snapshots available — use snapshot_dom first"}},
			}, nil, nil
		}

		before := s.domSnapshots.get(input.Before)
		if before == nil {
			return nil, nil, fmt.Errorf("dom_diff: snapshot %q not found", input.Before)
		}
		after := s.domSnapshots.get(input.After)
		if after == nil {
			return nil, nil, fmt.Errorf("dom_diff: snapshot %q not found", input.After)
		}

		diff := diffDom(before, after)
		if diff == "" {
			diff = "(no changes)"
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: diff}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_dom_snapshots",
		Description: "List all named DOM snapshots available for dom_diff.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		if s.domSnapshots == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no snapshots"}},
			}, nil, nil
		}

		names := s.domSnapshots.list()
		if len(names) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no snapshots"}},
			}, nil, nil
		}

		data, _ := json.Marshal(names)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}

func countNodes(n *domNode) int {
	if n == nil {
		return 0
	}
	count := 1
	for _, c := range n.Children {
		count += countNodes(c)
	}
	return count
}
