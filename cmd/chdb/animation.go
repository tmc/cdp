package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/cdproto/animation"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pkg/errors"
)

// AnimationController handles animation debugging and control
type AnimationController struct {
	debugger   *ChromeDebugger
	verbose    bool
	animations map[string]*animation.Animation
}

// NewAnimationController creates a new animation controller
func NewAnimationController(debugger *ChromeDebugger, verbose bool) *AnimationController {
	return &AnimationController{
		debugger:   debugger,
		verbose:    verbose,
		animations: make(map[string]*animation.Animation),
	}
}

// ListAnimations gets all active animations
func (ac *AnimationController) ListAnimations(ctx context.Context) ([]map[string]interface{}, error) {
	if !ac.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var animations []map[string]interface{}
	err := chromedp.Run(ac.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Try to enable animation domain
			if err := animation.Enable().Do(ctx); err != nil {
				// If animation domain is not available, use JavaScript fallback
				expression := `
					(function() {
						const animations = [];

						// Get CSS animations
						document.getAnimations().forEach((anim, index) => {
							const animData = {
								id: 'js-' + index,
								type: anim.constructor.name,
								playState: anim.playState,
								startTime: anim.startTime,
								currentTime: anim.currentTime,
								playbackRate: anim.playbackRate,
								source: 'javascript-api'
							};

							if (anim.effect) {
								animData.effect = {
									duration: anim.effect.getComputedTiming().duration,
									iterations: anim.effect.getComputedTiming().iterations,
									direction: anim.effect.getComputedTiming().direction,
									fill: anim.effect.getComputedTiming().fill,
									easing: anim.effect.getComputedTiming().easing
								};

								if (anim.effect.target) {
									animData.target = {
										tagName: anim.effect.target.tagName,
										id: anim.effect.target.id,
										className: anim.effect.target.className
									};
								}
							}

							if (anim.animationName) {
								animData.animationName = anim.animationName;
							}

							animations.push(animData);
						});

						// Also check for transition animations
						const elements = document.querySelectorAll('*');
						elements.forEach((el, index) => {
							const computedStyle = window.getComputedStyle(el);
							if (computedStyle.transition !== 'all 0s ease 0s' && computedStyle.transition !== 'none') {
								animations.push({
									id: 'transition-' + index,
									type: 'CSSTransition',
									target: {
										tagName: el.tagName,
										id: el.id,
										className: el.className
									},
									transition: computedStyle.transition,
									source: 'css-transition'
								});
							}
						});

						return animations;
					})()
				`

				res, _, err := runtime.Evaluate(expression).Do(ctx)
				if err != nil {
					return err
				}

				if res.Value != nil {
					var result interface{}
					if json.Unmarshal(res.Value, &result) == nil {
						if animArray, ok := result.([]interface{}); ok {
							for _, anim := range animArray {
								if animMap, ok := anim.(map[string]interface{}); ok {
									animations = append(animations, animMap)
								}
							}
						}
					}
				}

				return nil
			}

			// If animation domain is available, use it (this is newer and might not be supported everywhere)
			// For now, stick with JavaScript fallback as it's more reliable
			return nil
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to list animations")
	}

	return animations, nil
}

// PauseAnimation pauses a specific animation
func (ac *AnimationController) PauseAnimation(ctx context.Context, animationID string) error {
	if !ac.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(ac.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := fmt.Sprintf(`
				(function() {
					const animations = document.getAnimations();

					// Try to find animation by ID
					let targetAnim = null;
					if ('%s'.startsWith('js-')) {
						const index = parseInt('%s'.replace('js-', ''));
						targetAnim = animations[index];
					} else {
						// Try to find by animation name
						targetAnim = animations.find(anim =>
							anim.animationName === '%s' ||
							anim.id === '%s'
						);
					}

					if (targetAnim) {
						targetAnim.pause();
						return {
							success: true,
							animationId: '%s',
							playState: targetAnim.playState
						};
					} else {
						return { error: 'Animation not found: %s' };
					}
				})()
			`, animationID, animationID, animationID, animationID, animationID, animationID)

			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			var result interface{}
			if res.Value != nil {
				json.Unmarshal(res.Value, &result)
			}

			// Check for errors
			if resultMap, ok := result.(map[string]interface{}); ok {
				if errorMsg, hasError := resultMap["error"]; hasError {
					return errors.New(errorMsg.(string))
				}
			}

			return nil
		}),
	)
}

