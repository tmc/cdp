package browser

import (
	"context"
	"encoding/json"
	"fmt"

	"errors"

	"github.com/chromedp/cdproto/input"
	"github.com/tmc/cdp/internal/chromedp"
)

// ElementHandle represents a handle to a DOM element
type ElementHandle struct {
	ctx      context.Context
	page     *Page
	selector string
	index    int
}

// QuerySelector finds the first element matching the selector
func (p *Page) QuerySelector(selector string) (*ElementHandle, error) {
	var found bool
	if err := chromedp.Run(p.ctx, chromedp.Evaluate(
		fmt.Sprintf(`document.querySelector(%s) !== null`, jsString(selector)),
		&found,
	)); err != nil {
		return nil, fmt.Errorf("querying selector %s: %w", selector, err)
	}

	if !found {
		return nil, nil // No element found
	}

	return &ElementHandle{
		ctx:      p.ctx,
		page:     p,
		selector: selector,
		index:    0,
	}, nil
}

// QuerySelectorAll finds all elements matching the selector
func (p *Page) QuerySelectorAll(selector string) ([]*ElementHandle, error) {
	var count int
	if err := chromedp.Run(p.ctx, chromedp.Evaluate(
		fmt.Sprintf(`document.querySelectorAll(%s).length`, jsString(selector)),
		&count,
	)); err != nil {
		return nil, fmt.Errorf("querying selector %s: %w", selector, err)
	}

	elements := make([]*ElementHandle, 0, count)
	for i := 0; i < count; i++ {
		elements = append(elements, &ElementHandle{
			ctx:      p.ctx,
			page:     p,
			selector: selector,
			index:    i,
		})
	}

	return elements, nil
}

func jsString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func (e *ElementHandle) elementExpr() string {
	return fmt.Sprintf(`document.querySelectorAll(%s)[%d]`, jsString(e.selector), e.index)
}

func (e *ElementHandle) evaluateElement(body string, result any) error {
	if e == nil {
		return errors.New("element is nil")
	}
	expr := fmt.Sprintf(`(() => {
		const el = %s;
		if (!el) throw new Error("element not found: " + %s);
		%s
	})()`, e.elementExpr(), jsString(e.selector), body)
	return chromedp.Run(e.ctx, chromedp.Evaluate(expr, result))
}

// Click clicks the element
func (e *ElementHandle) Click(opts ...ClickOption) error {
	options := &ClickOptions{
		Button: "left",
		Count:  1,
	}

	for _, opt := range opts {
		opt(options)
	}

	var ok bool
	return e.evaluateElement(`el.click(); return true;`, &ok)
}

// Type types text into the element
func (e *ElementHandle) Type(text string, opts ...TypeOption) error {
	options := &TypeOptions{}

	for _, opt := range opts {
		opt(options)
	}

	if err := e.Focus(); err != nil {
		return err
	}

	var ok bool
	return e.evaluateElement(fmt.Sprintf(`
		if ("value" in el) {
			el.value = %s;
			el.dispatchEvent(new Event("input", {bubbles: true}));
			el.dispatchEvent(new Event("change", {bubbles: true}));
		} else {
			el.textContent = %s;
		}
		return true;
	`, jsString(text), jsString(text)), &ok)
}

// Clear clears the element's value
func (e *ElementHandle) Clear() error {
	var ok bool
	return e.evaluateElement(`
		if ("value" in el) {
			el.value = "";
			el.dispatchEvent(new Event("input", {bubbles: true}));
			el.dispatchEvent(new Event("change", {bubbles: true}));
		} else {
			el.textContent = "";
		}
		return true;
	`, &ok)
}

// Focus focuses the element
func (e *ElementHandle) Focus() error {
	var ok bool
	return e.evaluateElement(`el.focus(); return true;`, &ok)
}

// GetText gets the text content
func (e *ElementHandle) GetText() (string, error) {
	var text string
	if err := e.evaluateElement(`return (el.innerText || el.textContent || "").trim();`, &text); err != nil {
		return "", fmt.Errorf("getting text: %w", err)
	}
	return text, nil
}

