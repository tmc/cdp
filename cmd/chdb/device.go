package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pkg/errors"
)

// DeviceController handles device emulation and touch operations
type DeviceController struct {
	debugger *ChromeDebugger
	verbose  bool
}

// DeviceProfile represents a mobile device profile
type DeviceProfile struct {
	Name       string
	Width      int64
	Height     int64
	DeviceScaleFactor float64
	Mobile     bool
	TouchEnabled bool
	UserAgent  string
}

// Predefined device profiles
var deviceProfiles = map[string]DeviceProfile{
	"iphone-14-pro": {
		Name: "iPhone 14 Pro",
		Width: 393, Height: 852,
		DeviceScaleFactor: 3.0,
		Mobile: true, TouchEnabled: true,
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1",
	},
	"iphone-se": {
		Name: "iPhone SE",
		Width: 375, Height: 667,
		DeviceScaleFactor: 2.0,
		Mobile: true, TouchEnabled: true,
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 15_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.0 Mobile/15E148 Safari/604.1",
	},
	"ipad-air": {
		Name: "iPad Air",
		Width: 820, Height: 1180,
		DeviceScaleFactor: 2.0,
		Mobile: true, TouchEnabled: true,
		UserAgent: "Mozilla/5.0 (iPad; CPU OS 15_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.0 Mobile/15E148 Safari/604.1",
	},
	"pixel-7": {
		Name: "Pixel 7",
		Width: 412, Height: 915,
		DeviceScaleFactor: 2.625,
		Mobile: true, TouchEnabled: true,
		UserAgent: "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Mobile Safari/537.36",
	},
	"galaxy-s23": {
		Name: "Galaxy S23",
		Width: 384, Height: 854,
		DeviceScaleFactor: 3.0,
		Mobile: true, TouchEnabled: true,
		UserAgent: "Mozilla/5.0 (Linux; Android 13; SM-S911B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Mobile Safari/537.36",
	},
	"desktop": {
		Name: "Desktop",
		Width: 1920, Height: 1080,
		DeviceScaleFactor: 1.0,
		Mobile: false, TouchEnabled: false,
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36",
	},
}

// NewDeviceController creates a new device controller
func NewDeviceController(debugger *ChromeDebugger, verbose bool) *DeviceController {
	return &DeviceController{
		debugger: debugger,
		verbose:  verbose,
	}
}

// ListDeviceProfiles lists available device profiles
func (dc *DeviceController) ListDeviceProfiles() []DeviceProfile {
	profiles := make([]DeviceProfile, 0, len(deviceProfiles))
	for _, profile := range deviceProfiles {
		profiles = append(profiles, profile)
	}
	return profiles
}

// SetDeviceProfile applies a device profile for emulation
func (dc *DeviceController) SetDeviceProfile(ctx context.Context, profileName string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	profile, exists := deviceProfiles[profileName]
	if !exists {
		return errors.Errorf("unknown device profile: %s", profileName)
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Set device metrics override
			if err := emulation.SetDeviceMetricsOverride(
				profile.Width,
				profile.Height,
				profile.DeviceScaleFactor,
				profile.Mobile,
			).WithScreenOrientation(&emulation.ScreenOrientation{
				Type:  emulation.OrientationTypePortraitPrimary,
				Angle: 0,
			}).Do(ctx); err != nil {
				return err
			}

			// Set touch emulation
			if profile.TouchEnabled {
				if err := emulation.SetTouchEmulationEnabled(true).Do(ctx); err != nil {
					return err
				}
			}

			// Set user agent override
			if err := emulation.SetUserAgentOverride(profile.UserAgent).Do(ctx); err != nil {
				return err
			}

			return nil
		}),
	)
}

// SetCustomDevice sets custom device metrics
func (dc *DeviceController) SetCustomDevice(ctx context.Context, width, height int64, scaleFactor float64, mobile, touch bool) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Set device metrics override
			if err := emulation.SetDeviceMetricsOverride(
				width,
				height,
				scaleFactor,
				mobile,
			).Do(ctx); err != nil {
				return err
			}

			// Set touch emulation
			if touch {
				if err := emulation.SetTouchEmulationEnabled(true).Do(ctx); err != nil {
					return err
				}
			}

			return nil
		}),
	)
}

// ResetDevice resets device emulation to desktop
func (dc *DeviceController) ResetDevice(ctx context.Context) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Clear device metrics override
			if err := emulation.ClearDeviceMetricsOverride().Do(ctx); err != nil {
				return err
			}

			// Disable touch emulation
			if err := emulation.SetTouchEmulationEnabled(false).Do(ctx); err != nil {
				return err
			}

			// Reset user agent
			if err := emulation.SetUserAgentOverride("").Do(ctx); err != nil {
				return err
			}

			return nil
		}),
	)
}

// SimulateTouch simulates a touch event at the specified coordinates
func (dc *DeviceController) SimulateTouch(ctx context.Context, x, y float64, touchType string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Use JavaScript to simulate touch events
			expression := fmt.Sprintf(`
				(function() {
					const touchType = '%s';
					const x = %f;
					const y = %f;

					let eventType;
					switch (touchType) {
						case 'start':
							eventType = 'touchstart';
							break;
						case 'move':
							eventType = 'touchmove';
							break;
						case 'end':
							eventType = 'touchend';
							break;
						case 'cancel':
							eventType = 'touchcancel';
							break;
						default:
							return { error: 'Invalid touch type: ' + touchType };
					}

					const touch = new Touch({
						identifier: 1,
						target: document.elementFromPoint(x, y) || document.body,
						clientX: x,
						clientY: y,
						radiusX: 11,
						radiusY: 11,
						rotationAngle: 0,
						force: 1
					});

					const touchEvent = new TouchEvent(eventType, {
						cancelable: true,
						bubbles: true,
						touches: eventType === 'touchend' || eventType === 'touchcancel' ? [] : [touch],
						targetTouches: eventType === 'touchend' || eventType === 'touchcancel' ? [] : [touch],
						changedTouches: [touch]
					});

					const target = document.elementFromPoint(x, y) || document.body;
					target.dispatchEvent(touchEvent);

					return { success: true, type: eventType, x: x, y: y };
				})()
			`, touchType, x, y)

			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}

