package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Start interactive console session",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := startConsole(ctx, tabID); err != nil {
			log.Fatalf("Console session failed: %v", err)
		}
	},
}

func init() {
	consoleCmd.Flags().String("tab", "", "Target tab ID")
}

func startConsole(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Enable runtime
	if err := debugger.EnableDomains(ctx, "Runtime"); err != nil {
		return err
	}

	fmt.Println("Interactive Console Session")
	fmt.Println("===========================")
	fmt.Println("Type JavaScript expressions. Type 'exit' to quit.")
	fmt.Println()

	// Start interactive console
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("js> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if line == "exit" || line == "quit" {
			break
		}

		// Execute JavaScript
		result, err := debugger.Execute(ctx, line)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			if resultBytes, err := json.MarshalIndent(result, "", "  "); err == nil {
				fmt.Println(string(resultBytes))
			} else {
				fmt.Printf("%v\n", result)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}