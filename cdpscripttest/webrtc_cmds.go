package cdpscripttest

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"rsc.io/script"
)

//go:embed webrtc_helper.js
var webrtcHelperJS string

// WebRTCCmds returns the map of rtc-* commands for WebRTC monitoring.
func WebRTCCmds() map[string]script.Cmd {
	return map[string]script.Cmd{
		"rtc-inject":           RTCInject(),
		"rtc-select":           RTCSelect(),
		"rtc-peers":            RTCPeers(),
		"rtc-state":            RTCState(),
		"rtc-wait":             RTCWait(),
		"rtc-sdp":              RTCSdp(),
		"rtc-ice":              RTCIce(),
		"rtc-events":           RTCEvents(),
		"rtc-tracks":           RTCTracks(),
		"rtc-devices":          RTCDevices(),
		"rtc-stats":            RTCStats(),
		"rtc-stats-video":      RTCStatsVideo(),
		"rtc-stats-audio":      RTCStatsAudio(),
		"rtc-stats-transport":  RTCStatsTransport(),
		"rtc-stats-poll":       RTCStatsPoll(),
		"rtc-mock-screenshare": RTCMockScreenShare(),
	}
}

// requireRTC extracts the cdpState and verifies rtc-inject has been called.
func requireRTC(s *script.State, cmd string) (*State, error) {
	cs, err := cdpState(s)
	if err != nil {
		return nil, err
	}
	if !cs.rtcInjected {
		return nil, fmt.Errorf("%s: monitoring not active (call rtc-inject before navigate)", cmd)
	}
	return cs, nil
}

// rtcEval evaluates a JS expression and unmarshals the result into dst.
func rtcEval(cs *State, expr string, dst any) error {
	var raw json.RawMessage
	if err := chromedp.Run(cs.cdpCtx,
		chromedp.Evaluate(expr, &raw),
	); err != nil {
		return err
	}
	if dst != nil {
		return json.Unmarshal(raw, dst)
	}
	return nil
}

// RTCInject injects the WebRTC monitoring script via
// Page.addScriptToEvaluateOnNewDocument. Must be called before navigate.
//
// Usage: rtc-inject
func RTCInject() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "inject WebRTC monitoring script (call before navigate)",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				var id page.ScriptIdentifier
				if err := chromedp.Run(cs.cdpCtx, chromedp.ActionFunc(func(ctx context.Context) error {
					var err error
					id, err = page.AddScriptToEvaluateOnNewDocument(webrtcHelperJS).Do(ctx)
					return err
				})); err != nil {
					return "", "", fmt.Errorf("rtc-inject: %w", err)
				}
				cs.injectedScripts = append(cs.injectedScripts, id)
				cs.rtcInjected = true
				return "", "", nil
			}, nil
		},
	)
}

// RTCSelect sets the current peer connection ID for subsequent rtc-* commands.
//
// Usage: rtc-select <id>
func RTCSelect() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "set current peer connection by stable ID",
			Args:    "<id>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-select")
			if err != nil {
				return nil, err
			}
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return nil, fmt.Errorf("rtc-select: %w", err)
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				cs.rtcSelectedPeer = id
				return "", "", nil
			}, nil
		},
	)
}

// RTCPeers prints the count and states of all tracked peer connections.
//
// Usage: rtc-peers
func RTCPeers() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "print peer connection count and states",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-peers")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				var peers []struct {
					ID              int    `json:"id"`
					ConnectionState string `json:"connectionState"`
				}
				if err := rtcEval(cs, `window.__cdpst_rtc_getPeers()`, &peers); err != nil {
					return "", "", fmt.Errorf("rtc-peers: %w", err)
				}
				var b strings.Builder
				fmt.Fprintf(&b, "count: %d\n", len(peers))
				for _, p := range peers {
					fmt.Fprintf(&b, "%d %s\n", p.ID, p.ConnectionState)
				}
				return b.String(), "", nil
			}, nil
		},
	)
}

// RTCState prints the connection state of the current peer connection.
//
// Usage: rtc-state
func RTCState() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "print connection state of current peer connection",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-state")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				var state string
				expr := fmt.Sprintf(`window.__cdpst_rtc_getState(%d)`, cs.rtcSelectedPeer)
				if err := rtcEval(cs, expr, &state); err != nil {
					return "", "", fmt.Errorf("rtc-state: %w", err)
				}
				return state + "\n", "", nil
			}, nil
		},
	)
}

