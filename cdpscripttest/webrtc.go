// WebRTC testing support.
//
// Chrome flags for fake media streams must be set at allocator creation time,
// before any browser contexts are created. Use WebRTCAllocatorOptions or
// WebRTCAllocatorOptionsWithMedia when building the exec allocator.

package cdpscripttest

import (
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// WebRTCAllocatorOptions returns chromedp exec allocator options for WebRTC testing.
// It enables fake device and UI for media streams, allowing getUserMedia to
// succeed without real camera/microphone hardware.
func WebRTCAllocatorOptions() []chromedp.ExecAllocatorOption {
	return []chromedp.ExecAllocatorOption{
		chromedp.Flag("use-fake-device-for-media-stream", true),
		chromedp.Flag("use-fake-ui-for-media-stream", true),
	}
}

// WebRTCAllocatorOptionsWithMedia returns allocator options using custom media files
// for fake video and audio capture. Non-empty paths are added as Chrome flags;
// empty paths are skipped.
func WebRTCAllocatorOptionsWithMedia(videoPath, audioPath string) []chromedp.ExecAllocatorOption {
	opts := WebRTCAllocatorOptions()
	if videoPath != "" {
		opts = append(opts, chromedp.Flag("use-file-for-fake-video-capture", videoPath))
	}
	if audioPath != "" {
		opts = append(opts, chromedp.Flag("use-file-for-fake-audio-capture", audioPath))
	}
	return opts
}

// evalAwaitPromise configures a Runtime.evaluate call to await the result
// if it is a Promise. Useful for evaluating async JS expressions.
func evalAwaitPromise(p *runtime.EvaluateParams) *runtime.EvaluateParams {
	return p.WithAwaitPromise(true)
}
