package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pkg/errors"
)

// DOMController handles DOM manipulation operations
type DOMController struct {
	debugger *ChromeDebugger
	verbose  bool

	// Cache the document node
	documentNode *cdp.Node
}

// NewDOMController creates a new DOM controller
func NewDOMController(debugger *ChromeDebugger, verbose bool) *DOMController {
	return &DOMController{
		debugger: debugger,
		verbose:  verbose,
	}
}

// GetDocumentNode gets the root document node
func (dc *DOMController) GetDocumentNode(ctx context.Context) (*cdp.Node, error) {
	if !dc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var node *cdp.Node
	err := chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable DOM domain
			if err := dom.Enable().Do(ctx); err != nil {
				return err
			}

			// Get document
			root, err := dom.GetDocument().Do(ctx)
			if err != nil {
				return err
			}

			node = root
			return nil
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to get document node")
	}

	dc.documentNode = node
	return node, nil
}

// QuerySelector finds an element by CSS selector
func (dc *DOMController) QuerySelector(ctx context.Context, selector string) (*cdp.Node, error) {
	if !dc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	// Ensure we have the document node
	if dc.documentNode == nil {
		if _, err := dc.GetDocumentNode(ctx); err != nil {
			return nil, err
		}
	}

	var nodeID cdp.NodeID
	err := chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			id, err := dom.QuerySelector(dc.documentNode.NodeID, selector).Do(ctx)
			nodeID = id
			return err
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to query selector")
	}

	if nodeID == 0 {
		return nil, fmt.Errorf("element not found: %s", selector)
	}

	// Get node details
	return dc.DescribeNode(ctx, nodeID)
}

// QuerySelectorAll finds all elements matching a CSS selector
func (dc *DOMController) QuerySelectorAll(ctx context.Context, selector string) ([]*cdp.Node, error) {
	if !dc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	// Ensure we have the document node
	if dc.documentNode == nil {
		if _, err := dc.GetDocumentNode(ctx); err != nil {
			return nil, err
		}
	}

	var nodeIDs []cdp.NodeID
	err := chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			ids, err := dom.QuerySelectorAll(dc.documentNode.NodeID, selector).Do(ctx)
			nodeIDs = ids
			return err
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to query selector all")
	}

	// Get details for each node
	var nodes []*cdp.Node
	for _, id := range nodeIDs {
		node, err := dc.DescribeNode(ctx, id)
		if err == nil && node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// DescribeNode gets detailed information about a node
func (dc *DOMController) DescribeNode(ctx context.Context, nodeID cdp.NodeID) (*cdp.Node, error) {
	if !dc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var node *cdp.Node
	err := chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			n, err := dom.DescribeNode().WithNodeID(nodeID).WithDepth(1).Do(ctx)
			node = n
			return err
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to describe node")
	}

	return node, nil
}

// GetOuterHTML gets the outer HTML of an element
func (dc *DOMController) GetOuterHTML(ctx context.Context, selector string) (string, error) {
	if !dc.debugger.connected {
		return "", errors.New("not connected to Chrome")
	}

	node, err := dc.QuerySelector(ctx, selector)
	if err != nil {
		return "", err
	}

	var html string
	err = chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			h, err := dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
			html = h
			return err
		}),
	)

	if err != nil {
		return "", errors.Wrap(err, "failed to get outer HTML")
	}

	return html, nil
}

// SetOuterHTML sets the outer HTML of an element
func (dc *DOMController) SetOuterHTML(ctx context.Context, selector string, html string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	node, err := dc.QuerySelector(ctx, selector)
	if err != nil {
		return err
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return dom.SetOuterHTML(node.NodeID, html).Do(ctx)
		}),
	)
}

// GetAttributes gets all attributes of an element
func (dc *DOMController) GetAttributes(ctx context.Context, selector string) (map[string]string, error) {
	if !dc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	node, err := dc.QuerySelector(ctx, selector)
	if err != nil {
		return nil, err
	}

	var attributes []string
	err = chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			attrs, err := dom.GetAttributes(node.NodeID).Do(ctx)
			attributes = attrs
			return err
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to get attributes")
	}

	// Convert to map
	result := make(map[string]string)
	for i := 0; i < len(attributes); i += 2 {
		if i+1 < len(attributes) {
			result[attributes[i]] = attributes[i+1]
		}
	}

	return result, nil
}

