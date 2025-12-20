// vz_computer.go - VM computer control via vmctl socket
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// VZComputer implements computer control for a macOS VM via vmctl socket.
type VZComputer struct {
	socketPath      string
	width           int
	height          int
	verbose         bool
	workDir         string
	screenshotCount int
}

// vmctlCommand represents a command to send to the VM control socket
type vmctlCommand struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// vmctlResponse represents a response from the VM control socket
type vmctlResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    string `json:"data,omitempty"` // Base64 encoded for screenshots
}

// vmctlMouseData for mouse commands
type vmctlMouseData struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Action   string  `json:"action"`
	Button   int     `json:"button,omitempty"`
	Absolute bool    `json:"absolute,omitempty"`
}

// vmctlKeyData for key commands
type vmctlKeyData struct {
	KeyCode   uint16 `json:"keyCode,omitempty"`
	Character string `json:"character,omitempty"`
	Modifiers uint   `json:"modifiers,omitempty"`
	KeyDown   bool   `json:"keyDown"`
}

// vmctlTextData for text commands
type vmctlTextData struct {
	Text string `json:"text"`
}

// NewVZComputer creates a new VM computer instance.
func NewVZComputer(socketPath string, verbose bool, workDir string) (*VZComputer, error) {
	// Verify socket exists
	if _, err := os.Stat(socketPath); err != nil {
		return nil, fmt.Errorf("socket not found at %s: %w", socketPath, err)
	}

	vc := &VZComputer{
		socketPath:      socketPath,
		width:           1280,
		height:          800,
		verbose:         verbose,
		workDir:         workDir,
		screenshotCount: 0,
	}

	// Test connection with ping
	resp, err := vc.sendCommand(vmctlCommand{Type: "ping"})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to VM: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("VM ping failed: %s", resp.Error)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Connected to VM control socket: %s\n", socketPath)
	}

	return vc, nil
}

