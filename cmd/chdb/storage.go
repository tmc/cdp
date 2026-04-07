package main

import (
	"context"
	"encoding/json"
	"fmt"

	"errors"

	"github.com/chromedp/cdproto/indexeddb"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// StorageController handles storage operations
type StorageController struct {
	debugger *ChromeDebugger
	verbose  bool
}

// NewStorageController creates a new storage controller
func NewStorageController(debugger *ChromeDebugger, verbose bool) *StorageController {
	return &StorageController{
		debugger: debugger,
		verbose:  verbose,
	}
}

// GetLocalStorage gets all localStorage items
func (sc *StorageController) GetLocalStorage(ctx context.Context) (map[string]string, error) {
	if !sc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	// Use JavaScript to get localStorage since it's more reliable
	expression := `
		(function() {
			const items = {};
			for (let i = 0; i < localStorage.length; i++) {
				const key = localStorage.key(i);
				items[key] = localStorage.getItem(key);
			}
			return items;
		})()
	`

	var result interface{}
	err := chromedp.Run(sc.debugger.chromeCtx,
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
		return nil, fmt.Errorf("failed to get localStorage: %w", err)
	}

	// Convert to string map
	items := make(map[string]string)
	if resultMap, ok := result.(map[string]interface{}); ok {
		for k, v := range resultMap {
			if str, ok := v.(string); ok {
				items[k] = str
			}
		}
	}

	return items, nil
}

// SetLocalStorage sets a localStorage item
func (sc *StorageController) SetLocalStorage(ctx context.Context, key string, value string) error {
	if !sc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	expression := fmt.Sprintf(`localStorage.setItem(%s, %s)`,
		jsonString(key), jsonString(value))

	return chromedp.Run(sc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}

// RemoveLocalStorage removes a localStorage item
func (sc *StorageController) RemoveLocalStorage(ctx context.Context, key string) error {
	if !sc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	expression := fmt.Sprintf(`localStorage.removeItem(%s)`, jsonString(key))

	return chromedp.Run(sc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}

// ClearLocalStorage clears all localStorage
func (sc *StorageController) ClearLocalStorage(ctx context.Context) error {
	if !sc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(sc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, err := runtime.Evaluate("localStorage.clear()").Do(ctx)
			return err
		}),
	)
}

// GetSessionStorage gets all sessionStorage items
func (sc *StorageController) GetSessionStorage(ctx context.Context) (map[string]string, error) {
	if !sc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	expression := `
		(function() {
			const items = {};
			for (let i = 0; i < sessionStorage.length; i++) {
				const key = sessionStorage.key(i);
				items[key] = sessionStorage.getItem(key);
			}
			return items;
		})()
	`

	var result interface{}
	err := chromedp.Run(sc.debugger.chromeCtx,
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
		return nil, fmt.Errorf("failed to get sessionStorage: %w", err)
	}

	// Convert to string map
	items := make(map[string]string)
	if resultMap, ok := result.(map[string]interface{}); ok {
		for k, v := range resultMap {
			if str, ok := v.(string); ok {
				items[k] = str
			}
		}
	}

	return items, nil
}

// SetSessionStorage sets a sessionStorage item
func (sc *StorageController) SetSessionStorage(ctx context.Context, key string, value string) error {
	if !sc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	expression := fmt.Sprintf(`sessionStorage.setItem(%s, %s)`,
		jsonString(key), jsonString(value))

	return chromedp.Run(sc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}

// GetCookies gets all cookies for the current domain
func (sc *StorageController) GetCookies(ctx context.Context) ([]map[string]interface{}, error) {
	if !sc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var cookies []map[string]interface{}
	err := chromedp.Run(sc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Storage domain not needed for JavaScript cookie access

			// Get cookies using JavaScript since CDP cookies can be complex
			expression := `document.cookie.split(';').map(c => {
				const [name, value] = c.trim().split('=');
				return { name: name, value: value || '' };
			}).filter(c => c.name)`

			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				var result interface{}
				if json.Unmarshal(res.Value, &result) == nil {
					if cookieArray, ok := result.([]interface{}); ok {
						for _, cookie := range cookieArray {
							if cookieMap, ok := cookie.(map[string]interface{}); ok {
								cookies = append(cookies, cookieMap)
							}
						}
					}
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get cookies: %w", err)
	}

	return cookies, nil
}

// SetCookie sets a cookie
func (sc *StorageController) SetCookie(ctx context.Context, name string, value string, options map[string]interface{}) error {
	if !sc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	// Build cookie string
	cookieStr := fmt.Sprintf("%s=%s", name, value)

	// Add options
	if path, ok := options["path"].(string); ok {
		cookieStr += fmt.Sprintf("; path=%s", path)
	}
	if domain, ok := options["domain"].(string); ok {
		cookieStr += fmt.Sprintf("; domain=%s", domain)
	}
	if maxAge, ok := options["maxAge"].(int); ok {
		cookieStr += fmt.Sprintf("; max-age=%d", maxAge)
	}
	if secure, ok := options["secure"].(bool); ok && secure {
		cookieStr += "; secure"
	}
	if httpOnly, ok := options["httpOnly"].(bool); ok && httpOnly {
		cookieStr += "; httponly"
	}

	expression := fmt.Sprintf(`document.cookie = %s`, jsonString(cookieStr))

	return chromedp.Run(sc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}

// GetIndexedDBDatabases gets list of IndexedDB databases
func (sc *StorageController) GetIndexedDBDatabases(ctx context.Context) ([]map[string]interface{}, error) {
	if !sc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var databases []map[string]interface{}
	err := chromedp.Run(sc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable IndexedDB domain
			if err := indexeddb.Enable().Do(ctx); err != nil {
				return err
			}

			// Use JavaScript to get databases since CDP IndexedDB can be complex
			expression := `
				(async function() {
					try {
						if ('indexedDB' in window) {
							const databases = await indexedDB.databases();
							return databases.map(db => ({
								name: db.name,
								version: db.version
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
					if dbArray, ok := result.([]interface{}); ok {
						for _, db := range dbArray {
							if dbMap, ok := db.(map[string]interface{}); ok {
								databases = append(databases, dbMap)
							}
						}
					}
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get IndexedDB databases: %w", err)
	}

	return databases, nil
}

// ClearIndexedDB clears all IndexedDB databases
func (sc *StorageController) ClearIndexedDB(ctx context.Context) error {
	if !sc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	expression := `
		(async function() {
			try {
				if ('indexedDB' in window) {
					const databases = await indexedDB.databases();
					const promises = databases.map(db =>
						new Promise((resolve) => {
							const deleteReq = indexedDB.deleteDatabase(db.name);
							deleteReq.onsuccess = () => resolve(db.name);
							deleteReq.onerror = () => resolve('Error deleting ' + db.name);
						})
					);
					const results = await Promise.all(promises);
					return { deleted: results };
				}
				return { message: 'IndexedDB not available' };
			} catch (e) {
				return { error: e.message };
			}
		})()
	`

	return chromedp.Run(sc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, err := runtime.Evaluate(expression).WithAwaitPromise(true).Do(ctx)
			return err
		}),
	)
}

// GetCacheStorage gets cache storage information
func (sc *StorageController) GetCacheStorage(ctx context.Context) ([]map[string]interface{}, error) {
	if !sc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var caches []map[string]interface{}
	err := chromedp.Run(sc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := `
				(async function() {
					try {
						if ('caches' in window) {
							const cacheNames = await caches.keys();
							const cacheInfo = await Promise.all(
								cacheNames.map(async name => {
									const cache = await caches.open(name);
									const keys = await cache.keys();
									return {
										name: name,
										requestCount: keys.length,
										requests: keys.slice(0, 5).map(req => req.url) // First 5 URLs
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
		return nil, fmt.Errorf("failed to get cache storage: %w", err)
	}

	return caches, nil
}

// ClearAllStorage clears all storage types
func (sc *StorageController) ClearAllStorage(ctx context.Context) error {
	if !sc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	expression := `
		(async function() {
			const results = [];

			// Clear localStorage
			try {
				localStorage.clear();
				results.push('localStorage cleared');
			} catch (e) {
				results.push('localStorage error: ' + e.message);
			}

			// Clear sessionStorage
			try {
				sessionStorage.clear();
				results.push('sessionStorage cleared');
			} catch (e) {
				results.push('sessionStorage error: ' + e.message);
			}

			// Clear cookies (best effort)
			try {
				document.cookie.split(";").forEach(c => {
					const eqPos = c.indexOf("=");
					const name = eqPos > -1 ? c.substr(0, eqPos).trim() : c.trim();
					if (name) {
						document.cookie = name + "=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/";
					}
				});
				results.push('cookies cleared');
			} catch (e) {
				results.push('cookies error: ' + e.message);
			}

			// Clear IndexedDB
			try {
				if ('indexedDB' in window) {
					const databases = await indexedDB.databases();
					await Promise.all(databases.map(db =>
						new Promise(resolve => {
							const deleteReq = indexedDB.deleteDatabase(db.name);
							deleteReq.onsuccess = deleteReq.onerror = () => resolve();
						})
					));
					results.push('IndexedDB cleared');
				}
			} catch (e) {
				results.push('IndexedDB error: ' + e.message);
			}

			// Clear cache storage
			try {
				if ('caches' in window) {
					const cacheNames = await caches.keys();
					await Promise.all(cacheNames.map(name => caches.delete(name)));
					results.push('Cache storage cleared');
				}
			} catch (e) {
				results.push('Cache storage error: ' + e.message);
			}

			return results;
		})()
	`

	return chromedp.Run(sc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, err := runtime.Evaluate(expression).WithAwaitPromise(true).Do(ctx)
			return err
		}),
	)
}

// Helper function to create JSON string
func jsonString(s string) string {
	bytes, _ := json.Marshal(s)
	return string(bytes)
}
