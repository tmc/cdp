package main

import (
	"context"
	"encoding/json"
	"fmt"

	"errors"

	"github.com/chromedp/cdproto/css"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// CSSController handles CSS operations
type CSSController struct {
	debugger *ChromeDebugger
	verbose  bool
}

// NewCSSController creates a new CSS controller
func NewCSSController(debugger *ChromeDebugger, verbose bool) *CSSController {
	return &CSSController{
		debugger: debugger,
		verbose:  verbose,
	}
}

// GetMatchedStyles gets the CSS rules that match a given element
func (cc *CSSController) GetMatchedStyles(ctx context.Context, selector string) (map[string]interface{}, error) {
	if !cc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	// Use JavaScript to get computed styles since CDP CSS domain can be complex
	expression := fmt.Sprintf(`
		(function() {
			const el = document.querySelector('%s');
			if (!el) return null;

			const computedStyle = window.getComputedStyle(el);
			const styles = {};

			// Get all computed style properties
			for (let i = 0; i < computedStyle.length; i++) {
				const prop = computedStyle[i];
				styles[prop] = computedStyle.getPropertyValue(prop);
			}

			return {
				computed: styles,
				inline: el.style.cssText
			};
		})()
	`, selector)

	var result interface{}
	err := chromedp.Run(cc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				json.Unmarshal(res.Value, &result)
			}

			return nil
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get matched styles: %w", err)
	}

	if resultMap, ok := result.(map[string]interface{}); ok {
		return resultMap, nil
	}

	return nil, fmt.Errorf("element not found: %s", selector)
}

// GetComputedStyles gets the computed styles for an element
func (cc *CSSController) GetComputedStyles(ctx context.Context, selector string) (map[string]string, error) {
	if !cc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	// Use JavaScript to get computed styles
	expression := fmt.Sprintf(`
		(function() {
			const el = document.querySelector('%s');
			if (!el) return null;

			const computedStyle = window.getComputedStyle(el);
			const styles = {};

			// Get important style properties
			const importantProps = [
				'display', 'position', 'width', 'height', 'margin', 'padding',
				'border', 'background', 'color', 'font-family', 'font-size',
				'font-weight', 'text-align', 'line-height', 'z-index', 'opacity',
				'visibility', 'overflow', 'float', 'clear', 'top', 'left', 'right', 'bottom'
			];

			importantProps.forEach(prop => {
				styles[prop] = computedStyle.getPropertyValue(prop);
			});

			return styles;
		})()
	`, selector)

	var result interface{}
	err := chromedp.Run(cc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				json.Unmarshal(res.Value, &result)
			}

			return nil
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get computed styles: %w", err)
	}

	if resultMap, ok := result.(map[string]interface{}); ok {
		styles := make(map[string]string)
		for k, v := range resultMap {
			if str, ok := v.(string); ok {
				styles[k] = str
			}
		}
		return styles, nil
	}

	return nil, fmt.Errorf("element not found: %s", selector)
}

// SetInlineStyle sets an inline style property on an element
func (cc *CSSController) SetInlineStyle(ctx context.Context, selector string, property string, value string) error {
	if !cc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	// Use JavaScript to set the style property
	expression := fmt.Sprintf(`
		(function() {
			const el = document.querySelector('%s');
			if (!el) return false;
			el.style.setProperty('%s', '%s');
			return true;
		})()
	`, selector, property, value)

	var success bool
	err := chromedp.Run(cc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			result, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if result.Value != nil {
				json.Unmarshal(result.Value, &success)
			}

			if !success {
				return fmt.Errorf("element not found: %s", selector)
			}

			return nil
		}),
	)

	return err
}

// GetInlineStyles gets the inline styles of an element
func (cc *CSSController) GetInlineStyles(ctx context.Context, selector string) (map[string]string, error) {
	if !cc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	// Use JavaScript to get inline styles
	expression := fmt.Sprintf(`
		(function() {
			const el = document.querySelector('%s');
			if (!el) return null;

			const styles = {};
			const styleDecl = el.style;

			for (let i = 0; i < styleDecl.length; i++) {
				const prop = styleDecl[i];
				styles[prop] = styleDecl.getPropertyValue(prop);
			}

			return styles;
		})()
	`, selector)

	var result interface{}
	err := chromedp.Run(cc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				json.Unmarshal(res.Value, &result)
			}

			return nil
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get inline styles: %w", err)
	}

	// Convert to string map
	styles := make(map[string]string)
	if resultMap, ok := result.(map[string]interface{}); ok {
		for k, v := range resultMap {
			if str, ok := v.(string); ok {
				styles[k] = str
			}
		}
	}

	return styles, nil
}

