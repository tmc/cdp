package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"errors"

	"github.com/gorilla/websocket"
)

// V8InspectorClient provides a comprehensive Node.js debugging client
// that matches Chrome DevTools capabilities using direct WebSocket connections
type V8InspectorClient struct {
	host  string
	port  string
	wsURL string
	conn  *websocket.Conn

	// Message handling
	messageID     int
	pendingCalls  map[int]chan *CDPResponse
	eventHandlers map[string][]func(map[string]interface{})
	mu            sync.RWMutex

	// State management
	connected       bool
	debuggerEnabled bool
	runtimeEnabled  bool
	profilerEnabled bool

	// Debugging state
	breakpoints map[string]*V8Breakpoint
	scripts     map[string]*V8Script
	callFrames  []*V8CallFrame
	paused      bool

	// Settings
	verbose       bool
	autoReconnect bool
}

// V8Target represents a Node.js debugging target
type V8Target struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	DevtoolsFrontendURL  string `json:"devtoolsFrontendUrl,omitempty"`
	FaviconURL           string `json:"faviconUrl,omitempty"`
	Description          string `json:"description,omitempty"`
}

// CDPMessage represents a Chrome DevTools Protocol message
type CDPMessage struct {
	ID     int                    `json:"id,omitempty"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// CDPResponse represents a CDP response or event
type CDPResponse struct {
	ID     *int                   `json:"id,omitempty"`
	Method string                 `json:"method,omitempty"`
	Params map[string]interface{} `json:"params,omitempty"`
	Result map[string]interface{} `json:"result,omitempty"`
	Error  *CDPError              `json:"error,omitempty"`
}

// CDPError represents a CDP error
type CDPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// V8Breakpoint represents a breakpoint in V8
type V8Breakpoint struct {
	ID           string `json:"breakpointId"`
	Location     string `json:"location"`
	LineNumber   int    `json:"lineNumber"`
	ColumnNumber int    `json:"columnNumber,omitempty"`
	Condition    string `json:"condition,omitempty"`
	URL          string `json:"url,omitempty"`
	URLRegex     string `json:"urlRegex,omitempty"`
	Resolved     bool   `json:"resolved"`
}

// V8Script represents a loaded script
type V8Script struct {
	ScriptID    string `json:"scriptId"`
	URL         string `json:"url"`
	StartLine   int    `json:"startLine"`
	StartColumn int    `json:"startColumn"`
	EndLine     int    `json:"endLine"`
	EndColumn   int    `json:"endColumn"`
	Hash        string `json:"hash,omitempty"`
	Source      string `json:"source,omitempty"`
}

// V8CallFrame represents a call frame in the execution stack
type V8CallFrame struct {
	CallFrameID  string                   `json:"callFrameId"`
	FunctionName string                   `json:"functionName"`
	Location     map[string]interface{}   `json:"location"`
	URL          string                   `json:"url"`
	ScopeChain   []map[string]interface{} `json:"scopeChain"`
	This         map[string]interface{}   `json:"this"`
}

// NewV8InspectorClient creates a new V8 Inspector client
func NewV8InspectorClient(host, port string, verbose bool) *V8InspectorClient {
	return &V8InspectorClient{
		host:          host,
		port:          port,
		pendingCalls:  make(map[int]chan *CDPResponse),
		eventHandlers: make(map[string][]func(map[string]interface{})),
		breakpoints:   make(map[string]*V8Breakpoint),
		scripts:       make(map[string]*V8Script),
		verbose:       verbose,
		autoReconnect: true,
	}
}

// DiscoverTargets discovers available Node.js debugging targets
func (c *V8InspectorClient) DiscoverTargets(ctx context.Context) ([]*V8Target, error) {
	url := fmt.Sprintf("http://%s:%s/json/list", c.host, c.port)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf(fmt.Sprintf("failed to discover targets at %s", url)+": %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read discovery response: %w", err)
	}

	var targets []*V8Target
	if err := json.Unmarshal(body, &targets); err != nil {
		return nil, fmt.Errorf("failed to parse discovery response: %w", err)
	}

	if c.verbose {
		log.Printf("Discovered %d Node.js debugging targets", len(targets))
	}

	return targets, nil
}

// Connect establishes a WebSocket connection to the specified target
func (c *V8InspectorClient) Connect(ctx context.Context, target *V8Target) error {
	if target.WebSocketDebuggerURL == "" {
		return errors.New("target has no WebSocket debugger URL")
	}

	c.wsURL = target.WebSocketDebuggerURL

	// Parse and validate WebSocket URL
	_, err := url.Parse(c.wsURL)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("invalid WebSocket URL: %s", c.wsURL)+": %w", err)
	}

	// Establish WebSocket connection
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(c.wsURL, nil)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("failed to connect to WebSocket: %s", c.wsURL)+": %w", err)
	}

	c.conn = conn
	c.connected = true

	if c.verbose {
		log.Printf("Connected to Node.js target: %s", target.Title)
		log.Printf("WebSocket URL: %s", c.wsURL)
	}

	// Start message handling goroutine
	go c.handleMessages()

	return nil
}

// ConnectByPort connects to a Node.js process by port number
func (c *V8InspectorClient) ConnectByPort(ctx context.Context, port string) error {
	c.port = port
	if c.host == "" {
		c.host = "127.0.0.1"
	}

	targets, err := c.DiscoverTargets(ctx)
	if err != nil {
		return err
	}

	if len(targets) == 0 {
		return fmt.Errorf("no debugging targets found on port %s", port)
	}

	// Use the first available target
	return c.Connect(ctx, targets[0])
}

// Disconnect closes the WebSocket connection
func (c *V8InspectorClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}

	return nil
}

// IsConnected returns true if the client is connected
func (c *V8InspectorClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// SendCommand sends a CDP command and waits for the response
func (c *V8InspectorClient) SendCommand(method string, params map[string]interface{}) (map[string]interface{}, error) {
	if !c.connected {
		return nil, errors.New("not connected to debugging target")
	}

	c.mu.Lock()
	c.messageID++
	msgID := c.messageID

	// Create response channel
	responseChan := make(chan *CDPResponse, 1)
	c.pendingCalls[msgID] = responseChan
	c.mu.Unlock()

	// Clean up on exit
	defer func() {
		c.mu.Lock()
		delete(c.pendingCalls, msgID)
		c.mu.Unlock()
	}()

	// Send message
	message := CDPMessage{
		ID:     msgID,
		Method: method,
		Params: params,
	}

	if err := c.conn.WriteJSON(message); err != nil {
		return nil, fmt.Errorf(fmt.Sprintf("failed to send command %s", method)+": %w", err)
	}

	if c.verbose {
		log.Printf("Sent command: %s (ID: %d)", method, msgID)
	}

	// Wait for response with timeout
	select {
	case response := <-responseChan:
		if response.Error != nil {
			return nil, fmt.Errorf("CDP error %d: %s", response.Error.Code, response.Error.Message)
		}
		return response.Result, nil

	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout waiting for response to %s", method)
	}
}

// handleMessages processes incoming WebSocket messages
func (c *V8InspectorClient) handleMessages() {
	defer func() {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
	}()

	for {
		var response CDPResponse
		if err := c.conn.ReadJSON(&response); err != nil {
			if c.verbose {
				log.Printf("WebSocket read error: %v", err)
			}
			return
		}

		// Handle response to command
		if response.ID != nil {
			c.mu.RLock()
			responseChan, exists := c.pendingCalls[*response.ID]
			c.mu.RUnlock()

			if exists {
				select {
				case responseChan <- &response:
				default:
					// Channel full, skip
				}
				continue
			}
		}

		// Handle event
		if response.Method != "" {
			c.handleEvent(response.Method, response.Params)
		}
	}
}

// handleEvent processes CDP events
func (c *V8InspectorClient) handleEvent(method string, params map[string]interface{}) {
	if c.verbose {
		log.Printf("Received event: %s", method)
	}

	// Update internal state based on events
	switch method {
	case "Debugger.scriptParsed":
		c.handleScriptParsed(params)
	case "Debugger.paused":
		c.handleDebuggerPaused(params)
	case "Debugger.resumed":
		c.handleDebuggerResumed(params)
	case "Debugger.breakpointResolved":
		c.handleBreakpointResolved(params)
	}

	// Call registered event handlers
	c.mu.RLock()
	handlers := c.eventHandlers[method]
	c.mu.RUnlock()

	for _, handler := range handlers {
		go handler(params)
	}
}

// OnEvent registers an event handler
func (c *V8InspectorClient) OnEvent(method string, handler func(map[string]interface{})) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.eventHandlers[method] = append(c.eventHandlers[method], handler)
}

// Helper methods for handling specific events
func (c *V8InspectorClient) handleScriptParsed(params map[string]interface{}) {
	script := &V8Script{}
	if data, err := json.Marshal(params); err == nil {
		json.Unmarshal(data, script)
		c.scripts[script.ScriptID] = script
	}
}

func (c *V8InspectorClient) handleDebuggerPaused(params map[string]interface{}) {
	c.paused = true
	if callFrames, ok := params["callFrames"].([]interface{}); ok {
		c.callFrames = nil
		for _, frame := range callFrames {
			if frameData, err := json.Marshal(frame); err == nil {
				var callFrame V8CallFrame
				if json.Unmarshal(frameData, &callFrame) == nil {
					c.callFrames = append(c.callFrames, &callFrame)
				}
			}
		}
	}
}

func (c *V8InspectorClient) handleDebuggerResumed(params map[string]interface{}) {
	c.paused = false
	c.callFrames = nil
}

func (c *V8InspectorClient) handleBreakpointResolved(params map[string]interface{}) {
	if breakpointID, ok := params["breakpointId"].(string); ok {
		if bp, exists := c.breakpoints[breakpointID]; exists {
			bp.Resolved = true
		}
	}
}

// GetTargetInfo returns information about the current target
func (c *V8InspectorClient) GetTargetInfo() map[string]interface{} {
	return map[string]interface{}{
		"connected":       c.connected,
		"debuggerEnabled": c.debuggerEnabled,
		"runtimeEnabled":  c.runtimeEnabled,
		"profilerEnabled": c.profilerEnabled,
		"paused":          c.paused,
		"wsURL":           c.wsURL,
		"breakpointCount": len(c.breakpoints),
		"scriptCount":     len(c.scripts),
	}
}

// Scripts returns all loaded scripts.
func (c *V8InspectorClient) Scripts() map[string]*V8Script {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]*V8Script, len(c.scripts))
	for k, v := range c.scripts {
		out[k] = v
	}
	return out
}
