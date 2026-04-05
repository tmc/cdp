// Package coverage collects per-file, per-line JavaScript and CSS coverage
// data from a Chrome DevTools Protocol session. It supports repeated
// non-destructive snapshots and differential analysis between snapshots.
package coverage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/css"
	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/cdproto/profiler"
	"github.com/chromedp/chromedp"
)

// Collector gathers code coverage from an active browser context.
// It does not write to disk; callers handle persistence.
type Collector struct {
	mu        sync.Mutex
	outerCtx  context.Context // chromedp context (survives page reloads)
	running   bool
	snapshots []*Snapshot
	verbose   bool
}

// Snapshot holds coverage data captured at a point in time.
type Snapshot struct {
	Name      string                     `json:"name"`
	Timestamp time.Time                  `json:"timestamp"`
	Scripts   map[string]*ScriptCoverage `json:"scripts,omitempty"` // URL -> coverage
	CSS       []*CSSCoverage             `json:"css,omitempty"`
}

// ScriptCoverage holds per-script coverage data.
type ScriptCoverage struct {
	URL        string             `json:"url"`
	Source     string             `json:"-"` // excluded from API responses
	Functions  []FunctionCoverage `json:"functions,omitempty"`
	Lines      map[int]int        `json:"lines,omitempty"`       // line number -> hit count
	ByteRanges []CoverageRange    `json:"byte_ranges,omitempty"` // raw CDP ranges
}

// FunctionCoverage holds coverage for a single function.
type FunctionCoverage struct {
	Name      string          `json:"name"`
	StartLine int             `json:"start_line"`
	EndLine   int             `json:"end_line"`
	HitCount  int             `json:"hit_count"`
	Ranges    []CoverageRange `json:"ranges,omitempty"`
}

// CoverageRange is a byte-offset range with an execution count.
type CoverageRange struct {
	StartOffset int `json:"start_offset"`
	EndOffset   int `json:"end_offset"`
	Count       int `json:"count"`
}

// CSSCoverage holds per-stylesheet coverage data.
type CSSCoverage struct {
	URL        string          `json:"url"`
	Ranges     []CoverageRange `json:"ranges,omitempty"`
	UsedBytes  int             `json:"used_bytes"`
	TotalBytes int             `json:"total_bytes"`
}

// Delta describes lines newly covered between two snapshots.
type Delta struct {
	Name    string                  `json:"name"`
	Scripts map[string]*ScriptDelta `json:"scripts,omitempty"`
}

// ScriptDelta holds per-file differential coverage.
type ScriptDelta struct {
	URL           string      `json:"url"`
	NewlyCovered  map[int]int `json:"newly_covered,omitempty"` // line -> hit count (was 0, now >0)
	TotalLines    int         `json:"total_lines"`
	CoveredBefore int         `json:"covered_before"`
	CoveredAfter  int         `json:"covered_after"`
}

// FileSummary is a per-file coverage summary.
type FileSummary struct {
	URL             string  `json:"url"`
	TotalLines      int     `json:"total_lines"`
	CoveredLines    int     `json:"covered_lines"`
	CoveragePercent float64 `json:"coverage_percent"`
}

// New creates a coverage collector.
func New(verbose bool) *Collector {
	return &Collector{
		verbose: verbose,
	}
}

// Start enables profiling and begins coverage collection on the given
// chromedp browser context. The context must be a chromedp context (from
// chromedp.NewContext or passed through chromedp.Run).
func (c *Collector) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("coverage collection already running")
	}

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if err := profiler.Enable().Do(ctx); err != nil {
			return fmt.Errorf("enable profiler: %w", err)
		}
		if _, err := debugger.Enable().Do(ctx); err != nil {
			return fmt.Errorf("enable debugger: %w", err)
		}
		if _, err := profiler.StartPreciseCoverage().WithCallCount(true).WithDetailed(true).Do(ctx); err != nil {
			return fmt.Errorf("start precise coverage: %w", err)
		}
		if err := css.Enable().Do(ctx); err != nil {
			return fmt.Errorf("enable css: %w", err)
		}
		if err := css.StartRuleUsageTracking().Do(ctx); err != nil {
			return fmt.Errorf("start css rule tracking: %w", err)
		}
		return nil
	})); err != nil {
		return err
	}

	c.outerCtx = ctx
	c.running = true
	c.snapshots = nil
	return nil
}

