package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type browserObserveOutput struct {
	StateID          string `json:"state_id"`
	TargetID         string `json:"target_id"`
	FrameID          string `json:"frame_id"`
	LoaderID         string `json:"loader_id"`
	DocBackendNodeID int64  `json:"doc_backend_node_id"`
	URL              string `json:"url"`
	Tree             string `json:"tree"`
}

type expectClause struct {
	Selector string `json:"selector"`
	Text     string `json:"text"`
}

type browserActInput struct {
	StateID   string        `json:"state_id"`
	TargetID  string        `json:"target_id"`
	Action    string        `json:"action"`
	Ref       string        `json:"ref"`
	Text      string        `json:"text,omitempty"`
	Expect    *expectClause `json:"expect,omitempty"`
	TimeoutMS int           `json:"timeout_ms,omitempty"`
}

type browserActOutput struct {
	Execution     string                `json:"execution"`
	Observation   string                `json:"observation"`
	Postcondition string                `json:"postcondition"`
	FreshState    *browserObserveOutput `json:"fresh_state,omitempty"`
	ErrorText     string                `json:"error_text,omitempty"`
}

type browserObservation struct {
	output browserObserveOutput
	ctx    context.Context
	refs   *refRegistry
}

func registerBrowserObservationTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "browser_observe",
		Description: "Observe the active browser target. Returns a state_id, exact target/document identity, and a tree with @refs. Replaces the previous observation. Use browser_act with this state_id and target_id. Same-process subframe selection is not yet supported.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, browserObserveOutput, error) {
		var out browserObserveOutput
		err := s.withBrowserObservation(ctx, 30*time.Second, func(ctx, actx context.Context) error {
			state, err := s.captureBrowserObservation(ctx, actx)
			if err != nil {
				return err
			}
			s.observation = state
			out = state.output
			return nil
		})
		return nil, out, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "browser_act",
		Description: "Click or type using a browser_observe state_id, target_id, and @ref. Consumes that state before attempting the action; stale nodes are never recovered by name. Returns independent execution, observation and postcondition results and, when captured, a fresh_state. expect compares exact textContent of one CSS match immediately after capture. Never automatically replay an uncertain action. Timeout is milliseconds, default 30000, maximum 60000.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in browserActInput) (*mcp.CallToolResult, browserActOutput, error) {
		out := newBrowserActOutput(in)
		if in.TimeoutMS < 0 || in.TimeoutMS > 60000 {
			out.ErrorText = "timeout_ms must be between 0 and 60000"
			return nil, out, nil
		}
		timeout := 30 * time.Second
		if in.TimeoutMS != 0 {
			timeout = time.Duration(in.TimeoutMS) * time.Millisecond
		}
		err := s.withBrowserObservation(ctx, timeout, func(ctx, actx context.Context) error {
			out = s.actOnBrowserObservation(ctx, actx, in)
			return nil
		})
		if err != nil {
			out.ErrorText = err.Error()
		}
		return nil, out, nil
	})
}

// withBrowserObservation serializes this observation/action pair. Other tool
// calls and page scripts are not excluded; every operation pins its target.
func (s *mcpSession) withBrowserObservation(ctx context.Context, timeout time.Duration, f func(context.Context, context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	s.mu.Lock()
	if s.observationGate == nil {
		s.observationGate = make(chan struct{}, 1)
	}
	gate := s.observationGate
	s.mu.Unlock()
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	actx, err := s.activeContext(ctx)
	if err != nil {
		return err
	}
	runCtx, stop := requestToolCtx(ctx, actx, timeout)
	defer stop()
	return chromedp.Run(runCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return f(ctx, actx)
	}))
}

