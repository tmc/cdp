package cdpscripttest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// ScreenRecordFormat identifies a screen recording artifact format.
type ScreenRecordFormat string

// Screen recording formats.
const (
	ScreenRecordGIF    ScreenRecordFormat = "gif"    // animated GIF
	ScreenRecordFrames ScreenRecordFormat = "frames" // directory of numbered PNG frames plus manifest.json
	ScreenRecordPNG    ScreenRecordFormat = "png"    // PNG of the last captured frame
	ScreenRecordWebM   ScreenRecordFormat = "webm"   // VP9 WebM; requires ffmpeg in PATH
)

// DefaultScreenRecordMaxFrames is the default and the largest allowed value
// of ScreenRecordOptions.MaxFrames.
const DefaultScreenRecordMaxFrames = 300

// ScreenRecordOptions configures a screen recording.
// The zero value records a GIF named screenrecord.gif.
type ScreenRecordOptions struct {
	// Filename is the artifact path relative to the artifact directory.
	// If empty, it is "screenrecord" plus the format's extension
	// ("screenrecord" alone for ScreenRecordFrames). If it has no
	// extension, the format's extension is added. An extension must agree
	// with Format; ScreenRecordFrames takes a directory name with none.
	Filename string

	// Format is the artifact format. If empty, it is inferred from the
	// Filename extension, defaulting to ScreenRecordGIF.
	Format ScreenRecordFormat

	// Selector, if set, crops every frame to the border box the matching
	// element has when the recording starts. The element must be visible.
	Selector string

	// Quality is the screencast JPEG quality, 1 to 100. Zero means 80.
	Quality int

	// EveryNthFrame keeps one of every n received frames. Zero means 1.
	EveryNthFrame int

	// MaxFrames caps the kept frames, 1 to DefaultScreenRecordMaxFrames.
	// Zero means DefaultScreenRecordMaxFrames. Frames past the cap are
	// dropped and the result is marked Truncated.
	MaxFrames int
}

// ScreenRecordResult describes a completed screen recording.
type ScreenRecordResult struct {
	Path      string             // artifact path
	Format    ScreenRecordFormat // artifact format
	Frames    int                // number of frames kept
	Duration  time.Duration      // time from start to stop
	Selector  string             // crop selector, if any
	Truncated bool               // frames were dropped at MaxFrames or a frame write failed
}

type screenCrop struct{ X, Y, Width, Height float64 }

type screenRecorder struct {
	ctx    context.Context
	path   string
	opts   ScreenRecordOptions
	crop   *screenCrop
	ffmpeg string

	mu        sync.Mutex
	frames    []screenFrame
	started   time.Time
	ended     time.Time
	seen      int
	count     int
	stopped   bool
	truncated bool
}

type screenFrame struct {
	img image.Image
	t   time.Time
}

