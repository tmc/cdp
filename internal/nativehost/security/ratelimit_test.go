package security

import (
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         20,
		CleanupInterval:   1 * time.Minute,
		BlockDuration:     5 * time.Minute,
	}

	rl := NewRateLimiter(config)
	clientID := "test-client"

	// Should allow burst size requests immediately
	for i := 0; i < config.BurstSize; i++ {
		if err := rl.Allow(clientID); err != nil {
			t.Errorf("Request %d should be allowed, got error: %v", i, err)
		}
	}

	// Next request should be blocked
	if err := rl.Allow(clientID); err == nil {
		t.Error("Request should be rate limited after burst")
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         10,
		CleanupInterval:   1 * time.Minute,
		BlockDuration:     1 * time.Second,
	}

	rl := NewRateLimiter(config)
	clientID := "test-client"

	// Exhaust tokens
	for i := 0; i < config.BurstSize; i++ {
		rl.Allow(clientID)
	}

	// Wait for some tokens to refill
	time.Sleep(500 * time.Millisecond)

	// Should have ~5 tokens refilled (10 per second * 0.5 seconds)
	allowed := 0
	for i := 0; i < 10; i++ {
		if err := rl.Allow(clientID); err == nil {
			allowed++
		}
	}

	if allowed < 3 || allowed > 7 {
		t.Errorf("Expected ~5 tokens refilled, got %d allowed requests", allowed)
	}
}

func TestRateLimiter_MultipleClients(t *testing.T) {
	config := DefaultRateLimitConfig()
	rl := NewRateLimiter(config)

	// Each client should have independent rate limits
	for i := 0; i < 3; i++ {
		clientID := string(rune('A' + i))
		for j := 0; j < config.BurstSize; j++ {
			if err := rl.Allow(clientID); err != nil {
				t.Errorf("Client %s request %d should be allowed", clientID, j)
			}
		}
	}
}

func TestRateLimiter_GetStats(t *testing.T) {
	config := DefaultRateLimitConfig()
	rl := NewRateLimiter(config)
	clientID := "test-client"

	// Make some requests
	for i := 0; i < 5; i++ {
		rl.Allow(clientID)
	}

	stats := rl.GetStats(clientID)
	if !stats["exists"].(bool) {
		t.Error("Client stats should exist")
	}

	totalRequests := stats["total_requests"].(uint64)
	if totalRequests != 5 {
		t.Errorf("Expected 5 total requests, got %d", totalRequests)
	}
}

func TestRateLimiter_BlockDuration(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
		BlockDuration:     500 * time.Millisecond,
		CleanupInterval:   1 * time.Minute,
	}

	rl := NewRateLimiter(config)
	clientID := "test-client"

	// Exhaust tokens
	for i := 0; i < config.BurstSize; i++ {
		rl.Allow(clientID)
	}

	// Next request triggers block
	rl.Allow(clientID)

	// Should be blocked
	if err := rl.Allow(clientID); err == nil {
		t.Error("Client should be blocked")
	}

	// Wait for block to expire
	time.Sleep(600 * time.Millisecond)

	// Should be unblocked with refilled tokens
	if err := rl.Allow(clientID); err != nil {
		t.Errorf("Client should be unblocked after block duration, got error: %v", err)
	}
}