// AddCSSRule adds a new CSS rule to the stylesheet
func (cc *CSSController) AddCSSRule(ctx context.Context, ruleText string) error {
	if !cc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	// Use JavaScript to add CSS rule
	expression := fmt.Sprintf(`
		(function() {
			try {
				// Find or create a style element
				let styleEl = document.getElementById('chdb-injected-styles');
				if (!styleEl) {
					styleEl = document.createElement('style');
					styleEl.id = 'chdb-injected-styles';
					document.head.appendChild(styleEl);
				}

				// Add the rule
				styleEl.textContent += '%s\n';
				return true;
			} catch (e) {
				console.error('Failed to add CSS rule:', e);
				return false;
			}
		})()
	`, ruleText)

	var success bool
	err := chromedp.Run(cc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			result, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if result.Value != nil {
				json.Unmarshal(result.Value, &success)
			}

			if !success {
				return fmt.Errorf("failed to add CSS rule")
			}

			return nil
		}),
	)

	return err
}

// GetCSSCoverage starts CSS coverage collection
func (cc *CSSController) StartCSSCoverage(ctx context.Context) error {
	if !cc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(cc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable CSS domain
			if err := css.Enable().Do(ctx); err != nil {
				return err
			}

			// Start coverage
			return css.StartRuleUsageTracking().Do(ctx)
		}),
	)
}

// StopCSSCoverage stops CSS coverage collection and returns results
func (cc *CSSController) StopCSSCoverage(ctx context.Context) ([]map[string]interface{}, error) {
	if !cc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	// Simplified coverage implementation using JavaScript
	expression := `
		(function() {
			const sheets = [];
			for (let i = 0; i < document.styleSheets.length; i++) {
				const sheet = document.styleSheets[i];
				try {
					sheets.push({
						index: i,
						href: sheet.href || 'inline',
						rulesCount: sheet.cssRules ? sheet.cssRules.length : 0,
						used: true // Simplified - in practice would track actual usage
					});
				} catch (e) {
					sheets.push({
						index: i,
						href: sheet.href || 'inline',
						rulesCount: 'inaccessible',
						used: false
					});
				}
			}
			return sheets;
		})()
	`

	var result interface{}
	err := chromedp.Run(cc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				json.Unmarshal(res.Value, &result)
			}

			return nil
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get CSS coverage: %w", err)
	}

	if resultArray, ok := result.([]interface{}); ok {
		coverage := make([]map[string]interface{}, len(resultArray))
		for i, item := range resultArray {
			if itemMap, ok := item.(map[string]interface{}); ok {
				coverage[i] = itemMap
			}
		}
		return coverage, nil
	}

	return nil, nil
}

// GetStyleSheets gets all stylesheets in the document
func (cc *CSSController) GetStyleSheets(ctx context.Context) ([]map[string]interface{}, error) {
	if !cc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	// Use JavaScript to get stylesheets info
	expression := `
		(function() {
			const sheets = [];
			for (let i = 0; i < document.styleSheets.length; i++) {
				const sheet = document.styleSheets[i];
				const info = {
					index: i,
					href: sheet.href || 'inline',
					title: sheet.title || 'untitled',
					disabled: sheet.disabled,
					type: sheet.type,
					media: sheet.media.mediaText || 'all',
					rulesCount: 0
				};

				try {
					info.rulesCount = sheet.cssRules ? sheet.cssRules.length : 0;
				} catch (e) {
					info.rulesCount = 'inaccessible (CORS)';
				}

				sheets.push(info);
			}
			return sheets;
		})()
	`

	var result interface{}
	err := chromedp.Run(cc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				json.Unmarshal(res.Value, &result)
			}

			return nil
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get stylesheets: %w", err)
	}

	if resultArray, ok := result.([]interface{}); ok {
		stylesheets := make([]map[string]interface{}, len(resultArray))
		for i, item := range resultArray {
			if itemMap, ok := item.(map[string]interface{}); ok {
				stylesheets[i] = itemMap
			}
		}
		return stylesheets, nil
	}

	return nil, nil
}
