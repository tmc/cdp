package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var (
	unminifyOutDir string
	unminifyApiKey string
)

var unminifyCmd = &cobra.Command{
	Use:   "unminify <url>",
	Short: "Backfill sourcemaps using AI",
	Long:  `Downloads a minified script, beautifies it, uses AI to rename variables, and generates a sourcemap.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := runUnminify(ctx, tabID, args[0]); err != nil {
			log.Fatalf("Unminify failed: %v", err)
		}
	},
}

func init() {
	unminifyCmd.Flags().String("tab", "", "Target tab ID")
	unminifyCmd.Flags().StringVarP(&unminifyOutDir, "out-dir", "o", "unminified", "Output directory")
	unminifyCmd.Flags().StringVar(&unminifyApiKey, "api-key", os.Getenv("GEMINI_API_KEY"), "Gemini API Key")
	// Register with root
	// root.AddCommand(unminifyCmd) // Assuming main.go handles this or we add to a parent
}

func runUnminify(ctx context.Context, tabID, url string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// 1. Fetch Content
	log.Printf("Fetching content for %s...", url)
	content, err := fetchScriptContent(ctx, debugger, url)
	if err != nil {
		return fmt.Errorf("failed to fetch content: %w", err)
	}
	log.Printf("Fetched %d bytes.", len(content))

	// 2. Format (Beautify) & Generate Base Sourcemap
	// Todo: Implement robust formatter
	beautified, baseMap := fastBeautify(content, url)

	// 3. AI Analysis (Rename)
	log.Printf("Analyzing for renames (this may take a moment)...")
	renames, err := analyzeRenames(ctx, beautified)
	if err != nil {
		log.Printf("Renaming failed (skipping): %v", err)
	}

	// 4. Apply Renames & Update Sourcemap
	finalCode := beautified
	if len(renames) > 0 {
		log.Printf("Applying %d renames...", len(renames))
		finalCode, _ = applyRenames(beautified, baseMap, renames)
	}

	// For Prototype: Just save beautified

	// Save
	if err := os.MkdirAll(unminifyOutDir, 0755); err != nil {
		return err
	}

	filename := filepath.Base(url)
	if filename == "" || filename == "." {
		filename = "script.js"
	}

	jsPath := filepath.Join(unminifyOutDir, filename)
	if err := os.WriteFile(jsPath, []byte(finalCode), 0644); err != nil {
		return err
	}

	log.Printf("Saved unminified script to %s", jsPath)
	return nil
}

func fetchScriptContent(ctx context.Context, d *ChromeDebugger, url string) (string, error) {
	// Enable page domain
	if err := chromedp.Run(d.chromeCtx, page.Enable()); err != nil {
		return "", err
	}

	// Get resource tree
	var tree *page.FrameResourceTree
	if err := chromedp.Run(d.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		tree, err = page.GetResourceTree().Do(ctx)
		return err
	})); err != nil {
		return "", fmt.Errorf("failed to get resource tree: %w", err)
	}

	// Recursive finder
	var result []byte
	var found bool

	// Normalize target URL (simple string match for now)
	targetURL := url

	var findResource func(*page.FrameResourceTree)
	findResource = func(frame *page.FrameResourceTree) {
		if found {
			return
		}
		// Check frame main resource
		if frame.Frame.URL == targetURL {
			// Found in frame main resource
			if err := chromedp.Run(d.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				result, err = page.GetResourceContent(frame.Frame.ID, url).Do(ctx)
				return err
			})); err == nil {
				found = true
				return
			}
		}

		// Check resources
		for _, res := range frame.Resources {
			if res.URL == targetURL {
				if err := chromedp.Run(d.chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error {
					var err error
					result, err = page.GetResourceContent(frame.Frame.ID, url).Do(ctx)
					return err
				})); err == nil {
					found = true
					return
				}
			}
		}

		for _, child := range frame.ChildFrames {
			findResource(child)
		}
	}

	findResource(tree)

	if !found {
		// Fallback: If not found in tree, use Runtime.evaluate (maybe it's dynamically loaded?)
		// But for now, just return error to be explicit
		return "", fmt.Errorf("resource %s not found in resource tree", url)
	}

	return string(result), nil
}

func fastBeautify(minified string, filename string) (string, string) {
	// Very dumb "formatter" for prototype
	// Just indent on { and ;
	var sb strings.Builder
	depth := 0

	for _, r := range minified {
		sb.WriteRune(r)
		if r == '{' {
			depth++
			sb.WriteRune('\n')
			sb.WriteString(strings.Repeat("  ", depth))
		} else if r == '}' {
			depth--
			if depth < 0 {
				depth = 0
			}
			sb.WriteRune('\n') // Pre-newline?
			sb.WriteString(strings.Repeat("  ", depth))
		} else if r == ';' {
			sb.WriteRune('\n')
			sb.WriteString(strings.Repeat("  ", depth))
		}
	}

	return sb.String(), "" // TODO: Sourcemap
}

// Placeholder for GenAI usage
// analyzeRenames is disabled for now.
func analyzeRenames(ctx context.Context, code string) (map[string]string, error) {
	return nil, nil
}

func applyRenames(code string, baseMap string, renames map[string]string) (string, string) {
	return code, baseMap
}
