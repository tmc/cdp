// Package wsorigin checks WebSocket handshake origins for local debug servers.
//
// Debug bridges and proxies in this repository accept WebSocket connections
// from CLI tools, the Chrome DevTools frontend, and their own web UIs served
// from localhost. Accepting every Origin would let any web page a developer
// visits drive the debug session (cross-site WebSocket hijacking). Check
// permits only the expected local clients.
package wsorigin

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Check reports whether the request's Origin is acceptable for a local
// debug WebSocket server. It permits requests with no Origin header
// (non-browser clients), devtools:// and chrome-extension:// origins,
// and http/https origins on a loopback host. Use it as a gorilla/websocket
// Upgrader CheckOrigin.
func Check(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "devtools", "chrome-extension":
		return true
	case "http", "https":
		return loopback(u.Hostname())
	}
	return false
}

// loopback reports whether host names the local machine.
func loopback(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
