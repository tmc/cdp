package browser

import (
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
	return raw[:comma+1] + escapeDataURL(data)
}

// dataURLUnsafe lists the ASCII bytes that break or truncate a data: URL
// payload when left unescaped.
const dataURLUnsafe = " \t\r\n#<>\"'`{}|\\^[]"

func needsDataURLEscape(data string) bool {
	return strings.ContainsAny(data, dataURLUnsafe)
}

// escapeDataURL percent-encodes the unsafe, control, and non-ASCII bytes of
// data. Existing %XX escapes are left alone so they are not encoded twice.
func escapeDataURL(data string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(data); i++ {
		c := data[i]
		if c < 0x20 || c >= 0x7f || strings.IndexByte(dataURLUnsafe, c) >= 0 {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xf])
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
