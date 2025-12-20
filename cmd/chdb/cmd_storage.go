package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Storage commands
var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Storage management and inspection",
	Long:  "Manage localStorage, sessionStorage, cookies, IndexedDB, and cache storage",
}

var storageLocalCmd = &cobra.Command{
	Use:   "local <action> [key] [value]",
	Short: "LocalStorage operations (get, set, remove, clear)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		action := args[0]
		switch action {
		case "get":
			if err := getLocalStorage(ctx, tabID); err != nil {
				log.Fatalf("Failed to get localStorage: %v", err)
			}
		case "set":
			if len(args) < 3 {
				log.Fatalf("set requires key and value: storage local set <key> <value>")
			}
			if err := setLocalStorage(ctx, args[1], args[2], tabID); err != nil {
				log.Fatalf("Failed to set localStorage: %v", err)
			}
		case "remove":
			if len(args) < 2 {
				log.Fatalf("remove requires key: storage local remove <key>")
			}
			if err := removeLocalStorage(ctx, args[1], tabID); err != nil {
				log.Fatalf("Failed to remove localStorage: %v", err)
			}
		case "clear":
			if err := clearLocalStorage(ctx, tabID); err != nil {
				log.Fatalf("Failed to clear localStorage: %v", err)
			}
		default:
			log.Fatalf("Unknown action: %s (available: get, set, remove, clear)", action)
		}
	},
}

var storageSessionCmd = &cobra.Command{
	Use:   "session <action> [key] [value]",
	Short: "SessionStorage operations (get, set, remove, clear)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		action := args[0]
		switch action {
		case "get":
			if err := getSessionStorage(ctx, tabID); err != nil {
				log.Fatalf("Failed to get sessionStorage: %v", err)
			}
		case "set":
			if len(args) < 3 {
				log.Fatalf("set requires key and value: storage session set <key> <value>")
			}
			if err := setSessionStorage(ctx, args[1], args[2], tabID); err != nil {
				log.Fatalf("Failed to set sessionStorage: %v", err)
			}
		case "remove":
			if len(args) < 2 {
				log.Fatalf("remove requires key: storage session remove <key>")
			}
			if err := removeSessionStorage(ctx, args[1], tabID); err != nil {
				log.Fatalf("Failed to remove sessionStorage: %v", err)
			}
		case "clear":
			if err := clearSessionStorage(ctx, tabID); err != nil {
				log.Fatalf("Failed to clear sessionStorage: %v", err)
			}
		default:
			log.Fatalf("Unknown action: %s (available: get, set, remove, clear)", action)
		}
	},
}

var storageCookiesCmd = &cobra.Command{
	Use:   "cookies <action> [name] [value] [options...]",
	Short: "Cookie operations (get, set, remove)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		action := args[0]
		switch action {
		case "get":
			if err := getCookies(ctx, tabID); err != nil {
				log.Fatalf("Failed to get cookies: %v", err)
			}
		case "set":
			if len(args) < 3 {
				log.Fatalf("set requires name and value: storage cookies set <name> <value> [options...]")
			}
			options := parseCookieOptions(args[3:])
			if err := setCookie(ctx, args[1], args[2], options, tabID); err != nil {
				log.Fatalf("Failed to set cookie: %v", err)
			}
		case "remove":
			if len(args) < 2 {
				log.Fatalf("remove requires name: storage cookies remove <name>")
			}
			if err := removeCookie(ctx, args[1], tabID); err != nil {
				log.Fatalf("Failed to remove cookie: %v", err)
			}
		default:
			log.Fatalf("Unknown action: %s (available: get, set, remove)", action)
		}
	},
}

var storageIndexeddbCmd = &cobra.Command{
	Use:   "indexeddb <action>",
	Short: "IndexedDB operations (list, clear)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		action := args[0]
		switch action {
		case "list":
			if err := getIndexedDBDatabases(ctx, tabID); err != nil {
				log.Fatalf("Failed to list IndexedDB databases: %v", err)
			}
		case "clear":
			if err := clearIndexedDB(ctx, tabID); err != nil {
				log.Fatalf("Failed to clear IndexedDB: %v", err)
			}
		default:
			log.Fatalf("Unknown action: %s (available: list, clear)", action)
		}
	},
}

