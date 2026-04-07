package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"errors"

	"github.com/chromedp/cdproto/debugger"
	cdptarget "github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// SessionType represents the type of debugging session
type SessionType string

const (
	SessionTypeNode   SessionType = "node"
	SessionTypeChrome SessionType = "chrome"
)

// DebugTarget represents a debuggable target
type DebugTarget struct {
	ID                   string      `json:"id"`
	Type                 SessionType `json:"type"`
	Title                string      `json:"title"`
	URL                  string      `json:"url,omitempty"`
	Description          string      `json:"description,omitempty"`
	Port                 string      `json:"port"`
	WebSocketDebuggerURL string      `json:"webSocketDebuggerUrl,omitempty"`
	PID                  int         `json:"pid,omitempty"`
	Connected            bool        `json:"connected"`
}

// Session represents a debug session
type Session struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       SessionType            `json:"type"`
	Target     DebugTarget            `json:"target"`
	Context    context.Context        `json:"-"`
	Cancel     context.CancelFunc     `json:"-"`
	ChromeCtx  context.Context        `json:"-"`
	Connection *CDPConnection         `json:"-"`
	Created    time.Time              `json:"created"`
	State      map[string]interface{} `json:"state,omitempty"`
	mu         sync.RWMutex
}

// CDPConnection wraps a Chrome DevTools Protocol connection
type CDPConnection struct {
	URL       string
	Context   context.Context
	Cancel    context.CancelFunc
	ChromeCtx context.Context
	verbose   bool
}

// SessionManager manages debug sessions and connections
type SessionManager struct {
	sessions  map[string]*Session
	configDir string
	verbose   bool
	mu        sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager(verbose bool) *SessionManager {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	configDir := filepath.Join(homeDir, ".ndp", "sessions")
	os.MkdirAll(configDir, 0755)

	return &SessionManager{
		sessions:  make(map[string]*Session),
		configDir: configDir,
		verbose:   verbose,
	}
}

// CreateSession creates a new debug session
func (sm *SessionManager) CreateSession(ctx context.Context, target DebugTarget) (*Session, error) {
	sessionID := fmt.Sprintf("%s-%d", target.Type, time.Now().Unix())

	session := &Session{
		ID:      sessionID,
		Type:    target.Type,
		Target:  target,
		Created: time.Now(),
		State:   make(map[string]interface{}),
	}

	// Create CDP connection based on target type
	conn, err := sm.connectToTarget(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to target: %w", err)
	}

	session.Connection = conn
	session.Context = conn.Context
	session.Cancel = conn.Cancel
	session.ChromeCtx = conn.ChromeCtx

	sm.mu.Lock()
	sm.sessions[sessionID] = session
	sm.mu.Unlock()

	// Store in global tracker
	globalSessionTracker.SetCurrentSession(session)

	if sm.verbose {
		log.Printf("Created session %s for target %s", sessionID, target.ID)
	}

	return session, nil
}

// connectToTarget establishes a CDP connection to the target
func (sm *SessionManager) connectToTarget(ctx context.Context, target DebugTarget) (*CDPConnection, error) {
	var wsURL string

	switch target.Type {
	case SessionTypeNode:
		// For Node.js, get the exact WebSocket URL from the target list
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/json/list", target.Port))
		if err != nil {
			return nil, fmt.Errorf("Node.js inspector not available: %w", err)
		}
		defer resp.Body.Close()

		var targets []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
			return nil, fmt.Errorf("failed to parse Node.js targets: %w", err)
		}

		// Find the target with matching ID and get its WebSocket URL
		for _, t := range targets {
			if targetID, ok := t["id"].(string); ok && targetID == target.ID {
				if wsDebuggerURL, ok := t["webSocketDebuggerUrl"].(string); ok {
					wsURL = wsDebuggerURL
					break
				}
			}
		}

		if wsURL == "" {
			return nil, fmt.Errorf("WebSocket debugger URL not found for target %s", target.ID)
		}

	case SessionTypeChrome:
		if target.ID != "" {
			wsURL = fmt.Sprintf("ws://localhost:%s/devtools/page/%s", target.Port, target.ID)
		} else {
			wsURL = fmt.Sprintf("ws://localhost:%s", target.Port)
		}

	default:
		return nil, fmt.Errorf("unsupported target type: %s", target.Type)
	}

	// Create allocator and context
	var allocCtx context.Context
	var allocCancel context.CancelFunc

	if target.Type == SessionTypeNode {
		// For Node.js, use NoModifyURL to prevent chromedp from modifying the WebSocket URL
		allocCtx, allocCancel = chromedp.NewRemoteAllocator(ctx, wsURL, chromedp.NoModifyURL)
	} else {
		allocCtx, allocCancel = chromedp.NewRemoteAllocator(ctx, wsURL)
	}

	var opts []chromedp.ContextOption
	if sm.verbose {
		opts = append(opts, chromedp.WithLogf(log.Printf))
	}

	// For Node.js, use WithTargetID to connect to existing target instead of creating new one
	if target.Type == SessionTypeNode {
		opts = append(opts, chromedp.WithTargetID(cdptarget.ID(target.ID)))
	}

	chromeCtx, chromeCancel := chromedp.NewContext(allocCtx, opts...)

	// Combined cancel function
	cancel := func() {
		chromeCancel()
		allocCancel()
	}

	conn := &CDPConnection{
		URL:       wsURL,
		Context:   allocCtx,
		Cancel:    cancel,
		ChromeCtx: chromeCtx,
		verbose:   sm.verbose,
	}

	// Skip connection test for now due to chromedp URL parsing issues with Node.js WebSocket URLs
	// TODO: Find alternative way to test connection

	if sm.verbose {
		log.Printf("Connected to %s target at %s", target.Type, wsURL)
	}

	return conn, nil
}

