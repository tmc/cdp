package main

import (
	"log"

	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Start the interactive terminal interface",
	Long: `Starts the interactive NDP terminal interface.

The previous Bubble Tea UI has been retired. This command now runs the
standard line-oriented REPL so existing scripts and habits continue to work.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		targetID, _ := cmd.Flags().GetString("target")
		repl := NewREPL(verbose)

		if err := repl.Start(ctx, targetID); err != nil {
			log.Fatalf("TUI error: %v", err)
		}
	},
}
