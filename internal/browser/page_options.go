package browser

import "time"

// NavigateOptions configures page navigation
type NavigateOptions struct {
	Timeout   time.Duration
	WaitUntil string // "load", "domcontentloaded", "networkidle"
}

// NavigateOption is a function that modifies NavigateOptions
type NavigateOption func(*NavigateOptions)

// WithNavigateTimeout sets navigation timeout
func WithNavigateTimeout(timeout time.Duration) NavigateOption {
	return func(o *NavigateOptions) {
		o.Timeout = timeout
	}
}

// WithWaitUntil sets what to wait for
func WithWaitUntil(state string) NavigateOption {
	return func(o *NavigateOptions) {
		o.WaitUntil = state
	}
}

// ClickOptions configures click behavior
type ClickOptions struct {
	Button  string        // "left", "right", "middle"
	Count   int           // Number of clicks
	Delay   time.Duration // Delay between clicks
	Timeout time.Duration
}

// ClickOption is a function that modifies ClickOptions
type ClickOption func(*ClickOptions)

// WithClickButton sets mouse button
func WithClickButton(button string) ClickOption {
	return func(o *ClickOptions) {
		o.Button = button
	}
}

// WithClickCount sets click count
func WithClickCount(count int) ClickOption {
	return func(o *ClickOptions) {
		o.Count = count
	}
}

// WithClickDelay sets delay between clicks
func WithClickDelay(delay time.Duration) ClickOption {
	return func(o *ClickOptions) {
		o.Delay = delay
	}
}

// WithClickTimeout sets click timeout
func WithClickTimeout(timeout time.Duration) ClickOption {
	return func(o *ClickOptions) {
		o.Timeout = timeout
	}
}

// TypeOptions configures typing behavior
type TypeOptions struct {
	Delay   time.Duration // Delay between keystrokes
	Timeout time.Duration
}

// TypeOption is a function that modifies TypeOptions
type TypeOption func(*TypeOptions)

// WithTypeDelay sets typing delay
func WithTypeDelay(delay time.Duration) TypeOption {
	return func(o *TypeOptions) {
		o.Delay = delay
	}
}

// WithTypeTimeout sets type timeout
func WithTypeTimeout(timeout time.Duration) TypeOption {
	return func(o *TypeOptions) {
		o.Timeout = timeout
	}
}

// WaitOptions configures wait behavior
type WaitOptions struct {
	State   string // "attached", "detached", "visible", "hidden"
	Timeout time.Duration
}

// WaitOption is a function that modifies WaitOptions
type WaitOption func(*WaitOptions)

// WithWaitState sets what state to wait for
func WithWaitState(state string) WaitOption {
	return func(o *WaitOptions) {
		o.State = state
	}
}

// WithWaitTimeout sets wait timeout
func WithWaitTimeout(timeout time.Duration) WaitOption {
	return func(o *WaitOptions) {
		o.Timeout = timeout
	}
}

// ScreenshotOptions configures screenshot behavior
type ScreenshotOptions struct {
	FullPage bool
	Selector string
	Quality  int    // JPEG quality 0-100
	Type     string // "png" or "jpeg"
	Clip     *Clip
}

// Clip defines screenshot clipping area
type Clip struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// ScreenshotOption is a function that modifies ScreenshotOptions
type ScreenshotOption func(*ScreenshotOptions)

// WithFullPage captures full page
func WithFullPage() ScreenshotOption {
	return func(o *ScreenshotOptions) {
		o.FullPage = true
	}
}

// WithScreenshotSelector captures specific element
func WithScreenshotSelector(selector string) ScreenshotOption {
	return func(o *ScreenshotOptions) {
		o.Selector = selector
	}
}

// WithScreenshotQuality sets JPEG quality
func WithScreenshotQuality(quality int) ScreenshotOption {
	return func(o *ScreenshotOptions) {
		o.Quality = quality
		o.Type = "jpeg"
	}
}

// WithScreenshotType sets image type
func WithScreenshotType(imgType string) ScreenshotOption {
	return func(o *ScreenshotOptions) {
		o.Type = imgType
	}
}

// WithScreenshotClip sets clipping area
func WithScreenshotClip(x, y, width, height float64) ScreenshotOption {
	return func(o *ScreenshotOptions) {
		o.Clip = &Clip{X: x, Y: y, Width: width, Height: height}
	}
}

// Compatibility functions for existing tests

// NavigateWithTimeout creates a navigate option with timeout (compatibility)
func NavigateWithTimeout(timeout time.Duration) NavigateOption {
	return WithNavigateTimeout(timeout)
}

// ClickWithTimeout creates a click option with timeout (compatibility)
func ClickWithTimeout(timeout time.Duration) ClickOption {
	return WithClickTimeout(timeout)
}

