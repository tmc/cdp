package browser

import (
	"net/url"
	"strings"
)

func normalizeNavigateURL(raw string) string {
	if !strings.HasPrefix(strings.ToLower(raw), "data:") {
		return raw
	}

	comma := strings.IndexByte(raw, ',')
	if comma < 0 {
		return raw
	}

	mediaType := strings.ToLower(raw[len("data:"):comma])
	if !strings.HasPrefix(mediaType, "text/html") || strings.Contains(mediaType, ";base64") {
		return raw
	}

	data := raw[comma+1:]
	if !needsDataURLEscape(data) {
		return raw
	}
	return raw[:comma+1] + url.PathEscape(data)
}

func needsDataURLEscape(data string) bool {
	return strings.ContainsAny(data, " \t\r\n#<>\"'`{}|\\^[]")
}
