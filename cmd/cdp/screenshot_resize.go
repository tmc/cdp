package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"github.com/chromedp/cdproto/page"
	"golang.org/x/image/draw"
)

// screenshotFormat returns the CDP format and MIME type for the given input.
func screenshotFormat(format string, quality int) (page.CaptureScreenshotFormat, string) {
	switch format {
	case "jpeg", "jpg":
		return page.CaptureScreenshotFormatJpeg, "image/jpeg"
	case "webp":
		return page.CaptureScreenshotFormatWebp, "image/webp"
	case "png", "":
		if quality > 0 {
			return page.CaptureScreenshotFormatJpeg, "image/jpeg"
		}
		return page.CaptureScreenshotFormatPng, "image/png"
	default:
		return page.CaptureScreenshotFormatPng, "image/png"
	}
}

// downsizeImage decodes an image, scales it so width <= maxWidth (preserving
// aspect ratio), and re-encodes it. If the image is already small enough it
// is returned unchanged.
func downsizeImage(data []byte, maxWidth int, mimeType string) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	bounds := src.Bounds()
	origW := bounds.Dx()
	if origW <= maxWidth {
		return data, nil
	}

	// Scale proportionally.
	scale := float64(maxWidth) / float64(origW)
	newW := maxWidth
	newH := int(float64(bounds.Dy()) * scale)

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	var buf bytes.Buffer
	switch mimeType {
	case "image/jpeg":
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
	default:
		if err := png.Encode(&buf, dst); err != nil {
			return nil, fmt.Errorf("encode png: %w", err)
		}
	}
	return buf.Bytes(), nil
}
