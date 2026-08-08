package browser

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// PaperSize is a page size in inches, the unit Page.printToPDF takes.
type PaperSize struct {
	Width  float64
	Height float64
}

// paperSizes are the named sizes, in inches, matching Chrome's own print
// dialog. Keys are lowercase; lookup lowercases its argument.
var paperSizes = map[string]PaperSize{
	"letter":  {8.5, 11},
	"legal":   {8.5, 14},
	"tabloid": {11, 17},
	"ledger":  {17, 11},
	// ISO sizes are defined in millimetres; these are the exact conversions,
	// so "a4" and "210mmx297mm" produce an identical page box.
	"a0": {33.11024, 46.81102},
	"a1": {23.38583, 33.11024},
	"a2": {16.53543, 23.38583},
	"a3": {11.69291, 16.53543},
	"a4": {8.26772, 11.69291},
	"a5": {5.82677, 8.26772},
	"a6": {4.13386, 5.82677},
}

// PaperSizeNames returns the named sizes in a stable, human-facing order.
func PaperSizeNames() []string {
	return []string{"letter", "legal", "tabloid", "ledger", "a0", "a1", "a2", "a3", "a4", "a5", "a6"}
}

// ParsePaperSize interprets a page size: either a name from PaperSizeNames, or
// explicit dimensions as WxH. Dimensions are inches unless suffixed with in,
// mm, or cm — "210mmx297mm" and "8.27x11.7" both work, and a suffix on either
// side applies to that side only.
func ParsePaperSize(s string) (PaperSize, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return PaperSize{}, fmt.Errorf("empty page size")
	}
	if size, ok := paperSizes[s]; ok {
		return size, nil
	}

	w, h, ok := strings.Cut(s, "x")
	if !ok {
		return PaperSize{}, fmt.Errorf("unknown page size %q: use one of %s, or WxH such as 8.5x11 or 210mmx297mm",
			s, strings.Join(PaperSizeNames(), ", "))
	}
	width, err := parseLength(w)
	if err != nil {
		return PaperSize{}, fmt.Errorf("page width: %w", err)
	}
	height, err := parseLength(h)
	if err != nil {
		return PaperSize{}, fmt.Errorf("page height: %w", err)
	}
	return PaperSize{Width: width, Height: height}, nil
}

// ParseMargins interprets a margin specification in CSS shorthand order and
// returns top, right, bottom, left in inches. One value sets all four; two set
// vertical and horizontal; four set each side. Values accept the same units as
// ParsePaperSize.
func ParseMargins(s string) (top, right, bottom, left float64, err error) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	if len(fields) == 0 {
		return 0, 0, 0, 0, fmt.Errorf("empty margin")
	}

	v := make([]float64, len(fields))
	for i, f := range fields {
		v[i], err = parseLength(f)
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("margin %d: %w", i+1, err)
		}
	}

	switch len(v) {
	case 1:
		return v[0], v[0], v[0], v[0], nil
	case 2:
		return v[0], v[1], v[0], v[1], nil
	case 4:
		return v[0], v[1], v[2], v[3], nil
	default:
		return 0, 0, 0, 0, fmt.Errorf("margin needs 1, 2, or 4 values, got %d", len(v))
	}
}

// parseLength converts a length to inches. A bare number is already inches.
func parseLength(s string) (float64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty length")
	}

	factor := 1.0
	switch {
	case strings.HasSuffix(s, "mm"):
		s, factor = strings.TrimSuffix(s, "mm"), 1.0/25.4
	case strings.HasSuffix(s, "cm"):
		s, factor = strings.TrimSuffix(s, "cm"), 1.0/2.54
	case strings.HasSuffix(s, "in"):
		s = strings.TrimSuffix(s, "in")
	case strings.HasSuffix(s, "px"):
		// CSS px at 96dpi, which is what Chrome's print pipeline assumes.
		s, factor = strings.TrimSuffix(s, "px"), 1.0/96.0
	}

	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid length %q", s)
	}
	// ParseFloat accepts "nan" and "inf"; neither is a page dimension.
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("invalid length %q", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("negative length %q", s)
	}
	return n * factor, nil
}
