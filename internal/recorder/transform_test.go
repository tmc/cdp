package recorder

import (
	"context"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/chromedp/cdproto/har"
	"github.com/chromedp/cdproto/network"
)

// record feeds r one complete request/response/finished triple per URL.
func record(t *testing.T, r *Recorder, urls ...string) {
	t.Helper()

	handler := r.HandleNetworkEvent(context.Background())
	for i, url := range urls {
		id := network.RequestID(strconv.Itoa(i))
		handler(&network.EventRequestWillBeSent{
			RequestID: id,
			Request:   &network.Request{URL: url, Method: "GET"},
		})
		handler(&network.EventResponseReceived{
			RequestID: id,
			Response:  &network.Response{URL: url, Status: 200},
		})
		handler(&network.EventLoadingFinished{
			RequestID: id,
			Timestamp: timeToMonotonicTime(time.Now()),
		})
	}
}

func entryURLs(h *har.HAR) []string {
	var urls []string
	for _, e := range h.Log.Entries {
		if e.Request != nil {
			urls = append(urls, e.Request.URL)
		}
	}
	sort.Strings(urls)
	return urls
}

// TestHARAppliesFilter checks that -filter and -template mean the same thing
// for the assembled HAR as they do for the streamed entries. They used to
// apply only while streaming, so writing a HAR file silently ignored them.
func TestHARAppliesFilter(t *testing.T) {
	const (
		api  = "https://example.com/api/data"
		page = "https://example.com/index.html"
	)
	tests := []struct {
		name string
		opts []Option
		want []string
	}{
		{
			name: "no filter",
			want: []string{api, page},
		},
		{
			name: "filter keeps matching entries",
			opts: []Option{WithFilter(`select(.request.url | test("/api/"))`)},
			want: []string{api},
		},
		{
			name: "filter drops every entry",
			opts: []Option{WithFilter(`select(.request.url | test("/nothing/"))`)},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(tt.opts...)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			defer r.Close()

			record(t, r, api, page)

			h, err := r.HAR()
			if err != nil {
				t.Fatalf("HAR() error = %v", err)
			}
			got := entryURLs(h)
			if len(got) != len(tt.want) {
				t.Fatalf("HAR entries = %v, want %v", got, tt.want)
			}
			for i, url := range got {
				if url != tt.want[i] {
					t.Fatalf("HAR entries = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestHARAppliesTemplate(t *testing.T) {
	r, err := New(WithTemplate("{{.Request.Method}} {{.Request.URL}}"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	record(t, r, "https://example.com/api/data")

	h, err := r.HAR()
	if err != nil {
		t.Fatalf("HAR() error = %v", err)
	}
	if len(h.Log.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(h.Log.Entries))
	}
	got := h.Log.Entries[0].Response.Content.Text
	if want := "GET https://example.com/api/data"; got != want {
		t.Errorf("templated entry = %q, want %q", got, want)
	}
}
