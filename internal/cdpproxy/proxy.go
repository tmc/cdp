// Package cdpproxy provides a Chrome DevTools Protocol proxy for observing and logging CDP messages.
package cdpproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// CDPMessage represents a parsed CDP protocol message
type CDPMessage struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     json.RawMessage `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

// TargetInfo represents information about a browser target (tab, worker, etc.)
type TargetInfo struct {
	TargetID  string `json:"targetId"`
	Type      string `json:"type"` // "page", "iframe", "worker", "service_worker", etc.
	Title     string `json:"title"`
	URL       string `json:"url"`
	SessionID string `json:"sessionId,omitempty"`
	Attached  bool   `json:"attached"`
}

// LogEntry represents a logged CDP message with metadata
type LogEntry struct {
	Timestamp  time.Time       `json:"timestamp"`
	Direction  string          `json:"direction"` // "client->browser" or "browser->client"
	SessionID  string          `json:"sessionId,omitempty"`
	TargetID   string          `json:"targetId,omitempty"`
	TargetInfo *TargetInfo     `json:"targetInfo,omitempty"`
	Raw        json.RawMessage `json:"raw"`
	Parsed     *CDPMessage     `json:"parsed,omitempty"`
}

// observerClient wraps a websocket connection with a mutex for safe concurrent writes
type observerClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *observerClient) WriteMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(messageType, data)
}

// Proxy handles CDP proxying between client and browser
type Proxy struct {
	ListenPort int
	TargetPort int
	Verbose    bool

	// Message log
	logMu   sync.RWMutex
	log     []LogEntry
	maxLog  int
	logFile *os.File

	// Target tracking
	targetsMu          sync.RWMutex
	targets            map[string]*TargetInfo // targetId -> TargetInfo
	sessionToTarget    map[string]string      // sessionId -> targetId
	connectionToTarget map[string]string      // WebSocket path -> targetId (for /devtools/page/xxx)

	// WebSocket clients for live updates
	clientsMu sync.RWMutex
	clients   map[*observerClient]bool

	upgrader websocket.Upgrader
}

// New creates a new CDP proxy
func New(listenPort, targetPort int, verbose bool, logPath string) (*Proxy, error) {
	p := &Proxy{
		ListenPort:         listenPort,
		TargetPort:         targetPort,
		Verbose:            verbose,
		log:                make([]LogEntry, 0, 1000),
		maxLog:             10000,
		targets:            make(map[string]*TargetInfo),
		sessionToTarget:    make(map[string]string),
		connectionToTarget: make(map[string]string),
		clients:            make(map[*observerClient]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		p.logFile = f
	}

	return p, nil
}

// Close closes the proxy and releases resources
func (p *Proxy) Close() {
	if p.logFile != nil {
		p.logFile.Close()
	}
}

// updateTargetFromMessage parses CDP messages to track target/tab information
func (p *Proxy) updateTargetFromMessage(msg *CDPMessage, raw json.RawMessage) {
	if msg == nil || msg.Method == "" {
		return
	}

	p.targetsMu.Lock()
	defer p.targetsMu.Unlock()

	switch msg.Method {
	case "Target.targetCreated", "Target.targetInfoChanged":
		// Parse targetInfo from params
		var params struct {
			TargetInfo struct {
				TargetID string `json:"targetId"`
				Type     string `json:"type"`
				Title    string `json:"title"`
				URL      string `json:"url"`
			} `json:"targetInfo"`
		}
		if json.Unmarshal(msg.Params, &params) == nil && params.TargetInfo.TargetID != "" {
			ti := params.TargetInfo
			if existing, ok := p.targets[ti.TargetID]; ok {
				existing.Type = ti.Type
				existing.Title = ti.Title
				existing.URL = ti.URL
			} else {
				p.targets[ti.TargetID] = &TargetInfo{
					TargetID: ti.TargetID,
					Type:     ti.Type,
					Title:    ti.Title,
					URL:      ti.URL,
				}
			}
		}

	case "Target.attachedToTarget":
		// Map session to target
		var params struct {
			SessionID  string `json:"sessionId"`
			TargetInfo struct {
				TargetID string `json:"targetId"`
				Type     string `json:"type"`
				Title    string `json:"title"`
				URL      string `json:"url"`
			} `json:"targetInfo"`
		}
		if json.Unmarshal(msg.Params, &params) == nil {
			if params.TargetInfo.TargetID != "" {
				ti := params.TargetInfo
				p.targets[ti.TargetID] = &TargetInfo{
					TargetID:  ti.TargetID,
					Type:      ti.Type,
					Title:     ti.Title,
					URL:       ti.URL,
					SessionID: params.SessionID,
					Attached:  true,
				}
				if params.SessionID != "" {
					p.sessionToTarget[params.SessionID] = ti.TargetID
				}
			}
		}

	case "Target.detachedFromTarget":
		var params struct {
			SessionID string `json:"sessionId"`
			TargetID  string `json:"targetId"`
		}
		if json.Unmarshal(msg.Params, &params) == nil {
			if params.SessionID != "" {
				delete(p.sessionToTarget, params.SessionID)
			}
			if params.TargetID != "" {
				if t, ok := p.targets[params.TargetID]; ok {
					t.Attached = false
				}
			}
		}

	case "Target.targetDestroyed":
		var params struct {
			TargetID string `json:"targetId"`
		}
		if json.Unmarshal(msg.Params, &params) == nil && params.TargetID != "" {
			delete(p.targets, params.TargetID)
		}

	case "Page.frameNavigated":
		// Update URL/title for the target associated with this session
		if msg.SessionID != "" {
			if targetID, ok := p.sessionToTarget[msg.SessionID]; ok {
				if t, ok := p.targets[targetID]; ok {
					var params struct {
						Frame struct {
							URL string `json:"url"`
						} `json:"frame"`
					}
					if json.Unmarshal(msg.Params, &params) == nil {
						t.URL = params.Frame.URL
					}
				}
			}
		}

	case "Page.domContentEventFired", "Page.loadEventFired":
		// Could trigger a title update request, but we'll rely on targetInfoChanged
	}
}

// getTargetForSession returns the target info for a given session ID
func (p *Proxy) getTargetForSession(sessionID string) *TargetInfo {
	if sessionID == "" {
		return nil
	}
	p.targetsMu.RLock()
	defer p.targetsMu.RUnlock()

	if targetID, ok := p.sessionToTarget[sessionID]; ok {
		if t, ok := p.targets[targetID]; ok {
			// Return a copy
			copy := *t
			return &copy
		}
	}
	return nil
}

// getTargets returns all known targets
func (p *Proxy) getTargets() []*TargetInfo {
	p.targetsMu.RLock()
	defer p.targetsMu.RUnlock()

	result := make([]*TargetInfo, 0, len(p.targets))
	for _, t := range p.targets {
		copy := *t
		result = append(result, &copy)
	}
	return result
}

// registerConnectionTarget associates a WebSocket path with a target ID
func (p *Proxy) registerConnectionTarget(path string) {
	// Extract target ID from paths like /devtools/page/TARGETID
	if strings.HasPrefix(path, "/devtools/page/") {
		targetID := strings.TrimPrefix(path, "/devtools/page/")
		p.targetsMu.Lock()
		p.connectionToTarget[path] = targetID
		// Create a placeholder target if we don't have info yet
		if _, ok := p.targets[targetID]; !ok {
			p.targets[targetID] = &TargetInfo{
				TargetID: targetID,
				Type:     "page",
				Attached: true,
			}
		}
		p.targetsMu.Unlock()
	}
}

func (p *Proxy) addLogEntry(entry LogEntry) {
	p.logMu.Lock()
	p.log = append(p.log, entry)
	if len(p.log) > p.maxLog {
		p.log = p.log[len(p.log)-p.maxLog:]
	}
	p.logMu.Unlock()

	// Write to log file
	if p.logFile != nil {
		data, _ := json.Marshal(entry)
		fmt.Fprintln(p.logFile, string(data))
	}

	// Broadcast to WebSocket clients
	p.broadcastEntry(entry)
}

func (p *Proxy) broadcastEntry(entry LogEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	p.clientsMu.RLock()
	defer p.clientsMu.RUnlock()

	for client := range p.clients {
		client.WriteMessage(websocket.TextMessage, data)
	}
}

func (p *Proxy) getLog() []LogEntry {
	p.logMu.RLock()
	defer p.logMu.RUnlock()
	result := make([]LogEntry, len(p.log))
	copy(result, p.log)
	return result
}

// proxyWebSocket proxies a WebSocket connection between client and browser
func (p *Proxy) proxyWebSocket(clientConn *websocket.Conn, targetURL string) {
	// Connect to the target browser
	browserConn, _, err := websocket.DefaultDialer.Dial(targetURL, nil)
	if err != nil {
		log.Printf("Failed to connect to browser at %s: %v", targetURL, err)
		return
	}
	defer browserConn.Close()

	var wg sync.WaitGroup
	var once sync.Once
	done := make(chan struct{})
	closeDone := func() { once.Do(func() { close(done) }) }

	// Client -> Browser
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}

			msgType, data, err := clientConn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					if p.Verbose {
						log.Printf("Client read error: %v", err)
					}
				}
				closeDone()
				return
			}

			// Log the message
			entry := LogEntry{
				Timestamp: time.Now(),
				Direction: "client->browser",
				Raw:       data,
			}
			var msg CDPMessage
			if json.Unmarshal(data, &msg) == nil {
				entry.Parsed = &msg
				entry.SessionID = msg.SessionID
				// Enrich with target info
				if ti := p.getTargetForSession(msg.SessionID); ti != nil {
					entry.TargetID = ti.TargetID
					entry.TargetInfo = ti
				}
			}
			p.addLogEntry(entry)

			if p.Verbose {
				if entry.Parsed != nil && entry.Parsed.Method != "" {
					log.Printf("-> %s (id=%d)", entry.Parsed.Method, entry.Parsed.ID)
				}
			}

			if err := browserConn.WriteMessage(msgType, data); err != nil {
				if p.Verbose {
					log.Printf("Browser write error: %v", err)
				}
				closeDone()
				return
			}
		}
	}()

	// Browser -> Client
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}

			msgType, data, err := browserConn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					if p.Verbose {
						log.Printf("Browser read error: %v", err)
					}
				}
				closeDone()
				return
			}

			// Log the message
			entry := LogEntry{
				Timestamp: time.Now(),
				Direction: "browser->client",
				Raw:       data,
			}
			var msg CDPMessage
			if json.Unmarshal(data, &msg) == nil {
				entry.Parsed = &msg
				entry.SessionID = msg.SessionID
				// Track target info from events
				p.updateTargetFromMessage(&msg, data)
				// Enrich with target info
				if ti := p.getTargetForSession(msg.SessionID); ti != nil {
					entry.TargetID = ti.TargetID
					entry.TargetInfo = ti
				}
			}
			p.addLogEntry(entry)

			if p.Verbose {
				if entry.Parsed != nil {
					if entry.Parsed.Method != "" {
						log.Printf("<- %s", entry.Parsed.Method)
					} else if entry.Parsed.ID > 0 {
						log.Printf("<- response (id=%d)", entry.Parsed.ID)
					}
				}
			}

			if err := clientConn.WriteMessage(msgType, data); err != nil {
				if p.Verbose {
					log.Printf("Client write error: %v", err)
				}
				closeDone()
				return
			}
		}
	}()

	wg.Wait()
}

// handleCDPHTTP proxies HTTP requests to the browser's CDP HTTP endpoints
func (p *Proxy) handleCDPHTTP(w http.ResponseWriter, r *http.Request) {
	// Forward to target browser
	targetURL := fmt.Sprintf("http://localhost:%d%s", p.TargetPort, r.URL.Path)

	resp, err := http.Get(targetURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to connect to browser: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy headers
	for k, v := range resp.Header {
		w.Header()[k] = v
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read response: %v", err), http.StatusBadGateway)
		return
	}

	// Rewrite WebSocket URLs to point to our proxy
	bodyStr := string(body)
	oldWS := fmt.Sprintf("ws://localhost:%d", p.TargetPort)
	newWS := fmt.Sprintf("ws://localhost:%d", p.ListenPort)
	bodyStr = strings.ReplaceAll(bodyStr, oldWS, newWS)

	w.WriteHeader(resp.StatusCode)
	w.Write([]byte(bodyStr))
}

// handleWebSocket handles WebSocket upgrade and proxying
func (p *Proxy) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	clientConn, err := p.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer clientConn.Close()

	// Build target URL
	targetURL := fmt.Sprintf("ws://localhost:%d%s", p.TargetPort, r.URL.Path)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	log.Printf("Proxying WebSocket: %s -> %s", r.URL.Path, targetURL)
	p.proxyWebSocket(clientConn, targetURL)
}

// handleObserverWS handles WebSocket connections from the web UI
func (p *Proxy) handleObserverWS(w http.ResponseWriter, r *http.Request) {
	conn, err := p.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Observer WebSocket upgrade failed: %v", err)
		return
	}

	client := &observerClient{conn: conn}

	p.clientsMu.Lock()
	p.clients[client] = true
	p.clientsMu.Unlock()

	defer func() {
		p.clientsMu.Lock()
		delete(p.clients, client)
		p.clientsMu.Unlock()
		conn.Close()
	}()

	// Send existing log entries
	for _, entry := range p.getLog() {
		data, _ := json.Marshal(entry)
		client.WriteMessage(websocket.TextMessage, data)
	}

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// handleObserverAPI handles REST API for log access
func (p *Proxy) handleObserverAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	entries := p.getLog()
	json.NewEncoder(w).Encode(entries)
}

// handleObserverClear clears the log
func (p *Proxy) handleObserverClear(w http.ResponseWriter, r *http.Request) {
	p.logMu.Lock()
	p.log = p.log[:0]
	p.logMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"cleared"}`))
}

