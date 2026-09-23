package recorder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// captureFileFor runs a set of injected WebRTC console payloads through a
// recorder configured with the given streams and returns the contents of the
// _capture.jsonl file (empty string if none was written).
func captureFileFor(t *testing.T, streams WebRTCStreams, payloads ...string) string {
	t.Helper()
	dir := t.TempDir()
	r, err := New(WithOutputDir(dir), WithWebRTCStreams(streams))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, p := range payloads {
		r.handleDataChannelCapture(p)
	}
	data, err := os.ReadFile(filepath.Join(dir, "_capture.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(data)
}

func TestWebRTCCaptureGating(t *testing.T) {
	const (
		sdp  = `{"type":"sdp-local","sdpType":"offer","sdp":"v=0 SDP-MARKER"}`
		dc   = `{"type":"message","label":"chat","dir":"outgoing","data":"DC-MARKER"}`
		ice  = `{"type":"ice-local","candidate":"candidate ICE-MARKER","sdpMid":"0"}`
		none = ``
	)

	tests := []struct {
		name    string
		streams WebRTCStreams
		want    []string // markers that must appear
		absent  []string // markers that must not appear
	}{
		{
			name:    "default keeps sdp and datachannel, drops ice",
			streams: DefaultWebRTCStreams(),
			want:    []string{"SDP-MARKER", "DC-MARKER"},
			absent:  []string{"ICE-MARKER"},
		},
		{
			name:    "sdp only",
			streams: WebRTCStreams{SDP: true},
			want:    []string{"SDP-MARKER"},
			absent:  []string{"DC-MARKER", "ICE-MARKER"},
		},
		{
			name:    "datachannel only",
			streams: WebRTCStreams{DataChannel: true},
			want:    []string{"DC-MARKER"},
			absent:  []string{"SDP-MARKER", "ICE-MARKER"},
		},
		{
			name:    "all types",
			streams: WebRTCStreams{SDP: true, DataChannel: true, ICE: true},
			want:    []string{"SDP-MARKER", "DC-MARKER", "ICE-MARKER"},
		},
		{
			name:    "none drops everything",
			streams: WebRTCStreams{},
			absent:  []string{"SDP-MARKER", "DC-MARKER", "ICE-MARKER"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := captureFileFor(t, tt.streams, sdp, dc, ice)
			for _, m := range tt.want {
				if !strings.Contains(got, m) {
					t.Errorf("capture missing %q\ngot:\n%s", m, got)
				}
			}
			for _, m := range tt.absent {
				if strings.Contains(got, m) {
					t.Errorf("capture should not contain %q\ngot:\n%s", m, got)
				}
			}
		})
	}
}

// TestStreamingCaptureDoesNotDeadlock checks that injected gRPC-Web and WebRTC
// captures stream entries without re-acquiring the recorder lock the capture
// handlers already hold.
func TestStreamingCaptureDoesNotDeadlock(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		handle  func(*Recorder, string)
	}{
		{"grpc complete", `{"type":"complete","url":"https://api.example.com/rpc","method":"POST","status":200,"full":"GRPC-MARKER"}`, (*Recorder).handleGRPCCapture},
		{"sdp", `{"type":"sdp-local","sdpType":"offer","sdp":"v=0 SDP-MARKER"}`, (*Recorder).handleDataChannelCapture},
		{"datachannel", `{"type":"message","label":"chat","dir":"outgoing","data":"DC-MARKER"}`, (*Recorder).handleDataChannelCapture},
		{"ice", `{"type":"ice-local","candidate":"candidate ICE-MARKER","sdpMid":"0"}`, (*Recorder).handleDataChannelCapture},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(WithStreaming(true), WithOutputDir(t.TempDir()),
				WithWebRTCStreams(WebRTCStreams{SDP: true, DataChannel: true, ICE: true}))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			done := make(chan struct{})
			go func() {
				tt.handle(r, tt.payload)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("capture handler deadlocked in streaming mode")
			}
		})
	}
}