// StartScreenRecording starts recording the current tab with opts and
// returns the artifact path. The artifact is written by StopScreenRecording.
func (s *State) StartScreenRecording(opts ScreenRecordOptions) (string, error) {
	if s.recorder != nil {
		return "", fmt.Errorf("screenrecord: already recording")
	}
	if err := normalizeScreenRecordOptions(&opts); err != nil {
		return "", err
	}
	ffmpeg := ""
	if opts.Format == ScreenRecordWebM {
		var err error
		ffmpeg, err = findWebMEncoder(exec.LookPath)
		if err != nil {
			return "", err
		}
	}
	path := filepath.Join(s.artifactDirFn(), opts.Filename)
	if opts.Format == ScreenRecordFrames {
		if err := os.MkdirAll(path, 0o777); err != nil {
			return "", fmt.Errorf("screenrecord: create frames directory: %w", err)
		}
	} else if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return "", fmt.Errorf("screenrecord: create artifact directory: %w", err)
	}
	var crop *screenCrop
	if opts.Selector != "" {
		var box screenCrop
		js := fmt.Sprintf(`(() => { const e = document.querySelector(%q); if (!e) return null; const r = e.getBoundingClientRect(); const c = getComputedStyle(e); if (!r.width || !r.height || c.display === "none" || c.visibility === "hidden" || r.right <= 0 || r.bottom <= 0 || r.left >= innerWidth || r.top >= innerHeight) return null; return {X:r.x,Y:r.y,Width:r.width,Height:r.height}; })()`, opts.Selector)
		if err := chromedp.Run(s.cdpCtx, chromedp.Evaluate(js, &box)); err != nil {
			return "", fmt.Errorf("screenrecord: query selector %q: %w", opts.Selector, err)
		}
		if box.Width <= 0 || box.Height <= 0 {
			return "", fmt.Errorf("screenrecord: selector %q is missing or hidden", opts.Selector)
		}
		crop = &box
	}
	r := &screenRecorder{ctx: s.cdpCtx, path: path, opts: opts, crop: crop, ffmpeg: ffmpeg, started: time.Now()}
	chromedp.ListenTarget(s.cdpCtx, func(ev any) {
		if f, ok := ev.(*page.EventScreencastFrame); ok {
			if !r.addFrame(f) {
				return
			}
			// Chrome sends more frames only after an ack. The ack must run
			// off the listener goroutine: chromedp dispatches events
			// synchronously, so calling Run here would deadlock.
			go chromedp.Run(s.cdpCtx, page.ScreencastFrameAck(f.SessionID))
		}
	})
	if err := chromedp.Run(s.cdpCtx, page.StartScreencast().WithFormat(page.ScreencastFormatJpeg).WithQuality(int64(opts.Quality)).WithEveryNthFrame(1)); err != nil {
		return "", fmt.Errorf("screenrecord: start screencast: %w", err)
	}
	s.recorder = r
	return path, nil
}

func normalizeScreenRecordOptions(opts *ScreenRecordOptions) error {
	if opts.Format == "" {
		opts.Format = formatFromFilename(opts.Filename)
	}
	if opts.Format == "" {
		opts.Format = ScreenRecordGIF
	}
	if opts.Format != ScreenRecordGIF && opts.Format != ScreenRecordFrames && opts.Format != ScreenRecordPNG && opts.Format != ScreenRecordWebM {
		return fmt.Errorf("screenrecord: unsupported format %q", opts.Format)
	}
	if opts.Filename == "" {
		switch opts.Format {
		case ScreenRecordGIF:
			opts.Filename = "screenrecord.gif"
		case ScreenRecordPNG:
			opts.Filename = "screenrecord.png"
		case ScreenRecordWebM:
			opts.Filename = "screenrecord.webm"
		case ScreenRecordFrames:
			opts.Filename = "screenrecord"
		}
	}
	ext := strings.ToLower(filepath.Ext(opts.Filename))
	if ext == "" && opts.Format != ScreenRecordFrames {
		switch opts.Format {
		case ScreenRecordGIF:
			opts.Filename += ".gif"
		case ScreenRecordPNG:
			opts.Filename += ".png"
		case ScreenRecordWebM:
			opts.Filename += ".webm"
		}
		ext = filepath.Ext(opts.Filename)
	}
	if ext != "" {
		want := formatFromFilename(opts.Filename)
		if want == "" || want != opts.Format {
			return fmt.Errorf("screenrecord: filename extension conflicts with format %q", opts.Format)
		}
	}
	if opts.Format == ScreenRecordFrames && ext != "" {
		return fmt.Errorf("screenrecord: frames output needs a directory name")
	}
	if opts.Quality == 0 {
		opts.Quality = 80
	}
	if opts.Quality < 1 || opts.Quality > 100 {
		return fmt.Errorf("screenrecord: quality must be between 1 and 100")
	}
	if opts.EveryNthFrame == 0 {
		opts.EveryNthFrame = 1
	}
	if opts.EveryNthFrame < 1 {
		return fmt.Errorf("screenrecord: every-nth-frame must be positive")
	}
	if opts.MaxFrames == 0 {
		opts.MaxFrames = DefaultScreenRecordMaxFrames
	}
	if opts.MaxFrames < 1 || opts.MaxFrames > DefaultScreenRecordMaxFrames {
		return fmt.Errorf("screenrecord: max-frames must be between 1 and %d", DefaultScreenRecordMaxFrames)
	}
	return nil
}

