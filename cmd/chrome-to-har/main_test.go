package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/har"
	"github.com/tmc/cdp/internal/browserprofile"
	"github.com/tmc/cdp/internal/differential"
	"github.com/tmc/cdp/internal/testutil"
)

// TestMain adds global test setup and teardown for browser cleanup
func TestMain(m *testing.M) {
	// Clean up before tests
	testutil.CleanupOrphanedBrowsers(&testing.T{})

	// Run tests
	code := m.Run()

	// Clean up after tests
	testutil.CleanupOrphanedBrowsers(&testing.T{})

	os.Exit(code)
}

func TestCaptureDifferentialCompletesCapture(t *testing.T) {
	workDir := t.TempDir()
	controller, err := differential.NewDifferentialController(&differential.DifferentialOptions{WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}

	captured := &har.HAR{Log: &har.Log{
		Version: "1.2",
		Creator: &har.Creator{Name: "test", Version: "1"},
		Entries: []*har.Entry{{
			Request:  &har.Request{Method: "GET", URL: "https://example.com/"},
			Response: &har.Response{Status: 200, StatusText: "OK", Content: &har.Content{}},
		}},
	}}
	runCapture := func(ctx context.Context, pm browserprofile.ProfileManager, opts options) error {
		if opts.streaming {
			t.Fatal("differential capture left streaming enabled")
		}
		data, err := json.Marshal(captured)
		if err != nil {
			return err
		}
		return os.WriteFile(opts.outputFile, data, 0644)
	}

	opts := options{
		captureName:   "baseline",
		captureLabels: "suite=smoke",
		startURL:      "https://example.com/",
		streaming:     true,
	}
	if err := captureDifferential(context.Background(), opts, controller, runCapture); err != nil {
		t.Fatal(err)
	}

	captures := controller.ListCaptures()
	if len(captures) != 1 {
		t.Fatalf("capture count = %d, want 1", len(captures))
	}
	metadata := captures[0]
	if metadata.Status != differential.CaptureStatusCompleted {
		t.Fatalf("capture status = %q, want %q", metadata.Status, differential.CaptureStatusCompleted)
	}
	if metadata.EntryCount != 1 {
		t.Fatalf("entry count = %d, want 1", metadata.EntryCount)
	}
	if _, err := os.Stat(filepath.Join(workDir, metadata.ID+".har")); err != nil {
		t.Fatalf("capture HAR was not written: %v", err)
	}
	if _, err := controller.CompareCapturesByID(metadata.ID, metadata.ID); err != nil {
		t.Fatalf("completed capture is not comparison-ready: %v", err)
	}
}

func TestBasicRun(t *testing.T) {
	// Not parallel — uses OutputCapture which redirects os.Stdout globally.
	testutil.SkipIfNoChrome(t)

	// Skip tests that require actual Chrome execution
	if testing.Short() || os.Getenv("CI") != "" {
		t.Skip("Skipping tests that require Chrome execution")
	}

	tests := []struct {
		name    string
		opts    options
		wantErr bool
		want    string
	}{
		{
			name: "missing_profile",
			opts: options{
				outputFile: "test.har",
			},
			wantErr: true,
		},
		{
			name: "basic_profile",
			opts: options{
				profileDir: "Test Profile 1",
				outputFile: "test.har",
				headless:   true,
			},
			wantErr: false,
			want:    "Chrome process allocator created",
		},
		{
			name: "with_url",
			opts: options{
				profileDir: "Test Profile 1",
				outputFile: "test.har",
				headless:   true,
				startURL:   "https://example.com",
			},
			wantErr: false,
			want:    "Chrome process allocator created",
		},
		{
			name: "interactive_mode",
			opts: options{
				profileDir:      "Test Profile 1",
				headless:        true,
				interactiveMode: true,
			},
			wantErr: false,
			want:    "Interactive CLI Mode. Type commands to execute JavaScript in the browser.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture, err := testutil.NewOutputCapture()
			if err != nil {
				t.Fatalf("Failed to capture output: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			mockPM := testutil.NewMockProfileManager()
			runner := NewRunner(mockPM)
			err = runner.Run(ctx, tt.opts)
			stdout, logs := capture.Stop()

			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check both stdout and logs for expected output
			output := stdout + logs
			if tt.want != "" && !strings.Contains(output, tt.want) {
				t.Errorf("Run() output = %q, want to contain %q", output, tt.want)
			}

			if tt.wantErr && stdout != "" {
				t.Errorf("Run() stdout = %q, want empty on error", stdout)
			}
		})
	}
}

func TestListProfiles(t *testing.T) {
	// Not parallel — this test redirects os.Stdout globally.
	testutil.SkipIfNoChrome(t)
	tests := []struct {
		name    string
		verbose bool
		want    []string
		wantErr bool
	}{
		{
			name:    "basic_list",
			verbose: false,
			want:    []string{"Test Profile 1", "Test Profile 2"},
		},
		{
			name:    "verbose_list",
			verbose: true,
			want:    []string{"Found valid profile: Test Profile 1", "Found valid profile: Test Profile 2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture output
			var buf bytes.Buffer
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Capture log output as well
			logBuf := &bytes.Buffer{}
			log.SetOutput(logBuf)

			mockPM := testutil.NewMockProfileManager()
			mockPM.Verbose = tt.verbose

			profiles, err := mockPM.ListProfiles()
			if err != nil {
				t.Fatalf("ListProfiles() error = %v", err)
			}

			fmt.Println("Available Chrome profiles:")
			for _, p := range profiles {
				fmt.Printf("  - %s\n", p)
			}

			// Close writer and restore stdout
			w.Close()
			os.Stdout = oldStdout
			io.Copy(&buf, r)

			stdout := buf.String()
			logs := logBuf.String()

			if (err != nil) != tt.wantErr {
				t.Errorf("ListProfiles() error = %v, wantErr %v", err, tt.wantErr)
			}

			output := stdout + logs
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("ListProfiles() output = %q, want to contain %q", output, want)
				}
			}
		})
	}
}

