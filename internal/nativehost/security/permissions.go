package security

import (
	"fmt"
	"strings"
	"sync"
)

// Capability represents a specific permission
type Capability string

const (
	CapPing             Capability = "ping"
	CapAIRequest        Capability = "ai_request"
	CapStatus           Capability = "status"
	CapFileRead         Capability = "file_read"
	CapFileWrite        Capability = "file_write"
	CapNetworkAccess    Capability = "network_access"
	CapExecCommand      Capability = "exec_command"
	CapCredentialRead   Capability = "credential_read"
	CapCredentialWrite  Capability = "credential_write"
	CapBrowserControl   Capability = "browser_control"
	CapSystemInfo       Capability = "system_info"
)

// PermissionLevel defines permission levels
type PermissionLevel int

const (
	PermissionNone PermissionLevel = iota
	PermissionReadOnly
	PermissionLimited
	PermissionFull
)

// PermissionConfig holds permission configuration
type PermissionConfig struct {
	DefaultLevel PermissionLevel
	Capabilities map[Capability]PermissionLevel
}

// DefaultPermissionConfig returns a safe default configuration
func DefaultPermissionConfig() PermissionConfig {
	return PermissionConfig{
		DefaultLevel: PermissionNone,
		Capabilities: map[Capability]PermissionLevel{
			CapPing:      PermissionFull,
			CapStatus:    PermissionReadOnly,
			CapAIRequest: PermissionLimited,
		},
	}
}

// StrictPermissionConfig returns a highly restrictive configuration
func StrictPermissionConfig() PermissionConfig {
	return PermissionConfig{
		DefaultLevel: PermissionNone,
		Capabilities: map[Capability]PermissionLevel{
			CapPing:   PermissionFull,
			CapStatus: PermissionReadOnly,
		},
	}
}

// PermissivePermissionConfig returns a more permissive configuration
func PermissivePermissionConfig() PermissionConfig {
	return PermissionConfig{
		DefaultLevel: PermissionLimited,
		Capabilities: map[Capability]PermissionLevel{
			CapPing:           PermissionFull,
			CapAIRequest:      PermissionFull,
			CapStatus:         PermissionFull,
			CapBrowserControl: PermissionFull,
			CapSystemInfo:     PermissionReadOnly,
		},
	}
}

// PermissionManager manages capability-based permissions
type PermissionManager struct {
	config PermissionConfig
	mu     sync.RWMutex
	grants map[string][]Capability // Session-specific grants
}

// NewPermissionManager creates a new permission manager
func NewPermissionManager(config PermissionConfig) *PermissionManager {
	return &PermissionManager{
		config: config,
		grants: make(map[string][]Capability),
	}
}

// CheckPermission checks if an action is permitted
func (pm *PermissionManager) CheckPermission(sessionID string, capability Capability, requiredLevel PermissionLevel) error {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Check if capability has specific permission level
	if level, exists := pm.config.Capabilities[capability]; exists {
		if level >= requiredLevel {
			return nil
		}
		return fmt.Errorf("permission denied: %s requires %v, have %v", capability, requiredLevel, level)
	}

	// Check default permission level
	if pm.config.DefaultLevel >= requiredLevel {
		return nil
	}

	// Check session-specific grants
	if grants, exists := pm.grants[sessionID]; exists {
		for _, granted := range grants {
			if granted == capability {
				return nil
			}
		}
	}

	return fmt.Errorf("permission denied: %s not granted", capability)
}

// GrantCapability grants a capability to a specific session
func (pm *PermissionManager) GrantCapability(sessionID string, capability Capability) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.grants[sessionID]; !exists {
		pm.grants[sessionID] = []Capability{}
	}
	pm.grants[sessionID] = append(pm.grants[sessionID], capability)
}

// RevokeCapability revokes a capability from a session
func (pm *PermissionManager) RevokeCapability(sessionID string, capability Capability) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if grants, exists := pm.grants[sessionID]; exists {
		filtered := []Capability{}
		for _, cap := range grants {
			if cap != capability {
				filtered = append(filtered, cap)
			}
		}
		pm.grants[sessionID] = filtered
	}
}

// RevokeAllCapabilities revokes all capabilities from a session
func (pm *PermissionManager) RevokeAllCapabilities(sessionID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.grants, sessionID)
}

// ListCapabilities returns all capabilities for a session
func (pm *PermissionManager) ListCapabilities(sessionID string) []Capability {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	caps := []Capability{}

	// Add configured capabilities
	for cap, level := range pm.config.Capabilities {
		if level > PermissionNone {
			caps = append(caps, cap)
		}
	}

	// Add session-specific grants
	if grants, exists := pm.grants[sessionID]; exists {
		caps = append(caps, grants...)
	}

	return caps
}

// ParseCapability converts a string to a Capability
func ParseCapability(s string) (Capability, error) {
	cap := Capability(strings.ToLower(s))
	switch cap {
	case CapPing, CapAIRequest, CapStatus, CapFileRead, CapFileWrite,
		CapNetworkAccess, CapExecCommand, CapCredentialRead, CapCredentialWrite,
		CapBrowserControl, CapSystemInfo:
		return cap, nil
	default:
		return "", fmt.Errorf("unknown capability: %s", s)
	}
}
