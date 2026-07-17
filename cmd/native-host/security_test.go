package main

import (
	"strings"
	"testing"
	"time"
)

func newTestSecurityManager(t *testing.T, secret string) *SecurityManager {
	t.Helper()
	cfg := DefaultSecurityConfig()
	cfg.HMACSecret = secret
	sm, err := NewSecurityManager(cfg)
	if err != nil {
		t.Fatalf("NewSecurityManager: %v", err)
	}
	return sm
}

func TestNewSecurityManagerRequiresSecret(t *testing.T) {
	if _, err := NewSecurityManager(DefaultSecurityConfig()); err == nil {
		t.Fatal("NewSecurityManager with empty HMACSecret: got nil error, want error")
	}
}

func TestVerifySignature(t *testing.T) {
	sm := newTestSecurityManager(t, "test-secret")
	msg := Message{Type: "request", ID: "1", Data: "hello"}

	sig, err := sm.GenerateSignature(msg)
	if err != nil {
		t.Fatalf("GenerateSignature: %v", err)
	}

	tests := []struct {
		name string
		msg  Message
		sig  string
		want bool
	}{
		{"valid", msg, sig, true},
		{"tampered data", Message{Type: "request", ID: "1", Data: "hacked"}, sig, false},
		{"tampered type", Message{Type: "response", ID: "1", Data: "hello"}, sig, false},
		{"wrong signature", msg, strings.Repeat("0", len(sig)), false},
		{"empty signature", msg, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sm.VerifySignature(tt.msg, tt.sig); got != tt.want {
				t.Errorf("VerifySignature = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVerifySignatureDifferentSecret(t *testing.T) {
	sm1 := newTestSecurityManager(t, "secret-one")
	sm2 := newTestSecurityManager(t, "secret-two")
	msg := Message{Type: "request", ID: "1"}

	sig, err := sm1.GenerateSignature(msg)
	if err != nil {
		t.Fatalf("GenerateSignature: %v", err)
	}
	if sm2.VerifySignature(msg, sig) {
		t.Error("signature from a different secret verified, want rejection")
	}
}

func TestValidateNonce(t *testing.T) {
	sm := newTestSecurityManager(t, "test-secret")

	if !sm.ValidateNonce("nonce-1") {
		t.Error("first use of nonce: got false, want true")
	}
	if sm.ValidateNonce("nonce-1") {
		t.Error("replayed nonce: got true, want false")
	}
	if !sm.ValidateNonce("nonce-2") {
		t.Error("distinct nonce: got false, want true")
	}
}

func TestCreateNonce(t *testing.T) {
	sm := newTestSecurityManager(t, "test-secret")

	seen := make(map[string]bool)
	for range 100 {
		n := sm.CreateNonce()
		if seen[n] {
			t.Fatalf("CreateNonce returned duplicate %q", n)
		}
		seen[n] = true
	}

	// A locally created nonce is recorded as seen, so reflecting it back
	// must be rejected as a replay.
	if sm.ValidateNonce(sm.CreateNonce()) {
		t.Error("reflected own nonce: got true, want false")
	}
}

func TestValidateTimestamp(t *testing.T) {
	sm := newTestSecurityManager(t, "test-secret") // NonceTimeout is 5 minutes

	now := time.Now().Unix()
	tests := []struct {
		name string
		ts   int64
		want bool
	}{
		{"current", now, true},
		{"one minute old", now - 60, true},
		{"expired", now - 600, false},
		{"one second future", now + 1, true},
		{"one minute future", now + 60, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sm.ValidateTimestamp(tt.ts); got != tt.want {
				t.Errorf("ValidateTimestamp(%d) = %v, want %v", tt.ts, got, tt.want)
			}
		})
	}
}

func TestVerifyAndUnwrapMessage(t *testing.T) {
	sender := newTestSecurityManager(t, "shared-secret")
	receiver := newTestSecurityManager(t, "shared-secret")
	msg := Message{Type: "request", ID: "42", Data: map[string]interface{}{"action": "status"}}

	wrapped, err := sender.SignAndWrapMessage(msg)
	if err != nil {
		t.Fatalf("SignAndWrapMessage: %v", err)
	}

	got, err := receiver.VerifyAndUnwrapMessage(wrapped)
	if err != nil {
		t.Fatalf("VerifyAndUnwrapMessage: %v", err)
	}
	if got.Type != msg.Type || got.ID != msg.ID {
		t.Errorf("unwrapped message = %+v, want %+v", got, msg)
	}

	// Replaying the same wrapped message must be rejected (duplicate nonce).
	if _, err := receiver.VerifyAndUnwrapMessage(wrapped); err == nil {
		t.Error("replayed wrapped message: got nil error, want replay rejection")
	}
}

func TestVerifyAndUnwrapMessageRejectsTampering(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(*SignedMessage)
	}{
		{"tampered message", func(w *SignedMessage) { w.Message.Data = "hacked" }},
		{"tampered signature", func(w *SignedMessage) { w.Signature = strings.Repeat("0", len(w.Signature)) }},
		{"stale timestamp", func(w *SignedMessage) { w.Timestamp = time.Now().Unix() - 3600 }},
		{"future timestamp", func(w *SignedMessage) { w.Timestamp = time.Now().Unix() + 3600 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := newTestSecurityManager(t, "shared-secret")
			receiver := newTestSecurityManager(t, "shared-secret")

			wrapped, err := sender.SignAndWrapMessage(Message{Type: "request", ID: "1", Data: "payload"})
			if err != nil {
				t.Fatalf("SignAndWrapMessage: %v", err)
			}
			tt.tamper(&wrapped)

			if _, err := receiver.VerifyAndUnwrapMessage(wrapped); err == nil {
				t.Error("VerifyAndUnwrapMessage accepted tampered message, want error")
			}
		})
	}
}
