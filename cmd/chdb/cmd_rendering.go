package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/spf13/cobra"
)

// Rendering commands
var renderCmd = &cobra.Command{
	Use:     "render",
	Aliases: []string{"rendering"},
	Short:   "Rendering and paint debugging tools",
	Long:    "Tools for debugging rendering performance, paint events, and compositing layers",
}

var renderFpsCmd = &cobra.Command{
	Use:   "fps [duration]",
	Short: "Measure frame rate",
	Long:  "Measure frame rate over a specified duration (default: 5000ms)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		duration := 5000 // default 5 seconds
		if len(args) > 0 {
			if parsedDuration, err := strconv.Atoi(args[0]); err != nil {
				log.Fatalf("Invalid duration: %v", err)
			} else {
				duration = parsedDuration
			}
		}

		if err := measureFrameRate(ctx, duration, tabID); err != nil {
			log.Fatalf("Failed to measure frame rate: %v", err)
		}
	},
}

var renderPaintFlashCmd = &cobra.Command{
	Use:   "paint-flash [on|off]",
	Short: "Toggle paint flashing",
	Long:  "Enable or disable paint flashing to visualize repaints",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		enabled := true
		if len(args) > 0 {
			switch args[0] {
			case "on", "true", "1":
				enabled = true
			case "off", "false", "0":
				enabled = false
			default:
				log.Fatalf("Invalid argument: %s (use: on, off)", args[0])
			}
		}

		if err := togglePaintFlashing(ctx, enabled, tabID); err != nil {
			log.Fatalf("Failed to toggle paint flashing: %v", err)
		}
	},
}

var renderLayersCmd = &cobra.Command{
	Use:   "layers",
	Short: "Show compositing layers",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := showCompositingLayers(ctx, tabID); err != nil {
			log.Fatalf("Failed to show compositing layers: %v", err)
		}
	},
}

var renderBordersCmd = &cobra.Command{
	Use:   "borders [on|off]",
	Short: "Toggle layer borders",
	Long:  "Show or hide compositing layer borders",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		enabled := true
		if len(args) > 0 {
			switch args[0] {
			case "on", "true", "1":
				enabled = true
			case "off", "false", "0":
				enabled = false
			default:
				log.Fatalf("Invalid argument: %s (use: on, off)", args[0])
			}
		}

		if err := toggleLayerBorders(ctx, enabled, tabID); err != nil {
			log.Fatalf("Failed to toggle layer borders: %v", err)
		}
	},
}

var renderHighlightCmd = &cobra.Command{
	Use:   "highlight <selector>",
	Short: "Highlight an element",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := highlightRenderingElement(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to highlight element: %v", err)
		}
	},
}

var renderClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all rendering highlights",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := clearRenderingHighlights(ctx, tabID); err != nil {
			log.Fatalf("Failed to clear highlights: %v", err)
		}
	},
}

var renderCompositingCmd = &cobra.Command{
	Use:   "compositing <selector>",
	Short: "Show compositing reasons for element",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := showCompositingReasons(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to show compositing reasons: %v", err)
		}
	},
}

var renderScrollCmd = &cobra.Command{
	Use:   "scroll-bottlenecks [on|off]",
	Short: "Show scroll performance bottlenecks",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		enabled := true
		if len(args) > 0 {
			switch args[0] {
			case "on", "true", "1":
				enabled = true
			case "off", "false", "0":
				enabled = false
			default:
				log.Fatalf("Invalid argument: %s (use: on, off)", args[0])
			}
		}

		if err := showScrollBottlenecks(ctx, enabled, tabID); err != nil {
			log.Fatalf("Failed to show scroll bottlenecks: %v", err)
		}
	},
}

func init() {
	// Add subcommands to render command
	renderCmd.AddCommand(renderFpsCmd)
	renderCmd.AddCommand(renderPaintFlashCmd)
	renderCmd.AddCommand(renderLayersCmd)
	renderCmd.AddCommand(renderBordersCmd)
	renderCmd.AddCommand(renderHighlightCmd)
	renderCmd.AddCommand(renderClearCmd)
	renderCmd.AddCommand(renderCompositingCmd)
	renderCmd.AddCommand(renderScrollCmd)

	// Add flags
	renderFpsCmd.Flags().String("tab", "", "Target tab ID")
	renderPaintFlashCmd.Flags().String("tab", "", "Target tab ID")
	renderLayersCmd.Flags().String("tab", "", "Target tab ID")
	renderBordersCmd.Flags().String("tab", "", "Target tab ID")
	renderHighlightCmd.Flags().String("tab", "", "Target tab ID")
	renderClearCmd.Flags().String("tab", "", "Target tab ID")
	renderCompositingCmd.Flags().String("tab", "", "Target tab ID")
	renderScrollCmd.Flags().String("tab", "", "Target tab ID")
}

