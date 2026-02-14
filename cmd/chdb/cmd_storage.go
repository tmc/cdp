package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/chromedp/cdproto/domstorage"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var storageIsLocal bool

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Inspect LocalStorage or SessionStorage",
	Long:  `Lists items from LocalStorage (default) or SessionStorage for the current page origin.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := runStorage(ctx, tabID); err != nil {
			log.Fatalf("Storage inspection failed: %v", err)
		}
	},
}

func init() {
	storageCmd.Flags().String("tab", "", "Target tab ID")
	storageCmd.Flags().BoolVarP(&storageIsLocal, "local", "l", true, "Inspect LocalStorage (set to false for SessionStorage)")
}

func runStorage(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	var items []domstorage.Item

	err := chromedp.Run(debugger.chromeCtx,
		domstorage.Enable(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Get security origin
			tree, err := page.GetResourceTree().Do(ctx)
			if err != nil {
				return fmt.Errorf("failed to get resource tree: %w", err)
			}
			origin := tree.Frame.SecurityOrigin

			// Construct StorageId
			storageID := &domstorage.StorageID{
				SecurityOrigin: origin,
				IsLocalStorage: storageIsLocal,
			}

			// Get items
			items, err = domstorage.GetDOMStorageItems(storageID).Do(ctx)
			return err
		}),
	)

	if err != nil {
		return err
	}

	// Output as JSON
	output := make(map[string]string)
	for _, item := range items {
		if len(item) == 2 {
			output[item[0]] = item[1]
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("failed to encode items: %w", err)
	}

	return nil
}
