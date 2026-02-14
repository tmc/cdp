package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var outDomFile string
var domSelector string

var domCmd = &cobra.Command{
	Use:   "dom",
	Short: "Dump the DOM tree or specific node",
	Long:  `Retrieves the DOM structure of the current page. By default dumps the full document. Use --selector to dump a specific node's outer HTML.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := dumpDom(ctx, tabID); err != nil {
			log.Fatalf("Failed to dump DOM: %v", err)
		}
	},
}

func init() {
	domCmd.Flags().String("tab", "", "Target tab ID")
	domCmd.Flags().StringVarP(&outDomFile, "output", "o", "", "Output file (default: stdout)")
	domCmd.Flags().StringVarP(&domSelector, "selector", "s", "", "CSS selector to dump (default: full document)")
}

func dumpDom(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	var res string
	var err error

	if domSelector == "" {
		// Dump full document
		err = chromedp.Run(debugger.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			root, err := dom.GetDocument().Do(ctx)
			if err != nil {
				return err
			}
			res, err = dom.GetOuterHTML().WithNodeID(root.NodeID).Do(ctx)
			return err
		}))
	} else {
		// Dump specific node
		err = chromedp.Run(debugger.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			var nodes []*cdp.Node
			if err := chromedp.Nodes(domSelector, &nodes, chromedp.ByQuery).Do(ctx); err != nil {
				return err
			}
			if len(nodes) == 0 {
				return fmt.Errorf("no nodes found for selector: %s", domSelector)
			}
			// Dump first match
			res, err = dom.GetOuterHTML().WithNodeID(nodes[0].NodeID).Do(ctx)
			return err
		}))
	}

	if err != nil {
		return fmt.Errorf("failed to get DOM: %w", err)
	}

	if outDomFile != "" {
		if err := os.WriteFile(outDomFile, []byte(res), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		if verbose {
			log.Printf("Wrote DOM to %s", outDomFile)
		}
		return nil
	}

	fmt.Println(res)
	return nil
}