// Implementation functions

func measureFrameRate(ctx context.Context, duration int, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	renderCtrl := NewRenderingController(debugger, verbose)
	frameRateInfo, err := renderCtrl.GetFrameRate(ctx, duration)
	if err != nil {
		return err
	}

	fmt.Printf("Frame Rate Measurement (Duration: %dms)\n", duration)
	fmt.Println("========================================")

	if infoJSON, err := json.MarshalIndent(frameRateInfo, "", "  "); err == nil {
		fmt.Println(string(infoJSON))
	} else {
		fmt.Printf("%v\n", frameRateInfo)
	}

	// Show summary
	if fps, ok := frameRateInfo["fps"].(float64); ok {
		fmt.Printf("\n📊 Summary:\n")
		fmt.Printf("  Average FPS: %.2f\n", fps)

		if fps >= 60 {
			fmt.Printf("  Performance: 🟢 Excellent (60+ FPS)\n")
		} else if fps >= 30 {
			fmt.Printf("  Performance: 🟡 Good (30-60 FPS)\n")
		} else {
			fmt.Printf("  Performance: 🔴 Poor (<30 FPS)\n")
		}
	}

	return nil
}

func togglePaintFlashing(ctx context.Context, enabled bool, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	renderCtrl := NewRenderingController(debugger, verbose)
	if err := renderCtrl.EnablePaintFlashing(ctx, enabled); err != nil {
		return err
	}

	status := "disabled"
	if enabled {
		status = "enabled"
	}
	fmt.Printf("✓ Paint flashing %s\n", status)
	return nil
}

func toggleLayerBorders(ctx context.Context, enabled bool, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	renderCtrl := NewRenderingController(debugger, verbose)
	if err := renderCtrl.ShowLayerBorders(ctx, enabled); err != nil {
		return err
	}

	status := "hidden"
	if enabled {
		status = "shown"
	}
	fmt.Printf("✓ Layer borders %s\n", status)
	return nil
}

func showCompositingLayers(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	renderCtrl := NewRenderingController(debugger, verbose)
	layers, err := renderCtrl.GetLayerTree(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Compositing Layers:")
	fmt.Println("==================")

	if len(layers) == 0 {
		fmt.Println("  (no layers detected)")
		return nil
	}

	for i, layer := range layers {
		fmt.Printf("\n[%d] Layer:\n", i+1)
		if layerJSON, err := json.MarshalIndent(layer, "  ", "  "); err == nil {
			fmt.Printf("  %s\n", string(layerJSON))
		} else {
			fmt.Printf("  %v\n", layer)
		}
	}

	fmt.Printf("\n📊 Total layers: %d\n", len(layers))
	return nil
}

func highlightRenderingElement(ctx context.Context, selector, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	renderCtrl := NewRenderingController(debugger, verbose)
	if err := renderCtrl.HighlightElement(ctx, selector); err != nil {
		return err
	}

	fmt.Printf("✓ Highlighted element: %s\n", selector)
	return nil
}

func clearRenderingHighlights(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	renderCtrl := NewRenderingController(debugger, verbose)
	if err := renderCtrl.ClearHighlight(ctx); err != nil {
		return err
	}

	fmt.Println("✓ Cleared all rendering highlights")
	return nil
}

func showCompositingReasons(ctx context.Context, selector, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	renderCtrl := NewRenderingController(debugger, verbose)
	compositingInfo, err := renderCtrl.GetCompositingReasons(ctx, selector)
	if err != nil {
		return err
	}

	fmt.Printf("Compositing Analysis for: %s\n", selector)
	fmt.Println("==============================")

	if infoJSON, err := json.MarshalIndent(compositingInfo, "", "  "); err == nil {
		fmt.Println(string(infoJSON))
	} else {
		fmt.Printf("%v\n", compositingInfo)
	}

	// Show summary if triggers are available
	if triggers, ok := compositingInfo["compositingTriggers"].(map[string]interface{}); ok {
		fmt.Printf("\n📊 Compositing Triggers:\n")
		for trigger, active := range triggers {
			if isActive, ok := active.(bool); ok && isActive {
				fmt.Printf("  ✓ %s\n", trigger)
			}
		}
	}

	return nil
}

func showScrollBottlenecks(ctx context.Context, enabled bool, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	renderCtrl := NewRenderingController(debugger, verbose)
	if err := renderCtrl.ShowScrollBottlenecks(ctx, enabled); err != nil {
		return err
	}

	status := "hidden"
	if enabled {
		status = "highlighted"
	}
	fmt.Printf("✓ Scroll bottlenecks %s\n", status)
	return nil
}