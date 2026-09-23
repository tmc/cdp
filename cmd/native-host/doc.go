/*
Native-host is the Chrome native messaging host for the extension in
extension/.

Chrome starts this program itself and speaks to it over stdin and stdout using
the native messaging framing: a little-endian uint32 length followed by that
many bytes of JSON. It is not a command you run directly, and it takes no
flags. Because stdout carries the protocol, diagnostics go to
/tmp/chrome-ai-native-host.log instead.

The host answers three message types:

	ping         Liveness check
	status       Report host status
	ai_request   Return a simulated AI response

# Security

The host runs outside the browser sandbox with the user's privileges. Each
request is checked against a per-principal capability set, rate limited with
a token bucket to bound the damage a runaway or hostile caller can do, and
recorded in an audit log.

Messages are not authenticated. HMAC signing and verification are
implemented in security.go but not applied to incoming messages, so the host
takes no secret.

# Installation

Chrome discovers the host through a manifest naming this binary and the
extension IDs allowed to talk to it, installed in the browser's native
messaging hosts directory. See extension/ for the extension side.
*/
package main
