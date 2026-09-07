package browser

import "github.com/tmc/cdp/internal/chromedp"

// OptimizationGuideOnDeviceModelFeatures is the Chromium feature list for
// chrome://flags/#optimization-guide-on-device-model on desktop.
const OptimizationGuideOnDeviceModelFeatures = "OptimizationGuideOnDeviceModel,OnDeviceModelPerformanceParams"

// EnableOptimizationGuideOnDeviceModel enables on-device Optimization Guide
// model execution for a Chrome launch.
func EnableOptimizationGuideOnDeviceModel() chromedp.ExecAllocatorOption {
	return chromedp.Flag("enable-features", OptimizationGuideOnDeviceModelFeatures)
}