func formatFromFilename(name string) ScreenRecordFormat {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".gif":
		return ScreenRecordGIF
	case ".png":
		return ScreenRecordPNG
	case ".webm":
		return ScreenRecordWebM
	}
	return ""
}

// StopScreenRecording stops the active recording, writes its artifact,
// and returns the result.
func (s *State) StopScreenRecording() (ScreenRecordResult, error) {
	if s.recorder == nil {
		return ScreenRecordResult{}, fmt.Errorf("screenrecord: not recording")
	}
	r := s.recorder
	s.recorder = nil
	return r.stop()
}

func (s *State) stopScreenRecordingIfActive() (string, int, error) {
	if s.recorder == nil {
		return "", 0, nil
	}
	r, err := s.StopScreenRecording()
	return r.Path, r.Frames, err
}

// addFrame records ev and reports whether the recording is still active,
// in which case the frame should be acked.
func (r *screenRecorder) addFrame(ev *page.EventScreencastFrame) bool {
	data, err := base64.StdEncoding.DecodeString(ev.Data)
	if err != nil {
		return !r.isStopped()
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return !r.isStopped()
	}
	if r.crop != nil {
		img, err = cropScreencast(img, r.crop, ev.Metadata)
		if err != nil {
			return !r.isStopped()
		}
	}
	t := time.Now()
	if ev.Metadata != nil && ev.Metadata.Timestamp != nil {
		t = time.Time(*ev.Metadata.Timestamp)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return false
	}
	if r.truncated {
		return true
	}
	r.seen++
	if r.seen%r.opts.EveryNthFrame != 0 {
		return true
	}
	if r.count >= r.opts.MaxFrames {
		r.truncated = true
		return true
	}
	if r.opts.Format == ScreenRecordFrames {
		if err := writePNG(filepath.Join(r.path, fmt.Sprintf("frame-%06d.png", r.count+1)), img); err != nil {
			r.truncated = true
			return true
		}
		r.count++
		return true
	}
	r.count++
	r.frames = append(r.frames, screenFrame{img: img, t: t})
	return true
}

func (r *screenRecorder) isStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

func cropScreencast(img image.Image, crop *screenCrop, meta *page.ScreencastFrameMetadata) (image.Image, error) {
	if meta == nil || meta.DeviceWidth <= 0 || meta.DeviceHeight <= 0 {
		return nil, fmt.Errorf("screenrecord: missing screencast metadata")
	}
	b := img.Bounds()
	sx := float64(b.Dx()) / meta.DeviceWidth
	sy := float64(b.Dy()) / meta.DeviceHeight
	r := image.Rect(int(math.Round(crop.X*sx)), int(math.Round(crop.Y*sy)), int(math.Round((crop.X+crop.Width)*sx)), int(math.Round((crop.Y+crop.Height)*sy))).Intersect(b)
	if r.Empty() {
		return nil, fmt.Errorf("screenrecord: selector crop is outside the screencast frame")
	}
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	return out, nil
}

