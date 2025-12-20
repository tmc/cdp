package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/spf13/cobra"
)

// Animation commands
var animationCmd = &cobra.Command{
	Use:     "animation",
	Aliases: []string{"anim"},
	Short:   "Animation debugging and control",
	Long:    "Debug, control, and analyze CSS animations and Web Animations API",
}

var animationListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active animations",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := listAnimations(ctx, tabID); err != nil {
			log.Fatalf("Failed to list animations: %v", err)
		}
	},
}

var animationPauseCmd = &cobra.Command{
	Use:   "pause [animation-id]",
	Short: "Pause animation(s)",
	Long:  "Pause a specific animation by ID, or all animations if no ID provided",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if len(args) == 0 {
			// Pause all animations
			if err := pauseAllAnimations(ctx, tabID); err != nil {
				log.Fatalf("Failed to pause animations: %v", err)
			}
		} else {
			// Pause specific animation
			if err := pauseAnimation(ctx, args[0], tabID); err != nil {
				log.Fatalf("Failed to pause animation: %v", err)
			}
		}
	},
}

var animationResumeCmd = &cobra.Command{
	Use:   "resume [animation-id]",
	Short: "Resume animation(s)",
	Long:  "Resume a specific animation by ID, or all animations if no ID provided",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if len(args) == 0 {
			// Resume all animations
			if err := resumeAllAnimations(ctx, tabID); err != nil {
				log.Fatalf("Failed to resume animations: %v", err)
			}
		} else {
			// Resume specific animation
			if err := resumeAnimation(ctx, args[0], tabID); err != nil {
				log.Fatalf("Failed to resume animation: %v", err)
			}
		}
	},
}

var animationSpeedCmd = &cobra.Command{
	Use:   "speed <rate>",
	Short: "Set animation playback speed",
	Long:  "Set the playback rate for all animations (1.0 = normal, 0.5 = half speed, 2.0 = double speed)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		speed, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			log.Fatalf("Invalid speed value: %v", err)
		}

		if err := setAnimationSpeed(ctx, speed, tabID); err != nil {
			log.Fatalf("Failed to set animation speed: %v", err)
		}
	},
}

var animationInspectCmd = &cobra.Command{
	Use:   "inspect <animation-id>",
	Short: "Inspect animation details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := inspectAnimation(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to inspect animation: %v", err)
		}
	},
}

var animationTimelineCmd = &cobra.Command{
	Use:   "timeline",
	Short: "Show animation timeline",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := showAnimationTimeline(ctx, tabID); err != nil {
			log.Fatalf("Failed to show animation timeline: %v", err)
		}
	},
}

func init() {
	// Add subcommands to animation command
	animationCmd.AddCommand(animationListCmd)
	animationCmd.AddCommand(animationPauseCmd)
	animationCmd.AddCommand(animationResumeCmd)
	animationCmd.AddCommand(animationSpeedCmd)
	animationCmd.AddCommand(animationInspectCmd)
	animationCmd.AddCommand(animationTimelineCmd)

	// Add flags
	animationListCmd.Flags().String("tab", "", "Target tab ID")
	animationPauseCmd.Flags().String("tab", "", "Target tab ID")
	animationResumeCmd.Flags().String("tab", "", "Target tab ID")
	animationSpeedCmd.Flags().String("tab", "", "Target tab ID")
	animationInspectCmd.Flags().String("tab", "", "Target tab ID")
	animationTimelineCmd.Flags().String("tab", "", "Target tab ID")
}

// Implementation functions