// GetAttribute gets an attribute value
func (e *ElementHandle) GetAttribute(name string) (string, error) {
	var value string
	if err := e.evaluateElement(fmt.Sprintf(`
		const name = %s;
		if (name === "value" && "value" in el) return el.value;
		return el.getAttribute(name) || "";
	`, jsString(name)), &value); err != nil {
		return "", fmt.Errorf("getting attribute: %w", err)
	}
	return value, nil
}

// SetAttribute sets an attribute value
func (e *ElementHandle) SetAttribute(name, value string) error {
	var ok bool
	return e.evaluateElement(fmt.Sprintf(`
		el.setAttribute(%s, %s);
		if (%s === "value" && "value" in el) el.value = %s;
		return true;
	`, jsString(name), jsString(value), jsString(name), jsString(value)), &ok)
}

// GetProperty gets a JavaScript property value
func (e *ElementHandle) GetProperty(property string) (interface{}, error) {
	var result interface{}
	if err := e.evaluateElement(fmt.Sprintf(`return el[%s];`, jsString(property)), &result); err != nil {
		return nil, fmt.Errorf("getting property: %w", err)
	}
	return result, nil
}

// IsVisible checks if the element is visible
func (e *ElementHandle) IsVisible() (bool, error) {
	var visible bool
	if err := e.evaluateElement(`
		const style = window.getComputedStyle(el);
		return style.display !== "none" &&
			style.visibility !== "hidden" &&
			style.opacity !== "0";
	`, &visible); err != nil {
		return false, fmt.Errorf("checking visibility: %w", err)
	}
	return visible, nil
}

// ScrollIntoView scrolls the element into view
func (e *ElementHandle) ScrollIntoView() error {
	var ok bool
	return e.evaluateElement(`el.scrollIntoView({behavior: "instant", block: "center"}); return true;`, &ok)
}

// Hover hovers over the element
func (e *ElementHandle) Hover() error {
	// Get element position
	box, err := e.GetBoundingBox()
	if err != nil {
		return err
	}

	// Move mouse to center of element
	centerX := box.X + box.Width/2
	centerY := box.Y + box.Height/2

	return chromedp.Run(e.ctx,
		chromedp.MouseEvent(input.MouseMoved, centerX, centerY),
	)
}

// GetBoundingBox gets the element's bounding box
func (e *ElementHandle) GetBoundingBox() (*BoundingBox, error) {
	var box BoundingBox
	if err := e.evaluateElement(`
		const rect = el.getBoundingClientRect();
		return {
			x: rect.x,
			y: rect.y,
			width: rect.width,
			height: rect.height
		};
	`, &box); err != nil {
		return nil, fmt.Errorf("getting bounding box: %w", err)
	}
	if box.Width == 0 && box.Height == 0 {
		return nil, errors.New("no bounding box returned")
	}
	return &box, nil
}

// BoundingBox represents element dimensions
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Screenshot takes a screenshot of the element
func (e *ElementHandle) Screenshot(opts ...ScreenshotOption) ([]byte, error) {
	options := &ScreenshotOptions{
		Type:    "png",
		Quality: 90,
	}

	for _, opt := range opts {
		opt(options)
	}

	var buf []byte
	if e.index != 0 {
		return nil, errors.New("element screenshot supports first matching element only")
	}
	if err := chromedp.Run(e.ctx, chromedp.Screenshot(e.selector, &buf, chromedp.ByQuery)); err != nil {
		return nil, fmt.Errorf("taking element screenshot: %w", err)
	}

	return buf, nil
}

// WaitForSelector waits for a child selector within this element
func (e *ElementHandle) WaitForSelector(selector string, opts ...WaitOption) (*ElementHandle, error) {
	options := &WaitOptions{
		State:   "visible",
		Timeout: 30000,
	}

	for _, opt := range opts {
		opt(options)
	}

	if err := e.page.WaitForSelector(selector, opts...); err != nil {
		return nil, err
	}

	return e.page.QuerySelector(selector)
}

// Evaluate evaluates JavaScript in the context of this element
func (e *ElementHandle) Evaluate(expression string, result interface{}) error {
	return e.evaluateElement(fmt.Sprintf(`return (function() { return (%s); }).call(el);`, expression), result)
}
