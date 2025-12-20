package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

// SimpleBreakpointSetter sets breakpoints using direct WebSocket connection
type SimpleBreakpointSetter struct {
	port      string
	messageID int
	conn      *websocket.Conn
}

// NewSimpleBreakpointSetter creates a new breakpoint setter
func NewSimpleBreakpointSetter(port string) *SimpleBreakpointSetter {
	return &SimpleBreakpointSetter{
		port: port,
	}
}

// SetBreakpoint sets a breakpoint directly via WebSocket
func (sbs *SimpleBreakpointSetter) SetBreakpoint(ctx context.Context, location string, condition string) error {
	// Parse location
	parts := strings.Split(location, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid location format, use file:line")
	}

	fileName := parts[0]
	lineNumber, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid line number: %v", err)
	}

	// Get WebSocket URL
	wsURL, err := sbs.getWebSocketURL()
	if err != nil {
		return err
	}

	// Connect to WebSocket
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 5 * time.Second

	conn, _, err := dialer.Dial(wsURL, http.Header{})
	if err != nil {
		return errors.Wrapf(err, "failed to connect to WebSocket")
	}
	defer conn.Close()

	sbs.conn = conn

	// Enable debugger
	if _, err := sbs.sendCommand("Debugger.enable", nil); err != nil {
		return errors.Wrap(err, "failed to enable debugger")
	}

	// Build file URL pattern
	fileURL := fmt.Sprintf("file://.*%s", fileName)

	// Set breakpoint by URL
	params := map[string]interface{}{
		"lineNumber": lineNumber - 1, // CDP uses 0-based line numbers
		"urlRegex":   fileURL,
	}

	if condition != "" {
		params["condition"] = condition
	}

	response, err := sbs.sendCommand("Debugger.setBreakpointByUrl", params)
	if err != nil {
		return errors.Wrap(err, "failed to set breakpoint")
	}

	// Parse response
	if breakpointID, ok := response["breakpointId"].(string); ok {
		fmt.Printf("Breakpoint set: %s at %s:%d\n", breakpointID, fileName, lineNumber)

		// Check if breakpoint was resolved
		if locations, ok := response["locations"].([]interface{}); ok && len(locations) > 0 {
			fmt.Println("Breakpoint resolved and active")
		} else {
			fmt.Println("Breakpoint pending (will activate when script loads)")
		}

		return nil
	}

	return errors.New("failed to get breakpoint ID from response")
}

// getWebSocketURL gets the WebSocket URL for the debug session
func (sbs *SimpleBreakpointSetter) getWebSocketURL() (string, error) {
	url := fmt.Sprintf("http://localhost:%s/json/list", sbs.port)
	resp, err := http.Get(url)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get debug targets")
	}
	defer resp.Body.Close()

	var targets []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return "", errors.Wrap(err, "failed to parse targets")
	}

	if len(targets) == 0 {
		return "", errors.New("no debug targets found")
	}

	// Get the first target's WebSocket URL
	if wsURL, ok := targets[0]["webSocketDebuggerUrl"].(string); ok {
		return wsURL, nil
	}

	return "", errors.New("WebSocket URL not found")
}

// sendCommand sends a command and waits for response
func (sbs *SimpleBreakpointSetter) sendCommand(method string, params map[string]interface{}) (map[string]interface{}, error) {
	sbs.messageID++

	message := map[string]interface{}{
		"id":     sbs.messageID,
		"method": method,
	}

	if params != nil {
		message["params"] = params
	}

	// Send message
	if err := sbs.conn.WriteJSON(message); err != nil {
		return nil, errors.Wrap(err, "failed to send message")
	}

	// Wait for response
	for {
		var response map[string]interface{}
		if err := sbs.conn.ReadJSON(&response); err != nil {
			return nil, errors.Wrap(err, "failed to read response")
		}

		// Check if this is our response
		if id, ok := response["id"].(float64); ok && int(id) == sbs.messageID {
			// Check for error
			if errObj, ok := response["error"].(map[string]interface{}); ok {
				return nil, fmt.Errorf("CDP error %v: %v", errObj["code"], errObj["message"])
			}

			// Return result
			if result, ok := response["result"].(map[string]interface{}); ok {
				return result, nil
			}

			return make(map[string]interface{}), nil
		}

		// If it's an event, ignore it and continue reading
		if _, ok := response["method"]; ok {
			continue
		}
	}
}

// ListBreakpoints lists active breakpoints in the session
func (sbs *SimpleBreakpointSetter) ListBreakpoints(ctx context.Context) error {
	// Get WebSocket URL
	wsURL, err := sbs.getWebSocketURL()
	if err != nil {
		return err
	}

	// Connect to WebSocket
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 5 * time.Second

	conn, _, err := dialer.Dial(wsURL, http.Header{})
	if err != nil {
		return errors.Wrapf(err, "failed to connect to WebSocket")
	}
	defer conn.Close()

	sbs.conn = conn

	// Enable debugger
	if _, err := sbs.sendCommand("Debugger.enable", nil); err != nil {
		return errors.Wrap(err, "failed to enable debugger")
	}

	// Note: CDP doesn't have a direct "list breakpoints" command
	// Breakpoints are tracked by the debugger domain internally
	// We'd need to maintain our own list or listen to breakpoint events

	fmt.Printf("Breakpoints for session on port %s:\n", sbs.port)
	fmt.Println("(Note: CDP doesn't expose a list breakpoints API - breakpoints are managed internally)")
	fmt.Println("Use Chrome DevTools or VS Code to see active breakpoints")

	return nil
}