// sendCommand sends a command to the VM control socket and returns the response.
func (c *VZComputer) sendCommand(cmd vmctlCommand) (*vmctlResponse, error) {
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to socket: %w", err)
	}
	defer conn.Close()

	// Set timeout
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// Send command as JSON line
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal command: %w", err)
	}
	cmdBytes = append(cmdBytes, '\n')

	if _, err := conn.Write(cmdBytes); err != nil {
		return nil, fmt.Errorf("write command: %w", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	respLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp vmctlResponse
	if err := json.Unmarshal([]byte(respLine), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}

// ScreenSize returns the viewport dimensions.
func (c *VZComputer) ScreenSize() (width, height int) {
	return c.width, c.height
}

// ClickAt clicks at normalized coordinates (0-1000 scale).
func (c *VZComputer) ClickAt(x, y int) (*EnvState, error) {
	if c.verbose {
		fmt.Fprintf(os.Stderr, "🖱️  VM Click: normalized=(%d, %d)\n", x, y)
	}

	// Convert from 0-1000 to 0-1 normalized
	normX := float64(x) / 1000.0
	normY := float64(y) / 1000.0

	mouseData := vmctlMouseData{
		X:      normX,
		Y:      normY,
		Action: "click",
		Button: 0, // Left button
	}

	dataBytes, _ := json.Marshal(mouseData)
	resp, err := c.sendCommand(vmctlCommand{
		Type: "mouse",
		Data: dataBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("click at (%d, %d): %w", x, y, err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("click failed: %s", resp.Error)
	}

	time.Sleep(500 * time.Millisecond)
	return c.CurrentState()
}

// HoverAt hovers at normalized coordinates (0-1000 scale).
func (c *VZComputer) HoverAt(x, y int) (*EnvState, error) {
	if c.verbose {
		fmt.Fprintf(os.Stderr, "👆 VM Hover: normalized=(%d, %d)\n", x, y)
	}

	normX := float64(x) / 1000.0
	normY := float64(y) / 1000.0

	mouseData := vmctlMouseData{
		X:      normX,
		Y:      normY,
		Action: "move",
	}

	dataBytes, _ := json.Marshal(mouseData)
	resp, err := c.sendCommand(vmctlCommand{
		Type: "mouse",
		Data: dataBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("hover at (%d, %d): %w", x, y, err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("hover failed: %s", resp.Error)
	}

	time.Sleep(200 * time.Millisecond)
	return c.CurrentState()
}

// TypeTextAt types text. For VM, we ignore coordinates and just type.
func (c *VZComputer) TypeTextAt(x, y int, text string, pressEnter, clearBefore bool) (*EnvState, error) {
	if c.verbose {
		fmt.Fprintf(os.Stderr, "⌨️  VM Type: text='%s' pressEnter=%v\n", text, pressEnter)
	}

	// First click to focus
	if _, err := c.ClickAt(x, y); err != nil {
		return nil, fmt.Errorf("focusing: %w", err)
	}

	// Clear before if requested (Cmd+A, Backspace)
	if clearBefore {
		// Cmd+A to select all
		if err := c.sendKeyWithModifiers(0, "\ue009a"); err != nil { // Ctrl+A
			return nil, fmt.Errorf("select all: %w", err)
		}
		// Backspace to delete
		if err := c.sendKey(51, ""); err != nil { // Backspace keycode
			return nil, fmt.Errorf("delete: %w", err)
		}
	}

	// Type the text
	textData := vmctlTextData{Text: text}
	dataBytes, _ := json.Marshal(textData)
	resp, err := c.sendCommand(vmctlCommand{
		Type: "text",
		Data: dataBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("type text: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("type failed: %s", resp.Error)
	}

	// Press Enter if requested
	if pressEnter {
		if err := c.sendKey(36, "\r"); err != nil { // Return keycode
			return nil, fmt.Errorf("press enter: %w", err)
		}
	}

	time.Sleep(500 * time.Millisecond)
	return c.CurrentState()
}

// sendKey sends a key press (down + up)
func (c *VZComputer) sendKey(keyCode uint16, char string) error {
	// Key down
	keyData := vmctlKeyData{KeyCode: keyCode, Character: char, KeyDown: true}
	dataBytes, _ := json.Marshal(keyData)
	resp, err := c.sendCommand(vmctlCommand{Type: "key", Data: dataBytes})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Error)
	}

	// Key up
	keyData.KeyDown = false
	dataBytes, _ = json.Marshal(keyData)
	resp, err = c.sendCommand(vmctlCommand{Type: "key", Data: dataBytes})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Error)
	}

	return nil
}

// sendKeyWithModifiers sends a key with modifiers
func (c *VZComputer) sendKeyWithModifiers(keyCode uint16, char string) error {
	// For now, just send the character sequence
	return c.sendKey(keyCode, char)
}

// DragAndDrop performs a drag and drop operation.
func (c *VZComputer) DragAndDrop(x, y, destX, destY int) (*EnvState, error) {
	// Convert coordinates
	normX := float64(x) / 1000.0
	normY := float64(y) / 1000.0
	normDestX := float64(destX) / 1000.0
	normDestY := float64(destY) / 1000.0

	// Mouse down at start
	mouseData := vmctlMouseData{X: normX, Y: normY, Action: "down", Button: 0}
	dataBytes, _ := json.Marshal(mouseData)
	if _, err := c.sendCommand(vmctlCommand{Type: "mouse", Data: dataBytes}); err != nil {
		return nil, err
	}

	// Move to destination
	mouseData = vmctlMouseData{X: normDestX, Y: normDestY, Action: "move"}
	dataBytes, _ = json.Marshal(mouseData)
	if _, err := c.sendCommand(vmctlCommand{Type: "mouse", Data: dataBytes}); err != nil {
		return nil, err
	}

	// Mouse up at destination
	mouseData = vmctlMouseData{X: normDestX, Y: normDestY, Action: "up", Button: 0}
	dataBytes, _ = json.Marshal(mouseData)
	if _, err := c.sendCommand(vmctlCommand{Type: "mouse", Data: dataBytes}); err != nil {
		return nil, err
	}

	time.Sleep(500 * time.Millisecond)
	return c.CurrentState()
}

// Navigate is a no-op for VM (no browser URL concept)
func (c *VZComputer) Navigate(url string) (*EnvState, error) {
	if c.verbose {
		fmt.Fprintf(os.Stderr, "🌐 VM Navigate (no-op): %s\n", url)
	}
	return c.CurrentState()
}

// GoBack is a no-op for VM
func (c *VZComputer) GoBack() (*EnvState, error) {
	return c.CurrentState()
}

// GoForward is a no-op for VM
func (c *VZComputer) GoForward() (*EnvState, error) {
	return c.CurrentState()
}

// Search is a no-op for VM
func (c *VZComputer) Search() (*EnvState, error) {
	return c.CurrentState()
}

// ScrollDocument scrolls the page
func (c *VZComputer) ScrollDocument(direction string) (*EnvState, error) {
	// Use keyboard for scrolling
	var keyCode uint16
	switch direction {
	case "up":
		keyCode = 116 // PageUp
	case "down":
		keyCode = 121 // PageDown
	default:
		return nil, fmt.Errorf("unknown scroll direction: %s", direction)
	}

	if err := c.sendKey(keyCode, ""); err != nil {
		return nil, fmt.Errorf("scroll %s: %w", direction, err)
	}

	time.Sleep(300 * time.Millisecond)
	return c.CurrentState()
}

// ScrollAt scrolls at specific coordinates
func (c *VZComputer) ScrollAt(x, y int, direction string, magnitude float64) (*EnvState, error) {
	// For VM, just use document scroll
	return c.ScrollDocument(direction)
}

// KeyCombination presses a key combination
func (c *VZComputer) KeyCombination(keys []string) (*EnvState, error) {
	// Map key names to keycodes (macOS virtual key codes)
	keyMap := map[string]uint16{
		"Enter":     36,
		"Return":    36,
		"Backspace": 51,
		"Tab":       48,
		"Escape":    53,
		"Esc":       53,
		"PageDown":  121,
		"PageUp":    116,
		"Space":     49,
		// Arrow keys
		"Up":        126,
		"Down":      125,
		"Left":      123,
		"Right":     124,
		"ArrowUp":   126,
		"ArrowDown": 125,
		"ArrowLeft": 123,
		"ArrowRight": 124,
		// Function keys
		"Home":      115,
		"End":       119,
		"Delete":    117,
	}

	for _, key := range keys {
		if keyCode, ok := keyMap[key]; ok {
			if err := c.sendKey(keyCode, ""); err != nil {
				return nil, fmt.Errorf("key %s: %w", key, err)
			}
		} else {
			// Try as single character
			if len(key) == 1 {
				if err := c.sendKey(0, key); err != nil {
					return nil, fmt.Errorf("key %s: %w", key, err)
				}
			}
		}
	}

	time.Sleep(200 * time.Millisecond)
	return c.CurrentState()
}

// Wait5Seconds waits for 5 seconds
func (c *VZComputer) Wait5Seconds() (*EnvState, error) {
	time.Sleep(5 * time.Second)
	return c.CurrentState()
}

// CurrentState returns the current VM state with screenshot
func (c *VZComputer) CurrentState() (*EnvState, error) {
	screenshot, err := c.takeScreenshot()
	if err != nil {
		return nil, fmt.Errorf("taking screenshot: %w", err)
	}

	return &EnvState{
		Screenshot: screenshot,
		URL:        "vm://macos", // Placeholder URL for VM
	}, nil
}

// takeScreenshot captures a PNG screenshot from the VM
func (c *VZComputer) takeScreenshot() ([]byte, error) {
	resp, err := c.sendCommand(vmctlCommand{Type: "screenshot"})
	if err != nil {
		return nil, fmt.Errorf("screenshot command: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("screenshot failed: %s", resp.Error)
	}

	// Decode base64 data
	data, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}

	// Save to work directory
	if c.workDir != "" {
		c.screenshotCount++
		screenshotFile := filepath.Join(c.workDir, fmt.Sprintf("vm-screenshot-%04d.png", c.screenshotCount))
		if err := os.WriteFile(screenshotFile, data, 0644); err != nil {
			if c.verbose {
				fmt.Fprintf(os.Stderr, "Warning: failed to save screenshot: %v\n", err)
			}
		} else if c.verbose {
			fmt.Fprintf(os.Stderr, "Saved screenshot: %s\n", screenshotFile)
		}
	}

	return data, nil
}

// OpenWebBrowser is a no-op for VM (already running)
func (c *VZComputer) OpenWebBrowser(url string) (*EnvState, error) {
	return c.CurrentState()
}

// Close closes the VM connection (no-op, socket is per-request)
func (c *VZComputer) Close() error {
	return nil
}
