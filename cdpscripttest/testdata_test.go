//go:build cdp

package cdpscripttest_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// startTestServer serves the testdata/ directory and returns the base URL.
// The server is automatically closed when the test completes.
func startTestServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	t.Cleanup(srv.Close)
	return srv.URL
}
