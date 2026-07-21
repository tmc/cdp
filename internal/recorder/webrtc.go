package recorder

import (
	"fmt"
	"sort"
	"strings"
)

// WebRTCStreams selects which kinds of WebRTC traffic are recorded. WebRTC
// capture rides on injected JavaScript (see WebRTCCaptureScript) that reports
// events via console.log; this set decides which of those events the recorder
// keeps. The zero value captures nothing.
type WebRTCStreams struct {
	SDP         bool // offer/answer signaling (setLocalDescription/setRemoteDescription)
	DataChannel bool // DataChannel messages, both directions
	ICE         bool // ICE candidates
}

// Known WebRTC stream type names accepted by ParseWebRTCStreams.
const (
	webRTCTypeSDP         = "sdp"
	webRTCTypeDataChannel = "datachannel"
	webRTCTypeICE         = "ice"
)

// DefaultWebRTCStreams is the set captured when --full-capture is on and no
// explicit selection is given: signaling and DataChannel messages, but not the
// higher-volume ICE candidate stream.
func DefaultWebRTCStreams() WebRTCStreams {
	return WebRTCStreams{SDP: true, DataChannel: true}
}

// Any reports whether any stream type is enabled. When false, the capture
// script need not be injected at all.
func (s WebRTCStreams) Any() bool {
	return s.SDP || s.DataChannel || s.ICE
}

// String renders the enabled types as a stable comma-separated list, or "none".
func (s WebRTCStreams) String() string {
	var parts []string
	if s.SDP {
		parts = append(parts, webRTCTypeSDP)
	}
	if s.DataChannel {
		parts = append(parts, webRTCTypeDataChannel)
	}
	if s.ICE {
		parts = append(parts, webRTCTypeICE)
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

// ParseWebRTCStreams parses a comma-separated list of WebRTC stream types into
// a WebRTCStreams set. Recognized types are "sdp", "datachannel", and "ice";
// the shorthands "all" and "none" select or clear every type. An empty string
// yields the zero (empty) set; callers that want a default should substitute
// DefaultWebRTCStreams themselves. Whitespace around entries is ignored and
// names are case-insensitive.
func ParseWebRTCStreams(spec string) (WebRTCStreams, error) {
	var s WebRTCStreams
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return s, nil
	}
	for _, raw := range strings.Split(spec, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		switch name {
		case "":
			continue
		case "all":
			s.SDP, s.DataChannel, s.ICE = true, true, true
		case "none":
			s.SDP, s.DataChannel, s.ICE = false, false, false
		case webRTCTypeSDP:
			s.SDP = true
		case webRTCTypeDataChannel:
			s.DataChannel = true
		case webRTCTypeICE:
			s.ICE = true
		default:
			return WebRTCStreams{}, fmt.Errorf("unknown webrtc stream type %q (want %s)", name, knownWebRTCTypes())
		}
	}
	return s, nil
}

func knownWebRTCTypes() string {
	types := []string{webRTCTypeSDP, webRTCTypeDataChannel, webRTCTypeICE, "all", "none"}
	sort.Strings(types)
	return strings.Join(types, ", ")
}
