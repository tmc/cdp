package main

import (
	"context"
	"fmt"
	"log"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/device"
	"github.com/spf13/cobra"
)

var (
	emulateDeviceName string
	emulateWidth      int64
	emulateHeight     int64
	emulateUA         string
)

var emulateCmd = &cobra.Command{
	Use:   "emulate",
	Short: "Emulate a device or custom metrics",
	Long:  `Sets device metrics and User-Agent to emulate a specific device (e.g., "iPhone 12") or custom viewport.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := runEmulation(ctx, tabID); err != nil {
			log.Fatalf("Emulation failed: %v", err)
		}
	},
}

func init() {
	emulateCmd.Flags().String("tab", "", "Target tab ID")
	emulateCmd.Flags().StringVar(&emulateDeviceName, "device", "", "Device name to emulate (e.g., 'iPhone 12')")
	emulateCmd.Flags().Int64Var(&emulateWidth, "width", 0, "Custom width (if device not specified)")
	emulateCmd.Flags().Int64Var(&emulateHeight, "height", 0, "Custom height (if device not specified)")
	emulateCmd.Flags().StringVar(&emulateUA, "user-agent", "", "Custom User-Agent")
}

func runEmulation(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	var actions []chromedp.Action

	if emulateDeviceName != "" {
		// Use chromedp/device to look up the device
		// Note: chromedp/device defines constants/types, but might not have a string lookup map exported directly
		// or easily accessible without a huge switch.
		// Actually, device.Info isn't a map, it's a type.
		// We might need to implement a simple lookup for common devices or check how chromedp does it.
		// For simplicity/MVP, let's support a few hardcoded popular ones found in device package
		// or allow the user to match exactly if we can iterate key mapping.
		// `device.IPhone12` is a variable of type Info.

		d, ok := lookupDevice(emulateDeviceName)
		if !ok {
			return fmt.Errorf("unknown device: %s", emulateDeviceName)
		}
		info := d.Device()
		actions = append(actions, chromedp.Emulate(info))
		log.Printf("Emulating %s: %dx%d (UA: %s)", emulateDeviceName, info.Width, info.Height, info.UserAgent)

	} else {
		// Custom metrics
		if emulateWidth > 0 && emulateHeight > 0 {
			actions = append(actions, emulation.SetDeviceMetricsOverride(emulateWidth, emulateHeight, 1.0, false))
			log.Printf("Setting metrics: %dx%d", emulateWidth, emulateHeight)
		}
		if emulateUA != "" {
			actions = append(actions, emulation.SetUserAgentOverride(emulateUA))
			log.Printf("Setting User-Agent: %s", emulateUA)
		}
	}

	if len(actions) == 0 {
		return fmt.Errorf("please specify --device or --width/--height/--user-agent")
	}

	// Apply emulation
	if err := chromedp.Run(debugger.chromeCtx, actions...); err != nil {
		return fmt.Errorf("failed to set emulation: %w", err)
	}

	log.Println("Emulation applied successfully.")
	return nil
}

// lookupDevice maps a string name to device.Info.
// Expanding this list as needed.
func lookupDevice(name string) (chromedp.Device, bool) {
	switch name {
	case "iPhone 12":
		return device.IPhone12, true
	case "iPhone 12 Pro":
		return device.IPhone12Pro, true
	case "Pixel 5":
		return device.Pixel5, true
	case "iPad Pro":
		return device.IPadPro, true
	default:
		return nil, false
	}
}