// RTCWait waits until any tracked peer connection reaches a target state.
// Polls from Go every 500ms, which handles connection retries gracefully.
// On success, updates the selected peer to the first matching connection.
//
// Usage: rtc-wait <state> [timeout]
func RTCWait() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "wait for peer connection to reach a state",
			Args:    "<state> [timeout]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) < 1 || len(args) > 2 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-wait")
			if err != nil {
				return nil, err
			}
			targetState := args[0]
			timeout := 30 * time.Second
			if len(args) == 2 {
				d, err := time.ParseDuration(args[1])
				if err != nil {
					return nil, fmt.Errorf("rtc-wait: %w", err)
				}
				timeout = d
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				deadline := time.After(timeout)
				ticker := time.NewTicker(500 * time.Millisecond)
				defer ticker.Stop()

				// Returns the stable ID of the first connection in the target state, or -1.
				check := fmt.Sprintf(`(function(){
					var conns = window.__cdpst_rtc_connections;
					if (!conns || conns.size === 0) return -1;
					for (var kv of conns.entries()) {
						if (kv[1].connectionState === %q) return kv[0];
					}
					return -1;
				})()`, targetState)

				for {
					var peerID float64
					if err := chromedp.Run(cs.cdpCtx,
						chromedp.Evaluate(check, &peerID),
					); err != nil {
						return "", "", fmt.Errorf("rtc-wait: %w", err)
					}
					if peerID >= 0 {
						cs.rtcSelectedPeer = int(peerID)
						return "", "", nil
					}
					select {
					case <-ticker.C:
					case <-deadline:
						return "", "", fmt.Errorf("rtc-wait: timeout after %s waiting for state %q", timeout, targetState)
					case <-cs.cdpCtx.Done():
						return "", "", fmt.Errorf("rtc-wait: %w", cs.cdpCtx.Err())
					}
				}
			}, nil
		},
	)
}

// RTCSdp prints the local and remote SDP for the current peer connection.
//
// Usage: rtc-sdp
func RTCSdp() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "print SDP for current peer connection",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-sdp")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				var sdp struct {
					Local  *string `json:"local"`
					Remote *string `json:"remote"`
				}
				expr := fmt.Sprintf(`window.__cdpst_rtc_getSDP(%d)`, cs.rtcSelectedPeer)
				if err := rtcEval(cs, expr, &sdp); err != nil {
					return "", "", fmt.Errorf("rtc-sdp: %w", err)
				}
				var b strings.Builder
				b.WriteString("local:\n")
				if sdp.Local != nil {
					b.WriteString(*sdp.Local)
					if !strings.HasSuffix(*sdp.Local, "\n") {
						b.WriteString("\n")
					}
				}
				b.WriteString("remote:\n")
				if sdp.Remote != nil {
					b.WriteString(*sdp.Remote)
					if !strings.HasSuffix(*sdp.Remote, "\n") {
						b.WriteString("\n")
					}
				}
				return b.String(), "", nil
			}, nil
		},
	)
}

// RTCIce prints the ICE candidates for the current peer connection.
//
// Usage: rtc-ice
func RTCIce() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "print ICE candidates for current peer connection",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-ice")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				var ice struct {
					Local  []string `json:"local"`
					Remote []string `json:"remote"`
				}
				expr := fmt.Sprintf(`window.__cdpst_rtc_getICE(%d)`, cs.rtcSelectedPeer)
				if err := rtcEval(cs, expr, &ice); err != nil {
					return "", "", fmt.Errorf("rtc-ice: %w", err)
				}
				var b strings.Builder
				b.WriteString("local:\n")
				for _, c := range ice.Local {
					b.WriteString(c + "\n")
				}
				b.WriteString("remote:\n")
				for _, c := range ice.Remote {
					b.WriteString(c + "\n")
				}
				return b.String(), "", nil
			}, nil
		},
	)
}

// RTCEvents prints the event log for the current peer connection.
//
// Usage: rtc-events
func RTCEvents() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "print event log for current peer connection",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-events")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				var events []struct {
					ID        int    `json:"id"`
					Type      string `json:"type"`
					State     string `json:"state"`
					Timestamp int64  `json:"timestamp"`
				}
				expr := fmt.Sprintf(`window.__cdpst_rtc_getEvents(%d)`, cs.rtcSelectedPeer)
				if err := rtcEval(cs, expr, &events); err != nil {
					return "", "", fmt.Errorf("rtc-events: %w", err)
				}
				var b strings.Builder
				for _, e := range events {
					fmt.Fprintf(&b, "%d %s %s\n", e.Timestamp, e.Type, e.State)
				}
				return b.String(), "", nil
			}, nil
		},
	)
}