var storageCacheCmd = &cobra.Command{
	Use:   "cache <action>",
	Short: "Cache storage operations (list, clear)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		action := args[0]
		switch action {
		case "list":
			if err := getCacheStorage(ctx, tabID); err != nil {
				log.Fatalf("Failed to list cache storage: %v", err)
			}
		case "clear":
			if err := clearCacheStorage(ctx, tabID); err != nil {
				log.Fatalf("Failed to clear cache storage: %v", err)
			}
		default:
			log.Fatalf("Unknown action: %s (available: list, clear)", action)
		}
	},
}

var storageClearAllCmd = &cobra.Command{
	Use:   "clear-all",
	Short: "Clear all storage types",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := clearAllStorage(ctx, tabID); err != nil {
			log.Fatalf("Failed to clear all storage: %v", err)
		}
	},
}

func init() {
	// Add subcommands to storage command
	storageCmd.AddCommand(storageLocalCmd)
	storageCmd.AddCommand(storageSessionCmd)
	storageCmd.AddCommand(storageCookiesCmd)
	storageCmd.AddCommand(storageIndexeddbCmd)
	storageCmd.AddCommand(storageCacheCmd)
	storageCmd.AddCommand(storageClearAllCmd)

	// Add flags
	storageLocalCmd.Flags().String("tab", "", "Target tab ID")
	storageSessionCmd.Flags().String("tab", "", "Target tab ID")
	storageCookiesCmd.Flags().String("tab", "", "Target tab ID")
	storageIndexeddbCmd.Flags().String("tab", "", "Target tab ID")
	storageCacheCmd.Flags().String("tab", "", "Target tab ID")
	storageClearAllCmd.Flags().String("tab", "", "Target tab ID")
}

// Implementation functions

func getLocalStorage(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	items, err := storageCtrl.GetLocalStorage(ctx)
	if err != nil {
		return err
	}

	fmt.Println("LocalStorage items:")
	if len(items) == 0 {
		fmt.Println("  (empty)")
		return nil
	}

	for key, value := range items {
		fmt.Printf("  %s: %s\n", key, value)
	}

	return nil
}

func setLocalStorage(ctx context.Context, key, value, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	if err := storageCtrl.SetLocalStorage(ctx, key, value); err != nil {
		return err
	}

	fmt.Printf("✓ Set localStorage['%s'] = '%s'\n", key, value)
	return nil
}

func removeLocalStorage(ctx context.Context, key, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	if err := storageCtrl.RemoveLocalStorage(ctx, key); err != nil {
		return err
	}

	fmt.Printf("✓ Removed localStorage['%s']\n", key)
	return nil
}

func clearLocalStorage(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	if err := storageCtrl.ClearLocalStorage(ctx); err != nil {
		return err
	}

	fmt.Println("✓ Cleared localStorage")
	return nil
}

func getSessionStorage(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	items, err := storageCtrl.GetSessionStorage(ctx)
	if err != nil {
		return err
	}

	fmt.Println("SessionStorage items:")
	if len(items) == 0 {
		fmt.Println("  (empty)")
		return nil
	}

	for key, value := range items {
		fmt.Printf("  %s: %s\n", key, value)
	}

	return nil
}

func setSessionStorage(ctx context.Context, key, value, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	if err := storageCtrl.SetSessionStorage(ctx, key, value); err != nil {
		return err
	}

	fmt.Printf("✓ Set sessionStorage['%s'] = '%s'\n", key, value)
	return nil
}

func removeSessionStorage(ctx context.Context, key, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Use JavaScript to remove from sessionStorage
	expression := fmt.Sprintf(`sessionStorage.removeItem(%s)`, jsonString(key))
	_, err := debugger.Execute(ctx, expression)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Removed sessionStorage['%s']\n", key)
	return nil
}

func clearSessionStorage(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	_, err := debugger.Execute(ctx, "sessionStorage.clear()")
	if err != nil {
		return err
	}

	fmt.Println("✓ Cleared sessionStorage")
	return nil
}

