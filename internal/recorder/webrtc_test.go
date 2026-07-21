package recorder

import "testing"

func TestParseWebRTCStreams(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    WebRTCStreams
		wantErr bool
	}{
		{"empty", "", WebRTCStreams{}, false},
		{"sdp only", "sdp", WebRTCStreams{SDP: true}, false},
		{"datachannel only", "datachannel", WebRTCStreams{DataChannel: true}, false},
		{"ice only", "ice", WebRTCStreams{ICE: true}, false},
		{"default pair", "sdp,datachannel", WebRTCStreams{SDP: true, DataChannel: true}, false},
		{"all types listed", "sdp,datachannel,ice", WebRTCStreams{SDP: true, DataChannel: true, ICE: true}, false},
		{"all shorthand", "all", WebRTCStreams{SDP: true, DataChannel: true, ICE: true}, false},
		{"none shorthand", "none", WebRTCStreams{}, false},
		{"none clears earlier", "sdp,none", WebRTCStreams{}, false},
		{"all then narrows nothing", "none,all", WebRTCStreams{SDP: true, DataChannel: true, ICE: true}, false},
		{"whitespace and case", " SDP , DataChannel ", WebRTCStreams{SDP: true, DataChannel: true}, false},
		{"empty entries ignored", "sdp,,ice,", WebRTCStreams{SDP: true, ICE: true}, false},
		{"unknown type", "sdp,bogus", WebRTCStreams{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseWebRTCStreams(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseWebRTCStreams(%q) err = %v, wantErr %v", tt.spec, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Fatalf("ParseWebRTCStreams(%q) = %+v, want %+v", tt.spec, got, tt.want)
			}
		})
	}
}

func TestWebRTCStreamsString(t *testing.T) {
	tests := []struct {
		s    WebRTCStreams
		want string
	}{
		{WebRTCStreams{}, "none"},
		{WebRTCStreams{SDP: true}, "sdp"},
		{WebRTCStreams{SDP: true, DataChannel: true}, "sdp,datachannel"},
		{WebRTCStreams{SDP: true, DataChannel: true, ICE: true}, "sdp,datachannel,ice"},
		{WebRTCStreams{ICE: true}, "ice"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("%+v.String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestWebRTCStreamsAny(t *testing.T) {
	if (WebRTCStreams{}).Any() {
		t.Error("empty set should report Any() == false")
	}
	if !(WebRTCStreams{ICE: true}).Any() {
		t.Error("ice-only set should report Any() == true")
	}
	if !DefaultWebRTCStreams().Any() {
		t.Error("default set should report Any() == true")
	}
}

func TestDefaultWebRTCStreams(t *testing.T) {
	got := DefaultWebRTCStreams()
	want := WebRTCStreams{SDP: true, DataChannel: true}
	if got != want {
		t.Fatalf("DefaultWebRTCStreams() = %+v, want %+v (ice must be opt-in)", got, want)
	}
}