// RTCTracks prints the active tracks for the current peer connection.
//
// Usage: rtc-tracks
func RTCTracks() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "print active tracks for current peer connection",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-tracks")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				var tracks []struct {
					Direction  string `json:"direction"`
					Kind       string `json:"kind"`
					ReadyState string `json:"readyState"`
				}
				expr := fmt.Sprintf(`window.__cdpst_rtc_getTracks(%d)`, cs.rtcSelectedPeer)
				if err := rtcEval(cs, expr, &tracks); err != nil {
					return "", "", fmt.Errorf("rtc-tracks: %w", err)
				}
				var b strings.Builder
				for _, t := range tracks {
					fmt.Fprintf(&b, "%s %s: %s\n", t.Direction, t.Kind, t.ReadyState)
				}
				return b.String(), "", nil
			}, nil
		},
	)
}

// RTCDevices prints available media devices from enumerateDevices.
//
// Usage: rtc-devices
func RTCDevices() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "print available media devices",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				var devices []struct {
					Kind  string `json:"kind"`
					Label string `json:"label"`
				}
				expr := `navigator.mediaDevices.enumerateDevices().then(function(d) {
					return d.map(function(dev) {
						return {kind: dev.kind, label: dev.label};
					});
				})`
				var raw json.RawMessage
				if err := chromedp.Run(cs.cdpCtx,
					chromedp.Evaluate(expr, &raw, evalAwaitPromise),
				); err != nil {
					return "", "", fmt.Errorf("rtc-devices: %w", err)
				}
				if err := json.Unmarshal(raw, &devices); err != nil {
					return "", "", fmt.Errorf("rtc-devices: %w", err)
				}
				var b strings.Builder
				for _, d := range devices {
					fmt.Fprintf(&b, "%s: %s\n", d.Kind, d.Label)
				}
				return b.String(), "", nil
			}, nil
		},
	)
}

// RTCStats prints all cached stats for the current peer connection as key: value lines.
//
// Usage: rtc-stats
func RTCStats() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "print all stats for current peer connection",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-stats")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				expr := fmt.Sprintf(`window.__cdpst_rtc_getStats(%d)`, cs.rtcSelectedPeer)
				return formatStats(cs, expr, "rtc-stats")
			}, nil
		},
	)
}

// RTCStatsVideo prints video RTP stats for the current peer connection.
//
// Usage: rtc-stats-video [--direction inbound|outbound]
func RTCStatsVideo() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "print video RTP stats for current peer connection",
			Args:    "[--direction inbound|outbound]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			direction := ""
			for len(args) >= 2 && args[0] == "--direction" {
				direction = args[1]
				args = args[2:]
			}
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-stats-video")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				if direction != "" {
					expr := fmt.Sprintf(`window.__cdpst_rtc_getStatsVideo(%d, %q)`,
						cs.rtcSelectedPeer, direction)
					return formatStatsKV(cs, expr, "rtc-stats-video")
				}
				var b strings.Builder
				for _, dir := range []string{"inbound", "outbound"} {
					expr := fmt.Sprintf(`window.__cdpst_rtc_getStatsVideo(%d, %q)`,
						cs.rtcSelectedPeer, dir)
					out, _, err := formatStatsKV(cs, expr, "rtc-stats-video")
					if err != nil {
						return "", "", err
					}
					if out != "" {
						b.WriteString(out)
					}
				}
				return b.String(), "", nil
			}, nil
		},
	)
}

// RTCStatsAudio prints audio RTP stats for the current peer connection.
//
// Usage: rtc-stats-audio [--direction inbound|outbound]
func RTCStatsAudio() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "print audio RTP stats for current peer connection",
			Args:    "[--direction inbound|outbound]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			direction := ""
			for len(args) >= 2 && args[0] == "--direction" {
				direction = args[1]
				args = args[2:]
			}
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-stats-audio")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				if direction != "" {
					expr := fmt.Sprintf(`window.__cdpst_rtc_getStatsAudio(%d, %q)`,
						cs.rtcSelectedPeer, direction)
					return formatStatsKV(cs, expr, "rtc-stats-audio")
				}
				var b strings.Builder
				for _, dir := range []string{"inbound", "outbound"} {
					expr := fmt.Sprintf(`window.__cdpst_rtc_getStatsAudio(%d, %q)`,
						cs.rtcSelectedPeer, dir)
					out, _, err := formatStatsKV(cs, expr, "rtc-stats-audio")
					if err != nil {
						return "", "", err
					}
					if out != "" {
						b.WriteString(out)
					}
				}
				return b.String(), "", nil
			}, nil
		},
	)
}

