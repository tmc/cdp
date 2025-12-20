package security

import (
	"testing"
	"time"
)

func TestAuthenticator_SignAndVerify(t *testing.T) {
	tests := []struct {
		name    string
		config  AuthConfig
		payload []byte
		wantErr bool
	}{
		{
			name:    "basic authentication",
			config:  DefaultAuthConfig(),
			payload: []byte("test message"),
			wantErr: false,
		},
		{
			name: "with timestamp validation",
			config: AuthConfig{
				SharedSecret:     []byte("test-secret"),
				EnableTimestamp:  true,
				TimestampWindow:  30 * time.Second,
				EnableNonceCheck: false,
			},
			payload: []byte("test message"),
			wantErr: false,
		},
		{
			name: "with nonce validation",
			config: AuthConfig{
				SharedSecret:     []byte("test-secret"),
				EnableTimestamp:  false,
				EnableNonceCheck: true,
				NonceWindow:      5 * time.Minute,
			},
			payload: []byte("test message"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewAuthenticator(tt.config)
			if err != nil {
				t.Fatalf("NewAuthenticator() error = %v", err)
			}

			// Sign the message
			msg, err := auth.Sign(tt.payload)
			if err != nil {
				t.Fatalf("Sign() error = %v", err)
			}

			// Verify the message
			err = auth.Verify(msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAuthenticator_ReplayAttack(t *testing.T) {
	config := AuthConfig{
		SharedSecret:     []byte("test-secret"),
		EnableNonceCheck: true,
		NonceWindow:      5 * time.Minute,
	}

	auth, err := NewAuthenticator(config)
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}

	// Sign a message
	msg, err := auth.Sign([]byte("test message"))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// First verification should succeed
	if err := auth.Verify(msg); err != nil {
		t.Fatalf("First Verify() failed: %v", err)
	}

	// Second verification should fail (replay attack)
	if err := auth.Verify(msg); err == nil {
		t.Error("Replay attack not detected - second Verify() should fail")
	}
}

func TestAuthenticator_TimestampValidation(t *testing.T) {
	config := AuthConfig{
		SharedSecret:    []byte("test-secret"),
		EnableTimestamp: true,
		TimestampWindow: 5 * time.Second,
	}

	auth, err := NewAuthenticator(config)
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}

	// Create a message with old timestamp
	msg, err := auth.Sign([]byte("test message"))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// Modify timestamp to be outside the window
	msg.Timestamp = time.Now().Add(-10 * time.Second).Unix()

	// Verification should fail due to timestamp
	if err := auth.Verify(msg); err == nil {
		t.Error("Timestamp validation failed - should reject old timestamps")
	}
}

func TestAuthenticator_InvalidSignature(t *testing.T) {
	config := DefaultAuthConfig()
	auth, err := NewAuthenticator(config)
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}

	msg, err := auth.Sign([]byte("test message"))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// Tamper with the payload
	msg.Payload = []byte("tampered message")

	// Verification should fail
	if err := auth.Verify(msg); err == nil {
		t.Error("Signature validation failed - should reject tampered messages")
	}
}
