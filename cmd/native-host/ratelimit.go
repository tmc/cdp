// Rate limiting for DoS protection
package main

import (
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	capacity   int
	tokens     float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket with given capacity and refill rate
func NewTokenBucket(capacity int, refillRate float64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     float64(capacity),
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed (consumes 1 token)
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

// AllowN checks if N requests are allowed (consumes N tokens)
func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}

	return false
}

// refill refills tokens based on elapsed time
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tokensToAdd := elapsed * tb.refillRate

	tb.tokens = tb.tokens + tokensToAdd
	if tb.tokens > float64(tb.capacity) {
		tb.tokens = float64(tb.capacity)
	}

	tb.lastRefill = now
}

// RateLimiter manages rate limiting per client
type RateLimiter struct {
	buckets     map[string]*TokenBucket
	mu          sync.RWMutex
	capacity    int
	refillRate  float64
	auditLog    *AuditLogger
	stopCleanup chan struct{}
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerSecond int, burstSize int) *RateLimiter {
	rl := &RateLimiter{
		buckets:     make(map[string]*TokenBucket),
		capacity:    burstSize,
		refillRate:  float64(requestsPerSecond),
		auditLog:    NewAuditLogger(true),
		stopCleanup: make(chan struct{}),
	}

	// Start periodic cleanup of idle buckets
	go rl.cleanupIdleBuckets()

	return rl
}

// Allow checks if a request from a client is allowed
func (rl *RateLimiter) Allow(clientID string) bool {
	rl.mu.Lock()

	bucket, ok := rl.buckets[clientID]
	if !ok {
		// Create new bucket for this client
		bucket = NewTokenBucket(rl.capacity, rl.refillRate)
		rl.buckets[clientID] = bucket
	}

	rl.mu.Unlock()

	// Check if allowed
	allowed := bucket.Allow()

	if !allowed {
		rl.auditLog.LogRateLimitEvent(clientID, true)
	}

	return allowed
}

// AllowN checks if N requests from a client are allowed
func (rl *RateLimiter) AllowN(clientID string, n int) bool {
	rl.mu.Lock()

	bucket, ok := rl.buckets[clientID]
	if !ok {
		// Create new bucket for this client
		bucket = NewTokenBucket(rl.capacity, rl.refillRate)
		rl.buckets[clientID] = bucket
	}

	rl.mu.Unlock()

	// Check if allowed
	allowed := bucket.AllowN(n)

	if !allowed {
		rl.auditLog.LogRateLimitEvent(clientID, true)
	}

	return allowed
}

// GetStats returns rate limiting statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["capacity"] = rl.capacity
	stats["refill_rate"] = rl.refillRate
	stats["active_clients"] = len(rl.buckets)

	return stats
}

// cleanupIdleBuckets periodically removes idle client buckets
func (rl *RateLimiter) cleanupIdleBuckets() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()

			// Remove clients that haven't been seen in 30 minutes
			cutoff := time.Now().Add(-30 * time.Minute)
			for clientID, bucket := range rl.buckets {
				if bucket.lastRefill.Before(cutoff) {
					delete(rl.buckets, clientID)
				}
			}

			rl.mu.Unlock()

		case <-rl.stopCleanup:
			return
		}
	}
}

// Stop stops the rate limiter cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
}
