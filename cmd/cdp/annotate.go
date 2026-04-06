package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/dom"
)

// annotationEntry describes one interactive element drawn on an annotated screenshot.
type annotationEntry struct {
	Number int    `json:"number"`
	Ref    int    `json:"ref"`
	Role   string `json:"role"`
	Name   string `json:"name"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	W      int    `json:"w"`
	H      int    `json:"h"`
}

// annotateScreenshot draws numbered rectangles around interactive elements
// on a screenshot PNG. It queries the AX tree for interactive elements,
// gets their bounding boxes via DOM.getContentQuads, and draws labels.
func annotateScreenshot(ctx context.Context, imgData []byte, refs *refRegistry) ([]byte, []annotationEntry, error) {
	src, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, nil, fmt.Errorf("decode screenshot: %w", err)
	}

	// Draw onto a mutable copy.
	bounds := src.Bounds()
	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, src, bounds.Min, draw.Src)

	// Get AX tree for interactive elements.
	ensureAccessibility(ctx)
	nodes, err := accessibility.GetFullAXTree().Do(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get ax tree: %w", err)
	}

	var annotations []annotationEntry
	num := 0

	for _, n := range nodes {
		if n.Ignored || n.BackendDOMNodeID == 0 {
			continue
		}
		role := axValueString(n.Role)
		if !interactiveRoles[role] {
			continue
		}

		// Get bounding box via content quads.
		quads, err := dom.GetContentQuads().WithBackendNodeID(n.BackendDOMNodeID).Do(ctx)
		if err != nil || len(quads) == 0 {
			continue
		}
		q := quads[0]
		if len(q) < 8 {
			continue
		}

		// Compute bounding rect from quad points.
		minX, minY := q[0], q[1]
		maxX, maxY := q[0], q[1]
		for i := 2; i < 8; i += 2 {
			if q[i] < minX {
				minX = q[i]
			}
			if q[i] > maxX {
				maxX = q[i]
			}
			if q[i+1] < minY {
				minY = q[i+1]
			}
			if q[i+1] > maxY {
				maxY = q[i+1]
			}
		}

		x, y := int(minX), int(minY)
		w, h := int(maxX-minX), int(maxY-minY)
		if w < 2 || h < 2 {
			continue
		}

		// Clip to image bounds.
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if x+w > bounds.Max.X {
			w = bounds.Max.X - x
		}
		if y+h > bounds.Max.Y {
			h = bounds.Max.Y - y
		}

		num++
		name := axValueString(n.Name)

		// Find or assign a ref.
		ref := 0
		refs.mu.RLock()
		for k, v := range refs.entries {
			if v.BackendNodeID == n.BackendDOMNodeID {
				ref = k
				break
			}
		}
		refs.mu.RUnlock()

		annotations = append(annotations, annotationEntry{
			Number: num,
			Ref:    ref,
			Role:   role,
			Name:   name,
			X:      x, Y: y, W: w, H: h,
		})

		// Draw rectangle outline.
		drawRect(canvas, x, y, w, h, annotationColor(num))

		// Draw number label in top-left corner.
		drawLabel(canvas, x, y, num, annotationColor(num))
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		return nil, nil, fmt.Errorf("encode annotated: %w", err)
	}
	return buf.Bytes(), annotations, nil
}

// annotationColor returns a distinct color for a given index.
func annotationColor(n int) color.RGBA {
	colors := []color.RGBA{
		{255, 0, 0, 255},     // red
		{0, 128, 255, 255},   // blue
		{0, 200, 0, 255},     // green
		{255, 165, 0, 255},   // orange
		{128, 0, 255, 255},   // purple
		{255, 0, 128, 255},   // pink
		{0, 200, 200, 255},   // teal
		{200, 200, 0, 255},   // yellow
	}
	return colors[(n-1)%len(colors)]
}

// drawRect draws a 2px rectangle outline on the canvas.
func drawRect(canvas *image.RGBA, x, y, w, h int, c color.RGBA) {
	bounds := canvas.Bounds()
	for t := 0; t < 2; t++ {
		// Top and bottom edges.
		for px := x; px < x+w && px < bounds.Max.X; px++ {
			if y+t >= 0 && y+t < bounds.Max.Y {
				canvas.SetRGBA(px, y+t, c)
			}
			if y+h-1-t >= 0 && y+h-1-t < bounds.Max.Y {
				canvas.SetRGBA(px, y+h-1-t, c)
			}
		}
		// Left and right edges.
		for py := y; py < y+h && py < bounds.Max.Y; py++ {
			if x+t >= 0 && x+t < bounds.Max.X {
				canvas.SetRGBA(x+t, py, c)
			}
			if x+w-1-t >= 0 && x+w-1-t < bounds.Max.X {
				canvas.SetRGBA(x+w-1-t, py, c)
			}
		}
	}
}

// drawLabel draws a small numbered badge at (x, y).
func drawLabel(canvas *image.RGBA, x, y, num int, c color.RGBA) {
	label := fmt.Sprintf("%d", num)
	// Badge size: 6px per digit + padding.
	badgeW := len(label)*6 + 4
	badgeH := 10

	// Draw background.
	for py := y - badgeH; py < y; py++ {
		for px := x; px < x+badgeW; px++ {
			if px >= 0 && px < canvas.Bounds().Max.X && py >= 0 && py < canvas.Bounds().Max.Y {
				canvas.SetRGBA(px, py, c)
			}
		}
	}

	// Draw digits as simple 5x7 bitmaps in white.
	white := color.RGBA{255, 255, 255, 255}
	ox := x + 2
	oy := y - badgeH + 1
	for _, ch := range label {
		glyph := digitGlyph(ch)
		for row := 0; row < 7; row++ {
			for col := 0; col < 5; col++ {
				if glyph[row]&(1<<(4-col)) != 0 {
					px, py := ox+col, oy+row
					if px >= 0 && px < canvas.Bounds().Max.X && py >= 0 && py < canvas.Bounds().Max.Y {
						canvas.SetRGBA(px, py, white)
					}
				}
			}
		}
		ox += 6
	}
}

// digitGlyph returns a 7-row bitmap for a digit character (5 bits wide).
func digitGlyph(ch rune) [7]byte {
	glyphs := map[rune][7]byte{
		'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
		'1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
		'2': {0x0E, 0x11, 0x01, 0x06, 0x08, 0x10, 0x1F},
		'3': {0x0E, 0x11, 0x01, 0x06, 0x01, 0x11, 0x0E},
		'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
		'5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
		'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E},
		'7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
		'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
		'9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	}
	if g, ok := glyphs[ch]; ok {
		return g
	}
	return [7]byte{}
}

// annotationsToJSON marshals annotations to a JSON string.
func annotationsToJSON(annotations []annotationEntry) string {
	data, _ := json.Marshal(annotations)
	return string(data)
}
