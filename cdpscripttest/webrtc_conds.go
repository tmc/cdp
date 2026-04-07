package cdpscripttest

import (
	"fmt"

	"github.com/chromedp/chromedp"
	"rsc.io/script"
)

// WebRTCConds returns conditions for WebRTC state inspection.
func WebRTCConds() map[string]script.Cond {
	return map[string]script.Cond{
		"rtc":       rtcCond(),
		"rtc-state": rtcStateCond(),
		"rtc-dc":    rtcDCCond(),
		"rtc-codec": rtcCodecCond(),
	}
}

// rtcCond returns a boolean condition that is true when the WebRTC monitoring
// script has been injected and the page has loaded (window.__cdpst_rtc_connections exists).
func rtcCond() script.Cond {
	return script.Condition(
		"WebRTC monitoring is active",
		func(s *script.State) (bool, error) {
			cs, err := cdpState(s)
			if err != nil {
				return false, nil
			}
			var found bool
			err = chromedp.Run(cs.cdpCtx,
				chromedp.Evaluate(`typeof window.__cdpst_rtc_connections !== 'undefined'`, &found),
			)
			if err != nil {
				return false, nil
			}
			return found, nil
		},
	)
}

// rtcStateCond returns a prefix condition that compares the current peer
// connection's state against the suffix. For example, [rtc-state:connected]
// is true when the selected peer is in state "connected".
func rtcStateCond() script.Cond {
	return script.PrefixCondition(
		"selected peer connection is in <state>",
		func(s *script.State, want string) (bool, error) {
			cs, err := cdpState(s)
			if err != nil {
				return false, nil
			}
			js := fmt.Sprintf(`(function(){
				if (typeof window.__cdpst_rtc_getState !== 'function') return '';
				return window.__cdpst_rtc_getState(%d);
			})()`, cs.rtcSelectedPeer)
			var got string
			err = chromedp.Run(cs.cdpCtx,
				chromedp.Evaluate(js, &got),
			)
			if err != nil {
				return false, nil
			}
			return got == want, nil
		},
	)
}

// rtcDCCond returns a prefix condition that checks whether a data channel with
// the given label exists on any tracked connection. For example, [rtc-dc:chat]
// is true when a data channel labeled "chat" exists.
func rtcDCCond() script.Cond {
	return script.PrefixCondition(
		"data channel with <label> exists on a tracked connection",
		func(s *script.State, label string) (bool, error) {
			cs, err := cdpState(s)
			if err != nil {
				return false, nil
			}
			js := fmt.Sprintf(`(function(){
				if (typeof window.__cdpst_rtc_connections === 'undefined') return false;
				var label = %q;
				var conns = window.__cdpst_rtc_connections;
				for (var pc of conns.values()) {
					// Check tracked data channels from helper script.
					var dcs = window.__cdpst_rtc_datachannels;
					if (dcs) {
						var pcDCs = dcs.get(pc.__cdpst_rtc_id);
						if (pcDCs) {
							for (var dc of pcDCs.values()) {
								if (label === '' || dc.label === label) return true;
							}
						}
					}
					// Also check via getStats for data-channel stat type.
					var stats = window.__cdpst_rtc_stats;
					if (stats) {
						var pcStats = stats.get(pc.__cdpst_rtc_id);
						if (pcStats) {
							for (var i = 0; i < pcStats.length; i++) {
								if (pcStats[i].type === 'data-channel') {
									if (label === '' || pcStats[i].label === label) return true;
								}
							}
						}
					}
				}
				return false;
			})()`, label)
			var found bool
			err = chromedp.Run(cs.cdpCtx,
				chromedp.Evaluate(js, &found),
			)
			if err != nil {
				return false, nil
			}
			return found, nil
		},
	)
}

// rtcCodecCond returns a prefix condition that checks whether a named codec
// appears in the cached stats. For example, [rtc-codec:VP9] is true when VP9
// is in active use.
func rtcCodecCond() script.Cond {
	return script.PrefixCondition(
		"codec <name> appears in cached WebRTC stats",
		func(s *script.State, name string) (bool, error) {
			cs, err := cdpState(s)
			if err != nil {
				return false, nil
			}
			js := fmt.Sprintf(`(function(){
				if (typeof window.__cdpst_rtc_stats === 'undefined') return false;
				var name = %q.toLowerCase();
				var stats = window.__cdpst_rtc_stats;
				for (var arr of stats.values()) {
					for (var i = 0; i < arr.length; i++) {
						if (arr[i].type === 'codec') {
							var mime = (arr[i].mimeType || '').toLowerCase();
							if (mime.indexOf(name) !== -1) return true;
						}
					}
				}
				return false;
			})()`, name)
			var found bool
			err = chromedp.Run(cs.cdpCtx,
				chromedp.Evaluate(js, &found),
			)
			if err != nil {
				return false, nil
			}
			return found, nil
		},
	)
}
