package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
	"golang.org/x/tools/txtar"
)

var outSources string
var outDir string

var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Dump page sources as txtar archive",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := dumpSources(ctx, tabID); err != nil {
			log.Fatalf("Failed to dump sources: %v", err)
		}
	},
}

func init() {
	sourcesCmd.Flags().String("tab", "", "Target tab ID")
	sourcesCmd.Flags().StringVarP(&outSources, "output", "o", "", "Output txtar file (default: stdout)")
	sourcesCmd.Flags().StringVarP(&outDir, "dir", "d", "", "Output directory (extract files)")
}

func dumpSources(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Enable Page domain
	if err := chromedp.Run(debugger.chromeCtx, page.Enable()); err != nil {
		return fmt.Errorf("failed to enable page domain: %w", err)
	}

	// Get resource tree
	var tree *page.FrameResourceTree
	if err := chromedp.Run(debugger.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		tree, err = page.GetResourceTree().Do(ctx)
		return err
	})); err != nil {
		return fmt.Errorf("failed to get resource tree: %w", err)
	}

	ar := &txtar.Archive{}

	// Helper to download content
	downloadContent := func(frameID cdp.FrameID, resourceURL string) {

		// Construct useful path (domain/path)
		path, err := urlToLocalPath(resourceURL)
		if err != nil {
			if verbose {
				log.Printf("Skipping invalid URL %s: %v", resourceURL, err)
			}
			return
		}

		var content []byte
		err = chromedp.Run(debugger.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			content, err = page.GetResourceContent(frameID, resourceURL).Do(ctx)
			return err
		}))

		if err != nil {
			if verbose {
				log.Printf("Failed to get content for %s: %v", resourceURL, err)
			}
			content = []byte(fmt.Sprintf("<< Error fetching content: %v >>", err))
		}

		ar.Files = append(ar.Files, txtar.File{
			Name: path,
			Data: content,
		})
	}

	// Helper to process frames recursively
	var processFrame func(*page.FrameResourceTree) error
	processFrame = func(frame *page.FrameResourceTree) error {
		if verbose {
			log.Printf("Frame %s: %s (Resources: %d, Children: %d)", frame.Frame.ID, frame.Frame.URL, len(frame.Resources), len(frame.ChildFrames))
		}

		// Process frame itself (main resource)
		// Only if it has a valid URL
		if frame.Frame.URL != "" && frame.Frame.URL != "about:blank" {
			downloadContent(frame.Frame.ID, frame.Frame.URL)
		}

		// Process resources in this frame
		for _, resource := range frame.Resources {
			downloadContent(frame.Frame.ID, resource.URL)
		}

		// Recurse children
		for _, child := range frame.ChildFrames {
			if err := processFrame(child); err != nil {
				return err
			}
		}
		return nil
	}

	if err := processFrame(tree); err != nil {
		return err
	}

	data := txtar.Format(ar)

	if outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
		for _, f := range ar.Files {
			// Join preserves relative paths safe? txtar paths are relative.
			// Ideally we ensure f.Name is safe.
			fp := filepath.Join(outDir, f.Name)
			if err := os.MkdirAll(filepath.Dir(fp), 0755); err != nil {
				return fmt.Errorf("failed to create directory for file %s: %w", f.Name, err)
			}
			if err := os.WriteFile(fp, f.Data, 0644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", f.Name, err)
			}
		}
		if verbose {
			log.Printf("Extracted %d files to %s", len(ar.Files), outDir)
		}
		return nil
	}

	if outSources != "" {
		return os.WriteFile(outSources, data, 0644)
	}

	fmt.Print(string(data))
	return nil
}
