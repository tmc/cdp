package browser

import (
	"math"
	"testing"
)

func TestParsePaperSize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want PaperSize
		bad  bool
	}{
		{name: "named", in: "letter", want: PaperSize{8.5, 11}},
		{name: "named uppercase", in: "A4", want: PaperSize{8.26772, 11.69291}},
		{name: "named padded", in: "  legal  ", want: PaperSize{8.5, 14}},
		{name: "inches implicit", in: "8.5x11", want: PaperSize{8.5, 11}},
		{name: "inches explicit", in: "8.5inx11in", want: PaperSize{8.5, 11}},
		{name: "millimetres", in: "210mmx297mm", want: PaperSize{8.26772, 11.69291}},
		{name: "centimetres", in: "21cmx29.7cm", want: PaperSize{8.26772, 11.69291}},
		{name: "mixed units", in: "210mmx11.69291in", want: PaperSize{8.26772, 11.69291}},
		{name: "unknown name", in: "foolscap", bad: true},
		{name: "empty", in: "", bad: true},
		{name: "not a number", in: "axb", bad: true},
		{name: "nan rejected", in: "8.5xNaN", bad: true},
		{name: "inf rejected", in: "infx11", bad: true},
		{name: "negative", in: "-8.5x11", bad: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePaperSize(tt.in)
			if tt.bad {
				if err == nil {
					t.Fatalf("ParsePaperSize(%q) = %v, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePaperSize(%q): %v", tt.in, err)
			}
			if !nearly(got.Width, tt.want.Width) || !nearly(got.Height, tt.want.Height) {
				t.Errorf("ParsePaperSize(%q) = %vx%v, want %vx%v",
					tt.in, got.Width, got.Height, tt.want.Width, tt.want.Height)
			}
		})
	}
}

// TestNamedISOSizesMatchMillimetres pins the property that made the rounded
// table wrong: "a4" and "210mmx297mm" must describe the same page.
func TestNamedISOSizesMatchMillimetres(t *testing.T) {
	tests := []struct {
		name string
		mm   string
	}{
		{"a3", "297mmx420mm"},
		{"a4", "210mmx297mm"},
		{"a5", "148mmx210mm"},
	}

	for _, tt := range tests {
		named, err := ParsePaperSize(tt.name)
		if err != nil {
			t.Fatal(err)
		}
		metric, err := ParsePaperSize(tt.mm)
		if err != nil {
			t.Fatal(err)
		}
		if !nearly(named.Width, metric.Width) || !nearly(named.Height, metric.Height) {
			t.Errorf("%s = %vx%v, %s = %vx%v", tt.name, named.Width, named.Height,
				tt.mm, metric.Width, metric.Height)
		}
	}
}

func TestParseMargins(t *testing.T) {
	tests := []struct {
		name                     string
		in                       string
		top, right, bottom, left float64
		bad                      bool
	}{
		{name: "one value", in: "0.5", top: 0.5, right: 0.5, bottom: 0.5, left: 0.5},
		{name: "zero", in: "0"},
		{name: "two values", in: "0.5,1.5", top: 0.5, right: 1.5, bottom: 0.5, left: 1.5},
		{name: "four values", in: "1,2,3,4", top: 1, right: 2, bottom: 3, left: 4},
		{name: "space separated", in: "1 2 3 4", top: 1, right: 2, bottom: 3, left: 4},
		{name: "units", in: "25.4mm", top: 1, right: 1, bottom: 1, left: 1},
		{name: "three values", in: "1,2,3", bad: true},
		{name: "five values", in: "1,2,3,4,5", bad: true},
		{name: "empty", in: "", bad: true},
		{name: "negative", in: "-1", bad: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			top, right, bottom, left, err := ParseMargins(tt.in)
			if tt.bad {
				if err == nil {
					t.Fatalf("ParseMargins(%q) = %v,%v,%v,%v, want error", tt.in, top, right, bottom, left)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMargins(%q): %v", tt.in, err)
			}
			if !nearly(top, tt.top) || !nearly(right, tt.right) || !nearly(bottom, tt.bottom) || !nearly(left, tt.left) {
				t.Errorf("ParseMargins(%q) = %v,%v,%v,%v, want %v,%v,%v,%v",
					tt.in, top, right, bottom, left, tt.top, tt.right, tt.bottom, tt.left)
			}
		})
	}
}

func nearly(a, b float64) bool { return math.Abs(a-b) < 0.0001 }

func TestParsePDFSpec(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want PDFOptions
		bad  bool
	}{
		{name: "empty", in: "", want: PDFOptions{}},
		{name: "page", in: "page=a4", want: PDFOptions{Format: "a4"}},
		{name: "explicit size", in: "page=8.5x11", want: PDFOptions{Format: "8.5x11"}},
		{name: "landscape", in: "landscape", want: PDFOptions{Landscape: true}},
		{name: "scale", in: "scale=0.8", want: PDFOptions{Scale: 0.8}},
		{
			name: "margin one value",
			in:   "margin=0.75",
			want: PDFOptions{MarginTop: 0.75, MarginBottom: 0.75, MarginLeft: 0.75, MarginRight: 0.75},
		},
		{
			name: "margin space separated",
			in:   "margin=1 2",
			want: PDFOptions{MarginTop: 1, MarginBottom: 1, MarginLeft: 2, MarginRight: 2},
		},
		{name: "ranges normalised to commas", in: "ranges=1-5 8", want: PDFOptions{PageRanges: "1-5,8"}},
		{name: "outline", in: "outline", want: PDFOptions{GenerateDocumentOutline: true}},
		{name: "tagged", in: "tagged", want: PDFOptions{GenerateTaggedPDF: true}},
		{name: "css page size", in: "css-page-size", want: PDFOptions{PreferCSSPageSize: true}},
		{
			name: "combined",
			in:   "page=a4,margin=0.5,landscape,outline",
			want: PDFOptions{Format: "a4", Landscape: true, GenerateDocumentOutline: true,
				MarginTop: 0.5, MarginBottom: 0.5, MarginLeft: 0.5, MarginRight: 0.5},
		},
		{name: "spaces around fields", in: " page=a4 , landscape ", want: PDFOptions{Format: "a4", Landscape: true}},
		{name: "unknown key", in: "papersize=a4", bad: true},
		{name: "bad page", in: "page=foolscap", bad: true},
		{name: "bad margin", in: "margin=1 2 3", bad: true},
		{name: "zero scale", in: "scale=0", bad: true},
		{name: "nan scale", in: "scale=NaN", bad: true},
		{name: "page without value", in: "page", bad: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := ParsePDFSpec(tt.in)
			if tt.bad {
				if err == nil {
					t.Fatalf("ParsePDFSpec(%q) = %v, want error", tt.in, opts)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePDFSpec(%q): %v", tt.in, err)
			}
			var got PDFOptions
			for _, o := range opts {
				o(&got)
			}
			if got != tt.want {
				t.Errorf("ParsePDFSpec(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}