// ResumeAnimation resumes a paused animation
func (ac *AnimationController) ResumeAnimation(ctx context.Context, animationID string) error {
	if !ac.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(ac.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := fmt.Sprintf(`
				(function() {
					const animations = document.getAnimations();

					let targetAnim = null;
					if ('%s'.startsWith('js-')) {
						const index = parseInt('%s'.replace('js-', ''));
						targetAnim = animations[index];
					} else {
						targetAnim = animations.find(anim =>
							anim.animationName === '%s' ||
							anim.id === '%s'
						);
					}

					if (targetAnim) {
						targetAnim.play();
						return {
							success: true,
							animationId: '%s',
							playState: targetAnim.playState
						};
					} else {
						return { error: 'Animation not found: %s' };
					}
				})()
			`, animationID, animationID, animationID, animationID, animationID, animationID)

			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			var result interface{}
			if res.Value != nil {
				json.Unmarshal(res.Value, &result)
			}

			if resultMap, ok := result.(map[string]interface{}); ok {
				if errorMsg, hasError := resultMap["error"]; hasError {
					return errors.New(errorMsg.(string))
				}
			}

			return nil
		}),
	)
}

// SetAnimationSpeed sets the playback rate for animations
func (ac *AnimationController) SetAnimationSpeed(ctx context.Context, speed float64) error {
	if !ac.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(ac.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := fmt.Sprintf(`
				(function() {
					const animations = document.getAnimations();
					let updatedCount = 0;

					animations.forEach(anim => {
						if (anim.playbackRate !== undefined) {
							anim.playbackRate = %f;
							updatedCount++;
						}
					});

					// Also set global animation speed for future animations
					if (document.documentElement.style.setProperty) {
						document.documentElement.style.setProperty('--animation-speed', '%fx');
					}

					return {
						success: true,
						speed: %f,
						updatedAnimations: updatedCount
					};
				})()
			`, speed, speed, speed)

			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			var result interface{}
			if res.Value != nil {
				json.Unmarshal(res.Value, &result)
			}

			return nil
		}),
	)
}

// PauseAllAnimations pauses all animations on the page
func (ac *AnimationController) PauseAllAnimations(ctx context.Context) error {
	if !ac.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(ac.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := `
				(function() {
					const animations = document.getAnimations();
					let pausedCount = 0;

					animations.forEach(anim => {
						if (anim.playState === 'running') {
							anim.pause();
							pausedCount++;
						}
					});

					// Also pause CSS animations by setting animation-play-state
					const style = document.createElement('style');
					style.id = 'chdb-pause-animations';
					style.textContent = '*, *::before, *::after { animation-play-state: paused !important; }';
					document.head.appendChild(style);

					return {
						success: true,
						pausedAnimations: pausedCount
					};
				})()
			`

			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}

// ResumeAllAnimations resumes all paused animations
func (ac *AnimationController) ResumeAllAnimations(ctx context.Context) error {
	if !ac.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(ac.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := `
				(function() {
					const animations = document.getAnimations();
					let resumedCount = 0;

					animations.forEach(anim => {
						if (anim.playState === 'paused') {
							anim.play();
							resumedCount++;
						}
					});

					// Remove CSS pause override
					const pauseStyle = document.getElementById('chdb-pause-animations');
					if (pauseStyle) {
						pauseStyle.remove();
					}

					return {
						success: true,
						resumedAnimations: resumedCount
					};
				})()
			`

			_, _, err := runtime.Evaluate(expression).Do(ctx)
			return err
		}),
	)
}

