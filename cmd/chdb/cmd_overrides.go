package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var overridesDir string

var overridesCmd = &cobra.Command{
	Use:   "overrides",
	Short: "Serve local files as network overrides",
	Long:  `Intercepts network requests and serves content from a local directory if a matching file exists. Mirrors Chrome DevTools "Local Overrides".`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if overridesDir == "" {
			log.Fatal("Please specify --dir")
		}

		if err := runOverrides(ctx, tabID, overridesDir); err != nil {
			log.Fatalf("Overrides failed: %v", err)
		}
	},
}

func init() {
	overridesCmd.Flags().String("tab", "", "Target tab ID")
	overridesCmd.Flags().StringVarP(&overridesDir, "dir", "d", "", "Directory containing override files (required)")
}

func runOverrides(ctx context.Context, tabID, dir string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Signalling channel
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Enable Fetch domain
	if err := chromedp.Run(debugger.chromeCtx, fetch.Enable().WithPatterns([]*fetch.RequestPattern{
		{
			URLPattern:   "*",
			RequestStage: fetch.RequestStageRequest,
		},
	})); err != nil {
		return fmt.Errorf("failed to enable fetch: %w", err)
	}

	log.Printf("Listening for requests. Serving overrides from %s. Press Ctrl+C to stop.", dir)

	// Event listener loop
	chromedp.ListenTarget(debugger.chromeCtx, func(ev interface{}) {
		if verbose {
			log.Printf("Event: %T", ev)
		}
		switch e := ev.(type) {
		case *fetch.EventRequestPaused:
			go handleRequest(debugger.chromeCtx, e, dir)
		}
	})

	// Wait for signal
	<-sigs
	log.Println("Stopping overrides...")
	return nil
}

func handleRequest(ctx context.Context, e *fetch.EventRequestPaused, dir string) {
	// Map URL to local path
	relPath, err := urlToLocalPath(e.Request.URL)
	if err != nil {
		if verbose {
			log.Printf("Error checking URL %s: %v", e.Request.URL, err)
		}
		continueRequest(ctx, e.RequestID)
		return
	}

	localPath := filepath.Join(dir, relPath)

	// Check if file exists
	info, err := os.Stat(localPath)
	if os.IsNotExist(err) || info.IsDir() {
		// Pass through
		if verbose {
			log.Printf("MISS: %s -> %s", e.Request.URL, localPath)
		}
		continueRequest(ctx, e.RequestID)
		return
	}

	// HIT: Serve file
	content, err := os.ReadFile(localPath)
	if err != nil {
		log.Printf("Error reading file %s: %v", localPath, err)
		continueRequest(ctx, e.RequestID)
		return
	}

	// Determine Content-Type
	// 1. Try by extension first
	ext := filepath.Ext(localPath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		// 2. Fallback to sniffing
		contentType = http.DetectContentType(content)
	}

	log.Printf("HIT: %s -> %s (%s)", e.Request.URL, localPath, contentType)

	// Fulfill request
	err = chromedp.Run(ctx, fetch.FulfillRequest(e.RequestID, 200).
		WithBody(base64.StdEncoding.EncodeToString(content)).
		WithResponseHeaders([]*fetch.HeaderEntry{
			{Name: "Content-Type", Value: contentType},
			{Name: "X-Chdb-Override", Value: "true"},
		}))

	if err != nil {
		log.Printf("Failed to fulfill request %s: %v", e.RequestID, err)
	}
}

func continueRequest(ctx context.Context, requestID fetch.RequestID) {
	if err := chromedp.Run(ctx, fetch.ContinueRequest(requestID)); err != nil {
		// Log but don't fail, request might have been cancelled
		if verbose {
			log.Printf("Failed to continue request %s: %v", requestID, err)
		}
	}
}