func TestInteractiveScript(t *testing.T) {
	t.Parallel()
	testutil.SkipIfNoChrome(t)

	// The test would be implemented as follows in a real environment:
	// 1. Create a temporary directory
	// 2. Create a script file with commands (document.title, exit, etc.)
	// 3. Build the chrome-to-har binary
	// 4. Run it with -interactive -headless -url=about:blank
	// 5. Feed the script as stdin
	// 6. Verify the output contains expected responses
}

func TestStreamingOutput(t *testing.T) {
	// Not parallel — uses OutputCapture which redirects os.Stdout globally.
	testutil.SkipIfNoChrome(t)

	// Skip tests that require actual Chrome execution
	if testing.Short() || os.Getenv("CI") != "" {
		t.Skip("Skipping tests that require Chrome execution")
	}

	// Create a test HTTP server that will generate network traffic
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!DOCTYPE html>
			<html>
			<head><title>Test Page</title></head>
			<body><h1>Test Content</h1></body>
			</html>`)
	}))
	defer ts.Close()

	// Create a 404 endpoint for testing filtered streaming
	ts404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Not Found")
	}))
	defer ts404.Close()

	tests := []struct {
		name    string
		opts    options
		want    []string
		wantErr bool
	}{
		{
			name: "basic_streaming",
			opts: options{
				profileDir: "Test Profile 1",
				streaming:  true,
				headless:   true,
				startURL:   ts.URL, // Navigate to test server!
			},
			want: []string{
				`"startedDateTime"`,
				`"request"`,
				`"response"`,
			},
		},
		{
			name: "filtered_streaming",
			opts: options{
				profileDir: "Test Profile 1",
				streaming:  true,
				headless:   true,
				startURL:   ts404.URL, // Navigate to 404 server
				filter:     "select(.response.status >= 400)",
			},
			want: []string{
				`"status":`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture, err := testutil.NewOutputCapture()
			if err != nil {
				t.Fatalf("Failed to capture output: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mockPM := testutil.NewMockProfileManager()
			runner := NewRunner(mockPM)
			err = runner.Run(ctx, tt.opts)
			stdout, _ := capture.Stop()

			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}

			for _, want := range tt.want {
				if !strings.Contains(stdout, want) {
					t.Errorf("Run() stdout = %q, want to contain %q", stdout, want)
				}
			}
		})
	}
}
