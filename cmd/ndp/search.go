package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <term>",
	Short: "Search for text in loaded scripts",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		term := args[0]
		ctx := createContext()
		targetID, _ := cmd.Flags().GetString("target")

		// Attach
		debugger := NewNodeDebugger(verbose)
		if targetID == "" {
			// Auto-attach default
			if err := debugger.Attach(ctx, "9229"); err != nil {
				log.Fatalf("Failed to attach check: %v", err)
			}
		} else {
			if err := debugger.Attach(ctx, targetID); err != nil {
				log.Fatalf("Failed to attach: %v", err)
			}
		}

		fmt.Printf("Searching for %q in loaded scripts...\n", term)

		results, err := debugger.SearchInAllScripts(ctx, term)
		if err != nil {
			log.Fatalf("Search failed: %v", err)
		}

		if len(results) == 0 {
			fmt.Println("No matches found.")
			return
		}

		for _, res := range results {
			fmt.Printf("\n%s (ID: %s)\n", res.URL, res.ScriptID)
			for _, match := range res.Matches {
				fmt.Printf("  %d: %s\n", match.LineNumber+1, strings.TrimSpace(match.LineContent))
			}
		}
	},
}

type SearchResult struct {
	ScriptID string
	URL      string
	Matches  []SearchMatch
}

type SearchMatch struct {
	LineNumber  int
	LineContent string
}

func init() {
	searchCmd.Flags().StringP("target", "t", "", "Target ID")
	rootCmd.AddCommand(searchCmd)
}
