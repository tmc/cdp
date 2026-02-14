package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Start a CDP proxy to inspect DevTools traffic",
	Long: `Starts a WebSocket proxy that sits between Chrome DevTools (or any CDP client) 
and the target Node.js process. This allows you to inspect the protocol traffic.

It listens on port 9230 by default and forwards to the target's debug port (e.g. 9229).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProxy()
	},
}

var (
	proxyPort   string
	proxyTarget string
)

func init() {
	proxyCmd.Flags().StringVar(&proxyPort, "port", "9230", "Port to listen on")
	proxyCmd.Flags().StringVar(&proxyTarget, "target", "localhost:9229", "Target Node.js debug port/host")
	rootCmd.AddCommand(proxyCmd)
}

func runProxy() error {
	log.Printf("Starting CDP Proxy on :%s, forwarding to %s", proxyPort, proxyTarget)

	// verify target availability
	listURL := fmt.Sprintf("http://%s/json/list", proxyTarget)
	resp, err := http.Get(listURL)
	if err != nil {
		return errors.Wrapf(err, "cannot connect to target at %s", proxyTarget)
	}
	defer resp.Body.Close()

	// Parse targets to get the UUIDs
	var targets []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return errors.Wrap(err, "failed to parse target list")
	}

	if len(targets) == 0 {
		return errors.New("no targets found")
	}

	// We serve a /json/list endpoint that mimics the target but points WebSockets to our proxy
	http.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		// Fetch fresh list from target
		tr, err := http.Get(listURL)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer tr.Body.Close()
		body, _ := io.ReadAll(tr.Body)

		// Rewrite webSocketDebuggerUrl to point to us
		var localTargets []map[string]interface{}
		json.Unmarshal(body, &localTargets)

		for _, t := range localTargets {
			id := t["id"].(string)
			// Construct our proxy URL
			// ws://localhost:9230/ws/<id>
			t["webSocketDebuggerUrl"] = fmt.Sprintf("ws://localhost:%s/ws/%s", proxyPort, id)
			t["devtoolsFrontendUrl"] = strings.Replace(t["devtoolsFrontendUrl"].(string), proxyTarget, "localhost:"+proxyPort, 1)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(localTargets)
	})

	// Also serve /json/version
	http.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		tr, err := http.Get(fmt.Sprintf("http://%s/json/version", proxyTarget))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer tr.Body.Close()
		body, _ := io.ReadAll(tr.Body)

		var version map[string]interface{}
		json.Unmarshal(body, &version)
		version["webSocketDebuggerUrl"] = fmt.Sprintf("ws://localhost:%s/ws/default", proxyPort)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(version)
	})

	// The WebSocket handler
	http.HandleFunc("/ws/", func(w http.ResponseWriter, r *http.Request) {
		// Extract ID from path
		parts := strings.Split(r.URL.Path, "/")
		id := parts[len(parts)-1]
		if id == "" {
			id = targets[0]["id"].(string) // Default to first
		}

		handleWSProxy(w, r, id)
	})

	return http.ListenAndServe(":"+proxyPort, nil)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func handleWSProxy(w http.ResponseWriter, r *http.Request, targetID string) {
	// 1. Upgrade client connection
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade failed: %v", err)
		return
	}
	defer clientConn.Close()

	// 2. Connect to Target
	// Find the true WebSocket URL for this ID
	// For simplicity, we assume we can construct it or find it.
	// Node usually uses: ws://localhost:9229/uuid
	targetWS := fmt.Sprintf("ws://%s/%s", proxyTarget, targetID)

	log.Printf("Proxying client -> %s", targetWS)

	targetConn, _, err := websocket.DefaultDialer.Dial(targetWS, nil)
	if err != nil {
		log.Printf("Failed to connect to target %s: %v", targetWS, err)
		clientConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error connecting to target: %v", err)))
		return
	}
	defer targetConn.Close()

	// 3. Pipe loop
	errChan := make(chan error, 2)

	// Client -> Target
	go func() {
		for {
			mt, msg, err := clientConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			// Log request
			if verbose {
				log.Printf("[C->T] %s", string(msg))
			} else {
				// Try to parse method to log briefly
				var m map[string]interface{}
				if json.Unmarshal(msg, &m) == nil {
					if method, ok := m["method"].(string); ok {
						if method != "Debugger.stepOver" { // ignore noise
							log.Printf(">> %s", method)
						}
					}
				}
			}

			if err := targetConn.WriteMessage(mt, msg); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// Target -> Client
	go func() {
		for {
			mt, msg, err := targetConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			// Log response
			if verbose {
				// Truncate long messages
				if len(msg) > 1000 {
					log.Printf("[T->C] %s... (truncated)", string(msg[:1000]))
				} else {
					log.Printf("[T->C] %s", string(msg))
				}
			} else {
				// Log events
				var m map[string]interface{}
				if json.Unmarshal(msg, &m) == nil {
					if method, ok := m["method"].(string); ok {
						log.Printf("<< Event: %s", method)
					} else if id, ok := m["id"]; ok {
						log.Printf("<< Resp: %v", id)
					}
				}
			}

			if err := clientConn.WriteMessage(mt, msg); err != nil {
				errChan <- err
				return
			}
		}
	}()

	<-errChan
	log.Printf("Proxy connection closed for %s", targetID)
}