func browserDocument(ctx context.Context) (browserObserveOutput, error) {
	var out browserObserveOutput
	c := chromedp.FromContext(ctx)
	if c == nil || c.Target == nil {
		return out, fmt.Errorf("browser target unavailable")
	}
	out.TargetID = string(c.Target.TargetID)
	tree, err := page.GetFrameTree().Do(ctx)
	if err != nil {
		return out, err
	}
	// GetDocument resets the protocol's frontend node bindings, invalidating
	// chromedp's cached tree. Describe the document through a runtime handle
	// instead so observation does not break later selector queries.
	object, exception, err := runtime.Evaluate("document").Do(ctx)
	if err != nil {
		return out, err
	}
	if exception != nil || object == nil || object.ObjectID == "" {
		return out, fmt.Errorf("browser document object unavailable")
	}
	defer runtime.ReleaseObject(object.ObjectID).Do(ctx)
	root, err := dom.DescribeNode().WithObjectID(object.ObjectID).Do(ctx)
	if err != nil {
		return out, err
	}
	if tree == nil || tree.Frame == nil || root == nil || root.BackendNodeID == 0 {
		return out, fmt.Errorf("browser document unavailable")
	}
	out.FrameID = string(tree.Frame.ID)
	out.LoaderID = string(tree.Frame.LoaderID)
	out.DocBackendNodeID = int64(root.BackendNodeID)
	out.URL = tree.Frame.URL
	return out, nil
}

func sameBrowserDocument(a, b browserObserveOutput) bool {
	return a.TargetID == b.TargetID && a.FrameID == b.FrameID &&
		a.LoaderID == b.LoaderID && a.DocBackendNodeID == b.DocBackendNodeID && a.URL == b.URL
}

func (s *mcpSession) captureBrowserObservation(ctx, actx context.Context) (*browserObservation, error) {
	before, err := browserDocument(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	frame, current := s.activeFrameID, s.ctx
	s.mu.Unlock()
	if current != actx {
		return nil, fmt.Errorf("active target changed during observation")
	}
	if frame != "" && string(frame) != before.FrameID {
		return nil, fmt.Errorf("same-process subframe observation is not supported yet")
	}
	refs := newRefRegistry()
	if err := accessibility.Enable().Do(ctx); err != nil {
		return nil, fmt.Errorf("accessibility observation: %w", err)
	}
	nodes, err := accessibility.GetFullAXTree().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("accessibility observation: %w", err)
	}
	tree := formatAXSnapshot(nodes, refs)
	after, err := browserDocument(ctx)
	if err != nil {
		return nil, err
	}
	if !sameBrowserDocument(before, after) {
		return nil, fmt.Errorf("document changed during observation; observe again")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, fmt.Errorf("observation id: %w", err)
	}
	after.StateID = hex.EncodeToString(id[:])
	after.Tree = tree
	return &browserObservation{output: after, ctx: actx, refs: refs}, nil
}

func newBrowserActOutput(in browserActInput) browserActOutput {
	out := browserActOutput{Execution: "not_dispatched", Observation: "unavailable", Postcondition: "not_requested"}
	if in.Expect != nil {
		out.Postcondition = "unknown"
	}
	return out
}

func (s *mcpSession) actOnBrowserObservation(ctx, actx context.Context, in browserActInput) browserActOutput {
	out := newBrowserActOutput(in)
	state := s.observation
	if state == nil || in.StateID == "" || state.output.StateID != in.StateID {
		out.ErrorText = "unknown or consumed state_id; call browser_observe"
		return out
	}
	// A failed attempt cannot leave a reusable action handle behind.
	s.observation = nil
	entry, err := s.validateBrowserAction(ctx, actx, state, in)
	if err != nil {
		out.ErrorText = err.Error()
		return out
	}
	tracker := &browserActionExecutor{base: cdp.ExecutorFromContext(ctx)}
	actionCtx := cdp.WithExecutor(ctx, tracker)
	switch in.Action {
	case "click":
		err = clickObservedBrowserNode(actionCtx, entry.BackendNodeID, cdp.FrameID(state.output.FrameID))
	case "type":
		err = page.BringToFront().Do(actionCtx)
		if err == nil {
			err = dom.Focus().WithBackendNodeID(entry.BackendNodeID).Do(actionCtx)
		}
		if err == nil {
			err = observedBrowserNodeFocused(actionCtx, entry.BackendNodeID)
		}
		if err == nil {
			err = chromedp.KeyEvent(in.Text).Do(actionCtx)
		}
	}
	if tracker.attempted {
		out.Execution = "dispatched_unknown"
	}
	if err != nil {
		out.ErrorText = err.Error()
	} else {
		out.Execution = "completed"
	}
	fresh, err := s.captureBrowserObservation(ctx, actx)
	if err != nil {
		if out.ErrorText != "" {
			out.ErrorText += "; "
		}
		out.ErrorText += fmt.Sprintf("post-action observation: %v", err)
		return out
	}
	s.observation = fresh
	out.Observation = "captured"
	out.FreshState = &fresh.output
	if in.Expect != nil {
		met, err := browserTextMatches(ctx, in.Expect, cdp.BackendNodeID(fresh.output.DocBackendNodeID))
		if err != nil {
			if out.ErrorText != "" {
				out.ErrorText += "; "
			}
			out.ErrorText += fmt.Sprintf("postcondition: %v", err)
		} else if met {
			out.Postcondition = "met"
		} else {
			out.Postcondition = "unmet"
		}
	}
	return out
}