// SetAttribute sets an attribute on an element
func (dc *DOMController) SetAttribute(ctx context.Context, selector string, name string, value string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	node, err := dc.QuerySelector(ctx, selector)
	if err != nil {
		return err
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return dom.SetAttributeValue(node.NodeID, name, value).Do(ctx)
		}),
	)
}

// RemoveAttribute removes an attribute from an element
func (dc *DOMController) RemoveAttribute(ctx context.Context, selector string, name string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	node, err := dc.QuerySelector(ctx, selector)
	if err != nil {
		return err
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return dom.RemoveAttribute(node.NodeID, name).Do(ctx)
		}),
	)
}

// RemoveNode removes a node from the DOM
func (dc *DOMController) RemoveNode(ctx context.Context, selector string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	node, err := dc.QuerySelector(ctx, selector)
	if err != nil {
		return err
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return dom.RemoveNode(node.NodeID).Do(ctx)
		}),
	)
}

// GetComputedStyles gets computed styles for an element
func (dc *DOMController) GetComputedStyles(ctx context.Context, selector string) (map[string]interface{}, error) {
	if !dc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	// Use JavaScript to get computed styles
	expression := fmt.Sprintf(`
		(function() {
			const el = document.querySelector('%s');
			if (!el) return null;
			const styles = window.getComputedStyle(el);
			const result = {};
			for (let prop of styles) {
				result[prop] = styles[prop];
			}
			return result;
		})()
	`, selector)

	var result *runtime.RemoteObject
	err := chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			res, _, err := runtime.Evaluate(expression).Do(ctx)
			result = res
			return err
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to get computed styles")
	}

	if result.Value != nil {
		var styles map[string]interface{}
		if err := json.Unmarshal(result.Value, &styles); err == nil {
			return styles, nil
		}
	}

	return nil, errors.New("failed to parse computed styles")
}

// HighlightNode highlights a node on the page
func (dc *DOMController) HighlightNode(ctx context.Context, selector string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	// Verify the element exists first
	_, err := dc.QuerySelector(ctx, selector)
	if err != nil {
		return err
	}

	// Use Overlay domain to highlight (DOM.highlightNode was removed in newer versions)
	// For now, use a JavaScript-based highlight
	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Use JavaScript to add a highlight
			highlightJS := fmt.Sprintf(`
				(function() {
					const el = document.querySelector('%s');
					if (!el) return false;
					const originalStyle = el.style.cssText;
					el.style.outline = '2px solid #6FA8DC';
					el.style.outlineOffset = '2px';
					el.style.backgroundColor = 'rgba(111, 168, 220, 0.3)';
					el.dataset.highlighted = 'true';
					el.dataset.originalStyle = originalStyle;
					return true;
				})()
			`, selector)
			_, _, err := runtime.Evaluate(highlightJS).Do(ctx)
			return err
		}),
	)
}

// HideHighlight removes any highlight from the page
func (dc *DOMController) HideHighlight(ctx context.Context) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Remove JavaScript-based highlights
			hideJS := `
				(function() {
					const els = document.querySelectorAll('[data-highlighted="true"]');
					els.forEach(el => {
						el.style.cssText = el.dataset.originalStyle || '';
						delete el.dataset.highlighted;
						delete el.dataset.originalStyle;
					});
					return true;
				})()
			`
			_, _, err := runtime.Evaluate(hideJS).Do(ctx)
			return err
		}),
	)
}

// GetBoxModel gets the box model for an element
func (dc *DOMController) GetBoxModel(ctx context.Context, selector string) (*dom.BoxModel, error) {
	if !dc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	node, err := dc.QuerySelector(ctx, selector)
	if err != nil {
		return nil, err
	}

	var model *dom.BoxModel
	err = chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			m, err := dom.GetBoxModel().WithNodeID(node.NodeID).Do(ctx)
			model = m
			return err
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to get box model")
	}

	return model, nil
}

// ScrollIntoView scrolls an element into view
func (dc *DOMController) ScrollIntoView(ctx context.Context, selector string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	node, err := dc.QuerySelector(ctx, selector)
	if err != nil {
		return err
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return dom.ScrollIntoViewIfNeeded().
				WithNodeID(node.NodeID).
				Do(ctx)
		}),
	)
}

// Focus focuses an element
func (dc *DOMController) Focus(ctx context.Context, selector string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	node, err := dc.QuerySelector(ctx, selector)
	if err != nil {
		return err
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return dom.Focus().WithNodeID(node.NodeID).Do(ctx)
		}),
	)
}