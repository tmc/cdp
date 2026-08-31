package cdpscripttest

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
)

func TestWriteGIF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recording.gif")
	frames := []screenFrame{
		{img: solidImage(color.RGBA{R: 0xff, A: 0xff}), t: time.Unix(0, 0)},
		{img: solidImage(color.RGBA{G: 0xff, A: 0xff}), t: time.Unix(0, int64(120*time.Millisecond))},
	}

	if err := writeGIF(path, frames); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Image) != 2 {
		t.Fatalf("frames = %d, want 2", len(got.Image))
	}
	if got.Delay[0] != 12 {
		t.Fatalf("delay[0] = %d, want 12", got.Delay[0])
	}
}

func TestScreenrecordCommandRegistered(t *testing.T) {
	cmds := DefaultCmds()
	if cmds["screenrecord"] == nil {
		t.Fatal("screenrecord command missing")
	}
	if cmds["screen-record"] == nil {
		t.Fatal("screen-record alias missing")
	}
	if cmds["video"] == nil {
		t.Fatal("video alias missing")
	}
}

func TestNormalizeScreenRecordOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    ScreenRecordOptions
		want    ScreenRecordOptions
		wantErr bool
	}{
		{"gif default", ScreenRecordOptions{}, ScreenRecordOptions{Filename: "screenrecord.gif", Format: ScreenRecordGIF, Quality: 80, EveryNthFrame: 1, MaxFrames: DefaultScreenRecordMaxFrames}, false},
		{"png extension", ScreenRecordOptions{Filename: "final.png"}, ScreenRecordOptions{Filename: "final.png", Format: ScreenRecordPNG, Quality: 80, EveryNthFrame: 1, MaxFrames: DefaultScreenRecordMaxFrames}, false},
		{"frames", ScreenRecordOptions{Filename: "capture", Format: ScreenRecordFrames}, ScreenRecordOptions{Filename: "capture", Format: ScreenRecordFrames, Quality: 80, EveryNthFrame: 1, MaxFrames: DefaultScreenRecordMaxFrames}, false},
		{"webm extension", ScreenRecordOptions{Filename: "final.webm"}, ScreenRecordOptions{Filename: "final.webm", Format: ScreenRecordWebM, Quality: 80, EveryNthFrame: 1, MaxFrames: DefaultScreenRecordMaxFrames}, false},
		{"conflict", ScreenRecordOptions{Filename: "final.gif", Format: ScreenRecordPNG}, ScreenRecordOptions{}, true},
		{"max frames", ScreenRecordOptions{MaxFrames: DefaultScreenRecordMaxFrames + 1}, ScreenRecordOptions{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			err := normalizeScreenRecordOptions(&opts)
			if tt.wantErr {
				if err == nil {
					t.Fatal("normalizeScreenRecordOptions succeeded")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if opts != tt.want {
				t.Fatalf("options = %#v, want %#v", opts, tt.want)
			}
		})
	}
}

func TestFindWebMEncoder(t *testing.T) {
	const encoder = "/test/ffmpeg"
	got, err := findWebMEncoder(func(name string) (string, error) {
		if name != "ffmpeg" {
			t.Fatalf("lookPath(%q), want ffmpeg", name)
		}
		return encoder, nil
	})
	if err != nil || got != encoder {
		t.Fatalf("findWebMEncoder() = %q, %v", got, err)
	}
	_, err = findWebMEncoder(func(string) (string, error) { return "", exec.ErrNotFound })
	if err == nil || !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("missing encoder error = %v", err)
	}
}

func TestWriteWebM(t *testing.T) {
	encoder, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "recording.webm")
	frames := []screenFrame{{img: solidImage(color.RGBA{R: 0xff, A: 0xff})}, {img: solidImage(color.RGBA{G: 0xff, A: 0xff})}}
	if err := writeWebM(t.Context(), encoder, path, frames); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// 0x1a45dfa3 is the EBML header every Matroska and WebM file starts with.
	if len(data) < 4 || !bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		t.Fatalf("output is not a webm file: %d bytes, first bytes %x", len(data), data[:min(4, len(data))])
	}
}

func TestCropScreencast(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 80))
	cropped, err := cropScreencast(img, &screenCrop{X: 10, Y: 20, Width: 30, Height: 20}, &page.ScreencastFrameMetadata{DeviceWidth: 100, DeviceHeight: 80})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cropped.Bounds().Size(), (image.Point{X: 30, Y: 20}); got != want {
		t.Fatalf("crop size = %v, want %v", got, want)
	}
	if _, err := cropScreencast(img, &screenCrop{X: 200, Y: 0, Width: 1, Height: 1}, &page.ScreencastFrameMetadata{DeviceWidth: 100, DeviceHeight: 80}); err == nil {
		t.Fatal("out of bounds crop succeeded")
	}
}

func TestWriteFrameManifest(t *testing.T) {
	dir := t.TempDir()
	result := ScreenRecordResult{Path: dir, Format: ScreenRecordFrames, Frames: 2, Duration: time.Second, Selector: "#box"}
	if err := writeFrameManifest(dir, result); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Format   string `json:"format"`
		Frames   int    `json:"frames"`
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Format != "frames" || got.Frames != 2 || got.Selector != "#box" {
		t.Fatalf("manifest = %#v", got)
	}
}

func TestFramesRecorderStreamsFrames(t *testing.T) {
	dir := t.TempDir()
	r := &screenRecorder{path: dir, opts: ScreenRecordOptions{Format: ScreenRecordFrames, MaxFrames: 2, EveryNthFrame: 1}}
	var data bytes.Buffer
	if err := jpeg.Encode(&data, solidImage(color.Black), nil); err != nil {
		t.Fatal(err)
	}
	event := &page.EventScreencastFrame{Data: base64.StdEncoding.EncodeToString(data.Bytes()), Metadata: &page.ScreencastFrameMetadata{DeviceWidth: 2, DeviceHeight: 2}}
	r.addFrame(event)
	r.addFrame(event)
	r.addFrame(event)
	if r.count != 2 || len(r.frames) != 0 || !r.truncated {
		t.Fatalf("count=%d retained=%d truncated=%v", r.count, len(r.frames), r.truncated)
	}
	for _, name := range []string{"frame-000001.png", "frame-000002.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("frame %q: %v", name, err)
		}
	}
}

func TestFramesRecorderEveryNthFrame(t *testing.T) {
	dir := t.TempDir()
	r := &screenRecorder{path: dir, opts: ScreenRecordOptions{Format: ScreenRecordFrames, MaxFrames: 3, EveryNthFrame: 2}}
	var data bytes.Buffer
	if err := jpeg.Encode(&data, solidImage(color.Black), nil); err != nil {
		t.Fatal(err)
	}
	event := &page.EventScreencastFrame{Data: base64.StdEncoding.EncodeToString(data.Bytes()), Metadata: &page.ScreencastFrameMetadata{DeviceWidth: 2, DeviceHeight: 2}}
	for range 5 {
		r.addFrame(event)
	}
	if r.count != 2 {
		t.Fatalf("stored frames = %d, want 2", r.count)
	}
}

func solidImage(c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}