// TypeWithTimeout creates a type option with timeout (compatibility)
func TypeWithTimeout(timeout time.Duration) TypeOption {
	return WithTypeTimeout(timeout)
}

// WaitWithTimeout creates a wait option with timeout (compatibility)
func WaitWithTimeout(timeout time.Duration) WaitOption {
	return WithWaitTimeout(timeout)
}

// WaitWithState creates a wait option with state (compatibility)
func WaitWithState(state string) WaitOption {
	return WithWaitState(state)
}

// ScreenshotFullPage creates a screenshot option for full page (compatibility)
func ScreenshotFullPage(fullPage bool) ScreenshotOption {
	if fullPage {
		return WithFullPage()
	}
	return func(*ScreenshotOptions) {} // No-op if false
}

// ScreenshotSelector creates a screenshot option for element selector (compatibility)
func ScreenshotSelector(selector string) ScreenshotOption {
	return WithScreenshotSelector(selector)
}

// PDFOptions configures PDF generation. Lengths are in inches, the unit
// Page.printToPDF uses.
type PDFOptions struct {
	// Format is a named paper size (see PaperSizeNames) or explicit
	// dimensions as WxH. Empty means Chrome's own default.
	Format          string
	Landscape       bool
	Scale           float64
	PrintBackground bool
	MarginTop       float64
	MarginBottom    float64
	MarginLeft      float64
	MarginRight     float64

	// HeaderTemplate and FooterTemplate are the HTML templates
	// Page.printToPDF renders in the page margins. They support the classes
	// date, title, url, pageNumber, and totalPages. Setting either enables
	// header/footer display; the margin on that edge must be large enough to
	// show it.
	HeaderTemplate string
	FooterTemplate string

	// PreferCSSPageSize honours @page size declarations in the document's own
	// CSS instead of scaling content to Format.
	PreferCSSPageSize bool

	// PageRanges limits output to the given one-based pages, e.g. "1-5, 8".
	PageRanges string

	// GenerateDocumentOutline embeds PDF bookmarks built from the document's
	// heading structure. Chrome derives them from the tag tree, so this
	// implies GenerateTaggedPDF.
	GenerateDocumentOutline bool

	// GenerateTaggedPDF emits a tagged (accessible) PDF.
	GenerateTaggedPDF bool
}

// PDFOption is a function that modifies PDFOptions
type PDFOption func(*PDFOptions)

// WithPDFFormat sets paper format
func WithPDFFormat(format string) PDFOption {
	return func(o *PDFOptions) {
		o.Format = format
	}
}

// WithPDFLandscape sets landscape orientation
func WithPDFLandscape() PDFOption {
	return func(o *PDFOptions) {
		o.Landscape = true
	}
}

// WithPDFScale sets scale
func WithPDFScale(scale float64) PDFOption {
	return func(o *PDFOptions) {
		o.Scale = scale
	}
}

// WithPDFBackground includes background
func WithPDFBackground() PDFOption {
	return func(o *PDFOptions) {
		o.PrintBackground = true
	}
}

// WithPDFMargins sets margins, in inches.
func WithPDFMargins(top, bottom, left, right float64) PDFOption {
	return func(o *PDFOptions) {
		o.MarginTop = top
		o.MarginBottom = bottom
		o.MarginLeft = left
		o.MarginRight = right
	}
}

// WithPDFHeader sets the header template rendered in the top margin.
func WithPDFHeader(tmpl string) PDFOption {
	return func(o *PDFOptions) {
		o.HeaderTemplate = tmpl
	}
}

// WithPDFFooter sets the footer template rendered in the bottom margin.
func WithPDFFooter(tmpl string) PDFOption {
	return func(o *PDFOptions) {
		o.FooterTemplate = tmpl
	}
}

// WithPDFPreferCSSPageSize honours @page size from the document's CSS.
func WithPDFPreferCSSPageSize() PDFOption {
	return func(o *PDFOptions) {
		o.PreferCSSPageSize = true
	}
}

// WithPDFPageRanges limits output to the given one-based pages, e.g. "1-5, 8".
func WithPDFPageRanges(ranges string) PDFOption {
	return func(o *PDFOptions) {
		o.PageRanges = ranges
	}
}

// WithPDFOutline embeds bookmarks built from the document's headings. It
// implies WithPDFTagged, without which Chrome emits no outline at all.
func WithPDFOutline() PDFOption {
	return func(o *PDFOptions) {
		o.GenerateDocumentOutline = true
	}
}

// WithPDFTagged emits a tagged (accessible) PDF.
func WithPDFTagged() PDFOption {
	return func(o *PDFOptions) {
		o.GenerateTaggedPDF = true
	}
}
