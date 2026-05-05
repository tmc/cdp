package cdpscripttest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tmc/cdp/internal/coverage"
)

const coverageFile = "coverage.json"

func coverageEnabled() bool {
	switch os.Getenv("CDPSCRIPTTEST_COVERAGE") {
	case "0", "false", "FALSE", "off", "OFF":
		return false
	}
	return true
}

func startCoverage(s *State) (*coverage.Collector, error) {
	if !coverageEnabled() {
		return nil, nil
	}
	c := coverage.New(false)
	if err := c.Start(s.cdpCtx); err != nil {
		return nil, fmt.Errorf("start coverage: %w", err)
	}
	return c, nil
}

func finishCoverage(c *coverage.Collector, s *State) error {
	if c == nil {
		return nil
	}
	var firstErr error
	snap, err := c.TakeSnapshot("final")
	if err != nil {
		firstErr = fmt.Errorf("take coverage snapshot: %w", err)
	} else {
		dir := s.artifactDirFn()
		if err := os.MkdirAll(dir, 0o777); err != nil {
			firstErr = fmt.Errorf("mkdir coverage dir: %w", err)
		} else {
			data, err := json.MarshalIndent(snap, "", "\t")
			if err != nil {
				firstErr = fmt.Errorf("marshal coverage: %w", err)
			} else if err := os.WriteFile(filepath.Join(dir, coverageFile), append(data, '\n'), 0o666); err != nil {
				firstErr = fmt.Errorf("write coverage: %w", err)
			}
		}
	}
	if err := c.Stop(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("stop coverage: %w", err)
	}
	return firstErr
}
