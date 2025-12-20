package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/spf13/cobra"
)

// Device emulation commands
var deviceCmd = &cobra.Command{
	Use:   "device",
	Short: "Device emulation and touch simulation",
	Long:  "Emulate mobile devices, simulate touch events, and manage device settings",
}

var deviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available device profiles",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()

		debugger := NewChromeDebugger(port, verbose)
		defer debugger.Close()

		if err := debugger.Connect(ctx, ""); err != nil {
			log.Fatalf("Failed to connect to Chrome: %v", err)
		}

		deviceCtrl := NewDeviceController(debugger, verbose)
		profiles := deviceCtrl.ListDeviceProfiles()

		fmt.Println("Available Device Profiles:")
		fmt.Println("========================")
		for _, profile := range profiles {
			fmt.Printf("\nProfile: %s\n", profile.Name)
			fmt.Printf("  Dimensions: %dx%d\n", profile.Width, profile.Height)
			fmt.Printf("  Scale Factor: %.1f\n", profile.DeviceScaleFactor)
			fmt.Printf("  Mobile: %t\n", profile.Mobile)
			fmt.Printf("  Touch: %t\n", profile.TouchEnabled)
			fmt.Printf("  User Agent: %s\n", profile.UserAgent)
		}
	},
}

var deviceSetCmd = &cobra.Command{
	Use:   "set <profile>",
	Short: "Set device emulation profile",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := setDeviceProfile(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to set device profile: %v", err)
		}
	},
}

var deviceCustomCmd = &cobra.Command{
	Use:   "custom <width> <height> [scale-factor] [mobile] [touch]",
	Short: "Set custom device metrics",
	Args:  cobra.RangeArgs(2, 5),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		width, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			log.Fatalf("Invalid width: %v", err)
		}

		height, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			log.Fatalf("Invalid height: %v", err)
		}

		scaleFactor := 1.0
		if len(args) > 2 {
			if scaleFactor, err = strconv.ParseFloat(args[2], 64); err != nil {
				log.Fatalf("Invalid scale factor: %v", err)
			}
		}

		mobile := false
		if len(args) > 3 {
			if mobile, err = strconv.ParseBool(args[3]); err != nil {
				log.Fatalf("Invalid mobile flag: %v", err)
			}
		}

		touch := false
		if len(args) > 4 {
			if touch, err = strconv.ParseBool(args[4]); err != nil {
				log.Fatalf("Invalid touch flag: %v", err)
			}
		}

		if err := setCustomDevice(ctx, width, height, scaleFactor, mobile, touch, tabID); err != nil {
			log.Fatalf("Failed to set custom device: %v", err)
		}
	},
}

var deviceResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset device emulation to desktop",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := resetDevice(ctx, tabID); err != nil {
			log.Fatalf("Failed to reset device: %v", err)
		}
	},
}

var deviceInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Get current device information",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := getDeviceInfo(ctx, tabID); err != nil {
			log.Fatalf("Failed to get device info: %v", err)
		}
	},
}

var deviceTouchCmd = &cobra.Command{
	Use:   "touch <x> <y> [type]",
	Short: "Simulate touch event at coordinates",
	Long:  "Simulate touch event. Type can be: start, move, end, cancel (default: tap)",
	Args:  cobra.RangeArgs(2, 3),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		x, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			log.Fatalf("Invalid x coordinate: %v", err)
		}

		y, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			log.Fatalf("Invalid y coordinate: %v", err)
		}

		touchType := "tap"
		if len(args) > 2 {
			touchType = args[2]
		}

		if err := simulateTouch(ctx, x, y, touchType, tabID); err != nil {
			log.Fatalf("Failed to simulate touch: %v", err)
		}
	},
}

var deviceSwipeCmd = &cobra.Command{
	Use:   "swipe <startX> <startY> <endX> <endY> [duration]",
	Short: "Simulate swipe gesture",
	Args:  cobra.RangeArgs(4, 5),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		startX, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			log.Fatalf("Invalid start X coordinate: %v", err)
		}

		startY, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			log.Fatalf("Invalid start Y coordinate: %v", err)
		}

		endX, err := strconv.ParseFloat(args[2], 64)
		if err != nil {
			log.Fatalf("Invalid end X coordinate: %v", err)
		}

		endY, err := strconv.ParseFloat(args[3], 64)
		if err != nil {
			log.Fatalf("Invalid end Y coordinate: %v", err)
		}

		duration := 500 // default 500ms
		if len(args) > 4 {
			if duration, err = strconv.Atoi(args[4]); err != nil {
				log.Fatalf("Invalid duration: %v", err)
			}
		}

		if err := simulateSwipe(ctx, startX, startY, endX, endY, duration, tabID); err != nil {
			log.Fatalf("Failed to simulate swipe: %v", err)
		}
	},
}

