package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// SessionFile represents a persisted session
type SessionFile struct {
	Port      string `json:"port"`
	PID       int    `json:"pid,omitempty"`
	TargetID  string `json:"target_id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Timestamp int64  `json:"timestamp"`
}

// GetSessionDir returns the directory for session files
func GetSessionDir() string {
	// Use XDG_RUNTIME_DIR if available (systemd systems)
	if runtime := os.Getenv("XDG_RUNTIME_DIR"); runtime != "" {
		return filepath.Join(runtime, "ndp")
	}
	// Fall back to /tmp
	return "/tmp/ndp"
}

// SaveSessionFile saves session info to a file
func SaveSessionFile(port string, session *SessionFile) error {
	dir := GetSessionDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	filename := filepath.Join(dir, fmt.Sprintf("%s.session", port))
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, data, 0600)
}

// LoadSessionFile loads session info from a file
func LoadSessionFile(port string) (*SessionFile, error) {
	filename := filepath.Join(GetSessionDir(), fmt.Sprintf("%s.session", port))
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var session SessionFile
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// RemoveSessionFile removes a session file
func RemoveSessionFile(port string) error {
	filename := filepath.Join(GetSessionDir(), fmt.Sprintf("%s.session", port))
	return os.Remove(filename)
}

// ListSessionFiles lists all active session files
func ListSessionFiles() ([]SessionFile, error) {
	dir := GetSessionDir()
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionFile{}, nil
		}
		return nil, err
	}

	var sessions []SessionFile
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".session") {
			port := strings.TrimSuffix(file.Name(), ".session")
			if session, err := LoadSessionFile(port); err == nil {
				sessions = append(sessions, *session)
			}
		}
	}

	return sessions, nil
}

// SetDefaultSession creates a symlink to the default session
func SetDefaultSession(port string) error {
	dir := GetSessionDir()
	defaultLink := filepath.Join(dir, "default")

	// Remove old symlink if exists
	os.Remove(defaultLink)

	// Create new symlink
	target := fmt.Sprintf("%s.session", port)
	return os.Symlink(target, defaultLink)
}

// GetDefaultSession returns the default session port
func GetDefaultSession() (string, error) {
	defaultLink := filepath.Join(GetSessionDir(), "default")

	// Read the symlink
	target, err := os.Readlink(defaultLink)
	if err != nil {
		return "", err
	}

	// Extract port from filename
	if strings.HasSuffix(target, ".session") {
		return strings.TrimSuffix(target, ".session"), nil
	}

	return "", fmt.Errorf("invalid default session link")
}

// GetOrSelectPort gets the port from args, env, or default session
func GetOrSelectPort(specifiedPort string) (string, error) {
	// 1. Use specified port if provided
	if specifiedPort != "" {
		return specifiedPort, nil
	}

	// 2. Check environment variable
	if envPort := os.Getenv("NDP_PORT"); envPort != "" {
		return envPort, nil
	}

	// 3. Check for default session
	if defaultPort, err := GetDefaultSession(); err == nil {
		return defaultPort, nil
	}

	// 4. List available sessions
	sessions, err := ListSessionFiles()
	if err != nil {
		return "", err
	}

	if len(sessions) == 0 {
		return "", fmt.Errorf("no active sessions. Use 'ndp node attach <port>' first")
	}

	if len(sessions) == 1 {
		// Only one session, use it
		return sessions[0].Port, nil
	}

	// Multiple sessions, list them
	fmt.Println("Multiple active sessions found:")
	for i, s := range sessions {
		fmt.Printf("  [%d] Port %s: %s\n", i+1, s.Port, s.Title)
	}
	return "", fmt.Errorf("specify port with -p flag or set NDP_PORT environment variable")
}