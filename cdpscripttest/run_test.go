package cdpscripttest

import (
	"path/filepath"
	"testing"

	"github.com/tmc/cdp/cdpscripttest/report"
)

func TestRunReportOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    RunOptions
		wantDir string
		wantNil bool
		wantArt string
	}{
		{"disabled", RunOptions{}, "", true, ""},
		{"legacy", RunOptions{EmitReport: true, ArtifactDir: "artifacts"}, "artifacts", false, "artifacts"},
		{"explicit", RunOptions{ArtifactDir: "artifacts", Report: &report.Options{Dir: "reports"}}, "reports", false, filepath.Join("reports", "script")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runReportOptions(tt.opts)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("runReportOptions() = %#v, want nil", got)
				}
				return
			}
			if got == nil || got.Dir != tt.wantDir {
				t.Fatalf("runReportOptions() = %#v, want directory %q", got, tt.wantDir)
			}
			if artifactDir := runReportArtifactDir(tt.opts, got, "script"); artifactDir != tt.wantArt {
				t.Fatalf("runReportArtifactDir() = %q, want %q", artifactDir, tt.wantArt)
			}
		})
	}
}
