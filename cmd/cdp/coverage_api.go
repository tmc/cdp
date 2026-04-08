package main

import (
	"github.com/tmc/cdp/internal/coverage"
)

// coverageProvider is the subset of session state needed by the coverage API.
type coverageProvider interface {
	getCoverageStore() coverage.Store
}

// startCoverageAPI starts an HTTP server exposing coverage data for the
// DevTools extension. It serves on the given port and never returns
// (intended to be called in a goroutine).
func startCoverageAPI(port int, provider coverageProvider) {
	coverage.StartAPI(port, provider.getCoverageStore())
}
