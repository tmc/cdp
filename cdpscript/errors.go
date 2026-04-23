package cdpscript

import "errors"

// Sentinel errors used by cdpscript callers to classify failures.
var (
	ErrAssertionFailed = errors.New("assertion failed")
	ErrUsage           = errors.New("usage error")
)