func observedBrowserNodeFocused(ctx context.Context, nodeID cdp.BackendNodeID) error {
	node, err := dom.ResolveNode().WithBackendNodeID(nodeID).Do(ctx)
	if err != nil {
		return err
	}
	defer runtime.ReleaseObject(node.ObjectID).Do(ctx)
	result, exception, err := runtime.CallFunctionOn(`function() { if (!this.isConnected) return false; for (let n = this; ; ) { const root = n.getRootNode(); if (root.activeElement !== n) return false; if (!root.host) return true; n = root.host; } }`).
		WithObjectID(node.ObjectID).WithReturnByValue(true).Do(ctx)
	if err != nil {
		return err
	}
	if exception != nil || result == nil || string(result.Value) != "true" {
		return fmt.Errorf("focus moved away from the observation node")
	}
	return nil
}

func clickObservedBrowserNode(ctx context.Context, nodeID cdp.BackendNodeID, frameID cdp.FrameID) error {
	if err := page.BringToFront().Do(ctx); err != nil {
		return err
	}
	if err := dom.ScrollIntoViewIfNeeded().WithBackendNodeID(nodeID).Do(ctx); err != nil {
		return fmt.Errorf("scroll into view: %w", err)
	}
	quads, err := dom.GetContentQuads().WithBackendNodeID(nodeID).Do(ctx)
	if err != nil {
		return err
	}
	if len(quads) == 0 || len(quads[0]) != 8 {
		return fmt.Errorf("observation node has no clickable quad")
	}
	q := quads[0]
	x, y := (q[0]+q[2]+q[4]+q[6])/4, (q[1]+q[3]+q[5]+q[7])/4
	hit, frame, _, err := dom.GetNodeForLocation(int64(x), int64(y)).Do(ctx)
	if err != nil {
		return err
	}
	if frame != frameID {
		return fmt.Errorf("click point belongs to another frame")
	}
	if hit != nodeID {
		node, err := dom.ResolveNode().WithBackendNodeID(nodeID).Do(ctx)
		if err != nil {
			return err
		}
		defer runtime.ReleaseObject(node.ObjectID).Do(ctx)
		other, err := dom.ResolveNode().WithBackendNodeID(hit).Do(ctx)
		if err != nil {
			return err
		}
		defer runtime.ReleaseObject(other.ObjectID).Do(ctx)
		// Follow the composed ancestry so a label inside a shadow tree can
		// still belong to the observed control, but an overlay cannot.
		result, exception, err := runtime.CallFunctionOn(`function(hit) { for (let n = hit; n; n = n.parentNode || (n.getRootNode && n.getRootNode().host)) { if (n === this) return this.isConnected; } return false; }`).
			WithObjectID(node.ObjectID).WithArguments([]*runtime.CallArgument{{ObjectID: other.ObjectID}}).WithReturnByValue(true).Do(ctx)
		if err != nil {
			return err
		}
		if exception != nil || result == nil || string(result.Value) != "true" {
			return fmt.Errorf("observation node is covered at its click point")
		}
	}
	return clickAt(ctx, viewportPoint{X: x, Y: y})
}

