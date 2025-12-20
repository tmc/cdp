// Security primitives for native messaging host
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// SecurityConfig holds security configuration
type SecurityConfig struct {
	HMACSecret              string
	NonceTimeout            time.Duration
	MaxMessageSize          int64
	RateLimitPerSecond      int
	RateLimitBurst          int
	AuditLoggingEnabled     bool
	CapabilityCheckEnabled  bool
	CommandSandboxingEnabled bool
}

// DefaultSecurityConfig returns secure defaults
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		HMACSecret:              "", // Must be set by caller
		NonceTimeout:            5 * time.Minute,
		MaxMessageSize:          1024 * 1024, // 1MB
		RateLimitPerSecond:      100,
		RateLimitBurst:          10,
		AuditLoggingEnabled:     true,
		CapabilityCheckEnabled:  true,
		CommandSandboxingEnabled: true,
	}
}

// SignedMessage wraps a message with authentication metadata
type SignedMessage struct {
	Message   Message `json:"message"`
	Signature string  `json:"signature"`
	Nonce     string  `json:"nonce"`
	Timestamp int64   `json:"timestamp"`
}

// SecurityManager handles all security operations
type SecurityManager struct {
	config     SecurityConfig
	mu         sync.RWMutex
	nonces     map[string]int64 // nonce -> timestamp seen
	seqNumber  int64            // monotonic sequence number for anti-replay
	auditLog   *AuditLogger
	capabilities *CapabilityManager
	rateLimiter *RateLimiter
}

// NewSecurityManager creates a new security manager
func NewSecurityManager(config SecurityConfig) (*SecurityManager, error) {
	if config.HMACSecret == "" {
		return nil, fmt.Errorf("HMACSecret must be configured for message authentication")
	}

	sm := &SecurityManager{
		config:   config,
		nonces:   make(map[string]int64),
		seqNumber: 0,
		auditLog: NewAuditLogger(config.AuditLoggingEnabled),
		capabilities: NewCapabilityManager(),
		rateLimiter: NewRateLimiter(config.RateLimitPerSecond, config.RateLimitBurst),
	}

	// Start nonce cleanup goroutine
	go sm.cleanupNonces()

	return sm, nil
}

// GenerateSignature creates HMAC-SHA256 signature for a message
func (sm *SecurityManager) GenerateSignature(msg interface{}) (string, error) {
	// Serialize message to JSON
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal message: %v", err)
	}

	// Create HMAC
	h := hmac.New(sha256.New, []byte(sm.config.HMACSecret))
	h.Write(msgJSON)

	// Return hex-encoded signature
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifySignature verifies a message signature
func (sm *SecurityManager) VerifySignature(msg interface{}, signature string) bool {
	expectedSig, err := sm.GenerateSignature(msg)
	if err != nil {
		return false
	}

	// Use constant-time comparison to prevent timing attacks
	return hmac.Equal([]byte(expectedSig), []byte(signature))
}

// CreateNonce creates a unique nonce for replay attack prevention
func (sm *SecurityManager) CreateNonce() string {
	// Get next sequence number atomically
	seq := atomic.AddInt64(&sm.seqNumber, 1)

	// Create nonce from sequence number and timestamp
	timestamp := time.Now().UnixNano()
	nonce := fmt.Sprintf("%d-%d", timestamp, seq)

	// Store nonce with creation time
	sm.mu.Lock()
	sm.nonces[nonce] = timestamp
	sm.mu.Unlock()

	return nonce
}

// ValidateNonce checks if a nonce is valid and hasn't been seen before
func (sm *SecurityManager) ValidateNonce(nonce string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if nonce has been seen
	if _, seen := sm.nonces[nonce]; seen {
		// Replay attack detected
		return false
	}

	// Nonce is valid, mark as seen
	sm.nonces[nonce] = time.Now().UnixNano()
	return true
}

// ValidateTimestamp checks if message timestamp is within acceptable range
func (sm *SecurityManager) ValidateTimestamp(ts int64) bool {
	now := time.Now().Unix()
	diff := now - ts

	// Check if timestamp is within tolerance (5 minutes by default)
	timeoutSeconds := int64(sm.config.NonceTimeout.Seconds())
	return diff >= -1 && diff <= timeoutSeconds // -1 allows for some clock skew
}

// SignAndWrapMessage signs a message and wraps it with authentication metadata
func (sm *SecurityManager) SignAndWrapMessage(msg Message) (SignedMessage, error) {
	nonce := sm.CreateNonce()
	timestamp := time.Now().Unix()

	// Generate signature over the message
	sig, err := sm.GenerateSignature(msg)
	if err != nil {
		return SignedMessage{}, err
	}

	wrapped := SignedMessage{
		Message:   msg,
		Signature: sig,
		Nonce:     nonce,
		Timestamp: timestamp,
	}

	return wrapped, nil
}

// VerifyAndUnwrapMessage verifies a signed message and extracts the original message
func (sm *SecurityManager) VerifyAndUnwrapMessage(wrapped SignedMessage) (Message, error) {
	// Check timestamp
	if !sm.ValidateTimestamp(wrapped.Timestamp) {
		sm.auditLog.Log("TIMESTAMP_INVALID", map[string]interface{}{
			"timestamp": wrapped.Timestamp,
			"current":   time.Now().Unix(),
		})
		return Message{}, fmt.Errorf("message timestamp outside acceptable range")
	}

	// Check nonce (replay attack prevention)
	if !sm.ValidateNonce(wrapped.Nonce) {
		sm.auditLog.Log("REPLAY_ATTACK_DETECTED", map[string]interface{}{
			"nonce": wrapped.Nonce,
		})
		return Message{}, fmt.Errorf("duplicate nonce detected - possible replay attack")
	}

	// Verify signature
	if !sm.VerifySignature(wrapped.Message, wrapped.Signature) {
		sm.auditLog.Log("SIGNATURE_INVALID", map[string]interface{}{
			"message_id": wrapped.Message.ID,
			"type":       wrapped.Message.Type,
		})
		return Message{}, fmt.Errorf("message signature verification failed")
	}

	return wrapped.Message, nil
}

// cleanupNonces periodically removes expired nonces
func (sm *SecurityManager) cleanupNonces() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sm.mu.Lock()
		now := time.Now().UnixNano()
		timeoutNanos := sm.config.NonceTimeout.Nanoseconds()

		for nonce, timestamp := range sm.nonces {
			if now-timestamp > timeoutNanos {
				delete(sm.nonces, nonce)
			}
		}
		sm.mu.Unlock()
	}
}

// GetSecurityStats returns current security statistics
func (sm *SecurityManager) GetSecurityStats() map[string]interface{} {
	sm.mu.RLock()
	nonceCount := len(sm.nonces)
	seqNum := atomic.LoadInt64(&sm.seqNumber)
	sm.mu.RUnlock()

	return map[string]interface{}{
		"active_nonces":   nonceCount,
		"sequence_number": seqNum,
		"audit_enabled":   sm.config.AuditLoggingEnabled,
		"capability_check": sm.config.CapabilityCheckEnabled,
		"sandboxing":      sm.config.CommandSandboxingEnabled,
	}
}
