package cdpscripttest

import (
	_ "embed"
	"net/http"
	"net/http/httptest"
	"testing"
)

//go:embed testdata/webrtc-loopback.html
var webrtcLoopbackHTML []byte

// WebRTCTestServer starts an httptest.Server that serves a WebRTC loopback
// test page at /. The page creates two local peer connections, exchanges
// offer/answer, adds fake video and audio tracks, and creates a data channel
// named "test-dc". The server is stopped when the test completes.
func WebRTCTestServer(t testing.TB) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(webrtcLoopbackHTML)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}