func (s *mcpSession) validateBrowserAction(ctx, actx context.Context, state *browserObservation, in browserActInput) (refEntry, error) {
	var empty refEntry
	if state.ctx != actx || state.output.TargetID != in.TargetID {
		return empty, fmt.Errorf("observation belongs to a different target")
	}
	if in.Action != "click" && in.Action != "type" {
		return empty, fmt.Errorf("action must be click or type")
	}
	if in.Action == "type" && in.Text == "" {
		return empty, fmt.Errorf("type requires non-empty text")
	}
	n, err := strconv.Atoi(strings.TrimPrefix(in.Ref, "@"))
	if err != nil || n <= 0 || fmt.Sprintf("@%d", n) != in.Ref {
		return empty, fmt.Errorf("invalid observation ref %q", in.Ref)
	}
	entry, ok := state.refs.get(n)
	if !ok {
		return empty, fmt.Errorf("ref %q is absent from this observation", in.Ref)
	}
	current, err := browserDocument(ctx)
	if err != nil {
		return empty, err
	}
	if !sameBrowserDocument(state.output, current) {
		return empty, fmt.Errorf("observation document is stale; call browser_observe")
	}
	s.mu.Lock()
	frame := s.activeFrameID
	s.mu.Unlock()
	if frame != "" && string(frame) != current.FrameID {
		return empty, fmt.Errorf("selected frame changed; call browser_observe")
	}
	if err := browserNodeConnected(ctx, entry.BackendNodeID, cdp.BackendNodeID(current.DocBackendNodeID)); err != nil {
		return empty, err
	}
	if in.Expect != nil {
		if in.Expect.Selector == "" {
			return empty, fmt.Errorf("expect requires a selector")
		}
		if _, err := browserTextMatches(ctx, in.Expect, cdp.BackendNodeID(current.DocBackendNodeID)); err != nil {
			return empty, fmt.Errorf("invalid expect: %w", err)
		}
	}
	return entry, ctx.Err()
}

func browserNodeConnected(ctx context.Context, nodeID, documentID cdp.BackendNodeID) error {
	node, err := dom.ResolveNode().WithBackendNodeID(nodeID).Do(ctx)
	if err != nil {
		return fmt.Errorf("stale observation node: %w", err)
	}
	defer runtime.ReleaseObject(node.ObjectID).Do(ctx)
	doc, err := dom.ResolveNode().WithBackendNodeID(documentID).Do(ctx)
	if err != nil {
		return err
	}
	defer runtime.ReleaseObject(doc.ObjectID).Do(ctx)
	result, exception, err := runtime.CallFunctionOn(`function(doc) { return this.isConnected && this.ownerDocument === doc; }`).
		WithObjectID(node.ObjectID).WithArguments([]*runtime.CallArgument{{ObjectID: doc.ObjectID}}).WithReturnByValue(true).Do(ctx)
	if err != nil {
		return err
	}
	if exception != nil || result == nil || string(result.Value) != "true" {
		return fmt.Errorf("observation node is detached or belongs to another document")
	}
	return nil
}

// browserActionExecutor records the boundary at which an action-related side
// effect is attempted. An error after this boundary cannot imply no dispatch.
type browserActionExecutor struct {
	base      cdp.Executor
	attempted bool
}

func (e *browserActionExecutor) Execute(ctx context.Context, method string, params, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(method, "Input.") || method == "DOM.scrollIntoViewIfNeeded" || method == "DOM.focus" || method == "Page.bringToFront" {
		e.attempted = true
	}
	return e.base.Execute(ctx, method, params, result)
}

func browserTextMatches(ctx context.Context, expect *expectClause, documentID cdp.BackendNodeID) (bool, error) {
	selector, _ := json.Marshal(expect.Selector)
	want, _ := json.Marshal(expect.Text)
	doc, err := dom.ResolveNode().WithBackendNodeID(documentID).Do(ctx)
	if err != nil {
		return false, err
	}
	defer runtime.ReleaseObject(doc.ObjectID).Do(ctx)
	expr := fmt.Sprintf(`function() { if (this !== document) throw new Error('stale document'); const nodes = this.querySelectorAll(%s); return nodes.length === 1 && nodes[0].textContent === %s; }`, selector, want)
	result, exception, err := runtime.CallFunctionOn(expr).WithObjectID(doc.ObjectID).WithReturnByValue(true).Do(ctx)
	if err != nil {
		return false, err
	}
	if exception != nil {
		return false, fmt.Errorf("text comparison: %s", exception.Text)
	}
	if result == nil || result.Type != runtime.TypeBoolean {
		return false, fmt.Errorf("text comparison returned no boolean")
	}
	return string(result.Value) == "true", nil
}