func getCookies(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	cookies, err := storageCtrl.GetCookies(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Cookies:")
	if len(cookies) == 0 {
		fmt.Println("  (none)")
		return nil
	}

	for _, cookie := range cookies {
		if name, ok := cookie["name"].(string); ok {
			if value, ok := cookie["value"].(string); ok {
				fmt.Printf("  %s: %s\n", name, value)
			}
		}
	}

	return nil
}

func setCookie(ctx context.Context, name, value string, options map[string]interface{}, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	if err := storageCtrl.SetCookie(ctx, name, value, options); err != nil {
		return err
	}

	fmt.Printf("✓ Set cookie '%s' = '%s'\n", name, value)
	if len(options) > 0 {
		fmt.Printf("  Options: %v\n", options)
	}

	return nil
}

func removeCookie(ctx context.Context, name, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Remove cookie by setting it to expire in the past
	expression := fmt.Sprintf(`document.cookie = %s + "=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/"`, jsonString(name))
	_, err := debugger.Execute(ctx, expression)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Removed cookie '%s'\n", name)
	return nil
}

func getIndexedDBDatabases(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	databases, err := storageCtrl.GetIndexedDBDatabases(ctx)
	if err != nil {
		return err
	}

	fmt.Println("IndexedDB databases:")
	if len(databases) == 0 {
		fmt.Println("  (none)")
		return nil
	}

	for _, db := range databases {
		if dbJSON, err := json.MarshalIndent(db, "  ", "  "); err == nil {
			fmt.Printf("  %s\n", string(dbJSON))
		} else {
			fmt.Printf("  %v\n", db)
		}
	}

	return nil
}

func clearIndexedDB(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	if err := storageCtrl.ClearIndexedDB(ctx); err != nil {
		return err
	}

	fmt.Println("✓ Cleared IndexedDB databases")
	return nil
}

func getCacheStorage(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	caches, err := storageCtrl.GetCacheStorage(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Cache storage:")
	if len(caches) == 0 {
		fmt.Println("  (none)")
		return nil
	}

	for _, cache := range caches {
		if cacheJSON, err := json.MarshalIndent(cache, "  ", "  "); err == nil {
			fmt.Printf("  %s\n", string(cacheJSON))
		} else {
			fmt.Printf("  %v\n", cache)
		}
	}

	return nil
}

func clearCacheStorage(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	expression := `
		(async function() {
			try {
				if ('caches' in window) {
					const cacheNames = await caches.keys();
					await Promise.all(cacheNames.map(name => caches.delete(name)));
					return { cleared: cacheNames.length };
				}
				return { message: 'Cache storage not available' };
			} catch (e) {
				return { error: e.message };
			}
		})()
	`

	result, err := debugger.Execute(ctx, expression)
	if err != nil {
		return err
	}

	fmt.Println("✓ Cleared cache storage")
	if resultJSON, err := json.MarshalIndent(result, "", "  "); err == nil {
		fmt.Printf("Result: %s\n", string(resultJSON))
	}

	return nil
}

func clearAllStorage(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	storageCtrl := NewStorageController(debugger, verbose)
	if err := storageCtrl.ClearAllStorage(ctx); err != nil {
		return err
	}

	fmt.Println("✓ Cleared all storage types")
	return nil
}

// parseCookieOptions parses cookie options from command line arguments
func parseCookieOptions(args []string) map[string]interface{} {
	options := make(map[string]interface{})

	for _, arg := range args {
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			key := parts[0]
			value := parts[1]

			switch key {
			case "path", "domain":
				options[key] = value
			case "maxAge":
				if intVal, err := strconv.Atoi(value); err == nil {
					options[key] = intVal
				}
			case "secure", "httpOnly":
				if boolVal, err := strconv.ParseBool(value); err == nil {
					options[key] = boolVal
				}
			}
		} else {
			// Boolean flags
			switch arg {
			case "secure":
				options["secure"] = true
			case "httpOnly":
				options["httpOnly"] = true
			}
		}
	}

	return options
}