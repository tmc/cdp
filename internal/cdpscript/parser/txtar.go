package parser

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// TxtarFile represents a file in a txtar archive.
type TxtarFile struct {
	Name string
	Data []byte
}

// TxtarArchive represents a parsed txtar archive.
type TxtarArchive struct {
	Comment string
	Files   []TxtarFile
}

// ParseTxtar parses a txtar-format file.
// Format:
//   comment text
//   -- filename1 --
//   file1 contents
//   -- filename2 --
//   file2 contents
func ParseTxtar(data []byte) (*TxtarArchive, error) {
	archive := &TxtarArchive{}

	// Split into sections
	marker := []byte("\n-- ")
	sections := bytes.Split(data, marker)

	// First section is the comment (before first file)
	if len(sections) > 0 {
		archive.Comment = string(bytes.TrimSpace(sections[0]))
	}

	// Parse remaining sections as files
	for i := 1; i < len(sections); i++ {
		section := sections[i]

		// Find end of filename (marked by " --\n")
		endMarker := []byte(" --\n")
		idx := bytes.Index(section, endMarker)
		if idx == -1 {
			return nil, fmt.Errorf("invalid txtar format: missing end marker for file %d", i)
		}

		filename := string(bytes.TrimSpace(section[:idx]))
		// Contents start right after the end marker
		// Keep trailing newlines as they are part of the content
		contents := section[idx+len(endMarker):]
		// But trim any trailing newline that comes from the next section marker
		// Only if this isn't the last file
		if i < len(sections)-1 && len(contents) > 0 && contents[len(contents)-1] == '\n' {
			contents = contents[:len(contents)-1]
		}

		archive.Files = append(archive.Files, TxtarFile{
			Name: filename,
			Data: contents,
		})
	}

	return archive, nil
}

// Format writes a txtar archive to the given writer.
func (a *TxtarArchive) Format(w io.Writer) error {
	if a.Comment != "" {
		if _, err := fmt.Fprintf(w, "%s\n", a.Comment); err != nil {
			return err
		}
	}

	for _, f := range a.Files {
		if _, err := fmt.Fprintf(w, "-- %s --\n", f.Name); err != nil {
			return err
		}
		if _, err := w.Write(f.Data); err != nil {
			return err
		}
		if !bytes.HasSuffix(f.Data, []byte("\n")) {
			if _, err := fmt.Fprint(w, "\n"); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetFile retrieves a file by name from the archive.
func (a *TxtarArchive) GetFile(name string) ([]byte, bool) {
	// Normalize the search name
	searchName := strings.TrimSpace(name)
	for _, f := range a.Files {
		// Normalize the file name
		fileName := strings.TrimSpace(f.Name)
		if fileName == searchName {
			return f.Data, true
		}
	}
	return nil, false
}

// HasFile checks if a file exists in the archive.
func (a *TxtarArchive) HasFile(name string) bool {
	_, found := a.GetFile(name)
	return found
}
