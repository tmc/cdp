package cdpscripttest

import (
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func solidImage(c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}
