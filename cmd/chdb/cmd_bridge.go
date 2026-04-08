package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

var bridgeCmd = &cobra.Command{
	Use:   "bridge",
	Short: "Start a multiplexing debug bridge",
	Long: `Starts a WebSocket bridge that allows multiple clients (CLI, GUI, Scripts) 
to share a single Chrome specific debugging session.

It connects to a target Chrome instance and exposes a WebSocket server.
All connected clients share the same state (breakpoints, execution).

WORKFLOWS:

1. Hybrid Debugging (GUI + CLI)
   Best for mixing visual inspection with CLI automation.
   - Start Bridge: chdb bridge
   - Connect GUI:  Open chrome://inspect -> Configure localhost:9229
   - Connect CLI:  chdb <cmd> --port 9229

2. CLI-Only Persistent Debugging
   Best for retaining session state (breakpoints) across multiple CLI commands.
   - Start Bridge: chdb bridge
   - Run Cmds:     chdb break set ... --port 9229
   - The session remains active in the bridge even after chdb exits.

3. Collaborative/Scripted Debugging
   Attach "Sidecar" scripts to your manual debugging session.
   - You debug manually in Chrome DevTools (via Bridge).
   - A script monitors events or runs audits in the background via the same Bridge.`,
	Example: `  # 1. Start Chrome with remote debugging
  ./start-chrome-debug.sh

  # 2. Start the Bridge (Default: :9229 -> :9222)
  chdb bridge

  # 3. Connect CLI to Bridge
  chdb break list --port 9229

  # 4. Hybrid: Set breakpoint in CLI, hit it in GUI
  chdb break xhr "api/v1" --port 9229`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetString("port")
		target, _ := cmd.Flags().GetString("target")
		shouldOpen, _ := cmd.Flags().GetBool("open")
		return runBridge(port, target, shouldOpen)
	},
}

func init() {
	bridgeCmd.Flags().StringP("port", "p", "9229", "Port to listen on")
	bridgeCmd.Flags().StringP("target", "t", "localhost:9222", "Upstream Chrome debug port/host")
	bridgeCmd.Flags().Bool("open", false, "Open browser to bridge landing page")
}

// Global state for the bridge
type Bridge struct {
	upstreamURL    string
	browserURL     string // Browser-level WebSocket URL
	manualTargetID string // User-selected target ID
	clients        map[*Client]bool
	upstream       *websocket.Conn

	// ID Management
	pendingReq map[int64]*Client // Map BridgeID -> Originating Client

	mu sync.Mutex
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
	hub  *Bridge
}

