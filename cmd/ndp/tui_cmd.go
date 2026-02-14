package main

import (
	"context"
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Start TUI (Text User Interface)",
	Run: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		targetID, _ := cmd.Flags().GetString("target")

		repl := NewREPL(verbose)

		// Initialize TUI Program
		p := tea.NewProgram(NewTUIModel(repl), tea.WithAltScreen())

		// Set REPL output to write to TUI
		repl.SetOutput(&ProgramWriter{p: p})

		// Connect if target provided
		if targetID != "" {
			// Connect async or sync?
			// Start needs context.
			// Currently REPL.Start handles connection logic.
			// But we are bypassing REPL.Start to run TUI.
			// So we call connectToTarget directly?
			go func() {
				if err := repl.connectToTarget(context.Background(), targetID); err != nil {
					p.Send(LogMsg(fmt.Sprintf("Error connecting: %v", err)))
				} else {
					p.Send(LogMsg(fmt.Sprintf("Connected to %s", targetID)))
					repl.running = true // Mimic Start
				}
			}()
		} else {
			go func() {
				// repl.showTargets uses r.println which sends to TUI
				repl.showTargets(context.Background())
			}()
		}

		if _, err := p.Run(); err != nil {
			log.Fatalf("Alas, there's been an error: %v", err)
		}
		fmt.Println("TUI exited.")
	},
}

type ProgramWriter struct {
	p *tea.Program
}

func (w *ProgramWriter) Write(p []byte) (n int, err error) {
	// Make sure we copy the slice string, though string(p) does that.
	w.p.Send(LogMsg(string(p)))
	return len(p), nil
}
