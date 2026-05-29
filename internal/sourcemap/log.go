package sourcemap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AnalysisLogEntry records the reasoning behind sourcemap naming decisions.
type AnalysisLogEntry struct {
	Timestamp    string              `json:"timestamp"`
	BundleURL    string              `json:"bundle_url"`
	Context      string              `json:"context,omitempty"`
	IsRefinement bool                `json:"is_refinement"`
	Summary      string              `json:"summary"`
	Files        []AnalysisFileEntry `json:"files"`
}

// AnalysisFileEntry records one inferred source file in an analysis log entry.
type AnalysisFileEntry struct {
	Path        string              `json:"path"`
	StartOffset int                 `json:"start_offset"`
	EndOffset   int                 `json:"end_offset"`
	Snippet     string              `json:"snippet"`
	Reasoning   string              `json:"reasoning"`
	Functions   []AnalysisFuncEntry `json:"functions,omitempty"`
}

// AnalysisFuncEntry records one inferred function in an analysis log entry.
type AnalysisFuncEntry struct {
	Name      string `json:"name"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Reasoning string `json:"reasoning"`
}

// AnalysisLogPath returns the .analysis-log.jsonl path for a bundle's .map path.
func AnalysisLogPath(mapPath string) string {
	if mapPath == "" {
		return ""
	}
	return strings.TrimSuffix(mapPath, ".map") + ".analysis-log.jsonl"
}

// AppendAnalysisLog appends an entry to the analysis log.
// bundleSource is used to extract short code snippets for each file.
func AppendAnalysisLog(mapPath, bundleURL, contextName string, structure *Structure, bundleSource string, isRefinement bool) error {
	logPath := AnalysisLogPath(mapPath)
	if logPath == "" {
		return fmt.Errorf("no log path")
	}
	if structure == nil {
		return fmt.Errorf("nil structure")
	}

	entry := AnalysisLogEntry{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		BundleURL:    bundleURL,
		Context:      contextName,
		IsRefinement: isRefinement,
		Summary:      structure.Summary,
	}

	for _, f := range structure.Files {
		snippet := ""
		if bundleSource != "" && f.StartOffset >= 0 && f.EndOffset > f.StartOffset && f.EndOffset <= len(bundleSource) {
			snippet = bundleSource[f.StartOffset:f.EndOffset]
		}
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}

		fe := AnalysisFileEntry{
			Path:        f.Path,
			StartOffset: f.StartOffset,
			EndOffset:   f.EndOffset,
			Snippet:     snippet,
			Reasoning:   f.Description,
		}
		for _, fn := range f.Functions {
			fe.Functions = append(fe.Functions, AnalysisFuncEntry{
				Name:      fn.Name,
				StartLine: fn.StartLine,
				EndLine:   fn.EndLine,
				Reasoning: fn.Description,
			})
		}
		entry.Files = append(entry.Files, fe)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(logPath), err)
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", logPath, err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", logPath, err)
	}
	return nil
}

// ReadAnalysisLog reads all valid entries from an analysis log file.
func ReadAnalysisLog(mapPath string) ([]AnalysisLogEntry, error) {
	logPath := AnalysisLogPath(mapPath)
	if logPath == "" {
		return nil, fmt.Errorf("no log path")
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil, err
	}
	var entries []AnalysisLogEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry AnalysisLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// CountAnalysisLogEntries returns the number of valid entries in an analysis log.
func CountAnalysisLogEntries(mapPath string) int {
	entries, err := ReadAnalysisLog(mapPath)
	if err != nil {
		return 0
	}
	return len(entries)
}
