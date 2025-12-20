package executor

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// AssertionResult represents the result of an assertion.
type AssertionResult struct {
	Name      string
	Type      string
	Passed    bool
	Message   string
	Timestamp time.Time
	Details   map[string]interface{}
}

// AssertionTracker tracks assertion results during script execution.
type AssertionTracker struct {
	mu      sync.RWMutex
	results []AssertionResult
}

// NewAssertionTracker creates a new assertion tracker.
func NewAssertionTracker() *AssertionTracker {
	return &AssertionTracker{
		results: make([]AssertionResult, 0),
	}
}

// AddResult adds an assertion result.
func (t *AssertionTracker) AddResult(result AssertionResult) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if result.Timestamp.IsZero() {
		result.Timestamp = time.Now()
	}

	t.results = append(t.results, result)
}

// GetResults returns all assertion results.
func (t *AssertionTracker) GetResults() []AssertionResult {
	t.mu.RLock()
	defer t.mu.RUnlock()

	results := make([]AssertionResult, len(t.results))
	copy(results, t.results)
	return results
}

// PassCount returns the number of passed assertions.
func (t *AssertionTracker) PassCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	for _, r := range t.results {
		if r.Passed {
			count++
		}
	}
	return count
}

// FailCount returns the number of failed assertions.
func (t *AssertionTracker) FailCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	for _, r := range t.results {
		if !r.Passed {
			count++
		}
	}
	return count
}

// AllPassed returns true if all assertions passed.
func (t *AssertionTracker) AllPassed() bool {
	return t.FailCount() == 0
}

// Summary returns a summary of assertion results.
func (t *AssertionTracker) Summary() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.results) == 0 {
		return "No assertions"
	}

	passed := 0
	failed := 0
	for _, r := range t.results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	return fmt.Sprintf("%d assertions: %d passed, %d failed", len(t.results), passed, failed)
}

// ConsoleLog represents a console log entry.
type ConsoleLog struct {
	Level     string
	Message   string
	Timestamp time.Time
	Source    string
	LineNo    int
}

// ConsoleTracker tracks console messages during script execution.
type ConsoleTracker struct {
	mu   sync.RWMutex
	logs []ConsoleLog
}

// NewConsoleTracker creates a new console tracker.
func NewConsoleTracker() *ConsoleTracker {
	return &ConsoleTracker{
		logs: make([]ConsoleLog, 0),
	}
}

// AddLog adds a console log entry.
func (t *ConsoleTracker) AddLog(log ConsoleLog) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}

	t.logs = append(t.logs, log)
}

// GetLogs returns all console logs.
func (t *ConsoleTracker) GetLogs() []ConsoleLog {
	t.mu.RLock()
	defer t.mu.RUnlock()

	logs := make([]ConsoleLog, len(t.logs))
	copy(logs, t.logs)
	return logs
}

// GetErrors returns all error-level logs.
func (t *ConsoleTracker) GetErrors() []ConsoleLog {
	t.mu.RLock()
	defer t.mu.RUnlock()

	errors := make([]ConsoleLog, 0)
	for _, log := range t.logs {
		if log.Level == "error" {
			errors = append(errors, log)
		}
	}
	return errors
}

// HasErrors returns true if there are any error-level logs.
func (t *ConsoleTracker) HasErrors() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, log := range t.logs {
		if log.Level == "error" {
			return true
		}
	}
	return false
}

// NetworkRequest represents a network request/response pair.
type NetworkRequest struct {
	URL        string
	Method     string
	Status     int
	StatusText string
	Headers    map[string]string
	Timestamp  time.Time
	Duration   time.Duration
}

// NetworkTracker tracks network requests during script execution.
type NetworkTracker struct {
	mu       sync.RWMutex
	requests []NetworkRequest
}

// NewNetworkTracker creates a new network tracker.
func NewNetworkTracker() *NetworkTracker {
	return &NetworkTracker{
		requests: make([]NetworkRequest, 0),
	}
}

// AddRequest adds a network request.
func (t *NetworkTracker) AddRequest(req NetworkRequest) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}

	t.requests = append(t.requests, req)
}

// GetRequests returns all network requests.
func (t *NetworkTracker) GetRequests() []NetworkRequest {
	t.mu.RLock()
	defer t.mu.RUnlock()

	reqs := make([]NetworkRequest, len(t.requests))
	copy(reqs, t.requests)
	return reqs
}

// FindRequest finds a request by URL pattern.
func (t *NetworkTracker) FindRequest(urlPattern string) *NetworkRequest {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for i := range t.requests {
		if strings.Contains(t.requests[i].URL, urlPattern) {
			return &t.requests[i]
		}
	}
	return nil
}

// PerformanceMetrics represents performance metrics.
type PerformanceMetrics struct {
	NavigationStart     time.Time
	DOMContentLoaded    time.Duration
	LoadComplete        time.Duration
	FirstPaint          time.Duration
	FirstContentfulPaint time.Duration
}

// PerformanceTracker tracks performance metrics.
type PerformanceTracker struct {
	mu      sync.RWMutex
	metrics PerformanceMetrics
}

// NewPerformanceTracker creates a new performance tracker.
func NewPerformanceTracker() *PerformanceTracker {
	return &PerformanceTracker{}
}

// SetMetrics sets the performance metrics.
func (t *PerformanceTracker) SetMetrics(metrics PerformanceMetrics) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.metrics = metrics
}

// GetMetrics returns the performance metrics.
func (t *PerformanceTracker) GetMetrics() PerformanceMetrics {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.metrics
}