// TakeSnapshot captures a non-destructive coverage snapshot.
func (c *Collector) TakeSnapshot(name string) (*Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil, fmt.Errorf("coverage collection not running")
	}

	snap := &Snapshot{
		Name:      name,
		Timestamp: time.Now(),
		Scripts:   make(map[string]*ScriptCoverage),
	}

	// Collect JS and CSS coverage via fresh chromedp.Run calls.
	// Using the outer chromedp context ensures we get a valid executor
	// even after page reloads (which reset the inner target context).
	if err := chromedp.Run(c.outerCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		jsCoverage, _, err := profiler.TakePreciseCoverage().Do(ctx)
		if err != nil {
			return fmt.Errorf("take js coverage: %w", err)
		}

		for _, sc := range jsCoverage {
			if sc.URL == "" || strings.HasPrefix(sc.URL, "data:") {
				continue
			}

			source, _, err := debugger.GetScriptSource(sc.ScriptID).Do(ctx)
			if err != nil {
				if c.verbose {
					fmt.Printf("coverage: skip script %s: %v\n", sc.URL, err)
				}
				continue
			}

			cov := &ScriptCoverage{
				URL:    sc.URL,
				Source: source,
				Lines:  make(map[int]int),
			}

			for _, fn := range sc.Functions {
				var ranges []CoverageRange
				var hitCount int
				for _, r := range fn.Ranges {
					cr := CoverageRange{
						StartOffset: int(r.StartOffset),
						EndOffset:   int(r.EndOffset),
						Count:       int(r.Count),
					}
					ranges = append(ranges, cr)
					cov.ByteRanges = append(cov.ByteRanges, cr)
					if r.Count > 0 {
						hitCount += int(r.Count)
					}
				}

				startLine, _ := offsetToLineCol(source, int(fn.Ranges[0].StartOffset))
				endLine, _ := offsetToLineCol(source, int(fn.Ranges[0].EndOffset))

				cov.Functions = append(cov.Functions, FunctionCoverage{
					Name:      fn.FunctionName,
					StartLine: startLine,
					EndLine:   endLine,
					HitCount:  hitCount,
					Ranges:    ranges,
				})

				for _, r := range fn.Ranges {
					if r.Count == 0 {
						continue
					}
					sl, _ := offsetToLineCol(source, int(r.StartOffset))
					el, _ := offsetToLineCol(source, int(r.EndOffset))
					for line := sl; line <= el; line++ {
						cov.Lines[line] += int(r.Count)
					}
				}
			}

			snap.Scripts[sc.URL] = cov
		}

		// Collect CSS coverage delta.
		cssRules, _, err := css.TakeCoverageDelta().Do(ctx)
		if err != nil {
			if c.verbose {
				fmt.Printf("coverage: css delta: %v\n", err)
			}
			return nil // CSS failure is non-fatal
		}
		bySheet := make(map[string]*CSSCoverage)
		for _, rule := range cssRules {
			id := string(rule.StyleSheetID)
			cc, ok := bySheet[id]
			if !ok {
				cc = &CSSCoverage{URL: id}
				bySheet[id] = cc
			}
			size := int(rule.EndOffset - rule.StartOffset)
			cc.TotalBytes += size
			if rule.Used {
				cc.UsedBytes += size
			}
			cc.Ranges = append(cc.Ranges, CoverageRange{
				StartOffset: int(rule.StartOffset),
				EndOffset:   int(rule.EndOffset),
				Count:       boolToInt(rule.Used),
			})
		}
		for _, cc := range bySheet {
			snap.CSS = append(snap.CSS, cc)
		}

		return nil
	})); err != nil {
		return nil, err
	}

	c.snapshots = append(c.snapshots, snap)
	return snap, nil
}

// Stop ends coverage collection and disables profiling.
func (c *Collector) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}
	c.running = false

	var firstErr error
	if err := chromedp.Run(c.outerCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		if err := profiler.StopPreciseCoverage().Do(ctx); err != nil {
			return fmt.Errorf("stop precise coverage: %w", err)
		}
		if err := profiler.Disable().Do(ctx); err != nil {
			return fmt.Errorf("disable profiler: %w", err)
		}
		if _, err := css.StopRuleUsageTracking().Do(ctx); err != nil {
			return fmt.Errorf("stop css tracking: %w", err)
		}
		return nil
	})); err != nil {
		firstErr = err
	}
	return firstErr
}