func listAnimations(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	animCtrl := NewAnimationController(debugger, verbose)
	animations, err := animCtrl.ListAnimations(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Active Animations:")
	fmt.Println("==================")

	if len(animations) == 0 {
		fmt.Println("  (no active animations)")
		return nil
	}

	for i, anim := range animations {
		fmt.Printf("\n[%d] Animation:\n", i+1)

		// Show key information in a formatted way
		if id, ok := anim["id"].(string); ok {
			fmt.Printf("  ID: %s\n", id)
		}
		if animType, ok := anim["type"].(string); ok {
			fmt.Printf("  Type: %s\n", animType)
		}
		if playState, ok := anim["playState"].(string); ok {
			fmt.Printf("  State: %s\n", playState)
		}
		if name, ok := anim["animationName"].(string); ok {
			fmt.Printf("  Name: %s\n", name)
		}

		// Show target element if available
		if target, ok := anim["target"].(map[string]interface{}); ok {
			if tagName, ok := target["tagName"].(string); ok {
				fmt.Printf("  Target: <%s", tagName)
				if id, ok := target["id"].(string); ok && id != "" {
					fmt.Printf(" id=\"%s\"", id)
				}
				if className, ok := target["className"].(string); ok && className != "" {
					fmt.Printf(" class=\"%s\"", className)
				}
				fmt.Printf(">\n")
			}
		}

		// Show timing information if available
		if effect, ok := anim["effect"].(map[string]interface{}); ok {
			if duration, ok := effect["duration"]; ok {
				fmt.Printf("  Duration: %v\n", duration)
			}
			if iterations, ok := effect["iterations"]; ok {
				fmt.Printf("  Iterations: %v\n", iterations)
			}
		}

		fmt.Printf("  Full details: %s\n", formatJSON(anim))
	}

	fmt.Printf("\n📊 Total animations: %d\n", len(animations))
	return nil
}

func pauseAnimation(ctx context.Context, animationID, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	animCtrl := NewAnimationController(debugger, verbose)
	if err := animCtrl.PauseAnimation(ctx, animationID); err != nil {
		return err
	}

	fmt.Printf("✓ Animation paused: %s\n", animationID)
	return nil
}

func resumeAnimation(ctx context.Context, animationID, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	animCtrl := NewAnimationController(debugger, verbose)
	if err := animCtrl.ResumeAnimation(ctx, animationID); err != nil {
		return err
	}

	fmt.Printf("✓ Animation resumed: %s\n", animationID)
	return nil
}

func pauseAllAnimations(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	animCtrl := NewAnimationController(debugger, verbose)
	if err := animCtrl.PauseAllAnimations(ctx); err != nil {
		return err
	}

	fmt.Println("✓ All animations paused")
	return nil
}

func resumeAllAnimations(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	animCtrl := NewAnimationController(debugger, verbose)
	if err := animCtrl.ResumeAllAnimations(ctx); err != nil {
		return err
	}

	fmt.Println("✓ All animations resumed")
	return nil
}

func setAnimationSpeed(ctx context.Context, speed float64, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	animCtrl := NewAnimationController(debugger, verbose)
	if err := animCtrl.SetAnimationSpeed(ctx, speed); err != nil {
		return err
	}

	fmt.Printf("✓ Animation speed set to: %.2fx\n", speed)
	return nil
}

func inspectAnimation(ctx context.Context, animationID, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	animCtrl := NewAnimationController(debugger, verbose)
	animationInfo, err := animCtrl.InspectAnimation(ctx, animationID)
	if err != nil {
		return err
	}

	fmt.Printf("Animation Details: %s\n", animationID)
	fmt.Println("===================")

	// Show formatted information
	if animType, ok := animationInfo["type"].(string); ok {
		fmt.Printf("Type: %s\n", animType)
	}
	if playState, ok := animationInfo["playState"].(string); ok {
		fmt.Printf("Play State: %s\n", playState)
	}
	if playbackRate, ok := animationInfo["playbackRate"].(float64); ok {
		fmt.Printf("Playback Rate: %.2fx\n", playbackRate)
	}

	// Show timing information
	if effect, ok := animationInfo["effect"].(map[string]interface{}); ok {
		fmt.Println("\nTiming:")
		for key, value := range effect {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}

	// Show target information
	if target, ok := animationInfo["target"].(map[string]interface{}); ok {
		fmt.Println("\nTarget Element:")
		for key, value := range target {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}

	fmt.Println("\nFull Details:")
	fmt.Println(formatJSON(animationInfo))

	return nil
}

func showAnimationTimeline(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	animCtrl := NewAnimationController(debugger, verbose)
	timeline, err := animCtrl.CreateAnimationTimeline(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Animation Timeline:")
	fmt.Println("==================")

	if totalAnims, ok := timeline["totalAnimations"].(float64); ok {
		fmt.Printf("Total Animations: %.0f\n", totalAnims)
	}

	if currentTime, ok := timeline["currentTime"].(float64); ok {
		fmt.Printf("Current Time: %.2fms\n", currentTime)
	}

	if animations, ok := timeline["animations"].([]interface{}); ok {
		fmt.Println("\nTimeline:")
		for i, anim := range animations {
			if animMap, ok := anim.(map[string]interface{}); ok {
				fmt.Printf("\n[%d] ", i+1)

				if id, ok := animMap["id"].(string); ok {
					fmt.Printf("ID: %s", id)
				}

				if playState, ok := animMap["playState"].(string); ok {
					fmt.Printf(" | State: %s", playState)
				}

				if timelineInfo, ok := animMap["timeline"].(map[string]interface{}); ok {
					if progress, ok := timelineInfo["percentComplete"].(string); ok {
						fmt.Printf(" | Progress: %s", progress)
					}
				}

				fmt.Println()
			}
		}
	}

	fmt.Println("\nFull Timeline Data:")
	fmt.Println(formatJSON(timeline))

	return nil
}

// Helper function to format JSON for display
func formatJSON(data interface{}) string {
	if jsonBytes, err := json.MarshalIndent(data, "", "  "); err == nil {
		return string(jsonBytes)
	}
	return fmt.Sprintf("%v", data)
}