var deviceOrientationCmd = &cobra.Command{
	Use:   "orientation <type> [angle]",
	Short: "Set device orientation",
	Long:  "Set device orientation. Type: portrait-primary, portrait-secondary, landscape-primary, landscape-secondary",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		orientation := args[0]

		angle := int64(0)
		if len(args) > 1 {
			if parsedAngle, err := strconv.ParseInt(args[1], 10, 64); err != nil {
				log.Fatalf("Invalid angle: %v", err)
			} else {
				angle = parsedAngle
			}
		}

		if err := setOrientation(ctx, orientation, angle, tabID); err != nil {
			log.Fatalf("Failed to set orientation: %v", err)
		}
	},
}

func init() {
	// Add subcommands to device command
	deviceCmd.AddCommand(deviceListCmd)
	deviceCmd.AddCommand(deviceSetCmd)
	deviceCmd.AddCommand(deviceCustomCmd)
	deviceCmd.AddCommand(deviceResetCmd)
	deviceCmd.AddCommand(deviceInfoCmd)
	deviceCmd.AddCommand(deviceTouchCmd)
	deviceCmd.AddCommand(deviceSwipeCmd)
	deviceCmd.AddCommand(deviceOrientationCmd)

	// Add flags
	deviceSetCmd.Flags().String("tab", "", "Target tab ID")
	deviceCustomCmd.Flags().String("tab", "", "Target tab ID")
	deviceResetCmd.Flags().String("tab", "", "Target tab ID")
	deviceInfoCmd.Flags().String("tab", "", "Target tab ID")
	deviceTouchCmd.Flags().String("tab", "", "Target tab ID")
	deviceSwipeCmd.Flags().String("tab", "", "Target tab ID")
	deviceOrientationCmd.Flags().String("tab", "", "Target tab ID")
}

// Implementation functions

func setDeviceProfile(ctx context.Context, profileName, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	deviceCtrl := NewDeviceController(debugger, verbose)
	if err := deviceCtrl.SetDeviceProfile(ctx, profileName); err != nil {
		return err
	}

	fmt.Printf("✓ Device profile set to: %s\n", profileName)
	return nil
}

func setCustomDevice(ctx context.Context, width, height int64, scaleFactor float64, mobile, touch bool, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	deviceCtrl := NewDeviceController(debugger, verbose)
	if err := deviceCtrl.SetCustomDevice(ctx, width, height, scaleFactor, mobile, touch); err != nil {
		return err
	}

	fmt.Printf("✓ Custom device set: %dx%d (scale: %.1f, mobile: %t, touch: %t)\n",
		width, height, scaleFactor, mobile, touch)
	return nil
}

func resetDevice(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	deviceCtrl := NewDeviceController(debugger, verbose)
	if err := deviceCtrl.ResetDevice(ctx); err != nil {
		return err
	}

	fmt.Println("✓ Device emulation reset to desktop")
	return nil
}

func getDeviceInfo(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	deviceCtrl := NewDeviceController(debugger, verbose)
	info, err := deviceCtrl.GetDeviceInfo(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Current Device Information:")
	fmt.Println("==========================")
	if infoJSON, err := json.MarshalIndent(info, "", "  "); err == nil {
		fmt.Println(string(infoJSON))
	} else {
		fmt.Printf("%v\n", info)
	}

	return nil
}

func simulateTouch(ctx context.Context, x, y float64, touchType, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	deviceCtrl := NewDeviceController(debugger, verbose)

	if touchType == "tap" {
		if err := deviceCtrl.Tap(ctx, x, y); err != nil {
			return err
		}
		fmt.Printf("✓ Tapped at coordinates (%.1f, %.1f)\n", x, y)
	} else {
		if err := deviceCtrl.SimulateTouch(ctx, x, y, touchType); err != nil {
			return err
		}
		fmt.Printf("✓ Touch %s at coordinates (%.1f, %.1f)\n", touchType, x, y)
	}

	return nil
}

func simulateSwipe(ctx context.Context, startX, startY, endX, endY float64, duration int, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	deviceCtrl := NewDeviceController(debugger, verbose)
	if err := deviceCtrl.Swipe(ctx, startX, startY, endX, endY, duration); err != nil {
		return err
	}

	fmt.Printf("✓ Swiped from (%.1f, %.1f) to (%.1f, %.1f) in %dms\n",
		startX, startY, endX, endY, duration)
	return nil
}

func setOrientation(ctx context.Context, orientation string, angle int64, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	deviceCtrl := NewDeviceController(debugger, verbose)
	if err := deviceCtrl.SetOrientation(ctx, orientation, angle); err != nil {
		return err
	}

	fmt.Printf("✓ Orientation set to: %s (angle: %d°)\n", orientation, angle)
	return nil
}