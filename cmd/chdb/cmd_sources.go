package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/debugger"
	cdppage "github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
	"golang.org/x/tools/txtar"
)

var outSources string
var outDir string

var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Dump page sources (resource tree + debugger scripts) as txtar archive",
	Long: `Extract all page sources using two complementary CDP mechanisms:

1. Page.getResourceTree + Page.getResourceContent: HTML, CSS, images, fonts
2. Debugger.scriptParsed + Debugger.getScriptSource: all JavaScript including
   dynamically loaded modules, eval'd code, and web workers

This captures everything visible in Chrome DevTools' Sources tab.`,
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
	dbg := NewChromeDebugger(port, verbose)
	defer dbg.Close()

	if err := dbg.Connect(ctx, tabID); err != nil {
		return err
	}

	ar := &txtar.Archive{}
	seen := map[string]bool{}

	addFile := func(name string, data []byte) {
		if seen[name] {
			return
		}
		seen[name] = true
		ar.Files = append(ar.Files, txtar.File{Name: name, Data: data})
	}

	// Phase 1: Page resource tree (HTML, CSS, images, fonts, etc.)
	if err := chromedp.Run(dbg.chromeCtx, cdppage.Enable()); err != nil {
		return fmt.Errorf("enabling page domain: %w", err)
	}

	var tree *cdppage.FrameResourceTree
	if err := chromedp.Run(dbg.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		tree, err = cdppage.GetResourceTree().Do(ctx)
		return err
	})); err != nil {
		return fmt.Errorf("getting resource tree: %w", err)
	}

	downloadContent := func(frameID cdp.FrameID, resourceURL string) {
		path, err := urlToLocalPath(resourceURL)
		if err != nil {
			if verbose {
				log.Printf("Skipping invalid URL %s: %v", resourceURL, err)
			}
			return
		}

		var content []byte
		err = chromedp.Run(dbg.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			content, err = cdppage.GetResourceContent(frameID, resourceURL).Do(ctx)
			return err
		}))
		if err != nil {
			if verbose {
				log.Printf("Failed to get content for %s: %v", resourceURL, err)
			}
			return
		}

		addFile(path, content)
	}

	var processFrame func(*cdppage.FrameResourceTree) error
	processFrame = func(frame *cdppage.FrameResourceTree) error {
		if verbose {
			log.Printf("Frame %s: %s (Resources: %d, Children: %d)",
				frame.Frame.ID, frame.Frame.URL,
				len(frame.Resources), len(frame.ChildFrames))
		}

		if frame.Frame.URL != "" && frame.Frame.URL != "about:blank" {
			downloadContent(frame.Frame.ID, frame.Frame.URL)
		}

		for _, resource := range frame.Resources {
			downloadContent(frame.Frame.ID, resource.URL)
		}

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

	if verbose {
		log.Printf("Phase 1 (resource tree): %d files", len(ar.Files))
	}

	// Phase 2: Debugger scripts (all JS parsed by V8)
	var mu sync.Mutex
	type scriptInfo struct {
		id  cdp.ScriptID
		url string
	}
	var scripts []scriptInfo

	// Listen for scriptParsed events before enabling debugger
	chromedp.ListenTarget(dbg.chromeCtx, func(ev any) {
		if sp, ok := ev.(*debugger.EventScriptParsed); ok {
			url := sp.URL
			if url == "" {
				return // skip anonymous eval scripts with no URL
			}
			mu.Lock()
			scripts = append(scripts, scriptInfo{id: sp.ScriptID, url: url})
			mu.Unlock()
		}
	})

	// Enable debugger to receive scriptParsed for already-loaded scripts
	if err := chromedp.Run(dbg.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := debugger.Enable().Do(ctx)
		return err
	})); err != nil {
		// Debugger.enable may fail on some targets; proceed with what we have
		if verbose {
			log.Printf("Debugger.enable failed (continuing with resource tree only): %v", err)
		}
	} else {
		// Give a moment for scriptParsed events to arrive
		time.Sleep(500 * time.Millisecond)

		mu.Lock()
		scriptsCopy := make([]scriptInfo, len(scripts))
		copy(scriptsCopy, scripts)
		mu.Unlock()

		if verbose {
			log.Printf("Phase 2 (debugger): %d scripts parsed", len(scriptsCopy))
		}

		// Fetch source for each script
		for _, s := range scriptsCopy {
			path, err := urlToLocalPath(s.url)
			if err != nil {
				if verbose {
					log.Printf("Skipping script URL %s: %v", s.url, err)
				}
				continue
			}

			// Prefer .js suffix for scripts if not already present
			if !strings.HasSuffix(path, ".js") && !strings.HasSuffix(path, ".mjs") && !strings.HasSuffix(path, ".cjs") && !strings.Contains(filepath.Base(path), ".") {
				path += ".js"
			}

			if seen[path] {
				continue
			}

			var source string
			err = chromedp.Run(dbg.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				source, _, err = debugger.GetScriptSource(s.id).Do(ctx)
				return err
			}))
			if err != nil {
				if verbose {
					log.Printf("Failed to get script source for %s: %v", s.url, err)
				}
				continue
			}

			addFile(path, []byte(source))
		}

		// Disable debugger to avoid pausing the app
		_ = chromedp.Run(dbg.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			debugger.Disable().Do(ctx)
			return nil
		}))
	}

	if verbose {
		log.Printf("Total: %d files", len(ar.Files))
	}

	data := txtar.Format(ar)

	if outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
		for _, f := range ar.Files {
			fp := filepath.Join(outDir, f.Name)
			if err := os.MkdirAll(filepath.Dir(fp), 0755); err != nil {
				return fmt.Errorf("creating directory for %s: %w", f.Name, err)
			}
			if err := os.WriteFile(fp, f.Data, 0644); err != nil {
				return fmt.Errorf("writing %s: %w", f.Name, err)
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