// GetSession retrieves a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, ok := sm.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return session, nil
}

// ListTargets lists all available debug targets
func (sm *SessionManager) ListTargets(ctx context.Context) ([]DebugTarget, error) {
	var targets []DebugTarget

	// Check for Node.js targets
	nodeTargets, err := sm.findNodeTargets(ctx)
	if err != nil && sm.verbose {
		log.Printf("Warning: failed to find Node.js targets: %v", err)
	}
	targets = append(targets, nodeTargets...)

	// Check for Chrome targets
	chromeTargets, err := sm.findChromeTargets(ctx)
	if err != nil && sm.verbose {
		log.Printf("Warning: failed to find Chrome targets: %v", err)
	}
	targets = append(targets, chromeTargets...)

	return targets, nil
}

// findNodeTargets discovers Node.js processes with debugging enabled
func (sm *SessionManager) findNodeTargets(ctx context.Context) ([]DebugTarget, error) {
	var targets []DebugTarget

	// Check common Node.js debug ports
	ports := []string{"9229", "9230", "9231", "9232", "9233", "9234", "9235", "9236", "9237", "9238", "9239"}

	for _, port := range ports {
		url := fmt.Sprintf("http://localhost:%s/json/list", port)

		client := &http.Client{Timeout: 1 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		var endpoints []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&endpoints); err != nil {
			continue
		}

		for _, endpoint := range endpoints {
			title, _ := endpoint["title"].(string)
			if title == "" {
				title = endpoint["url"].(string)
			}

			target := DebugTarget{
				ID:                   endpoint["id"].(string),
				Type:                 SessionTypeNode,
				Title:                title,
				URL:                  endpoint["url"].(string),
				Description:          endpoint["description"].(string),
				Port:                 port,
				WebSocketDebuggerURL: endpoint["webSocketDebuggerUrl"].(string),
			}

			targets = append(targets, target)
		}
	}

	return targets, nil
}

