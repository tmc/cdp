package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tmc/cdp/internal/coverage"
)

// V8CoverageCollector collects code coverage from a Node.js process via
// the V8 Inspector Protocol. It converts V8 precise coverage data into
// the internal coverage types shared with the cdp tool.
type V8CoverageCollector struct {
	mu        sync.Mutex
	profiler  *V8Profiler
	debugger  *V8Debugger
	running   bool
	snapshots []*coverage.Snapshot
	verbose   bool
}

// NewV8CoverageCollector creates a coverage collector backed by V8 inspector.
func NewV8CoverageCollector(profiler *V8Profiler, debugger *V8Debugger, verbose bool) *V8CoverageCollector {
	return &V8CoverageCollector{
		profiler: profiler,
		debugger: debugger,
		verbose:  verbose,
	}
}

// Start enables the profiler and begins precise coverage collection.
func (c *V8CoverageCollector) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("coverage collection already running")
	}

	if err := c.profiler.EnableProfiler(); err != nil {
		return fmt.Errorf("enable profiler: %w", err)
	}
	if err := c.debugger.EnableDebugger(); err != nil {
		return fmt.Errorf("enable debugger: %w", err)
	}
	if err := c.profiler.StartPreciseCoverage(true, true); err != nil {
		return fmt.Errorf("start precise coverage: %w", err)
	}

	c.running = true
	c.snapshots = nil
	return nil
}

// Stop ends coverage collection.
func (c *V8CoverageCollector) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}
	c.running = false
	return c.profiler.StopPreciseCoverage()
}

// Running reports whether coverage collection is active.
func (c *V8CoverageCollector) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// TakeSnapshot captures a non-destructive coverage snapshot, converting
// V8 precise coverage results to internal coverage types.
func (c *V8CoverageCollector) TakeSnapshot(name string) (*coverage.Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil, fmt.Errorf("coverage collection not running")
	}

	v8Cov, err := c.profiler.TakePreciseCoverage()
	if err != nil {
		return nil, fmt.Errorf("take precise coverage: %w", err)
	}

	snap := &coverage.Snapshot{
		Name:      name,
		Timestamp: time.Now(),
		Scripts:   make(map[string]*coverage.ScriptCoverage),
	}

	for _, sc := range v8Cov {
		if sc.URL == "" || strings.HasPrefix(sc.URL, "node:") {
			continue
		}

		source, err := c.debugger.GetScriptSource(sc.ScriptID)
		if err != nil {
			if c.verbose {
				fmt.Printf("coverage: skip script %s: %v\n", sc.URL, err)
			}
			continue
		}

		cov := &coverage.ScriptCoverage{
			URL:    sc.URL,
			Source: source,
			Lines:  make(map[int]int),
		}

		for _, fn := range sc.Functions {
			var ranges []coverage.CoverageRange
			var hitCount int
			for _, r := range fn.Ranges {
				cr := coverage.CoverageRange{
					StartOffset: r.StartOffset,
					EndOffset:   r.EndOffset,
					Count:       r.Count,
				}
				ranges = append(ranges, cr)
				cov.ByteRanges = append(cov.ByteRanges, cr)
				if r.Count > 0 {
					hitCount += r.Count
				}
			}

			if len(fn.Ranges) == 0 {
				continue
			}

			startLine, _ := offsetToLineCol(source, fn.Ranges[0].StartOffset)
			endLine, _ := offsetToLineCol(source, fn.Ranges[0].EndOffset)

			cov.Functions = append(cov.Functions, coverage.FunctionCoverage{
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
				sl, _ := offsetToLineCol(source, r.StartOffset)
				el, _ := offsetToLineCol(source, r.EndOffset)
				for line := sl; line <= el; line++ {
					cov.Lines[line] += r.Count
				}
			}
		}

		snap.Scripts[sc.URL] = cov
	}

	c.snapshots = append(c.snapshots, snap)
	return snap, nil
}

// Snapshots returns all captured snapshots.
func (c *V8CoverageCollector) Snapshots() []*coverage.Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*coverage.Snapshot, len(c.snapshots))
	copy(out, c.snapshots)
	return out
}

// ComputeDelta compares two snapshots and returns newly covered lines.
func (c *V8CoverageCollector) ComputeDelta(before, after *coverage.Snapshot) *coverage.Delta {
	d := &coverage.Delta{
		Name:    after.Name,
		Scripts: make(map[string]*coverage.ScriptDelta),
	}

	for url, afterCov := range after.Scripts {
		sd := &coverage.ScriptDelta{
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

// offsetToLineCol converts a byte offset to a 1-based line and column.
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
