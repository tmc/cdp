package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/cdproto/layertree"
	"github.com/chromedp/cdproto/overlay"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pkg/errors"
)

// RenderingController handles rendering and paint debugging
type RenderingController struct {
	debugger *ChromeDebugger
	verbose  bool
}

// NewRenderingController creates a new rendering controller
func NewRenderingController(debugger *ChromeDebugger, verbose bool) *RenderingController {
	return &RenderingController{
		debugger: debugger,
		verbose:  verbose,
	}
}

// EnablePaintFlashing enables paint flashing to visualize repaints
func (rc *RenderingController) EnablePaintFlashing(ctx context.Context, enabled bool) error {
	if !rc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(rc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Use JavaScript to enable paint flashing since overlay domain might not be available
			expression := fmt.Sprintf(`
				(function() {
					// Try to use Chrome DevTools API if available
					if (window.chrome && window.chrome.runtime) {
						chrome.runtime.sendMessage({
							type: 'togglePaintFlashing',
							enabled: %t
						});
						return { success: true, method: 'chrome-api' };
					}

					// Fallback: Add CSS animation to highlight repaints
					if (%t) {
						const style = document.createElement('style');
						style.id = 'paint-flash-style';
						style.textContent = '@keyframes paint-flash { 0%% { background-color: rgba(255, 0, 0, 0.5) !important; } 100%% { background-color: transparent !important; } } .paint-flash { animation: paint-flash 0.5s ease-out; }';
						document.head.appendChild(style);

						// Monitor DOM changes to add flash effect
						const observer = new MutationObserver((mutations) => {
							mutations.forEach((mutation) => {
								if (mutation.type === 'childList' || mutation.type === 'attributes') {
									const element = mutation.target;
									if (element.nodeType === Node.ELEMENT_NODE) {
										element.classList.add('paint-flash');
										setTimeout(() => {
											element.classList.remove('paint-flash');
										}, 500);
									}
								}
							});
						});

						observer.observe(document.body, {
							childList: true,
							subtree: true,
							attributes: true,
							attributeOldValue: true
						});

						window.paintFlashObserver = observer;
						return { success: true, method: 'css-fallback' };
					} else {
						// Disable paint flashing
						const style = document.getElementById('paint-flash-style');
						if (style) style.remove();

						if (window.paintFlashObserver) {
							window.paintFlashObserver.disconnect();
							delete window.paintFlashObserver;
						}
						return { success: true, method: 'disabled' };
					}
				})()
			`, enabled, enabled)

			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}

// ShowLayerBorders enables layer border visualization
func (rc *RenderingController) ShowLayerBorders(ctx context.Context, enabled bool) error {
	if !rc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(rc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := fmt.Sprintf(`
				(function() {
					if (%t) {
						const style = document.createElement('style');
						style.id = 'layer-borders-style';
						style.textContent = '* { outline: 1px solid rgba(0, 255, 0, 0.8) !important; outline-offset: -1px !important; } *:hover { outline: 2px solid rgba(255, 255, 0, 0.8) !important; }';
						document.head.appendChild(style);
						return { success: true, enabled: true };
					} else {
						const style = document.getElementById('layer-borders-style');
						if (style) style.remove();
						return { success: true, enabled: false };
					}
				})()
			`, enabled)

			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}

// GetCompositingReasons gets compositing reasons for elements
func (rc *RenderingController) GetCompositingReasons(ctx context.Context, selector string) (map[string]interface{}, error) {
	if !rc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var compositingInfo map[string]interface{}
	err := chromedp.Run(rc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := fmt.Sprintf(`
				(function() {
					const element = document.querySelector('%s');
					if (!element) {
						return { error: 'Element not found' };
					}

					const computedStyle = window.getComputedStyle(element);
					const rect = element.getBoundingClientRect();

					// Check various compositing triggers
					const compositingTriggers = {
						transform3d: computedStyle.transform !== 'none' && computedStyle.transform.includes('3d'),
						opacity: parseFloat(computedStyle.opacity) < 1,
						position: computedStyle.position === 'fixed',
						willChange: computedStyle.willChange !== 'auto',
						filter: computedStyle.filter !== 'none',
						backdropFilter: computedStyle.backdropFilter !== 'none',
						mixBlendMode: computedStyle.mixBlendMode !== 'normal',
						isolation: computedStyle.isolation === 'isolate',
						overflow: computedStyle.overflow === 'hidden' || computedStyle.overflowX === 'hidden' || computedStyle.overflowY === 'hidden',
						zIndex: computedStyle.zIndex !== 'auto',
						video: element.tagName === 'VIDEO',
						canvas: element.tagName === 'CANVAS',
						plugin: element.tagName === 'EMBED' || element.tagName === 'OBJECT'
					};

					// Get layer information if available
					const layerInfo = {
						hasOwnLayer: false,
						layerType: 'unknown',
						gpuAccelerated: false
					};

					// Try to detect GPU acceleration
					try {
						const canvas = document.createElement('canvas');
						const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
						if (gl) {
							layerInfo.gpuAccelerated = true;
						}
					} catch (e) {
						// GPU detection failed
					}

					return {
						selector: '%s',
						element: {
							tagName: element.tagName,
							id: element.id,
							className: element.className
						},
						boundingRect: {
							x: rect.x,
							y: rect.y,
							width: rect.width,
							height: rect.height
						},
						compositingTriggers: compositingTriggers,
						layerInfo: layerInfo,
						computedStyle: {
							transform: computedStyle.transform,
							opacity: computedStyle.opacity,
							position: computedStyle.position,
							willChange: computedStyle.willChange,
							filter: computedStyle.filter,
							zIndex: computedStyle.zIndex,
							isolation: computedStyle.isolation,
							mixBlendMode: computedStyle.mixBlendMode
						}
					};
				})()
			`, selector, selector)

			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				var result interface{}
				if json.Unmarshal(res.Value, &result) == nil {
					if resultMap, ok := result.(map[string]interface{}); ok {
						compositingInfo = resultMap
					}
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to get compositing reasons")
	}

	return compositingInfo, nil
}

// GetLayerTree gets the layer tree information
func (rc *RenderingController) GetLayerTree(ctx context.Context) ([]map[string]interface{}, error) {
	if !rc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var layers []map[string]interface{}
	err := chromedp.Run(rc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable layer tree domain
			if err := layertree.Enable().Do(ctx); err != nil {
				// If layertree domain is not available, use JavaScript fallback
				expression := `
					(function() {
						// Collect information about potential layers
						const layers = [];
						const walker = document.createTreeWalker(
							document.body,
							NodeFilter.SHOW_ELEMENT,
							null,
							false
						);

						let node;
						while (node = walker.nextNode()) {
							const computedStyle = window.getComputedStyle(node);
							const rect = node.getBoundingClientRect();

							// Check if element likely creates a layer
							const hasLayer = (
								computedStyle.transform !== 'none' ||
								parseFloat(computedStyle.opacity) < 1 ||
								computedStyle.position === 'fixed' ||
								computedStyle.willChange !== 'auto' ||
								computedStyle.filter !== 'none' ||
								computedStyle.isolation === 'isolate' ||
								node.tagName === 'VIDEO' ||
								node.tagName === 'CANVAS'
							);

							if (hasLayer && rect.width > 0 && rect.height > 0) {
								layers.push({
									element: {
										tagName: node.tagName,
										id: node.id,
										className: node.className
									},
									bounds: {
										x: rect.x,
										y: rect.y,
										width: rect.width,
										height: rect.height
									},
									style: {
										transform: computedStyle.transform,
										opacity: computedStyle.opacity,
										position: computedStyle.position,
										zIndex: computedStyle.zIndex,
										willChange: computedStyle.willChange
									},
									estimated: true
								});
							}
						}

						return layers;
					})()
				`

				res, _, err := runtime.Evaluate(expression).Do(ctx)
				if err != nil {
					return err
				}

				if res.Value != nil {
					var result interface{}
					if json.Unmarshal(res.Value, &result) == nil {
						if layerArray, ok := result.([]interface{}); ok {
							for _, layer := range layerArray {
								if layerMap, ok := layer.(map[string]interface{}); ok {
									layers = append(layers, layerMap)
								}
							}
						}
					}
				}

				return nil
			}

			// LayerTree domain is complex and may not be available
			// Stick with JavaScript fallback approach which is more reliable

			return nil
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to get layer tree")
	}

	return layers, nil
}

// GetFrameRate gets current frame rate information
func (rc *RenderingController) GetFrameRate(ctx context.Context, duration int) (map[string]interface{}, error) {
	if !rc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var frameRateInfo map[string]interface{}
	err := chromedp.Run(rc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := fmt.Sprintf(`
				(async function() {
					const duration = %d; // milliseconds
					let frameCount = 0;
					let lastTimestamp = performance.now();
					const frameIntervals = [];
					const startTime = performance.now();

					return new Promise((resolve) => {
						function countFrame(timestamp) {
							if (frameCount > 0) {
								const interval = timestamp - lastTimestamp;
								frameIntervals.push(interval);
							}
							lastTimestamp = timestamp;
							frameCount++;

							if (timestamp - startTime < duration) {
								requestAnimationFrame(countFrame);
							} else {
								const totalTime = timestamp - startTime;
								const avgInterval = frameIntervals.reduce((a, b) => a + b, 0) / frameIntervals.length;
								const fps = frameCount / (totalTime / 1000);

								// Calculate frame time statistics
								frameIntervals.sort((a, b) => a - b);
								const median = frameIntervals[Math.floor(frameIntervals.length / 2)];
								const min = Math.min(...frameIntervals);
								const max = Math.max(...frameIntervals);
								const p95 = frameIntervals[Math.floor(frameIntervals.length * 0.95)];

								resolve({
									duration: totalTime,
									frameCount: frameCount,
									fps: fps,
									avgFrameTime: avgInterval,
									medianFrameTime: median,
									minFrameTime: min,
									maxFrameTime: max,
									p95FrameTime: p95,
									frameIntervals: frameIntervals.slice(0, 100), // Include first 100 intervals
									performanceNow: performance.now(),
									measurementMethod: 'requestAnimationFrame'
								});
							}
						}

						requestAnimationFrame(countFrame);
					});
				})()
			`, duration)

			res, _, err := runtime.Evaluate(expression).WithAwaitPromise(true).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				var result interface{}
				if json.Unmarshal(res.Value, &result) == nil {
					if resultMap, ok := result.(map[string]interface{}); ok {
						frameRateInfo = resultMap
					}
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to get frame rate")
	}

	return frameRateInfo, nil
}

// HighlightElement highlights an element on the page
func (rc *RenderingController) HighlightElement(ctx context.Context, selector string) error {
	if !rc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(rc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Use JavaScript highlighting (more reliable than overlay domain)

			// Fallback to JavaScript highlighting
			expression := fmt.Sprintf(`
				(function() {
					const element = document.querySelector('%s');
					if (!element) {
						return { error: 'Element not found' };
					}

					// Remove any existing highlights
					const existingHighlight = document.getElementById('chdb-highlight');
					if (existingHighlight) {
						existingHighlight.remove();
					}

					// Create highlight overlay
					const highlight = document.createElement('div');
					highlight.id = 'chdb-highlight';
					highlight.style.cssText = 'position: absolute; pointer-events: none; border: 3px solid #ff0000; background-color: rgba(255, 165, 0, 0.3); z-index: 10000; box-shadow: 0 0 10px rgba(255, 0, 0, 0.8);';

					const rect = element.getBoundingClientRect();
					highlight.style.left = (rect.left + window.scrollX) + 'px';
					highlight.style.top = (rect.top + window.scrollY) + 'px';
					highlight.style.width = rect.width + 'px';
					highlight.style.height = rect.height + 'px';

					document.body.appendChild(highlight);

					// Auto-remove after 3 seconds
					setTimeout(() => {
						if (highlight.parentNode) {
							highlight.parentNode.removeChild(highlight);
						}
					}, 3000);

					return { success: true, highlighted: true };
				})()
			`, selector)

			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}

// ClearHighlight clears element highlighting
func (rc *RenderingController) ClearHighlight(ctx context.Context) error {
	if !rc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(rc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Try overlay domain first
			if err := overlay.HideHighlight().Do(ctx); err == nil {
				return nil
			}

			// Fallback to JavaScript
			expression := `
				(function() {
					const highlight = document.getElementById('chdb-highlight');
					if (highlight) {
						highlight.remove();
						return { success: true, cleared: true };
					}
					return { success: true, cleared: false };
				})()
			`

			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}

// ShowScrollBottlenecks highlights scroll performance bottlenecks
func (rc *RenderingController) ShowScrollBottlenecks(ctx context.Context, enabled bool) error {
	if !rc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(rc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := fmt.Sprintf(`
				(function() {
					if (%t) {
						// Create scroll bottleneck detector
						const style = document.createElement('style');
						style.id = 'scroll-bottleneck-style';
						style.textContent = '.scroll-bottleneck { outline: 3px solid red !important; outline-offset: -3px !important; background: repeating-linear-gradient(45deg, rgba(255, 0, 0, 0.1), rgba(255, 0, 0, 0.1) 10px, rgba(255, 255, 0, 0.1) 10px, rgba(255, 255, 0, 0.1) 20px) !important; }';
						document.head.appendChild(style);

						// Detect potential scroll bottlenecks
						const elements = document.querySelectorAll('*');
						elements.forEach(el => {
							const style = window.getComputedStyle(el);
							const rect = el.getBoundingClientRect();

							// Check for potential bottlenecks
							const hasBottleneck = (
								// Large elements without proper layering
								(rect.width > 1000 || rect.height > 1000) &&
								style.transform === 'none' &&
								style.willChange === 'auto'
							) || (
								// Fixed positioned elements without transform
								style.position === 'fixed' &&
								style.transform === 'none'
							) || (
								// Elements with expensive filters
								style.filter !== 'none' &&
								style.willChange === 'auto'
							) || (
								// Large images without optimization
								el.tagName === 'IMG' &&
								rect.width > 500 &&
								!el.loading
							);

							if (hasBottleneck) {
								el.classList.add('scroll-bottleneck');
							}
						});

						return { success: true, enabled: true };
					} else {
						// Disable scroll bottleneck highlighting
						const style = document.getElementById('scroll-bottleneck-style');
						if (style) style.remove();

						document.querySelectorAll('.scroll-bottleneck').forEach(el => {
							el.classList.remove('scroll-bottleneck');
						});

						return { success: true, enabled: false };
					}
				})()
			`, enabled)

			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}