// findChromeTargets discovers Chrome/Chromium debug targets
func (sm *SessionManager) findChromeTargets(ctx context.Context) ([]DebugTarget, error) {
	var targets []DebugTarget

	// Check common Chrome debug ports
	ports := []string{"9222", "9223", "9224", "9225"}

	for _, port := range ports {
		url := fmt.Sprintf("http://localhost:%s/json/list", port)

		client := &http.Client{Timeout: 1 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		var tabs []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
			continue
		}

		for _, tab := range tabs {
			if tabType, ok := tab["type"].(string); ok && tabType != "page" {
				continue // Skip non-page targets for now
			}

			target := DebugTarget{
				ID:          tab["id"].(string),
				Type:        SessionTypeChrome,
				Title:       tab["title"].(string),
				URL:         tab["url"].(string),
				Description: tab["description"].(string),
				Port:        port,
			}

			targets = append(targets, target)
		}
	}

	return targets, nil
}

// SaveSession saves a session to disk
func (sm *SessionManager) SaveSession(ctx context.Context, name string) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.sessions) == 0 {
		return errors.New("no active sessions to save")
	}

	// Get the first active session (can be extended to handle multiple)
	var session *Session
	for _, s := range sm.sessions {
		session = s
		break
	}

	if session == nil {
		return errors.New("no session found")
	}

	session.Name = name

	// Gather session state
	session.mu.Lock()

	// Get breakpoints
	if err := chromedp.Run(session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Basic session state capture - simplified for now
			session.State["connected"] = true
			session.State["timestamp"] = time.Now()
			return nil
		}),
	); err != nil && sm.verbose {
		log.Printf("Warning: failed to get session state: %v", err)
	}

	session.mu.Unlock()

	// Save to file
	filename := filepath.Join(sm.configDir, fmt.Sprintf("%s.json", name))

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := ioutil.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	if sm.verbose {
		log.Printf("Session saved to %s", filename)
	}

	return nil
}

// LoadSession loads a session from disk
func (sm *SessionManager) LoadSession(ctx context.Context, name string) error {
	filename := filepath.Join(sm.configDir, fmt.Sprintf("%s.json", name))

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read session file: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf("failed to unmarshal session: %w", err)
	}

	// Reconnect to target
	conn, err := sm.connectToTarget(ctx, session.Target)
	if err != nil {
		return fmt.Errorf("failed to reconnect to target: %w", err)
	}

	session.Connection = conn
	session.Context = conn.Context
	session.Cancel = conn.Cancel
	session.ChromeCtx = conn.ChromeCtx

	// Restore session state
	if breakpoints, ok := session.State["breakpoints"]; ok && breakpoints != nil {
		// Restore breakpoints
		if err := chromedp.Run(session.ChromeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				_, err := debugger.Enable().Do(ctx)
				return err
			}),
		); err != nil && sm.verbose {
			log.Printf("Warning: failed to restore breakpoints: %v", err)
		}
	}

	sm.mu.Lock()
	sm.sessions[session.ID] = &session
	sm.mu.Unlock()

	if sm.verbose {
		log.Printf("Session %s loaded successfully", name)
	}

	return nil
}

// ListSessions lists saved sessions
func (sm *SessionManager) ListSessions() ([]SessionInfo, error) {
	files, err := ioutil.ReadDir(sm.configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []SessionInfo
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		name := strings.TrimSuffix(file.Name(), ".json")
		sessions = append(sessions, SessionInfo{
			Name:    name,
			Created: file.ModTime().Format(time.RFC3339),
		})
	}

	return sessions, nil
}

// SessionInfo contains basic session information
type SessionInfo struct {
	Name    string `json:"name"`
	Created string `json:"created"`
}

// CloseSession closes a debug session
func (sm *SessionManager) CloseSession(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	if session.Cancel != nil {
		session.Cancel()
	}

	delete(sm.sessions, sessionID)

	if sm.verbose {
		log.Printf("Closed session %s", sessionID)
	}

	return nil
}

// ExecuteInSession executes a command in a specific session
func (s *Session) Execute(ctx context.Context, expression string) (interface{}, error) {
	var result interface{}

	err := chromedp.Run(s.ChromeCtx,
		chromedp.Evaluate(expression, &result),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to execute expression: %w", err)
	}

	return result, nil
}