func (r *screenRecorder) stop() (ScreenRecordResult, error) {
	r.mu.Lock()
	if r.stopped {
		result := r.resultLocked()
		r.mu.Unlock()
		return result, nil
	}
	r.stopped = true
	r.ended = time.Now()
	frames := append([]screenFrame(nil), r.frames...)
	result := r.resultLocked()
	r.mu.Unlock()
	if err := chromedp.Run(r.ctx, chromedp.ActionFunc(func(ctx context.Context) error { return page.StopScreencast().Do(ctx) })); err != nil {
		return result, fmt.Errorf("screenrecord: stop screencast: %w", err)
	}
	if result.Frames == 0 {
		return result, fmt.Errorf("screenrecord: no frames captured")
	}
	var err error
	switch r.opts.Format {
	case ScreenRecordGIF:
		err = writeGIF(r.path, frames)
	case ScreenRecordPNG:
		err = writePNG(r.path, frames[len(frames)-1].img)
	case ScreenRecordFrames:
		err = writeFrameManifest(r.path, result)
	case ScreenRecordWebM:
		err = writeWebM(r.ctx, r.ffmpeg, r.path, frames)
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func findWebMEncoder(lookPath func(string) (string, error)) (string, error) {
	path, err := lookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("screenrecord: webm requires ffmpeg: %w", err)
	}
	return path, nil
}

func writeWebM(ctx context.Context, encoder, path string, frames []screenFrame) error {
	cmd := exec.CommandContext(ctx, encoder,
		"-f", "image2pipe", "-vcodec", "png", "-framerate", "10", "-i", "pipe:0",
		"-c:v", "libvpx-vp9", "-pix_fmt", "yuv420p", "-y", path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("screenrecord: create webm input: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("screenrecord: start ffmpeg: %w", err)
	}
	for _, frame := range frames {
		if err := png.Encode(stdin, frame.img); err != nil {
			stdin.Close()
			cmd.Wait()
			return fmt.Errorf("screenrecord: write webm frame: %w", err)
		}
	}
	if err := stdin.Close(); err != nil {
		cmd.Wait()
		return fmt.Errorf("screenrecord: close webm input: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return fmt.Errorf("screenrecord: encode webm: %w: %s", err, message)
		}
		return fmt.Errorf("screenrecord: encode webm: %w", err)
	}
	return nil
}

func (r *screenRecorder) resultLocked() ScreenRecordResult {
	d := r.ended.Sub(r.started)
	if d < 0 {
		d = 0
	}
	return ScreenRecordResult{Path: r.path, Format: r.opts.Format, Frames: r.count, Duration: d, Selector: r.opts.Selector, Truncated: r.truncated}
}

func writeFrameManifest(dir string, result ScreenRecordResult) error {
	data, err := json.MarshalIndent(struct {
		Format    ScreenRecordFormat `json:"format"`
		Frames    int                `json:"frames"`
		Duration  string             `json:"duration"`
		Selector  string             `json:"selector,omitempty"`
		Truncated bool               `json:"truncated"`
	}{result.Format, result.Frames, result.Duration.String(), result.Selector, result.Truncated}, "", "  ")
	if err != nil {
		return fmt.Errorf("screenrecord: encode manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(data, '\n'), 0o666); err != nil {
		return fmt.Errorf("screenrecord: write manifest: %w", err)
	}
	return nil
}
func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("screenrecord: create png: %w", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("screenrecord: encode png: %w", err)
	}
	return nil
}
func writeGIF(path string, frames []screenFrame) error {
	out := &gif.GIF{LoopCount: 0}
	for i, frame := range frames {
		b := frame.img.Bounds()
		p := image.NewPaletted(b, palette.Plan9)
		draw.FloydSteinberg.Draw(p, b, frame.img, b.Min)
		out.Image = append(out.Image, p)
		out.Delay = append(out.Delay, gifDelay(frames, i))
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("screenrecord: create gif: %w", err)
	}
	defer f.Close()
	if err := gif.EncodeAll(f, out); err != nil {
		return fmt.Errorf("screenrecord: encode gif: %w", err)
	}
	return nil
}
func gifDelay(frames []screenFrame, i int) int {
	if i+1 >= len(frames) {
		return 10
	}
	d := frames[i+1].t.Sub(frames[i].t)
	if d <= 0 {
		return 10
	}
	cs := int(d / (10 * time.Millisecond))
	if cs < 2 {
		return 2
	}
	if cs > 100 {
		return 100
	}
	return cs
}
