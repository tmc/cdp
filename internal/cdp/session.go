// Package cdp provides shared Chrome DevTools Protocol functionality.
package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/cdproto/runtime"
	"github.com/pkg/errors"
)

// TargetType represents the type of debugging target
type TargetType string

const (
	TargetTypeNode   TargetType = "node"
	TargetTypeChrome TargetType = "chrome"
)

// Target represents a debuggable target
type Target struct {
	ID          string     `json:"id"`
	Type        TargetType `json:"type"`
	Title       string     `json:"title"`
	URL         string     `json:"url,omitempty"`
	Description string     `json:"description,omitempty"`
	Port        string     `json:"port"`
	PID         int        `json:"pid,omitempty"`
	Connected   bool       `json:"connected"`
}

// Session represents a CDP session
type Session struct {
	ID         string                 `json:"id"`
	Target     Target                 `json:"target"`
	Context    context.Context        `json:"-"`
	Cancel     context.CancelFunc     `json:"-"`
	ChromeCtx  context.Context        `json:"-"`
	Created    time.Time              `json:"created"`
	State      map[string]interface{} `json:"state,omitempty"`
	Verbose    bool                   `json:"-"`
	mu         sync.RWMutex
}

// Manager manages CDP sessions
type Manager struct {
	sessions map[string]*Session
	verbose  bool
	mu       sync.RWMutex
}

// NewManager creates a new CDP session manager
func NewManager(verbose bool) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		verbose:  verbose,
	}
}

// CreateSession creates a new CDP session
func (m *Manager) CreateSession(ctx context.Context, target Target) (*Session, error) {
	sessionID := fmt.Sprintf("%s-%d", target.Type, time.Now().Unix())

	// Create CDP connection
	conn, err := m.connectToTarget(ctx, target)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to target")
	}

	session := &Session{
		ID:        sessionID,
		Target:    target,
		Context:   conn.Context,
		Cancel:    conn.Cancel,
		ChromeCtx: conn.ChromeCtx,
		Created:   time.Now(),
		State:     make(map[string]interface{}),
		Verbose:   m.verbose,
	}

	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	if m.verbose {
		log.Printf("Created CDP session %s for target %s", sessionID, target.ID)
	}

	return session, nil
}

// Connection represents a CDP connection
type Connection struct {
	URL       string
	Context   context.Context
	Cancel    context.CancelFunc
	ChromeCtx context.Context
}

// connectToTarget establishes a CDP connection to the target
func (m *Manager) connectToTarget(ctx context.Context, target Target) (*Connection, error) {
	var wsURL string

	switch target.Type {
	case TargetTypeNode:
		wsURL = fmt.Sprintf("ws://localhost:%s", target.Port)

		// For Node.js, check if the inspector is available
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/json/version", target.Port))
		if err != nil {
			return nil, errors.Wrap(err, "Node.js inspector not available")
		}
		defer resp.Body.Close()

		var versionInfo map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
			return nil, errors.Wrap(err, "failed to get Node.js version info")
		}

		if wsDebuggerURL, ok := versionInfo["webSocketDebuggerUrl"].(string); ok {
			wsURL = wsDebuggerURL
		}

	case TargetTypeChrome:
		if target.ID != "" {
			wsURL = fmt.Sprintf("ws://localhost:%s/devtools/page/%s", target.Port, target.ID)
		} else {
			wsURL = fmt.Sprintf("ws://localhost:%s", target.Port)
		}

	default:
		return nil, fmt.Errorf("unsupported target type: %s", target.Type)
	}

	// Create allocator and context
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, wsURL)

	var opts []chromedp.ContextOption
	if m.verbose {
		opts = append(opts, chromedp.WithLogf(log.Printf))
	}

	chromeCtx, chromeCancel := chromedp.NewContext(allocCtx, opts...)

	// Combined cancel function
	cancel := func() {
		chromeCancel()
		allocCancel()
	}

	conn := &Connection{
		URL:       wsURL,
		Context:   allocCtx,
		Cancel:    cancel,
		ChromeCtx: chromeCtx,
	}

	// Test connection
	testCtx, testCancel := context.WithTimeout(chromeCtx, 5*time.Second)
	defer testCancel()

	if err := chromedp.Run(testCtx, chromedp.Evaluate("1+1", nil)); err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to test connection")
	}

	if m.verbose {
		log.Printf("Connected to %s target at %s", target.Type, wsURL)
	}

	return conn, nil
}

// GetSession retrieves a session by ID
func (m *Manager) GetSession(sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return session, nil
}

// CloseSession closes a CDP session
func (m *Manager) CloseSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	if session.Cancel != nil {
		session.Cancel()
	}

	delete(m.sessions, sessionID)

	if m.verbose {
		log.Printf("Closed CDP session %s", sessionID)
	}

	return nil
}

// EnableDomains enables common CDP domains
func (s *Session) EnableDomains(ctx context.Context, domains ...string) error {
	return chromedp.Run(s.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			for _, domain := range domains {
				switch domain {
				case "Runtime":
					if err := runtime.Enable().Do(ctx); err != nil {
						return errors.Wrapf(err, "failed to enable %s domain", domain)
					}
				case "Debugger":
					if err := debugger.Enable(debugger.EnableParams{}).Do(ctx); err != nil {
						return errors.Wrapf(err, "failed to enable %s domain", domain)
					}
				default:
					if s.Verbose {
						log.Printf("Domain %s not explicitly handled", domain)
					}
				}
			}

			if s.Verbose && len(domains) > 0 {
				log.Printf("Enabled domains: %v", domains)
			}

			return nil
		}),
	)
}

// Execute executes a JavaScript expression in the session
func (s *Session) Execute(ctx context.Context, expression string) (interface{}, error) {
	var result interface{}

	err := chromedp.Run(s.ChromeCtx,
		chromedp.Evaluate(expression, &result),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to execute expression")
	}

	return result, nil
}

// DiscoverTargets discovers available debug targets
func DiscoverTargets(ctx context.Context, verbose bool) ([]Target, error) {
	var targets []Target

	// Check for Node.js targets
	nodeTargets, err := discoverNodeTargets(ctx, verbose)
	if err != nil && verbose {
		log.Printf("Warning: failed to discover Node.js targets: %v", err)
	}
	targets = append(targets, nodeTargets...)

	// Check for Chrome targets
	chromeTargets, err := discoverChromeTargets(ctx, verbose)
	if err != nil && verbose {
		log.Printf("Warning: failed to discover Chrome targets: %v", err)
	}
	targets = append(targets, chromeTargets...)

	return targets, nil
}

// discoverNodeTargets discovers Node.js processes with debugging enabled
func discoverNodeTargets(ctx context.Context, verbose bool) ([]Target, error) {
	var targets []Target

	// Check common Node.js debug ports
	ports := []string{"9229", "9230", "9231", "9232"}

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

			target := Target{
				ID:          endpoint["id"].(string),
				Type:        TargetTypeNode,
				Title:       title,
				URL:         endpoint["url"].(string),
				Description: endpoint["description"].(string),
				Port:        port,
			}

			targets = append(targets, target)
		}
	}

	return targets, nil
}

// discoverChromeTargets discovers Chrome/Chromium debug targets
func discoverChromeTargets(ctx context.Context, verbose bool) ([]Target, error) {
	var targets []Target

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
				continue // Skip non-page targets for Chrome
			}

			target := Target{
				ID:          tab["id"].(string),
				Type:        TargetTypeChrome,
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