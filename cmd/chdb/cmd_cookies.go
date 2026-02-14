package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var clearCookies bool

var cookiesCmd = &cobra.Command{
	Use:   "cookies",
	Short: "Manage browser cookies",
	Long:  `List or clear cookies for the current page context.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := manageCookies(ctx, tabID); err != nil {
			log.Fatalf("Failed to manage cookies: %v", err)
		}
	},
}

func init() {
	cookiesCmd.Flags().String("tab", "", "Target tab ID")
	cookiesCmd.Flags().BoolVar(&clearCookies, "clear", false, "Clear all browser cookies")
}

func manageCookies(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	if clearCookies {
		if err := chromedp.Run(debugger.chromeCtx, network.ClearBrowserCookies()); err != nil {
			return fmt.Errorf("failed to clear cookies: %w", err)
		}
		if verbose {
			log.Println("Cookies cleared")
		}
		return nil
	}

	// List cookies
	var cookies []*network.Cookie
	if err := chromedp.Run(debugger.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Use GetCookies which returns cookies for current URL context
		// Alternatively GetAllCookies returns everything.
		// Let's rely on what the browser considers "cookies for this page" if we can,
		// but GetCookies() usually needs a list of URLs.
		// GetAllCookies() is generally what "Application -> Cookies" shows (all of them or filtered).
		// Let's stick to GetCookies() if we can default to current URL?
		// Actually GetCookies returns cookies for the current URL.
		var err error
		cookies, err = network.GetCookies().Do(ctx)
		return err
	})); err != nil {
		return fmt.Errorf("failed to get cookies: %w", err)
	}

	enc := json.NewEncoder(log.Writer())
	enc.SetIndent("", "  ")
	if err := enc.Encode(cookies); err != nil {
		return fmt.Errorf("failed to encode cookies: %w", err)
	}

	return nil
}
