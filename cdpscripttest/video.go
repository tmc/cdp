package cdpscripttest

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type screenRecorder struct {
	ctx    context.Context
	path   string
	format string

	mu      sync.Mutex
	frames  []screenFrame
	stopped bool
}

type screenFrame struct {
	img image.Image
	t   time.Time
}

// StartScreenRecording starts recording browser frames into an animated GIF.
// The returned path is absolute or relative in the same form as artifactDirFn.
func (s *State) StartScreenRecording(filename string) (string, error) {
	if s.recorder != nil {
		return "", fmt.Errorf("screenrecord: already recording")
	}
	if filename == "" {
		filename = "screenrecord.gif"
	}
	if filepath.Ext(filename) == "" {
		filename += ".gif"
	}
	if ext := filepath.Ext(filename); ext != ".gif" {
		return "", fmt.Errorf("screenrecord: only .gif output is supported")
	}
	path := filepath.Join(s.artifactDirFn(), filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return "", fmt.Errorf("screenrecord: mkdir: %w", err)
	}

	r := &screenRecorder{
		ctx:    s.cdpCtx,
		path:   path,
		format: "jpeg",
	}
	chromedp.ListenTarget(s.cdpCtx, func(ev any) {
		f, ok := ev.(*page.EventScreencastFrame)
		if !ok {
			return
		}
		r.addFrame(f)
		_ = page.ScreencastFrameAck(f.SessionID).Do(s.cdpCtx)
	})

	if err := chromedp.Run(s.cdpCtx, page.StartScreencast().
		WithFormat(page.ScreencastFormatJpeg).
		WithQuality(80).
		WithEveryNthFrame(1)); err != nil {
		return "", fmt.Errorf("screenrecord: start: %w", err)
	}
	s.recorder = r
	return path, nil
}

// StopScreenRecording stops the active recording and writes the GIF artifact.
func (s *State) StopScreenRecording() (string, int, error) {
	if s.recorder == nil {
		return "", 0, fmt.Errorf("screenrecord: not recording")
	}
	r := s.recorder
	s.recorder = nil
	path, frames, err := r.stop()
	if err != nil {
		return path, frames, err
	}
	return path, frames, nil
}

func (s *State) stopScreenRecordingIfActive() (string, int, error) {
	if s.recorder == nil {
		return "", 0, nil
	}
	return s.StopScreenRecording()
}

func (r *screenRecorder) addFrame(ev *page.EventScreencastFrame) {
	data, err := base64.StdEncoding.DecodeString(ev.Data)
	if err != nil {
		return
	}
	img, err := decodeScreencastImage(r.format, data)
	if err != nil {
		return
	}
	t := time.Now()
	if ev.Metadata != nil && ev.Metadata.Timestamp != nil {
		t = time.Time(*ev.Metadata.Timestamp)
	}
	r.mu.Lock()
	if !r.stopped {
		r.frames = append(r.frames, screenFrame{img: img, t: t})
	}
	r.mu.Unlock()
}

func (r *screenRecorder) stop() (string, int, error) {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return r.path, len(r.frames), nil
	}
	r.stopped = true
	r.mu.Unlock()

	_ = chromedp.Run(r.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.StopScreencast().Do(ctx)
	}))

	r.mu.Lock()
	frames := append([]screenFrame(nil), r.frames...)
	r.mu.Unlock()

	if len(frames) == 0 {
		return r.path, 0, fmt.Errorf("screenrecord: no frames captured")
	}
	if err := writeGIF(r.path, frames); err != nil {
		return r.path, len(frames), err
	}
	return r.path, len(frames), nil
}

func decodeScreencastImage(format string, data []byte) (image.Image, error) {
	switch format {
	case "jpeg":
		return jpeg.Decode(bytes.NewReader(data))
	case "png":
		return png.Decode(bytes.NewReader(data))
	default:
		return nil, fmt.Errorf("screenrecord: unsupported frame format %q", format)
	}
}

func writeGIF(path string, frames []screenFrame) error {
	out := &gif.GIF{LoopCount: 0}
	for i, frame := range frames {
		b := frame.img.Bounds()
		paletted := image.NewPaletted(b, palette.Plan9)
		draw.FloydSteinberg.Draw(paletted, b, frame.img, b.Min)
		out.Image = append(out.Image, paletted)
		out.Delay = append(out.Delay, gifDelay(frames, i))
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("screenrecord: create %s: %w", path, err)
	}
	defer f.Close()
	if err := gif.EncodeAll(f, out); err != nil {
		return fmt.Errorf("screenrecord: encode %s: %w", path, err)
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
