package security

import (
	"fmt"
	"sync"
	"time"
)

// SecurityConfig holds overall security configuration
type SecurityConfig struct {
	Auth        AuthConfig
	Permissions PermissionConfig
	RateLimit   RateLimitConfig
	Validation  ValidationConfig
	Audit       AuditConfig
	Sandbox     SandboxConfig
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		Auth:        DefaultAuthConfig(),
		Permissions: DefaultPermissionConfig(),
		RateLimit:   DefaultRateLimitConfig(),
		Validation:  DefaultValidationConfig(),
		Audit:       DefaultAuditConfig(),
		Sandbox:     DefaultSandboxConfig(),
	}
}

// StrictSecurityConfig returns a highly restrictive configuration
func StrictSecurityConfig() SecurityConfig {
	return SecurityConfig{
		Auth:        DefaultAuthConfig(),
		Permissions: StrictPermissionConfig(),
		RateLimit:   StrictRateLimitConfig(),
		Validation:  StrictValidationConfig(),
		Audit:       DefaultAuditConfig(),
		Sandbox:     DefaultSandboxConfig(),
	}
}

// PermissiveSecurityConfig returns a more permissive configuration
func PermissiveSecurityConfig() SecurityConfig {
	return SecurityConfig{
		Auth:        DefaultAuthConfig(),
		Permissions: PermissivePermissionConfig(),
		RateLimit:   DefaultRateLimitConfig(),
		Validation:  DefaultValidationConfig(),
		Audit:       DefaultAuditConfig(),
		Sandbox:     DefaultSandboxConfig(),
	}
}

// SecurityManager coordinates all security components
type SecurityManager struct {
	auth        *Authenticator
	permissions *PermissionManager
	rateLimit   *RateLimiter
	validator   *Validator
	audit       *AuditLogger
	sandbox     *Sandbox
	mu          sync.RWMutex
	sessions    map[string]*SessionInfo
}

// SessionInfo holds information about a session
type SessionInfo struct {
	ID          string
	ClientID    string
	CreatedAt   time.Time
	LastActive  time.Time
	Capabilities []Capability
}

// NewSecurityManager creates a new security manager
func NewSecurityManager(config SecurityConfig) (*SecurityManager, error) {
	auth, err := NewAuthenticator(config.Auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticator: %w", err)
	}

	audit, err := NewAuditLogger(config.Audit)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit logger: %w", err)
	}

	return &SecurityManager{
		auth:        auth,
		permissions: NewPermissionManager(config.Permissions),
		rateLimit:   NewRateLimiter(config.RateLimit),
		validator:   NewValidator(config.Validation),
		audit:       audit,
		sandbox:     NewSandbox(config.Sandbox),
		sessions:    make(map[string]*SessionInfo),
	}, nil
}

// ValidateMessage performs comprehensive message validation
func (sm *SecurityManager) ValidateMessage(sessionID, clientID string, msg interface{}) error {
	// Check rate limit
	if err := sm.rateLimit.Allow(clientID); err != nil {
		sm.audit.LogRateLimitExceeded(clientID)
		return fmt.Errorf("rate limit: %w", err)
	}

	// Validate message structure based on type
	if msgMap, ok := msg.(map[string]interface{}); ok {
		// Validate message type
		if msgType, ok := msgMap["type"].(string); ok {
			if err := sm.validator.ValidateMessageType(msgType); err != nil {
				sm.audit.LogSecurityViolation(sessionID, "invalid_message_type", map[string]interface{}{
					"error": err.Error(),
				})
				return fmt.Errorf("message type validation: %w", err)
			}
		}

		// Validate message ID
		if msgID, ok := msgMap["id"].(string); ok {
			if err := sm.validator.ValidateID(msgID); err != nil {
				sm.audit.LogSecurityViolation(sessionID, "invalid_message_id", map[string]interface{}{
					"error": err.Error(),
				})
				return fmt.Errorf("message ID validation: %w", err)
			}
		}

		// Validate JSON depth
		if err := sm.validator.ValidateJSONDepth(msg, 0); err != nil {
			sm.audit.LogSecurityViolation(sessionID, "json_depth_exceeded", map[string]interface{}{
				"error": err.Error(),
			})
			return fmt.Errorf("JSON validation: %w", err)
		}
	}

	sm.audit.LogMessageReceived(sessionID, fmt.Sprintf("%v", msg), "")
	return nil
}

// CheckPermission checks if a session has permission for a capability
func (sm *SecurityManager) CheckPermission(sessionID string, capability Capability, requiredLevel PermissionLevel) error {
	if err := sm.permissions.CheckPermission(sessionID, capability, requiredLevel); err != nil {
		sm.audit.LogPermissionDenied(sessionID, string(capability), err.Error())
		return err
	}

	sm.audit.Log(AuditEvent{
		Type:      EventPermissionGranted,
		SessionID: sessionID,
		Action:    "check_permission",
		Resource:  string(capability),
		Result:    "granted",
		Severity:  "info",
	})

	return nil
}

// UpdateSession updates session activity
func (sm *SecurityManager) UpdateSession(sessionID, clientID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[sessionID]; exists {
		session.LastActive = time.Now()
	} else {
		sm.sessions[sessionID] = &SessionInfo{
			ID:        sessionID,
			ClientID:  clientID,
			CreatedAt: time.Now(),
			LastActive: time.Now(),
		}
	}
}

// GetSecurityStats returns security statistics
func (sm *SecurityManager) GetSecurityStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return map[string]interface{}{
		"active_sessions": len(sm.sessions),
		"rate_limit":      sm.rateLimit.GlobalStats(),
	}
}

// Close closes the security manager and all components
func (sm *SecurityManager) Close() error {
	return sm.audit.Close()
}
