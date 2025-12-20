// Audit logging for security events
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AuditEntry represents a single audit log entry
type AuditEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	EventType   string                 `json:"event_type"`
	Severity    string                 `json:"severity"` // INFO, WARNING, CRITICAL
	SourceIP    string                 `json:"source_ip,omitempty"`
	MessageID   string                 `json:"message_id,omitempty"`
	MessageType string                 `json:"message_type,omitempty"`
	Details     map[string]interface{} `json:"details"`
}

// AuditLogger manages security audit logging
type AuditLogger struct {
	enabled    bool
	mu         sync.Mutex
	file       *os.File
	logDir     string
	maxSizeBytes int64
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(enabled bool) *AuditLogger {
	logger := &AuditLogger{
		enabled:      enabled,
		logDir:       "/tmp/chrome-native-host-audit",
		maxSizeBytes: 100 * 1024 * 1024, // 100MB per log file
	}

	if enabled {
		logger.initialize()
	}

	return logger
}

// initialize sets up the audit log directory and file
func (al *AuditLogger) initialize() {
	// Create log directory if needed
	if err := os.MkdirAll(al.logDir, 0700); err != nil {
		log.Printf("Warning: Failed to create audit log directory: %v", err)
		return
	}

	// Open or create audit log file
	logFile := filepath.Join(al.logDir, "audit.log")
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Printf("Warning: Failed to open audit log file: %v", err)
		return
	}

	al.file = file

	// Log initialization event
	al.logInternal(AuditEntry{
		Timestamp: time.Now(),
		EventType: "AUDIT_LOG_STARTED",
		Severity:  "INFO",
		Details: map[string]interface{}{
			"version": "1.0.0",
		},
	})
}

// Log logs a security event
func (al *AuditLogger) Log(eventType string, details map[string]interface{}) {
	if !al.enabled {
		return
	}

	// Determine severity based on event type
	severity := al.determineSeverity(eventType)

	entry := AuditEntry{
		Timestamp: time.Now(),
		EventType: eventType,
		Severity:  severity,
		Details:   details,
	}

	al.logInternal(entry)

	// Also log to stderr for critical events
	if severity == "CRITICAL" {
		log.Printf("SECURITY ALERT [%s]: %v", eventType, details)
	}
}

// LogAuthEvent logs authentication events
func (al *AuditLogger) LogAuthEvent(success bool, clientID string, reason string) {
	if !al.enabled {
		return
	}

	eventType := "AUTH_SUCCESS"
	severity := "INFO"
	if !success {
		eventType = "AUTH_FAILURE"
		severity = "WARNING"
	}

	entry := AuditEntry{
		Timestamp: time.Now(),
		EventType: eventType,
		Severity:  severity,
		Details: map[string]interface{}{
			"client_id": clientID,
			"reason":    reason,
		},
	}

	al.logInternal(entry)
}

// LogPermissionCheck logs permission verification events
func (al *AuditLogger) LogPermissionCheck(allowed bool, capability string, action string) {
	if !al.enabled {
		return
	}

	eventType := "PERMISSION_ALLOWED"
	severity := "INFO"
	if !allowed {
		eventType = "PERMISSION_DENIED"
		severity = "WARNING"
	}

	entry := AuditEntry{
		Timestamp: time.Now(),
		EventType: eventType,
		Severity:  severity,
		Details: map[string]interface{}{
			"capability": capability,
			"action":     action,
		},
	}

	al.logInternal(entry)
}

// LogRateLimitEvent logs rate limiting events
func (al *AuditLogger) LogRateLimitEvent(clientID string, rateExceeded bool) {
	if !al.enabled {
		return
	}

	eventType := "RATE_LIMIT_OK"
	severity := "INFO"
	if rateExceeded {
		eventType = "RATE_LIMIT_EXCEEDED"
		severity = "WARNING"
	}

	entry := AuditEntry{
		Timestamp: time.Now(),
		EventType: eventType,
		Severity:  severity,
		Details: map[string]interface{}{
			"client_id": clientID,
		},
	}

	al.logInternal(entry)
}

