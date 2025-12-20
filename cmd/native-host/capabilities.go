// Capability-based permission system
package main

import (
	"fmt"
	"sync"
)

// Capability represents a permission capability
type Capability string

const (
	// Core capabilities
	CapabilityRead     Capability = "read"
	CapabilityWrite    Capability = "write"
	CapabilityExecute  Capability = "execute"
	CapabilityAdmin    Capability = "admin"

	// Domain-specific capabilities
	CapabilityAIRequest      Capability = "ai_request"
	CapabilityBrowserControl Capability = "browser_control"
	CapabilityFileAccess     Capability = "file_access"
	CapabilityNetworkAccess  Capability = "network_access"
	CapabilityDebug          Capability = "debug"
)

// CapabilitySet represents a set of capabilities for a principal
type CapabilitySet struct {
	capabilities map[Capability]bool
	mu           sync.RWMutex
}

// NewCapabilitySet creates a new capability set
func NewCapabilitySet() *CapabilitySet {
	return &CapabilitySet{
		capabilities: make(map[Capability]bool),
	}
}

// Grant grants a capability
func (cs *CapabilitySet) Grant(cap Capability) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.capabilities[cap] = true
}

// Revoke revokes a capability
func (cs *CapabilitySet) Revoke(cap Capability) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.capabilities[cap] = false
}

// Has checks if a capability is granted
func (cs *CapabilitySet) Has(cap Capability) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.capabilities[cap]
}

// HasAny checks if any of the given capabilities are granted
func (cs *CapabilitySet) HasAny(caps ...Capability) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, cap := range caps {
		if cs.capabilities[cap] {
			return true
		}
	}
	return false
}

// HasAll checks if all given capabilities are granted
func (cs *CapabilitySet) HasAll(caps ...Capability) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, cap := range caps {
		if !cs.capabilities[cap] {
			return false
		}
	}
	return true
}

// GetAll returns all granted capabilities
func (cs *CapabilitySet) GetAll() []Capability {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	var caps []Capability
	for cap, granted := range cs.capabilities {
		if granted {
			caps = append(caps, cap)
		}
	}
	return caps
}

// CapabilityManager manages capabilities for different clients/extensions
type CapabilityManager struct {
	principals map[string]*CapabilitySet
	mu         sync.RWMutex
	auditLog   *AuditLogger
}

// NewCapabilityManager creates a new capability manager
func NewCapabilityManager() *CapabilityManager {
	return &CapabilityManager{
		principals: make(map[string]*CapabilitySet),
		auditLog:   NewAuditLogger(true),
	}
}

// RegisterPrincipal registers a new principal (e.g., extension ID) with default capabilities
func (cm *CapabilityManager) RegisterPrincipal(principalID string, capabilities []Capability) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cs := NewCapabilitySet()
	for _, cap := range capabilities {
		cs.Grant(cap)
	}

	cm.principals[principalID] = cs

	cm.auditLog.Log("PRINCIPAL_REGISTERED", map[string]interface{}{
		"principal_id": principalID,
		"capabilities": capabilities,
	})

	return nil
}

// GetCapabilities returns the capability set for a principal
func (cm *CapabilityManager) GetCapabilities(principalID string) *CapabilitySet {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cs, ok := cm.principals[principalID]; ok {
		return cs
	}

	// Return empty capability set for unknown principals
	return NewCapabilitySet()
}

// GrantCapability grants a capability to a principal
func (cm *CapabilityManager) GrantCapability(principalID string, cap Capability) error {
	cm.mu.Lock()
	cs, ok := cm.principals[principalID]
	cm.mu.Unlock()

	if !ok {
		return fmt.Errorf("principal not found: %s", principalID)
	}

	cs.Grant(cap)

	cm.auditLog.Log("CAPABILITY_GRANTED", map[string]interface{}{
		"principal_id": principalID,
		"capability":   cap,
	})

	return nil
}

// RevokeCapability revokes a capability from a principal
func (cm *CapabilityManager) RevokeCapability(principalID string, cap Capability) error {
	cm.mu.Lock()
	cs, ok := cm.principals[principalID]
	cm.mu.Unlock()

	if !ok {
		return fmt.Errorf("principal not found: %s", principalID)
	}

	cs.Revoke(cap)

	cm.auditLog.Log("CAPABILITY_REVOKED", map[string]interface{}{
		"principal_id": principalID,
		"capability":   cap,
	})

	return nil
}

// CheckCapability checks if a principal has a required capability
func (cm *CapabilityManager) CheckCapability(principalID string, cap Capability) bool {
	cs := cm.GetCapabilities(principalID)
	allowed := cs.Has(cap)

	cm.auditLog.LogPermissionCheck(allowed, string(cap), principalID)

	return allowed
}

// CheckCapabilities checks if a principal has all required capabilities
func (cm *CapabilityManager) CheckCapabilities(principalID string, caps ...Capability) bool {
	cs := cm.GetCapabilities(principalID)
	allowed := cs.HasAll(caps...)

	capNames := ""
	for _, cap := range caps {
		capNames += string(cap) + ","
	}

	cm.auditLog.LogPermissionCheck(allowed, capNames, principalID)

	return allowed
}

// GetPrincipalCapabilities returns all capabilities for a principal
func (cm *CapabilityManager) GetPrincipalCapabilities(principalID string) []Capability {
	cs := cm.GetCapabilities(principalID)
	return cs.GetAll()
}

// DefaultCapabilities returns a sensible default capability set for extensions
func DefaultCapabilities() []Capability {
	return []Capability{
		CapabilityRead,
		CapabilityAIRequest,
		CapabilityNetworkAccess,
	}
}

// AdminCapabilities returns all capabilities (for admin/privileged principals)
func AdminCapabilities() []Capability {
	return []Capability{
		CapabilityRead,
		CapabilityWrite,
		CapabilityExecute,
		CapabilityAdmin,
		CapabilityAIRequest,
		CapabilityBrowserControl,
		CapabilityFileAccess,
		CapabilityNetworkAccess,
		CapabilityDebug,
	}
}
