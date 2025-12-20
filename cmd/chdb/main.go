// Package main implements the CHDB (Chrome Debugger) CLI tool for
// Chrome and Chromium browser debugging using the Chrome DevTools Protocol.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var (
	verbose     bool
	timeout     int
	port        string
	headless    bool
	userDataDir string
)

var rootCmd = &cobra.Command{
	Use:   "chdb",
	Short: "Chrome Debugger - Advanced debugging for Chrome/Chromium browsers",
	Long: `CHDB provides advanced debugging capabilities for Chrome and Chromium browsers
using the Chrome DevTools Protocol.

Features:
- Attach to running Chrome instances
- Interactive debugging with breakpoints
- DOM inspection and manipulation
- Network monitoring and interception
- JavaScript execution and profiling
- Performance analysis
- Screenshot and PDF generation`,
	Version: "1.0.0",
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 60, "Operation timeout in seconds")
	rootCmd.PersistentFlags().StringVar(&port, "port", "9222", "Chrome debug port")

	// Add subcommands
	rootCmd.AddCommand(attachCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(navigateCmd)
	rootCmd.AddCommand(screenshotCmd)
	rootCmd.AddCommand(breakCmd)
	rootCmd.AddCommand(pauseCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(stepCmd)
	rootCmd.AddCommand(nextCmd)
	rootCmd.AddCommand(outCmd)
	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(monitorCmd)
	rootCmd.AddCommand(profileCmd)
	rootCmd.AddCommand(devtoolsCmd)
	rootCmd.AddCommand(domCmd)
	rootCmd.AddCommand(cssCmd)
	rootCmd.AddCommand(networkCmd)
	rootCmd.AddCommand(storageCmd)
	rootCmd.AddCommand(swCmd)
	rootCmd.AddCommand(deviceCmd)
	rootCmd.AddCommand(renderCmd)
	rootCmd.AddCommand(animationCmd)
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(consoleCmd)
	rootCmd.AddCommand(newTargetCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func createContext() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)

	// Handle interrupts
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		if verbose {
			log.Println("Interrupt received, shutting down...")
		}
		cancel()
	}()

	return ctx
}