// LogCommandExecution logs external command execution
func (al *AuditLogger) LogCommandExecution(command string, allowed bool, output string) {
	if !al.enabled {
		return
	}

	eventType := "COMMAND_EXECUTED"
	severity := "INFO"
	if !allowed {
		eventType = "COMMAND_DENIED"
		severity = "WARNING"
	}

	entry := AuditEntry{
		Timestamp: time.Now(),
		EventType: eventType,
		Severity:  severity,
		Details: map[string]interface{}{
			"command": command,
			"output":  truncateString(output, 500), // Limit output length
		},
	}

	al.logInternal(entry)
}

// logInternal writes an audit entry to the log file
func (al *AuditLogger) logInternal(entry AuditEntry) {
	al.mu.Lock()
	defer al.mu.Unlock()

	if al.file == nil {
		return
	}

	// Check if log file needs rotation
	if al.shouldRotateLog() {
		al.rotateLog()
	}

	// Marshal entry to JSON
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Warning: Failed to marshal audit entry: %v", err)
		return
	}

	// Write to file with newline
	if _, err := al.file.Write(append(entryJSON, '\n')); err != nil {
		log.Printf("Warning: Failed to write audit entry: %v", err)
	}
}

// shouldRotateLog checks if log file should be rotated
func (al *AuditLogger) shouldRotateLog() bool {
	if al.file == nil {
		return false
	}

	info, err := al.file.Stat()
	if err != nil {
		return false
	}

	return info.Size() >= al.maxSizeBytes
}

// rotateLog rotates the audit log file
func (al *AuditLogger) rotateLog() {
	if al.file != nil {
		al.file.Close()
	}

	// Rename current file with timestamp
	oldPath := filepath.Join(al.logDir, "audit.log")
	newPath := filepath.Join(al.logDir, fmt.Sprintf("audit-%d.log", time.Now().Unix()))

	os.Rename(oldPath, newPath)

	// Open new log file
	file, err := os.OpenFile(oldPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Printf("Warning: Failed to open new audit log file: %v", err)
		return
	}

	al.file = file

	// Log rotation event
	al.logInternal(AuditEntry{
		Timestamp: time.Now(),
		EventType: "AUDIT_LOG_ROTATED",
		Severity:  "INFO",
		Details: map[string]interface{}{
			"old_file": oldPath,
			"new_file": newPath,
		},
	})
}

// determineSeverity determines log severity based on event type
func (al *AuditLogger) determineSeverity(eventType string) string {
	switch eventType {
	case "REPLAY_ATTACK_DETECTED", "SIGNATURE_INVALID", "PERMISSION_DENIED",
		"RATE_LIMIT_EXCEEDED", "COMMAND_DENIED":
		return "WARNING"
	case "AUTH_FAILURE", "TIMESTAMP_INVALID":
		return "WARNING"
	default:
		return "INFO"
	}
}

// Close closes the audit log file
func (al *AuditLogger) Close() error {
	al.mu.Lock()
	defer al.mu.Unlock()

	if al.file != nil {
		return al.file.Close()
	}

	return nil
}

// truncateString truncates a string to max length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// GetAuditLogs retrieves recent audit log entries
func (al *AuditLogger) GetAuditLogs(limit int) ([]AuditEntry, error) {
	al.mu.Lock()
	defer al.mu.Unlock()

	if !al.enabled {
		return nil, fmt.Errorf("audit logging not enabled")
	}

	// Read audit log file
	data, err := os.ReadFile(filepath.Join(al.logDir, "audit.log"))
	if err != nil {
		return nil, fmt.Errorf("failed to read audit log: %v", err)
	}

	// Parse entries
	var entries []AuditEntry
	lines := strings.Split(string(data), "\n")

	// Process in reverse order to get most recent first
	start := len(lines) - 1 - limit
	if start < 0 {
		start = 0
	}

	for i := len(lines) - 1; i >= start && i >= 0; i-- {
		if lines[i] == "" {
			continue
		}

		var entry AuditEntry
		if err := json.Unmarshal([]byte(lines[i]), &entry); err != nil {
			continue
		}

		entries = append(entries, entry)

		if len(entries) >= limit {
			break
		}
	}

	return entries, nil
}
