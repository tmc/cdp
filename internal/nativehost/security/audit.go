package security

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// AuditEventType represents the type of audit event
type AuditEventType string

const (
	EventMessageReceived    AuditEventType = "message_received"
	EventMessageProcessed   AuditEventType = "message_processed"
	EventAuthSuccess        AuditEventType = "auth_success"
	EventAuthFailure        AuditEventType = "auth_failure"
	EventPermissionGranted  AuditEventType = "permission_granted"
	EventPermissionDenied   AuditEventType = "permission_denied"
	EventRateLimitExceeded  AuditEventType = "rate_limit_exceeded"
	EventValidationFailure  AuditEventType = "validation_failure"
	EventSecurityViolation  AuditEventType = "security_violation"
	EventCredentialAccess   AuditEventType = "credential_access"
	EventCommandExecution   AuditEventType = "command_execution"
	EventFileAccess         AuditEventType = "file_access"
	EventNetworkAccess      AuditEventType = "network_access"
)

// AuditEvent represents a security audit event
type AuditEvent struct {
	Timestamp time.Time               `json:"timestamp"`
	Type      AuditEventType          `json:"type"`
	SessionID string                  `json:"session_id,omitempty"`
	ClientID  string                  `json:"client_id,omitempty"`
	Action    string                  `json:"action"`
	Resource  string                  `json:"resource,omitempty"`
	Result    string                  `json:"result"` // success, failure, blocked
	Details   map[string]interface{}  `json:"details,omitempty"`
	Severity  string                  `json:"severity"` // info, warning, error, critical
}

// AuditConfig holds audit logging configuration
type AuditConfig struct {
	Enabled         bool
	LogFile         string
	MaxFileSize     int64 // Bytes
	MaxAge          time.Duration
	BufferSize      int
	FlushInterval   time.Duration
	LogToStderr     bool
	MinSeverity     string // Only log events at or above this severity
}

// DefaultAuditConfig returns default audit configuration
func DefaultAuditConfig() AuditConfig {
	return AuditConfig{
		Enabled:       true,
		LogFile:       "/tmp/chrome-native-host-audit.log",
		MaxFileSize:   10 * 1024 * 1024, // 10MB
		MaxAge:        7 * 24 * time.Hour, // 7 days
		BufferSize:    100,
		FlushInterval: 5 * time.Second,
		LogToStderr:   false,
		MinSeverity:   "info",
	}
}

// AuditLogger handles security audit logging
type AuditLogger struct {
	config AuditConfig
	file   *os.File
	buffer []AuditEvent
	mu     sync.Mutex
	done   chan struct{}
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(config AuditConfig) (*AuditLogger, error) {
	if !config.Enabled {
		return &AuditLogger{config: config, done: make(chan struct{})}, nil
	}

	file, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}

	logger := &AuditLogger{
		config: config,
		file:   file,
		buffer: make([]AuditEvent, 0, config.BufferSize),
		done:   make(chan struct{}),
	}

	// Start flush goroutine
	go logger.flushLoop()

	return logger, nil
}

// Log logs an audit event
func (al *AuditLogger) Log(event AuditEvent) {
	if !al.config.Enabled {
		return
	}

	// Check severity filter
	if !al.shouldLog(event.Severity) {
		return
	}

	event.Timestamp = time.Now()

	al.mu.Lock()
	al.buffer = append(al.buffer, event)
	needsFlush := len(al.buffer) >= al.config.BufferSize
	al.mu.Unlock()

	if needsFlush {
		al.Flush()
	}
}

// shouldLog checks if event meets severity threshold
func (al *AuditLogger) shouldLog(severity string) bool {
	severityOrder := map[string]int{
		"info":     1,
		"warning":  2,
		"error":    3,
		"critical": 4,
	}

	eventLevel := severityOrder[severity]
	minLevel := severityOrder[al.config.MinSeverity]

	return eventLevel >= minLevel
}

// Flush writes buffered events to disk
func (al *AuditLogger) Flush() error {
	if !al.config.Enabled || al.file == nil {
		return nil
	}

	al.mu.Lock()
	events := make([]AuditEvent, len(al.buffer))
	copy(events, al.buffer)
	al.buffer = al.buffer[:0]
	al.mu.Unlock()

	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			if al.config.LogToStderr {
				fmt.Fprintf(os.Stderr, "Failed to marshal audit event: %v\n", err)
			}
			continue
		}

		if _, err := al.file.Write(append(data, '\n')); err != nil {
			if al.config.LogToStderr {
				fmt.Fprintf(os.Stderr, "Failed to write audit event: %v\n", err)
			}
			return err
		}
	}

	return al.file.Sync()
}

// flushLoop periodically flushes the buffer
func (al *AuditLogger) flushLoop() {
	ticker := time.NewTicker(al.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			al.Flush()
		case <-al.done:
			al.Flush()
			return
		}
	}
}

// Close closes the audit logger
func (al *AuditLogger) Close() error {
	if !al.config.Enabled {
		return nil
	}

	close(al.done)
	if err := al.Flush(); err != nil {
		return err
	}

	if al.file != nil {
		return al.file.Close()
	}

	return nil
}

// LogMessageReceived logs when a message is received
func (al *AuditLogger) LogMessageReceived(sessionID, messageType, messageID string) {
	al.Log(AuditEvent{
		Type:      EventMessageReceived,
		SessionID: sessionID,
		Action:    "receive_message",
		Result:    "success",
		Details: map[string]interface{}{
			"message_type": messageType,
			"message_id":   messageID,
		},
		Severity: "info",
	})
}

// LogAuthFailure logs authentication failures
func (al *AuditLogger) LogAuthFailure(sessionID, reason string) {
	al.Log(AuditEvent{
		Type:      EventAuthFailure,
		SessionID: sessionID,
		Action:    "authenticate",
		Result:    "failure",
		Details: map[string]interface{}{
			"reason": reason,
		},
		Severity: "warning",
	})
}

// LogPermissionDenied logs permission denials
func (al *AuditLogger) LogPermissionDenied(sessionID, capability, reason string) {
	al.Log(AuditEvent{
		Type:      EventPermissionDenied,
		SessionID: sessionID,
		Action:    "check_permission",
		Resource:  capability,
		Result:    "denied",
		Details: map[string]interface{}{
			"reason": reason,
		},
		Severity: "warning",
	})
}

// LogRateLimitExceeded logs rate limit violations
func (al *AuditLogger) LogRateLimitExceeded(clientID string) {
	al.Log(AuditEvent{
		Type:     EventRateLimitExceeded,
		ClientID: clientID,
		Action:   "rate_check",
		Result:   "blocked",
		Severity: "warning",
	})
}

// LogSecurityViolation logs security violations
func (al *AuditLogger) LogSecurityViolation(sessionID, violation string, details map[string]interface{}) {
	al.Log(AuditEvent{
		Type:      EventSecurityViolation,
		SessionID: sessionID,
		Action:    "security_check",
		Result:    "violation",
		Details:   details,
		Severity:  "error",
	})
}

// LogCommandExecution logs command executions
func (al *AuditLogger) LogCommandExecution(sessionID, command, result string) {
	al.Log(AuditEvent{
		Type:      EventCommandExecution,
		SessionID: sessionID,
		Action:    "execute_command",
		Resource:  command,
		Result:    result,
		Severity:  "info",
	})
}