// RTCStatsTransport prints transport and candidate-pair stats.
//
// Usage: rtc-stats-transport
func RTCStatsTransport() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "print transport and candidate-pair stats",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-stats-transport")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				expr := fmt.Sprintf(`window.__cdpst_rtc_getStatsTransport(%d)`, cs.rtcSelectedPeer)
				return formatStatsKV(cs, expr, "rtc-stats-transport")
			}, nil
		},
	)
}

// RTCStatsPoll polls getStats() over a duration and prints min/max/avg summary.
//
// Usage: rtc-stats-poll --duration <dur> --interval <interval>
func RTCStatsPoll() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "poll stats over a duration and print summary",
			Args:    "--duration <dur> --interval <interval>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			var duration, interval time.Duration
			for len(args) >= 2 {
				switch args[0] {
				case "--duration":
					d, err := time.ParseDuration(args[1])
					if err != nil {
						return nil, fmt.Errorf("rtc-stats-poll: --duration: %w", err)
					}
					duration = d
					args = args[2:]
				case "--interval":
					d, err := time.ParseDuration(args[1])
					if err != nil {
						return nil, fmt.Errorf("rtc-stats-poll: --interval: %w", err)
					}
					interval = d
					args = args[2:]
				default:
					return nil, script.ErrUsage
				}
			}
			if duration == 0 || interval == 0 {
				return nil, fmt.Errorf("rtc-stats-poll: --duration and --interval required")
			}
			cs, err := requireRTC(s, "rtc-stats-poll")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				return pollStats(cs, duration, interval)
			}, nil
		},
	)
}

// RTCMockScreenShare injects a getDisplayMedia override that returns
// a canvas.captureStream() with a test pattern.
//
// Usage: rtc-mock-screenshare [--width W] [--height H] [--fps F]
func RTCMockScreenShare() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "mock getDisplayMedia with a canvas test pattern",
			Args:    "[--width W] [--height H] [--fps F]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			width, height, fps := 1280, 720, 30
			for len(args) >= 2 {
				switch args[0] {
				case "--width":
					v, err := strconv.Atoi(args[1])
					if err != nil {
						return nil, fmt.Errorf("rtc-mock-screenshare: --width: %w", err)
					}
					width = v
					args = args[2:]
				case "--height":
					v, err := strconv.Atoi(args[1])
					if err != nil {
						return nil, fmt.Errorf("rtc-mock-screenshare: --height: %w", err)
					}
					height = v
					args = args[2:]
				case "--fps":
					v, err := strconv.Atoi(args[1])
					if err != nil {
						return nil, fmt.Errorf("rtc-mock-screenshare: --fps: %w", err)
					}
					fps = v
					args = args[2:]
				default:
					return nil, script.ErrUsage
				}
			}
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := requireRTC(s, "rtc-mock-screenshare")
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				// Inject via addScriptToEvaluateOnNewDocument so the mock is
				// set up on every new document, before page JS calls getDisplayMedia.
				mockJS := fmt.Sprintf(`if (window.__cdpst_rtc_mockScreenShare) { window.__cdpst_rtc_mockScreenShare(%d, %d, %d); }`, width, height, fps)
				var id page.ScriptIdentifier
				if err := chromedp.Run(cs.cdpCtx, chromedp.ActionFunc(func(ctx context.Context) error {
					var err error
					id, err = page.AddScriptToEvaluateOnNewDocument(mockJS).Do(ctx)
					return err
				})); err != nil {
					return "", "", fmt.Errorf("rtc-mock-screenshare: %w", err)
				}
				cs.injectedScripts = append(cs.injectedScripts, id)
				return "", "", nil
			}, nil
		},
	)
}

// formatStats evaluates a JS expression that returns an array of stat objects,
// and formats each with a type header and indented key: value lines.
func formatStats(cs *State, expr, cmdName string) (string, string, error) {
	var stats []map[string]any
	if err := rtcEval(cs, expr, &stats); err != nil {
		return "", "", fmt.Errorf("%s: %w", cmdName, err)
	}
	var b strings.Builder
	for _, stat := range stats {
		typ, _ := stat["type"].(string)
		fmt.Fprintf(&b, "type: %s\n", typ)
		keys := make([]string, 0, len(stat))
		for k := range stat {
			if k == "type" {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "  %s: %v\n", k, stat[k])
		}
	}
	return b.String(), "", nil
}