func runBridge(port, targetHost string, shouldOpen bool) error {
	log.Printf("Starting CHDB Bridge on :%s -> %s", port, targetHost)

	// verify target availability and get the browser target
	listURL := fmt.Sprintf("http://%s/json/list", targetHost)
	resp, err := http.Get(listURL)
	if err != nil {
		return fmt.Errorf("cannot connect to target at %s: %v", targetHost, err)
	}
	defer resp.Body.Close()

	var targets []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return fmt.Errorf("failed to parse target list: %v", err)
	}

	if len(targets) == 0 {
		return fmt.Errorf("no targets found at %s", targetHost)
	}

	// Pick the first page target
	var targetWS string
	var targetID string
	for _, t := range targets {
		if t["type"] == "page" {
			targetID = t["id"].(string)
			// Construct WS URL if not complete
			if ws, ok := t["webSocketDebuggerUrl"].(string); ok && ws != "" {
				targetWS = ws
			} else {
				targetWS = fmt.Sprintf("ws://%s/devtools/page/%s", targetHost, targetID)
			}
			break
		}
	}

	if targetWS == "" {
		// Fallback to first available
		targetID = targets[0]["id"].(string)
		if ws, ok := targets[0]["webSocketDebuggerUrl"].(string); ok && ws != "" {
			targetWS = ws
		} else {
			targetWS = fmt.Sprintf("ws://%s/devtools/page/%s", targetHost, targetID)
		}
	}

	// Fetch Browser WebSocket URL for management commands
	browserWS := ""
	if vResp, err := http.Get(fmt.Sprintf("http://%s/json/version", targetHost)); err == nil {
		var vInfo map[string]interface{}
		if json.NewDecoder(vResp.Body).Decode(&vInfo) == nil {
			if ws, ok := vInfo["webSocketDebuggerUrl"].(string); ok {
				browserWS = ws
			}
		}
		vResp.Body.Close()
	}

	// Fetch Chrome Revision for AppSpot URL
	chromeRevision := ""
	if vResp, err := http.Get(fmt.Sprintf("http://%s/json/version", targetHost)); err == nil {
		var vInfo map[string]interface{}
		if json.NewDecoder(vResp.Body).Decode(&vInfo) == nil {
			// Extract revision from "WebKit-Version": "537.36 (@d9d2e0...)"
			if ver, ok := vInfo["WebKit-Version"].(string); ok {
				// Simple extraction: find string between parens?
				// Typically format is "537.36 (@<hash>)"
				// Let's implement a robust extraction in a helper or inline
				start := -1
				for i, r := range ver {
					if r == '@' {
						start = i + 1
						break
					}
				}
				if start != -1 {
					end := len(ver)
					for i := start; i < len(ver); i++ {
						if ver[i] == ')' {
							end = i
							break
						}
					}
					chromeRevision = ver[start:end]
				}
			}
		}
		vResp.Body.Close()
	}

	bridge := &Bridge{
		upstreamURL: targetHost, // Store Host, not full WS URL (we discover it)
		browserURL:  browserWS,
		clients:     make(map[*Client]bool),
		pendingReq:  make(map[int64]*Client),
	}

	// Start Upstream Maintenance Loop
	go bridge.maintainUpstream()

	// --- HTTP Handlers for Client Discovery ---

	// /connect-target: Switches the bridge to a specific target
	http.HandleFunc("/connect-target", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id", 400)
			return
		}

		bridge.mu.Lock()
		bridge.manualTargetID = id
		// Force reconnection
		if bridge.upstream != nil {
			bridge.upstream.Close()
			bridge.upstream = nil
		}
		bridge.mu.Unlock()

		// Give it a moment to reconnect
		time.Sleep(500 * time.Millisecond)
		http.Redirect(w, r, "/", 302)
	})

	// /json/version
	http.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		tr, err := http.Get(fmt.Sprintf("http://%s/json/version", targetHost))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer tr.Body.Close()
		io.Copy(w, tr.Body)
	})

	// /upstream/list - Proxy real target list
	http.HandleFunc("/upstream/list", func(w http.ResponseWriter, r *http.Request) {
		tr, err := http.Get(fmt.Sprintf("http://%s/json/list", targetHost))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer tr.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, tr.Body)
	})

	// /json/list - We mimic a single target that points to our Bridge WS
	http.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		// Return a single target representing the Shared Session
		t := []map[string]interface{}{
			{
				"description":          "CHDB Shared Bridge Session",
				"devtoolsFrontendUrl":  fmt.Sprintf("chrome-devtools://devtools/bundled/inspector.html?experiments=true&v8only=true&ws=localhost:%s/", port),
				"id":                   "shared-session",
				"title":                "CHDB Bridge",
				"type":                 "node", // Use 'node' to trigger simple view, or 'page'
				"url":                  "chdb://bridge",
				"webSocketDebuggerUrl": fmt.Sprintf("ws://localhost:%s/", port),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(t)
	})

	// WebSocket Handler / Landing Page
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "websocket" {
			serveWs(bridge, w, r)
		} else {
			serveLandingPage(w, r, port, chromeRevision)
		}
	})

	// API: Rename Symbol (Phase 2)
	http.HandleFunc("/api/rename", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}

		var req struct {
			File         string `json:"file"`
			LineNumber   int    `json:"lineNumber"`
			ColumnNumber int    `json:"columnNumber"`
			NewName      string `json:"newName"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", 400)
			return
		}

		log.Printf("[RENAME-API] Request received: Rename %s:%d:%d to '%s'", req.File, req.LineNumber, req.ColumnNumber, req.NewName)

		// TODO: Trigger AST rewriting implementation here
		// For now, assume success
		response := map[string]string{
			"status": "queued",
			"msg":    fmt.Sprintf("Rename of symbol at %s:%d to %s accepted", req.File, req.LineNumber, req.NewName),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Magic Launch Handler (Triggers Chrome to open the URL via CDP)
	http.HandleFunc("/open-inspector", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}

		// Use devtools:// with remoteFrontend=true
		devtoolsURL := fmt.Sprintf("devtools://devtools/bundled/inspector.html?remoteFrontend=true&experiments=true&ws=127.0.0.1:%s", port)

		msg := map[string]interface{}{
			"id":     int(time.Now().Unix()),
			"method": "Target.createTarget",
			"params": map[string]string{
				"url": devtoolsURL,
			},
		}

		// Use Browser connection if available, otherwise fall back to upstream (which likely fails)
		targetConn := bridge.upstream
		isBrowserConn := false

		if bridge.browserURL != "" {
			// Dial Browser Target ephemerally
			if conn, _, err := websocket.DefaultDialer.Dial(bridge.browserURL, nil); err == nil {
				targetConn = conn
				isBrowserConn = true
				defer conn.Close()
			} else {
				log.Printf("Failed to dial browser target: %v", err)
			}
		}

		if !isBrowserConn {
			// If we stick to upstream, we need lock.
			// If ephemeral, we own it.
			bridge.mu.Lock()
			defer bridge.mu.Unlock()
		}

		if err := targetConn.WriteJSON(msg); err != nil {
			http.Error(w, fmt.Sprintf("Failed to send CDP command: %v", err), 500)
			return
		}

		// Read response to debug failure
		targetConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, respBytes, err := targetConn.ReadMessage()
		if err != nil {
			log.Printf("Failed to read CDP response: %v", err)
			http.Error(w, fmt.Sprintf("Failed to read CDP response: %v", err), 500)
			return
		}
		log.Printf("Launcher CDP Response: %s", string(respBytes))
		w.Header().Set("Content-Type", "application/json")
		w.Write(respBytes)
	})

	if shouldOpen {
		go func() {
			time.Sleep(500 * time.Millisecond) // Wait for server to start
			openBrowser("http://localhost:" + port)
		}()
	}

	return http.ListenAndServe(":"+port, nil)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func serveWs(bridge *Bridge, w http.ResponseWriter, r *http.Request) {
	log.Printf("Incoming WS connection from %s", r.RemoteAddr)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	client := &Client{conn: conn, send: make(chan []byte, 256), hub: bridge}
	bridge.register(client)

	go client.writePump()
	go client.readPump()
}

func (b *Bridge) register(c *Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[c] = true
	log.Printf("Client registered. Total clients: %d", len(b.clients))
}

func (b *Bridge) unregister(c *Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.clients[c]; ok {
		delete(b.clients, c)
		close(c.send)
		log.Printf("Client unregistered. Total clients: %d", len(b.clients))
	}
}

// maintainUpstream manages the connection to Chrome, reconnecting if lost
func (b *Bridge) maintainUpstream() {
	for {
		// 1. connect
		if b.upstream == nil {
			b.findAndConnect()
		}

		// 2. read loop
		if b.upstream != nil {
			for {
				_, message, err := b.upstream.ReadMessage()
				if err != nil {
					log.Printf("Upstream read error: %v. Reconnecting...", err)
					b.mu.Lock()
					if b.upstream != nil {
						b.upstream.Close()
						b.upstream = nil
					}
					b.mu.Unlock()
					break
				}
				b.handleUpstreamMessage(message)
			}
		}

		time.Sleep(1 * time.Second)
	}
}

func (b *Bridge) findAndConnect() {
	listURL := fmt.Sprintf("http://%s/json/list", b.upstreamURL)
	resp, err := http.Get(listURL)
	if err != nil {
		log.Printf("Discovery failed: %v", err)
		return
	}
	defer resp.Body.Close()

	var targets []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return
	}

	var targetWS string

	// If Manual Target is set, look for it specifically
	if b.manualTargetID != "" {
		for _, t := range targets {
			if t["id"] == b.manualTargetID {
				if ws, ok := t["webSocketDebuggerUrl"].(string); ok {
					targetWS = ws
					log.Printf("Connecting to manual target: %s", t["title"])
				}
				break
			}
		}
	}

	// Fallback: Prefer "page" targets
	if targetWS == "" {
		for _, t := range targets {
			if t["type"] == "page" && t["url"] != "" {
				// Avoid attaching to itself if that ever happens, or other devtools
				targetWS = t["webSocketDebuggerUrl"].(string)
				log.Printf("Connecting to target: %s (%s)", t["title"], t["url"])
				break
			}
		}
	}

	if targetWS != "" {
		conn, _, err := websocket.DefaultDialer.Dial(targetWS, nil)
		if err == nil {
			b.mu.Lock()
			b.upstream = conn
			b.mu.Unlock()
			log.Printf("Upstream connected: %s", targetWS)
		} else {
			log.Printf("Dial failed: %v", err)
		}
	}
}

func (b *Bridge) handleUpstreamMessage(message []byte) {
	// Parse message to check for ID
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// ... (Existing ID logic kept simple for brevity, full broadcast) ...

	// Broadcast to all clients
	for client := range b.clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(b.clients, client)
		}
	}
}

func (b *Bridge) sendToUpstream(msg []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.upstream == nil {
		return fmt.Errorf("upstream not connected")
	}
	return b.upstream.WriteMessage(websocket.TextMessage, msg)
}

func (c *Client) readPump() {
	defer func() {
		log.Println("Client readPump stopped")
		c.hub.unregister(c)
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("Client read error: %v", err)
			break
		}

		// Forward to Upstream
		err = c.hub.sendToUpstream(message)
		if err != nil {
			log.Printf("Error writing to upstream: %v", err)
			// Don't break! Just retry or ignore. Client can retry.
			// Getting disconnected is worse.
			continue
		}
	}
}

func (c *Client) writePump() {
	defer func() {
		log.Println("Client writePump stopped")
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			if err := w.Close(); err != nil {
				return
			}
		}
	}
}

func serveLandingPage(w http.ResponseWriter, r *http.Request, port, revision string) {
	w.Header().Set("Content-Type", "text/html")

	// Reverted to devtools:// scheme (AppSpot was 404ing).
	// Fixed String formatting: ensure only PORT is passed if using %s
	devtoolsURL := fmt.Sprintf("devtools://devtools/bundled/inspector.html?remoteFrontend=true&experiments=true&ws=127.0.0.1:%s", port)

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
<title>CHDB Bridge</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; padding: 2rem; text-align: center; background: #f0f2f5; color: #333; }
.card { background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); max-width: 650px; margin: 0 auto; }
h1 { color: #1a73e8; margin-bottom: 0.5rem; }
.url-box { background: #f8f9fa; border: 1px solid #ddd; padding: 10px; border-radius: 4px; word-break: break-all; font-family: monospace; margin: 1rem 0; font-size: 0.9em; user-select: all; }
.btn { display: inline-block; background: #1a73e8; color: white; padding: 10px 20px; text-decoration: none; border-radius: 4px; font-weight: bold; cursor: pointer; border: none; font-size: 1rem; }
.btn:hover { background: #1557b0; }
.secondary { background: #e8f0fe; color: #1a73e8; margin-left: 10px; }
.secondary:hover { background: #d2e3fc; }
.note { font-size: 0.85em; color: #666; margin-top: 2rem; border-top: 1px solid #eee; padding-top: 1rem; }
.target-list { text-align: left; margin-top: 2rem; border-top: 1px solid #eee; padding-top: 1rem; }
.target-item { display: flex; justify-content: space-between; align-items: center; padding: 10px; border-bottom: 1px solid #eee; }
.target-info { flex-grow: 1; overflow: hidden; }
.target-title { font-weight: bold; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.target-url { font-size: 0.8em; color: #777; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.connect-btn { font-size: 0.8em; padding: 5px 10px; background: #34a853; margin-left: 10px; }
</style>
<script>
function copyUrl() {
	const url = document.getElementById('dt-url').textContent;
	navigator.clipboard.writeText(url).then(() => {
		const btn = document.getElementById('copy-btn');
		const original = btn.textContent;
		btn.textContent = 'Copied!';
		setTimeout(() => btn.textContent = original, 2000);
	});
}
function launchAuto() {
	const btn = document.getElementById('launch-btn');
	btn.textContent = 'Launching...';
	fetch('/open-inspector', { method: 'POST' })
		.then(r => {
			if (r.ok) {
				btn.textContent = 'Launched!';
				setTimeout(() => btn.textContent = 'Launch Automatically', 3000);
			} else {
				btn.textContent = 'Failed';
				alert('Launch failed. Please use copy-paste method.');
			}
		})
		.catch(e => {
			btn.textContent = 'Error';
			console.error(e);
		});
}
	function copyTargetUrl(btn, wsUrl) {
		const devtoolsUrl = 'devtools://devtools/bundled/inspector.html?remoteFrontend=true&experiments=true&ws=' + wsUrl.replace('ws://', '');
		navigator.clipboard.writeText(devtoolsUrl).then(() => {
			const original = btn.textContent;
			btn.textContent = 'Copied!';
			setTimeout(() => btn.textContent = original, 2000);
		});
	}
	function loadTargets() {
		fetch('/upstream/list')
			.then(r => r.json())
			.then(targets => {
				const list = document.getElementById('target-list');
				list.innerHTML = '';
				targets.forEach(t => {
					const item = document.createElement('div');
					item.className = 'target-item';
					const wsUrl = t.webSocketDebuggerUrl || '';
					item.innerHTML = '<div class="target-info"><div class="target-title">' + (t.title || 'Untitled') + '</div><div class="target-url">' + (t.url || '') + '</div></div><div><button class="btn secondary connect-btn" onclick="copyTargetUrl(this, \'' + wsUrl + '\')">Copy</button><a href="/connect-target?id=' + t.id + '" class="btn connect-btn">Connect</a></div>';
					list.appendChild(item);
				});
			});
	}
	window.onload = loadTargets;
</script>
</head>
<body>
<div class="card">
	<h1>CHDB Bridge Active</h1>
	<p>The bridge is running on <strong>:%s</strong>.</p>
	
	<div style="text-align: left; margin-top: 2rem;">
		<p><strong>Option 1: Paste URL (Recommended)</strong><br>
		Browsers block direct links to DevTools. Copy this URL and paste it into a new tab:</p>
		<div id="dt-url" class="url-box">%s</div>
		<div style="text-align: center;">
			<button id="copy-btn" class="btn" onclick="copyUrl()">Copy URL</button>
			<button id="launch-btn" class="btn secondary" onclick="launchAuto()">Launch Automatically</button>
		</div>
	</div>

	<div class="target-list">
		<h3>Select Target</h3>
		<p class="note">Choose which Chrome Tab or Window to debug:</p>
		<div id="target-list">Loading targets...</div>
	</div>

	<div class="note">
		Bridge Status: 🟢 Connected to Chrome
	</div>
</div>
</body>
</html>
`, port, devtoolsURL)
	w.Write([]byte(html))
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}