// Snapshots returns all captured snapshots.
func (c *Collector) Snapshots() []*Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*Snapshot, len(c.snapshots))
	copy(out, c.snapshots)
	return out
}

// ComputeDelta compares two snapshots and returns newly covered lines.
func (c *Collector) ComputeDelta(before, after *Snapshot) *Delta {
	d := &Delta{
		Name:    after.Name,
		Scripts: make(map[string]*ScriptDelta),
	}

	for url, afterCov := range after.Scripts {
		sd := &ScriptDelta{
			URL:          url,
			NewlyCovered: make(map[int]int),
			TotalLines:   countLines(afterCov.Source),
		}

		beforeCov, hasBefore := before.Scripts[url]

		for line, count := range afterCov.Lines {
			if count > 0 {
				sd.CoveredAfter++
			}
			beforeCount := 0
			if hasBefore {
				beforeCount = beforeCov.Lines[line]
			}
			if beforeCount == 0 && count > 0 {
				sd.NewlyCovered[line] = count
			}
		}

		if hasBefore {
			for _, count := range beforeCov.Lines {
				if count > 0 {
					sd.CoveredBefore++
				}
			}
		}

		d.Scripts[url] = sd
	}

	return d
}

// Summary returns per-file coverage summaries for the snapshot.
func (s *Snapshot) Summary() map[string]FileSummary {
	out := make(map[string]FileSummary, len(s.Scripts))
	for url, cov := range s.Scripts {
		total := countLines(cov.Source)
		covered := 0
		for _, count := range cov.Lines {
			if count > 0 {
				covered++
			}
		}
		pct := 0.0
		if total > 0 {
			pct = float64(covered) / float64(total) * 100
		}
		out[url] = FileSummary{
			URL:             url,
			TotalLines:      total,
			CoveredLines:    covered,
			CoveragePercent: pct,
		}
	}
	return out
}

// offsetToLineCol converts a byte offset in source to a 1-based line and column.
func offsetToLineCol(source string, offset int) (line, col int) {
	line = 1
	col = 1
	for i := 0; i < offset && i < len(source); i++ {
		if source[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func countLines(source string) int {
	if source == "" {
		return 0
	}
	return strings.Count(source, "\n") + 1
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SnapshotToLcov formats a snapshot as lcov tracefile content.
func SnapshotToLcov(snap *Snapshot) string {
	var b strings.Builder
	for url, cov := range snap.Scripts {
		fmt.Fprintf(&b, "SF:%s\n", url)
		for _, fn := range cov.Functions {
			fmt.Fprintf(&b, "FN:%d,%s\n", fn.StartLine, fn.Name)
		}
		for _, fn := range cov.Functions {
			fmt.Fprintf(&b, "FNDA:%d,%s\n", fn.HitCount, fn.Name)
		}
		fmt.Fprintf(&b, "FNF:%d\n", len(cov.Functions))
		hit := 0
		for _, fn := range cov.Functions {
			if fn.HitCount > 0 {
				hit++
			}
		}
		fmt.Fprintf(&b, "FNH:%d\n", hit)
		lines := sortedKeys(cov.Lines)
		for _, ln := range lines {
			fmt.Fprintf(&b, "DA:%d,%d\n", ln, cov.Lines[ln])
		}
		fmt.Fprintf(&b, "LF:%d\n", len(cov.Lines))
		lh := 0
		for _, count := range cov.Lines {
			if count > 0 {
				lh++
			}
		}
		fmt.Fprintf(&b, "LH:%d\n", lh)
		b.WriteString("end_of_record\n")
	}
	return b.String()
}

// DeltaToLcov formats a delta as lcov tracefile content showing only
// newly covered lines.
func DeltaToLcov(delta *Delta) string {
	var b strings.Builder
	for url, sd := range delta.Scripts {
		if len(sd.NewlyCovered) == 0 {
			continue
		}
		fmt.Fprintf(&b, "SF:%s\n", url)
		lines := sortedKeys(sd.NewlyCovered)
		for _, ln := range lines {
			fmt.Fprintf(&b, "DA:%d,%d\n", ln, sd.NewlyCovered[ln])
		}
		fmt.Fprintf(&b, "LF:%d\n", sd.TotalLines)
		fmt.Fprintf(&b, "LH:%d\n", len(sd.NewlyCovered))
		b.WriteString("end_of_record\n")
	}
	return b.String()
}

// Running returns true if the collector is currently active.
func (c *Collector) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func sortedKeys(m map[int]int) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
