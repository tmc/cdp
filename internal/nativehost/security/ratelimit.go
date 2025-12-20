package security

import (
	"fmt"
	"sync"
	"time"
)

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerSecond int
	BurstSize         int
	CleanupInterval   time.Duration
	BlockDuration     time.Duration // How long to block after exceeding limit
}

// DefaultRateLimitConfig returns default rate limiting configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         20,
		CleanupInterval:   1 * time.Minute,
		BlockDuration:     5 * time.Minute,
	}
}

// StrictRateLimitConfig returns a more restrictive configuration
func StrictRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 5,
		BurstSize:         10,
		CleanupInterval:   30 * time.Second,
		BlockDuration:     10 * time.Minute,
	}
}

// clientState tracks rate limit state for a client
type clientState struct {
	tokens      int
	lastRefill  time.Time
	blockedUntil time.Time
	totalRequests uint64
	blockedRequests uint64
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	config  RateLimitConfig
	clients sync.Map // map[string]*clientState
	mu      sync.RWMutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config: config,
	}

	// Start cleanup goroutine
	go rl.cleanupLoop()

	return rl
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow(clientID string) error {
	now := time.Now()

	// Get or create client state
	stateVal, _ := rl.clients.LoadOrStore(clientID, &clientState{
		tokens:     rl.config.BurstSize,
		lastRefill: now,
	})
	state := stateVal.(*clientState)

	// Check if client is blocked
	if now.Before(state.blockedUntil) {
		state.blockedRequests++
		return fmt.Errorf("rate limit exceeded: blocked until %v", state.blockedUntil)
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(state.lastRefill)
	tokensToAdd := int(elapsed.Seconds() * float64(rl.config.RequestsPerSecond))
	if tokensToAdd > 0 {
		state.tokens += tokensToAdd
		if state.tokens > rl.config.BurstSize {
			state.tokens = rl.config.BurstSize
		}
		state.lastRefill = now
	}

	// Check if we have tokens available
	if state.tokens <= 0 {
		// Block the client
		state.blockedUntil = now.Add(rl.config.BlockDuration)
		state.blockedRequests++
		return fmt.Errorf("rate limit exceeded: too many requests")
	}

	// Consume a token
	state.tokens--
	state.totalRequests++

	return nil
}

// GetStats returns rate limit statistics for a client
func (rl *RateLimiter) GetStats(clientID string) map[string]interface{} {
	stateVal, exists := rl.clients.Load(clientID)
	if !exists {
		return map[string]interface{}{
			"exists": false,
		}
	}

	state := stateVal.(*clientState)
	return map[string]interface{}{
		"exists":           true,
		"tokens":           state.tokens,
		"last_refill":      state.lastRefill,
		"blocked_until":    state.blockedUntil,
		"total_requests":   state.totalRequests,
		"blocked_requests": state.blockedRequests,
	}
}

// Reset resets rate limit state for a client
func (rl *RateLimiter) Reset(clientID string) {
	rl.clients.Delete(clientID)
}

// cleanupLoop periodically removes stale client states
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanup()
	}
}

// cleanup removes stale client states
func (rl *RateLimiter) cleanup() {
	now := time.Now()
	cutoff := now.Add(-2 * rl.config.CleanupInterval)

	rl.clients.Range(func(key, value interface{}) bool {
		state := value.(*clientState)
		// Remove if last activity was before cutoff and not blocked
		if state.lastRefill.Before(cutoff) && now.After(state.blockedUntil) {
			rl.clients.Delete(key)
		}
		return true
	})
}

// GlobalStats returns global rate limit statistics
func (rl *RateLimiter) GlobalStats() map[string]interface{} {
	var totalClients, blockedClients int
	var totalRequests, blockedRequests uint64

	now := time.Now()
	rl.clients.Range(func(key, value interface{}) bool {
		totalClients++
		state := value.(*clientState)
		totalRequests += state.totalRequests
		blockedRequests += state.blockedRequests
		if now.Before(state.blockedUntil) {
			blockedClients++
		}
		return true
	})

	return map[string]interface{}{
		"total_clients":    totalClients,
		"blocked_clients":  blockedClients,
		"total_requests":   totalRequests,
		"blocked_requests": blockedRequests,
		"config": map[string]interface{}{
			"requests_per_second": rl.config.RequestsPerSecond,
			"burst_size":          rl.config.BurstSize,
		},
	}
}
