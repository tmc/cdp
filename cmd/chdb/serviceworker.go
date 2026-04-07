package main

import (
	"context"
	"encoding/json"
	"fmt"

	"errors"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/serviceworker"
	"github.com/chromedp/chromedp"
)

// ServiceWorkerController handles service worker operations
type ServiceWorkerController struct {
	debugger *ChromeDebugger
	verbose  bool
}

// NewServiceWorkerController creates a new service worker controller
func NewServiceWorkerController(debugger *ChromeDebugger, verbose bool) *ServiceWorkerController {
	return &ServiceWorkerController{
		debugger: debugger,
		verbose:  verbose,
	}
}

// ListServiceWorkers gets all registered service workers
func (swc *ServiceWorkerController) ListServiceWorkers(ctx context.Context) ([]map[string]interface{}, error) {
	if !swc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var workers []map[string]interface{}
	err := chromedp.Run(swc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable service worker domain
			if err := serviceworker.Enable().Do(ctx); err != nil {
				return err
			}

			// Use JavaScript to get service worker info since CDP can be complex
			expression := `
				(async function() {
					try {
						if ('serviceWorker' in navigator) {
							const registrations = await navigator.serviceWorker.getRegistrations();
							return await Promise.all(registrations.map(async reg => {
								const sw = reg.active || reg.installing || reg.waiting;
								return {
									scope: reg.scope,
									scriptURL: sw ? sw.scriptURL : 'none',
									state: sw ? sw.state : 'none',
									updateViaCache: reg.updateViaCache,
									navigationPreload: reg.navigationPreload ?
										await reg.navigationPreload.getState() : null
								};
							}));
						}
						return [];
					} catch (e) {
						return [{ error: e.message }];
					}
				})()
			`

			res, _, err := runtime.Evaluate(expression).WithAwaitPromise(true).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				var result interface{}
				if json.Unmarshal(res.Value, &result) == nil {
					if workerArray, ok := result.([]interface{}); ok {
						for _, worker := range workerArray {
							if workerMap, ok := worker.(map[string]interface{}); ok {
								workers = append(workers, workerMap)
							}
						}
					}
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to list service workers: %w", err)
	}

	return workers, nil
}

// InspectServiceWorker gets detailed information about a specific service worker
func (swc *ServiceWorkerController) InspectServiceWorker(ctx context.Context, scope string) (map[string]interface{}, error) {
	if !swc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var workerInfo map[string]interface{}
	err := chromedp.Run(swc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := `
				(async function() {
					try {
						if ('serviceWorker' in navigator) {
							const registrations = await navigator.serviceWorker.getRegistrations();
							const reg = registrations.find(r => r.scope === '` + scope + `');
							if (!reg) return { error: 'Service worker not found for scope: ` + scope + `' };

							const sw = reg.active || reg.installing || reg.waiting;
							const info = {
								scope: reg.scope,
								scriptURL: sw ? sw.scriptURL : 'none',
								state: sw ? sw.state : 'none',
								updateViaCache: reg.updateViaCache,
								navigationPreload: null,
								caches: []
							};

							// Get navigation preload state
							if (reg.navigationPreload) {
								try {
									info.navigationPreload = await reg.navigationPreload.getState();
								} catch (e) {
									info.navigationPreload = { error: e.message };
								}
							}

							// Get associated caches
							try {
								if ('caches' in window) {
									const cacheNames = await caches.keys();
									const swCaches = [];
									for (const cacheName of cacheNames) {
										if (cacheName.includes(new URL(reg.scope).hostname)) {
											const cache = await caches.open(cacheName);
											const keys = await cache.keys();
											swCaches.push({
												name: cacheName,
												requestCount: keys.length,
												sampleRequests: keys.slice(0, 3).map(req => req.url)
											});
										}
									}
									info.caches = swCaches;
								}
							} catch (e) {
								info.caches = [{ error: e.message }];
							}

							return info;
						}
						return { error: 'Service workers not supported' };
					} catch (e) {
						return { error: e.message };
					}
				})()
			`

			res, _, err := runtime.Evaluate(expression).WithAwaitPromise(true).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				var result interface{}
				if json.Unmarshal(res.Value, &result) == nil {
					if resultMap, ok := result.(map[string]interface{}); ok {
						workerInfo = resultMap
					}
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to inspect service worker: %w", err)
	}

	return workerInfo, nil
}

// UnregisterServiceWorker unregisters a service worker by scope
func (swc *ServiceWorkerController) UnregisterServiceWorker(ctx context.Context, scope string) error {
	if !swc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(swc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := `
				(async function() {
					try {
						if ('serviceWorker' in navigator) {
							const registrations = await navigator.serviceWorker.getRegistrations();
							const reg = registrations.find(r => r.scope === '` + scope + `');
							if (reg) {
								const result = await reg.unregister();
								return { success: result, scope: '` + scope + `' };
							} else {
								return { error: 'Service worker not found for scope: ` + scope + `' };
							}
						}
						return { error: 'Service workers not supported' };
					} catch (e) {
						return { error: e.message };
					}
				})()
			`

			res, _, err := runtime.Evaluate(expression).WithAwaitPromise(true).Do(ctx)
			if err != nil {
				return err
			}

			var result interface{}
			if res.Value != nil {
				json.Unmarshal(res.Value, &result)
			}

			// Check if there was an error
			if resultMap, ok := result.(map[string]interface{}); ok {
				if errorMsg, hasError := resultMap["error"]; hasError {
					return errors.New(errorMsg.(string))
				}
			}

			return nil
		}),
	)
}