// formatStatsKV evaluates a JS expression that returns an array of stat objects,
// and formats each field as key: value lines (no grouping header).
func formatStatsKV(cs *State, expr, cmdName string) (string, string, error) {
	var stats []map[string]any
	if err := rtcEval(cs, expr, &stats); err != nil {
		return "", "", fmt.Errorf("%s: %w", cmdName, err)
	}
	var b strings.Builder
	for _, stat := range stats {
		keys := make([]string, 0, len(stat))
		for k := range stat {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "%s: %v\n", k, stat[k])
		}
	}
	return b.String(), "", nil
}

// pollStats collects getStats snapshots over duration at interval,
// then computes min/max/avg for key numeric metrics.
func pollStats(cs *State, duration, interval time.Duration) (string, string, error) {
	type sample struct {
		rtt        float64
		packetLoss float64
		bitrate    float64
	}

	var samples []sample
	deadline := time.Now().Add(duration)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ticker.C:
		case <-cs.cdpCtx.Done():
			return "", "", cs.cdpCtx.Err()
		}

		var transport []map[string]any
		expr := fmt.Sprintf(`window.__cdpst_rtc_getStatsTransport(%d)`, cs.rtcSelectedPeer)
		if err := rtcEval(cs, expr, &transport); err != nil {
			continue
		}
		var s sample
		for _, t := range transport {
			if v, ok := t["currentRoundTripTime"]; ok {
				if f, ok := v.(float64); ok {
					s.rtt = f
				}
			}
			if v, ok := t["availableOutgoingBitrate"]; ok {
				if f, ok := v.(float64); ok {
					s.bitrate = f
				}
			}
		}

		var inbound []map[string]any
		expr = fmt.Sprintf(`window.__cdpst_rtc_getStatsVideo(%d, "inbound")`, cs.rtcSelectedPeer)
		if err := rtcEval(cs, expr, &inbound); err == nil {
			for _, st := range inbound {
				if v, ok := st["packetsLost"]; ok {
					if f, ok := v.(float64); ok {
						s.packetLoss = f
					}
				}
			}
		}

		samples = append(samples, s)
	}

	if len(samples) == 0 {
		return "samples: 0\n", "", nil
	}

	var sumRtt, sumLoss, sumBitrate float64
	minRtt, maxRtt := math.MaxFloat64, 0.0
	minLoss, maxLoss := math.MaxFloat64, 0.0
	minBitrate, maxBitrate := math.MaxFloat64, 0.0

	for _, s := range samples {
		sumRtt += s.rtt
		sumLoss += s.packetLoss
		sumBitrate += s.bitrate
		if s.rtt < minRtt {
			minRtt = s.rtt
		}
		if s.rtt > maxRtt {
			maxRtt = s.rtt
		}
		if s.packetLoss < minLoss {
			minLoss = s.packetLoss
		}
		if s.packetLoss > maxLoss {
			maxLoss = s.packetLoss
		}
		if s.bitrate < minBitrate {
			minBitrate = s.bitrate
		}
		if s.bitrate > maxBitrate {
			maxBitrate = s.bitrate
		}
	}

	n := float64(len(samples))
	var b strings.Builder
	fmt.Fprintf(&b, "samples: %d\n", len(samples))
	fmt.Fprintf(&b, "avgRtt: %.4f\n", sumRtt/n)
	fmt.Fprintf(&b, "minRtt: %.4f\n", minRtt)
	fmt.Fprintf(&b, "maxRtt: %.4f\n", maxRtt)
	fmt.Fprintf(&b, "avgPacketLoss: %.0f\n", sumLoss/n)
	fmt.Fprintf(&b, "minPacketLoss: %.0f\n", minLoss)
	fmt.Fprintf(&b, "maxPacketLoss: %.0f\n", maxLoss)
	fmt.Fprintf(&b, "avgBitrate: %.0f\n", sumBitrate/n)
	fmt.Fprintf(&b, "minBitrate: %.0f\n", minBitrate)
	fmt.Fprintf(&b, "maxBitrate: %.0f\n", maxBitrate)
	return b.String(), "", nil
}
