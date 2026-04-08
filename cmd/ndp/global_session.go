package main

import (
	"sync"
)

// GlobalSessionTracker keeps track of the current active session
type GlobalSessionTracker struct {
	currentSession *Session
	nodeDebugger   *NodeDebugger
	mu             sync.RWMutex
}

var globalSessionTracker = &GlobalSessionTracker{}

// SetCurrentSession sets the current active session
func (gst *GlobalSessionTracker) SetCurrentSession(session *Session) {
	gst.mu.Lock()
	defer gst.mu.Unlock()
	gst.currentSession = session
}

// GetCurrentSession gets the current active session
func (gst *GlobalSessionTracker) GetCurrentSession() *Session {
	gst.mu.RLock()
	defer gst.mu.RUnlock()
	return gst.currentSession
}

// SetNodeDebugger sets the current node debugger
func (gst *GlobalSessionTracker) SetNodeDebugger(debugger *NodeDebugger) {
	gst.mu.Lock()
	defer gst.mu.Unlock()
	gst.nodeDebugger = debugger
}

// GetNodeDebugger gets the current node debugger.
func (gst *GlobalSessionTracker) GetNodeDebugger() *NodeDebugger {
	gst.mu.RLock()
	defer gst.mu.RUnlock()
	return gst.nodeDebugger
}
