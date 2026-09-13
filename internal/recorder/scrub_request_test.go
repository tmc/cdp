package recorder

import (
	"strings"
	"testing"

	"github.com/chromedp/cdproto/har"
	"github.com/tmc/cdp/internal/scrub"
)

// TestScrubEntryRedactsRequestBody covers the credentials that arrive in a
// request body rather than a header: form logins, OAuth token exchanges, and
// API keys posted as JSON.
func TestScrubEntryRedactsRequestBody(t *testing.T) {
	r := &Recorder{scrubber: scrub.New()}

	entry := &har.Entry{
		Request: &har.Request{
			URL: "https://example.com/login",
			PostData: &har.PostData{
				MimeType: "application/x-www-form-urlencoded",
				Text:     "username=alice&password=hunter2",
				Params: []*har.Param{
					{Name: "username", Value: "alice"},
					{Name: "password", Value: "hunter2"},
				},
			},
		},
	}
	r.scrubEntry(entry)

	for _, p := range entry.Request.PostData.Params {
		switch p.Name {
		case "password":
			if p.Value == "hunter2" {
				t.Errorf("PostData.Params[password] not redacted: %q", p.Value)
			}
		case "username":
			if p.Value != "alice" {
				t.Errorf("PostData.Params[username] = %q, want it left alone", p.Value)
			}
		}
	}

	// A body carrying a recognizable secret must not survive verbatim.
	entry = &har.Entry{
		Request: &har.Request{
			URL:      "https://example.com/token",
			PostData: &har.PostData{Text: `{"client_secret":"AKIAIOSFODNN7EXAMPLE"}`},
		},
	}
	r.scrubEntry(entry)
	if strings.Contains(entry.Request.PostData.Text, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("PostData.Text kept the secret: %q", entry.Request.PostData.Text)
	}
}

// TestScrubEntryNilPostData guards the common case: most requests have no body.
func TestScrubEntryNilPostData(t *testing.T) {
	r := &Recorder{scrubber: scrub.New()}
	entry := &har.Entry{Request: &har.Request{URL: "https://example.com/"}}
	r.scrubEntry(entry) // must not panic
}