// handleObserverTargets returns all known targets
func (p *Proxy) handleObserverTargets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	targets := p.getTargets()
	json.NewEncoder(w).Encode(targets)
}

// Run starts the proxy server
func (p *Proxy) Run(ctx context.Context) error {
	mux := http.NewServeMux()

	// Observer endpoints (specific paths first)
	mux.HandleFunc("/_/", func(w http.ResponseWriter, r *http.Request) {
		// Serve the web UI at /_/
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(WebUI))
	})
	mux.HandleFunc("/_/ws", p.handleObserverWS)
	mux.HandleFunc("/_/api/log", p.handleObserverAPI)
	mux.HandleFunc("/_/api/clear", p.handleObserverClear)
	mux.HandleFunc("/_/api/targets", p.handleObserverTargets)

	// Handle WebSocket connections (CDP protocol)
	mux.HandleFunc("/devtools/", func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			p.handleWebSocket(w, r)
		} else {
			p.handleCDPHTTP(w, r)
		}
	})

	// Handle everything else - CDP HTTP or WebSocket
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Don't forward /_/ paths - they should be handled by observer handlers
		if strings.HasPrefix(r.URL.Path, "/_/") {
			http.NotFound(w, r)
			return
		}
		if websocket.IsWebSocketUpgrade(r) {
			p.handleWebSocket(w, r)
		} else {
			p.handleCDPHTTP(w, r)
		}
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", p.ListenPort),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	log.Printf("CDP proxy listening on port %d -> forwarding to port %d", p.ListenPort, p.TargetPort)
	log.Printf("CDP Observer web UI at http://localhost:%d/_/", p.ListenPort)
	return server.ListenAndServe()
}

