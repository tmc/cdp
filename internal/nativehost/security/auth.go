// Package security provides security features for the native messaging host
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// AuthConfig holds authentication configuration
type AuthConfig struct {
	SharedSecret     []byte
	NonceWindow      time.Duration // How long a nonce is valid
	EnableTimestamp  bool          // Require timestamp validation
	TimestampWindow  time.Duration // How much clock skew to allow
	EnableNonceCheck bool          // Prevent replay attacks
}

// DefaultAuthConfig returns default authentication configuration
func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		NonceWindow:      5 * time.Minute,
		EnableTimestamp:  true,
		TimestampWindow:  30 * time.Second,
		EnableNonceCheck: true,
	}
}

// Authenticator handles message authentication
type Authenticator struct {
	config AuthConfig
	nonces sync.Map // Store used nonces
	mu     sync.RWMutex
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(config AuthConfig) (*Authenticator, error) {
	if len(config.SharedSecret) == 0 {
		// Generate a random shared secret if none provided
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, fmt.Errorf("failed to generate shared secret: %w", err)
		}
		config.SharedSecret = secret
	}

	return &Authenticator{
		config: config,
	}, nil
}

// AuthenticatedMessage represents a message with authentication
type AuthenticatedMessage struct {
	Payload   []byte `json:"payload"`
	Signature string `json:"signature"`
	Nonce     string `json:"nonce,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// Sign creates an authenticated message
func (a *Authenticator) Sign(payload []byte) (*AuthenticatedMessage, error) {
	msg := &AuthenticatedMessage{
		Payload: payload,
	}

	// Add timestamp if enabled
	if a.config.EnableTimestamp {
		msg.Timestamp = time.Now().Unix()
	}

	// Add nonce if enabled
	if a.config.EnableNonceCheck {
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return nil, fmt.Errorf("failed to generate nonce: %w", err)
		}
		msg.Nonce = base64.StdEncoding.EncodeToString(nonce)
	}

	// Create signature
	h := hmac.New(sha256.New, a.config.SharedSecret)
	h.Write(payload)
	if msg.Timestamp > 0 {
		h.Write([]byte(fmt.Sprintf("%d", msg.Timestamp)))
	}
	if msg.Nonce != "" {
		h.Write([]byte(msg.Nonce))
	}
	msg.Signature = base64.StdEncoding.EncodeToString(h.Sum(nil))

	return msg, nil
}

// Verify checks if a message is authentic
func (a *Authenticator) Verify(msg *AuthenticatedMessage) error {
	// Verify timestamp if enabled
	if a.config.EnableTimestamp {
		if msg.Timestamp == 0 {
			return fmt.Errorf("missing timestamp")
		}
		msgTime := time.Unix(msg.Timestamp, 0)
		now := time.Now()
		diff := now.Sub(msgTime)
		if diff < 0 {
			diff = -diff
		}
		if diff > a.config.TimestampWindow {
			return fmt.Errorf("timestamp out of window: %v", diff)
		}
	}

	// Check nonce if enabled
	if a.config.EnableNonceCheck {
		if msg.Nonce == "" {
			return fmt.Errorf("missing nonce")
		}

		// Check if nonce was already used
		if _, exists := a.nonces.LoadOrStore(msg.Nonce, time.Now()); exists {
			return fmt.Errorf("nonce replay detected")
		}

		// Clean up old nonces periodically
		go a.cleanupNonces()
	}

	// Verify signature
	h := hmac.New(sha256.New, a.config.SharedSecret)
	h.Write(msg.Payload)
	if msg.Timestamp > 0 {
		h.Write([]byte(fmt.Sprintf("%d", msg.Timestamp)))
	}
	if msg.Nonce != "" {
		h.Write([]byte(msg.Nonce))
	}
	expectedSig := base64.StdEncoding.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(expectedSig), []byte(msg.Signature)) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// cleanupNonces removes expired nonces
func (a *Authenticator) cleanupNonces() {
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := time.Now().Add(-a.config.NonceWindow)
	a.nonces.Range(func(key, value interface{}) bool {
		if timestamp, ok := value.(time.Time); ok {
			if timestamp.Before(cutoff) {
				a.nonces.Delete(key)
			}
		}
		return true
	})
}

// GetSharedSecret returns the shared secret (for initial setup)
func (a *Authenticator) GetSharedSecret() string {
	return base64.StdEncoding.EncodeToString(a.config.SharedSecret)
}