// Tap simulates a tap at the specified coordinates
func (dc *DeviceController) Tap(ctx context.Context, x, y float64) error {
	if err := dc.SimulateTouch(ctx, x, y, "start"); err != nil {
		return err
	}

	return dc.SimulateTouch(ctx, x, y, "end")
}

// Swipe simulates a swipe gesture from start to end coordinates
func (dc *DeviceController) Swipe(ctx context.Context, startX, startY, endX, endY float64, duration int) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	// Use JavaScript for smooth swipe animation
	expression := fmt.Sprintf(`
		(async function() {
			function sleep(ms) {
				return new Promise(resolve => setTimeout(resolve, ms));
			}

			const startX = %f;
			const startY = %f;
			const endX = %f;
			const endY = %f;
			const duration = %d;
			const steps = Math.max(10, duration / 16); // ~60fps

			const deltaX = (endX - startX) / steps;
			const deltaY = (endY - startY) / steps;

			// Dispatch touch start
			const touchStart = new TouchEvent('touchstart', {
				touches: [{
					clientX: startX,
					clientY: startY,
					identifier: 1
				}]
			});
			document.dispatchEvent(touchStart);

			// Animate swipe
			for (let i = 1; i <= steps; i++) {
				const currentX = startX + (deltaX * i);
				const currentY = startY + (deltaY * i);

				const touchMove = new TouchEvent('touchmove', {
					touches: [{
						clientX: currentX,
						clientY: currentY,
						identifier: 1
					}]
				});
				document.dispatchEvent(touchMove);

				await sleep(duration / steps);
			}

			// Dispatch touch end
			const touchEnd = new TouchEvent('touchend', {
				changedTouches: [{
					clientX: endX,
					clientY: endY,
					identifier: 1
				}]
			});
			document.dispatchEvent(touchEnd);

			return { success: true };
		})()
	`, startX, startY, endX, endY, duration)

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, err := runtime.Evaluate(expression).WithAwaitPromise(true).Do(ctx)
			return err
		}),
	)
}

// GetDeviceInfo gets current device emulation information
func (dc *DeviceController) GetDeviceInfo(ctx context.Context) (map[string]interface{}, error) {
	if !dc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var deviceInfo map[string]interface{}
	err := chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := `
				(function() {
					return {
						userAgent: navigator.userAgent,
						platform: navigator.platform,
						vendor: navigator.vendor,
						cookieEnabled: navigator.cookieEnabled,
						language: navigator.language,
						languages: navigator.languages,
						onLine: navigator.onLine,
						screen: {
							width: screen.width,
							height: screen.height,
							availWidth: screen.availWidth,
							availHeight: screen.availHeight,
							colorDepth: screen.colorDepth,
							pixelDepth: screen.pixelDepth,
							orientation: screen.orientation ? {
								type: screen.orientation.type,
								angle: screen.orientation.angle
							} : null
						},
						window: {
							innerWidth: window.innerWidth,
							innerHeight: window.innerHeight,
							outerWidth: window.outerWidth,
							outerHeight: window.outerHeight,
							devicePixelRatio: window.devicePixelRatio
						},
						touchSupport: {
							maxTouchPoints: navigator.maxTouchPoints,
							touchEvent: 'TouchEvent' in window,
							touchStart: 'ontouchstart' in window
						},
						mediaQueries: {
							mobile: window.matchMedia('(max-width: 768px)').matches,
							tablet: window.matchMedia('(min-width: 769px) and (max-width: 1024px)').matches,
							desktop: window.matchMedia('(min-width: 1025px)').matches,
							portrait: window.matchMedia('(orientation: portrait)').matches,
							landscape: window.matchMedia('(orientation: landscape)').matches,
							retina: window.matchMedia('(-webkit-min-device-pixel-ratio: 2)').matches
						}
					};
				})()
			`

			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				var result interface{}
				if json.Unmarshal(res.Value, &result) == nil {
					if resultMap, ok := result.(map[string]interface{}); ok {
						deviceInfo = resultMap
					}
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to get device info")
	}

	return deviceInfo, nil
}

// SetOrientation changes device orientation
func (dc *DeviceController) SetOrientation(ctx context.Context, orientation string, angle int64) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	var orientationType emulation.OrientationType
	switch orientation {
	case "portrait-primary":
		orientationType = emulation.OrientationTypePortraitPrimary
	case "portrait-secondary":
		orientationType = emulation.OrientationTypePortraitSecondary
	case "landscape-primary":
		orientationType = emulation.OrientationTypeLandscapePrimary
	case "landscape-secondary":
		orientationType = emulation.OrientationTypeLandscapeSecondary
	default:
		return errors.Errorf("invalid orientation: %s (use: portrait-primary, portrait-secondary, landscape-primary, landscape-secondary)", orientation)
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetDeviceMetricsOverride(0, 0, 0, false).
				WithScreenOrientation(&emulation.ScreenOrientation{
					Type:  orientationType,
					Angle: angle,
				}).Do(ctx)
		}),
	)
}