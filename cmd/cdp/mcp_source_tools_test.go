package main

import (
	"strings"
	"testing"

	"github.com/tmc/cdp/internal/coverage"
)

func TestFormatCoverageSummary(t *testing.T) {
	tests := []struct {
		name     string
		summary  map[string]coverage.FileSummary
		want     []string
		wantNote bool
	}{
		{
			name:     "empty",
			summary:  nil,
			want:     []string{"Coverage snapshot: s1\n", "Files: 0\n"},
			wantNote: true,
		},
		{
			name: "least covered first",
			summary: map[string]coverage.FileSummary{
				"https://example.com/a.js": {TotalLines: 10, CoveredLines: 9, CoveragePercent: 90},
				"https://example.com/b.js": {TotalLines: 10, CoveredLines: 1, CoveragePercent: 10},
			},
			want: []string{"Files: 2\n", " 10.0%     1/  10 lines  https://example.com/b.js\n   90.0%"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCoverageSummary("s1", tt.summary)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("formatCoverageSummary missing %q in:\n%s", w, got)
				}
			}
			if hasNote := strings.Contains(got, noCoverageNote); hasNote != tt.wantNote {
				t.Errorf("formatCoverageSummary note = %v, want %v:\n%s", hasNote, tt.wantNote, got)
			}
		})
	}
}
