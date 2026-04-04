package sourcemap

import (
	"encoding/json"
	"sort"
	"strings"
)

// Mapping represents a single sourcemap segment.
// All fields are 0-based.
type Mapping struct {
	GeneratedLine int // 0-based line in the generated file
	GeneratedCol  int // 0-based column in the generated file
	SourceIdx     int // index into the "sources" array
	OriginalLine  int // 0-based line in the original source
	OriginalCol   int // 0-based column in the original source
	NameIdx       int // index into "names" array, or -1 for no name
}

// sourceMapV3 is the JSON structure for a sourcemap version 3.
type sourceMapV3 struct {
	Version        int      `json:"version"`
	File           string   `json:"file,omitempty"`
	SourceRoot     string   `json:"sourceRoot,omitempty"`
	Sources        []string `json:"sources"`
	SourcesContent []string `json:"sourcesContent,omitempty"`
	Names          []string `json:"names,omitempty"`
	Mappings       string   `json:"mappings"`
}

// GenerateV3 produces a valid sourcemap v3 JSON document.
// mappings must be sorted by (GeneratedLine, GeneratedCol).
func GenerateV3(file string, sources, sourcesContent []string, mappings []Mapping, names []string) ([]byte, error) {
	sm := sourceMapV3{
		Version:        3,
		File:           file,
		Sources:        sources,
		SourcesContent: sourcesContent,
		Names:          names,
		Mappings:       encodeMappings(mappings, names),
	}
	return json.Marshal(sm)
}

// encodeMappings encodes a sorted list of mappings into the VLQ-encoded
// "mappings" string per the sourcemap v3 spec.
//
// Format: semicolons separate lines, commas separate segments within a line.
// Each segment is 4 or 5 VLQ-encoded fields (relative to previous segment):
//
//	[generatedCol, sourceIdx, originalLine, originalCol, nameIdx?]
//
// All fields except generatedCol are relative to the previous occurrence.
func encodeMappings(mappings []Mapping, names []string) string {
	if len(mappings) == 0 {
		return ""
	}

	// Sort by generated position.
	sort.Slice(mappings, func(i, j int) bool {
		if mappings[i].GeneratedLine != mappings[j].GeneratedLine {
			return mappings[i].GeneratedLine < mappings[j].GeneratedLine
		}
		return mappings[i].GeneratedCol < mappings[j].GeneratedCol
	})

	var b strings.Builder
	prevGenLine := 0
	prevGenCol := 0
	prevSourceIdx := 0
	prevOrigLine := 0
	prevOrigCol := 0
	prevNameIdx := 0

	for i, m := range mappings {
		// Emit semicolons for skipped lines.
		for prevGenLine < m.GeneratedLine {
			b.WriteByte(';')
			prevGenLine++
			prevGenCol = 0 // reset column at line boundary
		}

		if i > 0 && mappings[i-1].GeneratedLine == m.GeneratedLine {
			b.WriteByte(',')
		}

		// Field 1: generated column (relative to previous in this line).
		b.WriteString(EncodeVLQ(m.GeneratedCol - prevGenCol))
		prevGenCol = m.GeneratedCol

		// Field 2: source index (relative).
		b.WriteString(EncodeVLQ(m.SourceIdx - prevSourceIdx))
		prevSourceIdx = m.SourceIdx

		// Field 3: original line (relative).
		b.WriteString(EncodeVLQ(m.OriginalLine - prevOrigLine))
		prevOrigLine = m.OriginalLine

		// Field 4: original column (relative).
		b.WriteString(EncodeVLQ(m.OriginalCol - prevOrigCol))
		prevOrigCol = m.OriginalCol

		// Field 5: name index (optional, relative).
		if m.NameIdx >= 0 && len(names) > 0 {
			b.WriteString(EncodeVLQ(m.NameIdx - prevNameIdx))
			prevNameIdx = m.NameIdx
		}
	}

	return b.String()
}

// base64VLQ is the alphabet used for VLQ encoding in sourcemaps.
const base64VLQ = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// EncodeVLQ encodes a signed integer as a base64-VLQ string.
//
// The VLQ encoding: the value is converted to a sign-magnitude form
// where the least significant bit is the sign (0=positive, 1=negative).
// Then it's split into 5-bit groups, each encoded as a base64 character.
// The continuation bit (bit 5) is set on all groups except the last.
func EncodeVLQ(value int) string {
	// Convert to sign-magnitude VLQ representation.
	var vlq int
	if value < 0 {
		vlq = ((-value) << 1) | 1
	} else {
		vlq = value << 1
	}

	var result []byte
	for {
		digit := vlq & 0x1f // low 5 bits
		vlq >>= 5
		if vlq > 0 {
			digit |= 0x20 // set continuation bit
		}
		result = append(result, base64VLQ[digit])
		if vlq == 0 {
			break
		}
	}
	return string(result)
}

// DecodeVLQ decodes a base64-VLQ string, returning the value and
// the number of bytes consumed.
func DecodeVLQ(s string) (value int, consumed int) {
	var shift uint
	var vlq int
	for i := 0; i < len(s); i++ {
		idx := strings.IndexByte(base64VLQ, s[i])
		if idx < 0 {
			break
		}
		consumed = i + 1
		vlq |= (idx & 0x1f) << shift
		shift += 5
		if idx&0x20 == 0 {
			// No continuation bit — done.
			break
		}
	}
	// Convert from sign-magnitude.
	if vlq&1 == 1 {
		value = -(vlq >> 1)
	} else {
		value = vlq >> 1
	}
	return value, consumed
}

// BuildIdentityMappings creates 1:1 line mappings from a generated file
// back to a source file. Each line in the generated file maps to the
// corresponding line in the source. Useful for simple transformations
// where line numbers are preserved.
func BuildIdentityMappings(sourceIdx, lineCount int) []Mapping {
	mappings := make([]Mapping, lineCount)
	for i := 0; i < lineCount; i++ {
		mappings[i] = Mapping{
			GeneratedLine: i,
			GeneratedCol:  0,
			SourceIdx:     sourceIdx,
			OriginalLine:  i,
			OriginalCol:   0,
			NameIdx:       -1,
		}
	}
	return mappings
}

// BuildChunkMappings creates mappings from extracted code chunks back to
// their positions in the original bundle. Each chunk maps lines in the
// generated (extracted) output to the corresponding lines in the original
// bundle source.
//
// generatedOffset is the starting line (0-based) in the generated file
// where this chunk's content begins.
func BuildChunkMappings(sourceIdx, generatedOffset int, chunk CodeChunk) []Mapping {
	lines := CountLinesInString(chunk.Code)
	mappings := make([]Mapping, lines)
	for i := 0; i < lines; i++ {
		mappings[i] = Mapping{
			GeneratedLine: generatedOffset + i,
			GeneratedCol:  0,
			SourceIdx:     sourceIdx,
			OriginalLine:  chunk.StartLine - 1 + i, // convert 1-based to 0-based
			OriginalCol:   0,
			NameIdx:       -1,
		}
	}
	return mappings
}
