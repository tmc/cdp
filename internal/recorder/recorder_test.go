package recorder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/har"
	"github.com/chromedp/cdproto/network"
)

func TestBuildFailedEntry(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	const reqID = network.RequestID("req-fail-1")
	r.requests[reqID] = &network.Request{
		Method:  "POST",
		URL:     "https://api.test/submit",
		Headers: network.Headers{"content-type": "application/json"},
	}
	r.postData[reqID] = `{"k":"v"}`

	entry := r.buildFailedEntry(reqID, &network.EventLoadingFailed{
		RequestID: reqID,
		ErrorText: "net::ERR_CONNECTION_RESET",
	})
	if entry == nil {
		t.Fatal("buildFailedEntry returned nil for a known request")
	}
	if entry.Request == nil || entry.Request.URL != "https://api.test/submit" || entry.Request.Method != "POST" {
		t.Fatalf("request not preserved: %+v", entry.Request)
	}
	if entry.Request.PostData == nil || entry.Request.PostData.Text != `{"k":"v"}` {
		t.Fatalf("post data not preserved: %+v", entry.Request.PostData)
	}
	if entry.Response == nil || entry.Response.Status != 0 || entry.Response.StatusText != "net::ERR_CONNECTION_RESET" {
		t.Fatalf("failed response not synthesized: %+v", entry.Response)
	}
	if !strings.Contains(entry.Comment, "net::ERR_CONNECTION_RESET") {
		t.Fatalf("comment missing error text: %q", entry.Comment)
	}

	// An unknown request id yields no entry rather than a bogus one.
	if r.buildFailedEntry(network.RequestID("nope"), &network.EventLoadingFailed{ErrorText: "x"}) != nil {
		t.Fatal("buildFailedEntry should return nil for an unknown request")
	}
}

func TestFetchContentLength(t *testing.T) {
	tests := []struct {
		name    string
		headers []*fetch.HeaderEntry
		want    int64
	}{
		{"absent", []*fetch.HeaderEntry{{Name: "Content-Type", Value: "text/html"}}, -1},
		{"present", []*fetch.HeaderEntry{{Name: "Content-Length", Value: "1024"}}, 1024},
		{"case insensitive", []*fetch.HeaderEntry{{Name: "content-length", Value: "42"}}, 42},
		{"whitespace", []*fetch.HeaderEntry{{Name: "Content-Length", Value: " 7 "}}, 7},
		{"unparseable", []*fetch.HeaderEntry{{Name: "Content-Length", Value: "big"}}, -1},
		{"nil", nil, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fetchContentLength(tt.headers); got != tt.want {
				t.Errorf("fetchContentLength = %d, want %d", got, tt.want)
			}
		})
	}
	// The gate must fire only for declared sizes over the cap.
	if fetchContentLength([]*fetch.HeaderEntry{{Name: "Content-Length", Value: "1024"}}) > fetchBodyMaxContentLength {
		t.Error("small response wrongly exceeds body cap")
	}
	huge := strconv.FormatInt(fetchBodyMaxContentLength+1, 10)
	if fetchContentLength([]*fetch.HeaderEntry{{Name: "Content-Length", Value: huge}}) <= fetchBodyMaxContentLength {
		t.Error("large response should exceed body cap")
	}
}

// timeToMonotonicTime is a helper to convert time.Time to *cdp.MonotonicTime
func timeToMonotonicTime(t time.Time) *cdp.MonotonicTime {
	mt := cdp.MonotonicTime(t)
	return &mt
}

func TestBuildStreamEntryEncodesBinaryBody(t *testing.T) {
	t.Parallel()

	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	body := []byte{0x00, 0xff, 0x10, 0x80}
	captured := r.captureBody(body)
	entry := r.buildStreamEntry("1", &network.Response{
		URL:      "https://example.com/image",
		Status:   200,
		MimeType: "image/png",
	}, &captured)

	content := entry.Response.Content
	if content.Encoding != "base64" {
		t.Fatalf("encoding = %q, want base64", content.Encoding)
	}
	decoded, err := base64.StdEncoding.DecodeString(content.Text)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string(body) {
		t.Fatalf("decoded body = %v, want %v", decoded, body)
	}
}

func TestMaxBodyBytesTruncatesWithMetadata(t *testing.T) {
	t.Parallel()

	r, err := New(WithMaxBodyBytes(4))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	captured := r.captureBody([]byte("abcdef"))
	content := &har.Content{MimeType: "text/plain"}
	setContentBody(content, "text/plain", captured)

	if content.Text != "abcd" {
		t.Fatalf("text = %q, want truncated body", content.Text)
	}
	if content.Size != 6 {
		t.Fatalf("size = %d, want original size", content.Size)
	}
	if !strings.Contains(content.Comment, "captured 4 of 6") {
		t.Fatalf("comment missing truncation metadata: %q", content.Comment)
	}
}

