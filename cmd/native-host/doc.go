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
	ai_request   Perform an AI operation on behalf of the extension

# Security

The host runs outside the browser sandbox with the user's privileges, so it
authenticates what the extension asks of it rather than trusting the channel.
Requests are authenticated with HMAC, checked against a per-principal
capability set, rate limited with a token bucket to bound the damage a runaway
or hostile caller can do, and recorded in an audit log.

The HMAC secret comes from NATIVE_HOST_HMAC_SECRET, which the extension sets at
install time. If that variable is unset the host falls back to a well-known
development secret and logs a warning; it must be set for real use, as the
fallback authenticates nobody.

# Installation

Chrome discovers the host through a manifest naming this binary and the
extension IDs allowed to talk to it, installed in the browser's native
messaging hosts directory. See extension/ for the extension side.
*/
package main