// UpdateServiceWorker forces an update check for a service worker
func (swc *ServiceWorkerController) UpdateServiceWorker(ctx context.Context, scope string) error {
	if !swc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(swc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := `
				(async function() {
					try {
						if ('serviceWorker' in navigator) {
							const registrations = await navigator.serviceWorker.getRegistrations();
							const reg = registrations.find(r => r.scope === '` + scope + `');
							if (reg) {
								await reg.update();
								return { success: true, scope: '` + scope + `' };
							} else {
								return { error: 'Service worker not found for scope: ` + scope + `' };
							}
						}
						return { error: 'Service workers not supported' };
					} catch (e) {
						return { error: e.message };
					}
				})()
			`

			res, _, err := runtime.Evaluate(expression).WithAwaitPromise(true).Do(ctx)
			if err != nil {
				return err
			}

			var result interface{}
			if res.Value != nil {
				json.Unmarshal(res.Value, &result)
			}

			// Check if there was an error
			if resultMap, ok := result.(map[string]interface{}); ok {
				if errorMsg, hasError := resultMap["error"]; hasError {
					return errors.New(errorMsg.(string))
				}
			}

			return nil
		}),
	)
}

// GetServiceWorkerCaches gets caches associated with service workers
func (swc *ServiceWorkerController) GetServiceWorkerCaches(ctx context.Context) ([]map[string]interface{}, error) {
	if !swc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var caches []map[string]interface{}
	err := chromedp.Run(swc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := `
				(async function() {
					try {
						if ('caches' in window && 'serviceWorker' in navigator) {
							const [cacheNames, registrations] = await Promise.all([
								caches.keys(),
								navigator.serviceWorker.getRegistrations()
							]);

							const cacheInfo = await Promise.all(
								cacheNames.map(async name => {
									const cache = await caches.open(name);
									const keys = await cache.keys();

									// Try to associate with service worker
									let associatedSW = null;
									for (const reg of registrations) {
										const swUrl = (reg.active || reg.installing || reg.waiting)?.scriptURL;
										if (swUrl && name.includes(new URL(reg.scope).hostname)) {
											associatedSW = reg.scope;
											break;
										}
									}

									return {
										name: name,
										associatedServiceWorker: associatedSW,
										requestCount: keys.length,
										requests: keys.slice(0, 5).map(req => ({
											url: req.url,
											method: req.method
										}))
									};
								})
							);

							return cacheInfo;
						}
						return [];
					} catch (e) {
						return [{ error: e.message }];
					}
				})()
			`

			res, _, err := runtime.Evaluate(expression).WithAwaitPromise(true).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				var result interface{}
				if json.Unmarshal(res.Value, &result) == nil {
					if cacheArray, ok := result.([]interface{}); ok {
						for _, cache := range cacheArray {
							if cacheMap, ok := cache.(map[string]interface{}); ok {
								caches = append(caches, cacheMap)
							}
						}
					}
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get service worker caches: %w", err)
	}

	return caches, nil
}

// PostMessageToServiceWorker sends a message to a service worker
func (swc *ServiceWorkerController) PostMessageToServiceWorker(ctx context.Context, scope string, message string) error {
	if !swc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(swc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Escape the message for JavaScript
			messageJSON, _ := json.Marshal(message)

			expression := `
				(async function() {
					try {
						if ('serviceWorker' in navigator) {
							const registrations = await navigator.serviceWorker.getRegistrations();
							const reg = registrations.find(r => r.scope === '` + scope + `');
							if (reg && reg.active) {
								reg.active.postMessage(` + string(messageJSON) + `);
								return { success: true, scope: '` + scope + `' };
							} else {
								return { error: 'Active service worker not found for scope: ` + scope + `' };
							}
						}
						return { error: 'Service workers not supported' };
					} catch (e) {
						return { error: e.message };
					}
				})()
			`

			res, _, err := runtime.Evaluate(expression).WithAwaitPromise(true).Do(ctx)
			if err != nil {
				return err
			}

			var result interface{}
			if res.Value != nil {
				json.Unmarshal(res.Value, &result)
			}

			// Check if there was an error
			if resultMap, ok := result.(map[string]interface{}); ok {
				if errorMsg, hasError := resultMap["error"]; hasError {
					return errors.New(errorMsg.(string))
				}
			}

			return nil
		}),
	)
}