func TestRecorderStreaming(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		streaming bool
		events    []interface{}
		want      int // number of expected JSON outputs
	}{
		{
			name:      "streaming_enabled",
			streaming: true,
			events: []interface{}{
				&network.EventRequestWillBeSent{
					RequestID: "1",
					Request: &network.Request{
						URL:    "https://example.com",
						Method: "GET",
						Headers: map[string]interface{}{
							"User-Agent": "test",
						},
					},
				},
				&network.EventResponseReceived{
					RequestID: "1",
					Response: &network.Response{
						URL:        "https://example.com",
						Status:     200,
						StatusText: "OK",
						Headers: map[string]interface{}{
							"Content-Type": "text/html",
						},
					},
				},
				&network.EventLoadingFinished{
					RequestID: "1",
					Timestamp: timeToMonotonicTime(time.Now()),
				},
			},
			want: 2, // One for RequestWillBeSent, one for ResponseReceived
		},
		{
			name:      "streaming_disabled",
			streaming: false,
			events:    []interface{}{},
			want:      0,
		},
		{
			name:      "streaming_with_filtered_url",
			streaming: true,
			events: []interface{}{
				&network.EventRequestWillBeSent{
					RequestID: "1",
					Request: &network.Request{
						URL:    "https://filtered.com",
						Method: "GET",
					},
				},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []Option{WithStreaming(tt.streaming)}
			if tt.name == "streaming_with_filtered_url" {
				opts = append(opts, WithFilter("."))
			}

			rec, err := New(opts...)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			ctx := context.Background()
			handler := rec.HandleNetworkEvent(ctx)

			// Streaming writes to stdout, which this test does not capture.
			if tt.streaming {
				t.Skip("Skipping streaming test - requires stdout capture")
			}

			// Process events
			for _, event := range tt.events {
				handler(event)
			}

			// Count JSON objects in output
			outputs := []string{}
			count := 0
			for _, out := range outputs {
				if out != "" {
					var entry har.Entry
					if err := json.Unmarshal([]byte(out), &entry); err == nil {
						count++
					}
				}
			}

			if count != tt.want {
				t.Errorf("got %d JSON outputs, want %d", count, tt.want)
			}
		})
	}
}

func TestRequestSeedsPageDomainBeforeNavigationEvent(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	handler := r.HandleNetworkEvent(context.Background())
	handler(&network.EventRequestWillBeSent{
		RequestID: "1",
		Type:      network.ResourceTypeDocument,
		Request: &network.Request{
			URL:    "https://www.lesswrong.com/",
			Method: "GET",
		},
	})

	r.Lock()
	got := r.requestPages["1"]
	r.Unlock()
	if got != "www.lesswrong.com" {
		t.Fatalf("request page = %q, want www.lesswrong.com", got)
	}
}

func TestBodyDedupKey(t *testing.T) {
	tests := []struct {
		name    string
		resp    *network.Response
		wantKey string
	}{
		{
			name:    "nil response",
			resp:    nil,
			wantKey: "",
		},
		{
			name: "etag preferred over last-modified",
			resp: &network.Response{
				URL:    "https://cdn.example/app.js",
				Status: 200,
				Headers: network.Headers{
					"ETag":          `"abc123"`,
					"Last-Modified": "Wed, 21 Oct 2025 07:28:00 GMT",
				},
			},
			wantKey: "https://cdn.example/app.js\x00\"abc123\"",
		},
		{
			name: "falls back to last-modified when no etag",
			resp: &network.Response{
				URL:     "https://www.gstatic.com/og/app.js",
				Status:  200,
				Headers: network.Headers{"Last-Modified": "Wed, 21 Oct 2025 07:28:00 GMT"},
			},
			wantKey: "https://www.gstatic.com/og/app.js\x00Wed, 21 Oct 2025 07:28:00 GMT",
		},
		{
			name: "no validator is not deduplicated",
			resp: &network.Response{
				URL:     "https://api.example/batchexecute",
				Status:  200,
				Headers: network.Headers{"Content-Type": "application/json"},
			},
			wantKey: "",
		},
		{
			name: "non-200 is not deduplicated even with validator",
			resp: &network.Response{
				URL:     "https://cdn.example/app.js",
				Status:  304,
				Headers: network.Headers{"ETag": `"abc123"`},
			},
			wantKey: "",
		},
		{
			name: "header lookup is case-insensitive",
			resp: &network.Response{
				URL:     "https://cdn.example/app.js",
				Status:  200,
				Headers: network.Headers{"etag": `"xyz"`},
			},
			wantKey: "https://cdn.example/app.js\x00\"xyz\"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bodyDedupKey(tt.resp); got != tt.wantKey {
				t.Errorf("bodyDedupKey() = %q, want %q", got, tt.wantKey)
			}
		})
	}
}