// WebUI is the HTML/CSS/JS for the observer web interface
const WebUI = `<!DOCTYPE html>
<html>
<head>
    <title>CDP Observer</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, monospace; background: #1e1e1e; color: #d4d4d4; }
        .header { background: #252526; padding: 10px 20px; display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid #3c3c3c; }
        .header h1 { font-size: 16px; font-weight: 500; }
        .controls { display: flex; gap: 10px; align-items: center; }
        .controls button { background: #0e639c; color: white; border: none; padding: 6px 12px; border-radius: 3px; cursor: pointer; font-size: 12px; }
        .controls button:hover { background: #1177bb; }
        .controls button.danger { background: #c42b1c; }
        .controls button.danger:hover { background: #d63c2d; }
        .controls button.secondary { background: #3c3c3c; }
        .controls button.secondary:hover { background: #4c4c4c; }
        .filter { display: flex; gap: 10px; padding: 10px 20px; background: #252526; border-bottom: 1px solid #3c3c3c; flex-wrap: wrap; align-items: center; }
        .filter input { flex: 1; min-width: 200px; background: #3c3c3c; border: 1px solid #555; color: #d4d4d4; padding: 6px 10px; border-radius: 3px; font-size: 12px; }
        .filter select { background: #3c3c3c; border: 1px solid #555; color: #d4d4d4; padding: 6px 10px; border-radius: 3px; font-size: 12px; max-width: 300px; }
        .exclusions { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
        .exclusions:empty { display: none; }
        .exclusion-tag { background: #6e3030; color: #f0a0a0; padding: 3px 8px; border-radius: 3px; font-size: 11px; cursor: pointer; display: flex; align-items: center; gap: 4px; }
        .exclusion-tag:hover { background: #8e4040; }
        .exclusion-tag .remove { font-weight: bold; }
        .log { height: calc(100vh - 140px); overflow-y: auto; padding: 10px; }
        .entry { padding: 8px 12px; margin: 4px 0; border-radius: 4px; font-size: 12px; cursor: pointer; }
        .entry:hover { background: #2a2d2e; }
        .entry.client { border-left: 3px solid #4fc3f7; background: #1a2733; }
        .entry.browser { border-left: 3px solid #81c784; background: #1a2a1a; }
        .entry .meta { display: flex; gap: 10px; margin-bottom: 4px; flex-wrap: wrap; align-items: center; }
        .entry .time { color: #888; }
        .entry .direction { font-weight: 500; }
        .entry.client .direction { color: #4fc3f7; }
        .entry.browser .direction { color: #81c784; }
        .entry .method { color: #dcdcaa; font-weight: 500; cursor: pointer; padding: 1px 4px; border-radius: 2px; }
        .entry .method:hover { background: #6e3030; color: #f0a0a0; }
        .entry .method[title]:hover::after { content: ' (click to hide)'; font-size: 10px; color: #888; }
        .entry .id { color: #888; }
        .entry .target-badge { background: #3c3c3c; padding: 2px 6px; border-radius: 3px; font-size: 10px; color: #9cdcfe; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
        .entry .target-badge.page { border-left: 2px solid #4fc3f7; }
        .entry .target-badge.worker { border-left: 2px solid #f0a020; }
        .entry .target-badge.iframe { border-left: 2px solid #a070f0; }
        .entry .params { color: #9cdcfe; white-space: pre-wrap; word-break: break-all; display: none; margin-top: 8px; padding: 8px; background: #1e1e1e; border-radius: 3px; max-height: 300px; overflow: auto; }
        .entry.expanded .params { display: block; }
        .stats { color: #888; font-size: 12px; }
        .connected { color: #81c784; }
        .disconnected { color: #f44336; }
        .targets-panel { background: #252526; border-bottom: 1px solid #3c3c3c; padding: 10px 20px; display: none; }
        .targets-panel.visible { display: block; }
        .targets-panel h3 { font-size: 12px; margin-bottom: 8px; color: #888; }
        .target-list { display: flex; flex-wrap: wrap; gap: 8px; }
        .target-item { background: #3c3c3c; padding: 6px 10px; border-radius: 4px; font-size: 11px; cursor: pointer; max-width: 250px; }
        .target-item:hover { background: #4c4c4c; }
        .target-item.selected { background: #0e639c; }
        .target-item .target-type { color: #888; font-size: 10px; text-transform: uppercase; }
        .target-item .target-title { color: #d4d4d4; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
        .target-item .target-url { color: #9cdcfe; font-size: 10px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
        .presets { display: flex; gap: 4px; }
        .presets button { background: #3c3c3c; color: #d4d4d4; border: none; padding: 4px 8px; border-radius: 3px; cursor: pointer; font-size: 10px; }
        .presets button:hover { background: #4c4c4c; }
    </style>
</head>
<body>
    <div class="header">
        <h1>CDP Observer</h1>
        <div class="controls">
            <span class="stats"><span id="status" class="disconnected">Disconnected</span> | <span id="count">0</span> msgs | <span id="targetCount">0</span> targets | <span id="excludeCount">0</span> excluded</span>
            <button onclick="toggleTargets()">Targets</button>
            <button onclick="togglePause()"><span id="pauseBtn">Pause</span></button>
            <button onclick="clearLog()" class="danger">Clear</button>
        </div>
    </div>
    <div class="targets-panel" id="targetsPanel">
        <h3>Active Targets (click to filter)</h3>
        <div class="target-list" id="targetList"></div>
    </div>
    <div class="filter">
        <input type="text" id="filterInput" placeholder="Filter by method name..." oninput="applyFilter()">
        <select id="directionFilter" onchange="applyFilter()">
            <option value="">All directions</option>
            <option value="client->browser">Client -> Browser</option>
            <option value="browser->client">Browser -> Client</option>
        </select>
        <select id="targetFilter" onchange="applyFilter()">
            <option value="">All targets</option>
        </select>
        <select id="domainFilter" onchange="applyFilter()">
            <option value="">All domains</option>
            <option value="Page">Page</option>
            <option value="Network">Network</option>
            <option value="Runtime">Runtime</option>
            <option value="DOM">DOM</option>
            <option value="Target">Target</option>
            <option value="Log">Log</option>
            <option value="Console">Console</option>
            <option value="Debugger">Debugger</option>
        </select>
        <div class="presets">
            <button onclick="applyPreset('quiet')" title="Hide noisy events">Quiet</button>
            <button onclick="applyPreset('network')" title="Focus on network">Network</button>
            <button onclick="clearExclusions()" title="Clear all exclusions">Reset</button>
        </div>
    </div>
    <div class="filter" id="exclusionsBar" style="padding-top: 0; display: none;">
        <span style="color: #888; font-size: 11px;">Excluded:</span>
        <div class="exclusions" id="exclusionsList"></div>
    </div>
    <div class="log" id="log"></div>

    <script>
        let entries = [];
        let targets = {};
        let paused = false;
        let ws = null;
        let autoScroll = true;
        let selectedTargetId = '';
        let excludedMethods = new Set(JSON.parse(localStorage.getItem('cdp-excluded') || '[]'));

        // Preset exclusion lists
        const presets = {
            quiet: [
                'Network.dataReceived',
                'Network.loadingFinished',
                'Network.webSocketFrameReceived',
                'Network.webSocketFrameSent',
                'Page.screencastFrame',
                'Page.screencastFrameAck',
                'Runtime.consoleAPICalled',
                'Runtime.exceptionThrown',
                'Log.entryAdded',
                'Target.receivedMessageFromTarget',
                'Target.targetInfoChanged'
            ],
            network: [
                'Page.screencastFrame',
                'Page.screencastFrameAck',
                'Runtime.consoleAPICalled',
                'Runtime.exceptionThrown',
                'Runtime.executionContextCreated',
                'Runtime.executionContextDestroyed',
                'Log.entryAdded',
                'DOM.documentUpdated',
                'DOM.setChildNodes',
                'Target.receivedMessageFromTarget',
                'Target.targetInfoChanged',
                'Page.frameStartedLoading',
                'Page.frameStoppedLoading',
                'Page.domContentEventFired',
                'Page.loadEventFired'
            ]
        };

        function saveExclusions() {
            localStorage.setItem('cdp-excluded', JSON.stringify([...excludedMethods]));
            updateExclusionsUI();
        }

        function excludeMethod(method, event) {
            if (event) event.stopPropagation();
            if (!method || method === 'response' || method === 'message') return;
            excludedMethods.add(method);
            saveExclusions();
            applyFilter();
        }

        function removeExclusion(method) {
            excludedMethods.delete(method);
            saveExclusions();
            applyFilter();
        }

        function clearExclusions() {
            excludedMethods.clear();
            saveExclusions();
            applyFilter();
        }

        function applyPreset(name) {
            const methods = presets[name] || [];
            methods.forEach(m => excludedMethods.add(m));
            saveExclusions();
            applyFilter();
        }

        function updateExclusionsUI() {
            const list = document.getElementById('exclusionsList');
            const bar = document.getElementById('exclusionsBar');
            const count = excludedMethods.size;

            document.getElementById('excludeCount').textContent = count;

            if (count === 0) {
                bar.style.display = 'none';
                list.innerHTML = '';
                return;
            }

            bar.style.display = 'flex';
            list.innerHTML = [...excludedMethods].sort().map(m =>
                '<span class="exclusion-tag" onclick="removeExclusion(\'' + escapeHtml(m) + '\')" title="Click to remove">' +
                escapeHtml(m) + ' <span class="remove">x</span></span>'
            ).join('');
        }

        function connect() {
            ws = new WebSocket('ws://' + location.host + '/_/ws');
            ws.onopen = () => {
                document.getElementById('status').textContent = 'Connected';
                document.getElementById('status').className = 'connected';
                fetchTargets();
            };
            ws.onclose = () => {
                document.getElementById('status').textContent = 'Disconnected';
                document.getElementById('status').className = 'disconnected';
                setTimeout(connect, 1000);
            };
            ws.onmessage = (e) => {
                if (paused) return;
                const entry = JSON.parse(e.data);
                entries.push(entry);
                if (entries.length > 5000) entries = entries.slice(-5000);
                document.getElementById('count').textContent = entries.length;

                // Update targets from entry
                if (entry.targetInfo) {
                    targets[entry.targetInfo.targetId] = entry.targetInfo;
                    updateTargetUI();
                }

                renderEntry(entry);
            };
        }

        function fetchTargets() {
            fetch('/_/api/targets')
                .then(r => r.json())
                .then(data => {
                    if (Array.isArray(data)) {
                        data.forEach(t => { targets[t.targetId] = t; });
                        updateTargetUI();
                    }
                })
                .catch(() => {});
        }

        function updateTargetUI() {
            const targetList = document.getElementById('targetList');
            const targetFilter = document.getElementById('targetFilter');
            const targetArr = Object.values(targets);

            document.getElementById('targetCount').textContent = targetArr.length;

            // Update target list panel
            targetList.innerHTML = targetArr.map(t => {
                const selected = t.targetId === selectedTargetId ? 'selected' : '';
                const title = t.title || 'Untitled';
                const url = t.url || '';
                return '<div class="target-item ' + selected + '" onclick="selectTarget(\'' + t.targetId + '\')" title="' + escapeHtml(url) + '">' +
                    '<div class="target-type">' + (t.type || 'unknown') + '</div>' +
                    '<div class="target-title">' + escapeHtml(title) + '</div>' +
                    '<div class="target-url">' + escapeHtml(url) + '</div>' +
                '</div>';
            }).join('');

            // Update target filter dropdown
            const currentValue = targetFilter.value;
            targetFilter.innerHTML = '<option value="">All targets</option>' +
                targetArr.map(t => {
                    const label = (t.title || t.url || t.targetId).substring(0, 40);
                    return '<option value="' + t.targetId + '">' + escapeHtml(label) + '</option>';
                }).join('');
            targetFilter.value = currentValue;
        }

        function selectTarget(targetId) {
            selectedTargetId = selectedTargetId === targetId ? '' : targetId;
            document.getElementById('targetFilter').value = selectedTargetId;
            updateTargetUI();
            applyFilter();
        }

        function escapeHtml(str) {
            return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
        }

        function renderEntry(entry) {
            const log = document.getElementById('log');
            const filter = document.getElementById('filterInput').value.toLowerCase();
            const dirFilter = document.getElementById('directionFilter').value;
            const targetFilterVal = document.getElementById('targetFilter').value;
            const domainFilter = document.getElementById('domainFilter').value;

            const method = entry.parsed?.method || '';

            // Check exclusions
            if (method && excludedMethods.has(method)) return;

            if (filter && !method.toLowerCase().includes(filter)) return;
            if (dirFilter && entry.direction !== dirFilter) return;
            if (targetFilterVal && entry.targetId !== targetFilterVal && entry.sessionId !== targetFilterVal) return;
            if (domainFilter && !method.startsWith(domainFilter + '.')) return;

            const div = document.createElement('div');
            div.className = 'entry ' + (entry.direction === 'client->browser' ? 'client' : 'browser');

            const time = new Date(entry.timestamp).toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3 });
            const dir = entry.direction === 'client->browser' ? '->' : '<-';
            const id = entry.parsed?.id ? '#' + entry.parsed.id : '';
            const methodName = entry.parsed?.method || (entry.parsed?.result !== undefined ? 'response' : 'message');

            // Target badge
            let targetBadge = '';
            if (entry.targetInfo) {
                const ti = entry.targetInfo;
                const badgeClass = ti.type === 'page' ? 'page' : (ti.type === 'worker' || ti.type === 'service_worker' ? 'worker' : 'iframe');
                const label = ti.title || ti.url || ti.targetId.substring(0, 8);
                targetBadge = '<span class="target-badge ' + badgeClass + '" title="' + escapeHtml(ti.url || '') + '">' + escapeHtml(label.substring(0, 30)) + '</span>';
            } else if (entry.sessionId) {
                // Show truncated session ID if no target info
                targetBadge = '<span class="target-badge">' + entry.sessionId.substring(0, 8) + '</span>';
            }

            // Method span with click-to-exclude (only for actual methods)
            const methodSpan = method ?
                '<span class="method" title="Click to exclude" onclick="excludeMethod(\'' + escapeHtml(method) + '\', event)">' + escapeHtml(methodName) + '</span>' :
                '<span class="method">' + escapeHtml(methodName) + '</span>';

            div.innerHTML = '<div class="meta"><span class="time">' + time + '</span><span class="direction">' + dir + '</span>' + methodSpan + '<span class="id">' + id + '</span>' + targetBadge + '</div><div class="params">' + escapeHtml(JSON.stringify(entry.raw, null, 2)) + '</div>';
            div.onclick = (e) => { if (e.target.className !== 'method') div.classList.toggle('expanded'); };

            log.appendChild(div);
            if (autoScroll) log.scrollTop = log.scrollHeight;
        }

        function applyFilter() {
            document.getElementById('log').innerHTML = '';
            entries.forEach(renderEntry);
        }

        function togglePause() {
            paused = !paused;
            document.getElementById('pauseBtn').textContent = paused ? 'Resume' : 'Pause';
        }

        function toggleTargets() {
            const panel = document.getElementById('targetsPanel');
            panel.classList.toggle('visible');
            if (panel.classList.contains('visible')) {
                fetchTargets();
            }
        }

        function clearLog() {
            entries = [];
            document.getElementById('log').innerHTML = '';
            document.getElementById('count').textContent = '0';
            fetch('/_/api/clear');
        }

        // Auto-scroll control
        document.getElementById('log').addEventListener('scroll', function() {
            autoScroll = this.scrollTop + this.clientHeight >= this.scrollHeight - 50;
        });

        // Periodically refresh targets
        setInterval(fetchTargets, 5000);

        // Initialize
        updateExclusionsUI();
        connect();
    </script>
</body>
</html>
`