// InspectAnimation gets detailed information about a specific animation
func (ac *AnimationController) InspectAnimation(ctx context.Context, animationID string) (map[string]interface{}, error) {
	if !ac.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var animationInfo map[string]interface{}
	err := chromedp.Run(ac.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := fmt.Sprintf(`
				(function() {
					const animations = document.getAnimations();

					let targetAnim = null;
					if ('%s'.startsWith('js-')) {
						const index = parseInt('%s'.replace('js-', ''));
						targetAnim = animations[index];
					} else {
						targetAnim = animations.find(anim =>
							anim.animationName === '%s' ||
							anim.id === '%s'
						);
					}

					if (!targetAnim) {
						return { error: 'Animation not found: %s' };
					}

					const info = {
						id: '%s',
						type: targetAnim.constructor.name,
						playState: targetAnim.playState,
						startTime: targetAnim.startTime,
						currentTime: targetAnim.currentTime,
						playbackRate: targetAnim.playbackRate,
						ready: targetAnim.ready ? 'ready' : 'not-ready',
						finished: targetAnim.finished ? 'finished' : 'not-finished'
					};

					// Get effect details
					if (targetAnim.effect) {
						const timing = targetAnim.effect.getComputedTiming();
						info.effect = {
							duration: timing.duration,
							iterations: timing.iterations,
							direction: timing.direction,
							fill: timing.fill,
							easing: timing.easing,
							delay: timing.delay,
							endDelay: timing.endDelay,
							iterationStart: timing.iterationStart,
							activeDuration: timing.activeDuration,
							endTime: timing.endTime,
							localTime: timing.localTime,
							progress: timing.progress
						};

						// Get target element info
						if (targetAnim.effect.target) {
							const target = targetAnim.effect.target;
							const rect = target.getBoundingClientRect();
							info.target = {
								tagName: target.tagName,
								id: target.id,
								className: target.className,
								boundingRect: {
									x: rect.x,
									y: rect.y,
									width: rect.width,
									height: rect.height
								}
							};
						}

						// Get keyframes if available
						if (targetAnim.effect.getKeyframes) {
							try {
								info.keyframes = targetAnim.effect.getKeyframes();
							} catch (e) {
								info.keyframes = 'Not available: ' + e.message;
							}
						}
					}

					// Get timeline info
					if (targetAnim.timeline) {
						info.timeline = {
							currentTime: targetAnim.timeline.currentTime,
							duration: targetAnim.timeline.duration || 'auto'
						};
					}

					// Animation-specific properties
					if (targetAnim.animationName) {
						info.animationName = targetAnim.animationName;
					}

					if (targetAnim.transitionProperty) {
						info.transitionProperty = targetAnim.transitionProperty;
					}

					return info;
				})()
			`, animationID, animationID, animationID, animationID, animationID, animationID)

			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				var result interface{}
				if json.Unmarshal(res.Value, &result) == nil {
					if resultMap, ok := result.(map[string]interface{}); ok {
						if errorMsg, hasError := resultMap["error"]; hasError {
							return errors.New(errorMsg.(string))
						}
						animationInfo = resultMap
					}
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to inspect animation")
	}

	return animationInfo, nil
}

// CreateAnimationTimeline creates a visual timeline of animations
func (ac *AnimationController) CreateAnimationTimeline(ctx context.Context) (map[string]interface{}, error) {
	if !ac.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var timeline map[string]interface{}
	err := chromedp.Run(ac.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			expression := `
				(function() {
					const animations = document.getAnimations();
					const timeline = {
						totalAnimations: animations.length,
						currentTime: performance.now(),
						animations: []
					};

					animations.forEach((anim, index) => {
						const animData = {
							id: 'js-' + index,
							type: anim.constructor.name,
							playState: anim.playState,
							startTime: anim.startTime,
							currentTime: anim.currentTime,
							playbackRate: anim.playbackRate
						};

						if (anim.effect) {
							const timing = anim.effect.getComputedTiming();
							animData.timing = {
								duration: timing.duration,
								iterations: timing.iterations,
								delay: timing.delay,
								endDelay: timing.endDelay,
								activeDuration: timing.activeDuration,
								endTime: timing.endTime,
								progress: timing.progress
							};

							// Calculate timeline position
							if (timing.duration && timing.duration !== 'auto') {
								const totalDuration = timing.activeDuration || timing.duration;
								const currentProgress = timing.progress || 0;
								animData.timeline = {
									totalDuration: totalDuration,
									currentProgress: currentProgress,
									percentComplete: (currentProgress * 100).toFixed(2) + '%'
								};
							}
						}

						if (anim.effect && anim.effect.target) {
							animData.target = {
								tagName: anim.effect.target.tagName,
								id: anim.effect.target.id,
								className: anim.effect.target.className
							};
						}

						timeline.animations.push(animData);
					});

					// Sort by start time
					timeline.animations.sort((a, b) => {
						const aStart = a.startTime || 0;
						const bStart = b.startTime || 0;
						return aStart - bStart;
					});

					return timeline;
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
						timeline = resultMap
					}
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to create animation timeline")
	}

	return timeline, nil
}