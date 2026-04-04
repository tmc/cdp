package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/chromedp/cdproto/css"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var cssSelector string

var cssCmd = &cobra.Command{
	Use:   "css",
	Short: "Inspect computed styles",
	Long:  `Dumps computed styles for a specific node selected by CSS selector.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if cssSelector == "" {
			log.Fatal("Please specify --selector")
		}

		if err := runCSS(ctx, tabID); err != nil {
			log.Fatalf("CSS inspection failed: %v", err)
		}
	},
}

func init() {
	cssCmd.Flags().String("tab", "", "Target tab ID")
	cssCmd.Flags().StringVarP(&cssSelector, "selector", "s", "", "CSS selector (e.g. 'body', '#app')")
}

func runCSS(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Enable DOM and CSS domains
	if err := chromedp.Run(debugger.chromeCtx, dom.Enable(), css.Enable()); err != nil {
		return fmt.Errorf("failed to enable domains: %w", err)
	}

	var computedStyles []*css.ComputedStyleProperty

	err := chromedp.Run(debugger.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Get document root
		rootNode, err := dom.GetDocument().Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to get document: %w", err)
		}

		// Query selector
		nodeID, err := dom.QuerySelector(rootNode.NodeID, cssSelector).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to query selector '%s': %w", cssSelector, err)
		}
		if nodeID == 0 {
			return fmt.Errorf("node not found for selector '%s'", cssSelector)
		}

		// Get computed style
		computedStyles, _, err = css.GetComputedStyleForNode(nodeID).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to get computed styles: %w", err)
		}

		return nil
	}))

	if err != nil {
		return err
	}

	// Output
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(computedStyles); err != nil {
		return fmt.Errorf("failed to encode output: %w", err)
	}

	return nil
}
