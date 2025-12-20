package main

import (
	"os"
	pathpkg "path/filepath"
	"testing"
)

// TestOutputWriter tests the OutputWriter struct
func TestOutputWriter(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		filepath  string
		format    string
		content   string
		wantError bool
		checkFile bool
	}{
		{
			name:      "write HTML to file",
			filepath:  pathpkg.Join(tmpDir, "output.html"),
			format:    "html",
			content:   "<html><body>Test</body></html>",
			wantError: false,
			checkFile: true,
			// Note: WriteString doesn't add newline for files
		},
		{
			name:      "write to stdout when filepath is empty",
			filepath:  "",
			format:    "html",
			content:   "<html><body>Test</body></html>",
			wantError: false,
			checkFile: false,
		},
		{
			name:      "write to stdout when filepath is dash",
			filepath:  "-",
			format:    "html",
			content:   "<html><body>Test</body></html>",
			wantError: false,
			checkFile: false,
		},
		{
			name:      "atomic write with temp file",
			filepath:  pathpkg.Join(tmpDir, "atomic.html"),
			format:    "html",
			content:   "Test atomic write",
			wantError: false,
			checkFile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := NewOutputWriter(tt.filepath, tt.format)
			err := writer.WriteHTML(tt.content)

			if (err != nil) != tt.wantError {
				t.Errorf("WriteHTML() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if tt.checkFile {
				// Verify file exists
				data, err := os.ReadFile(tt.filepath)
				if err != nil {
					t.Errorf("Failed to read file: %v", err)
					return
				}

				// Verify content matches (WriteString doesn't add newline for files)
				if string(data) != tt.content {
					t.Errorf("File content mismatch. Expected: %q, Got: %q", tt.content, string(data))
				}

				// Verify atomic write: check that no temp files remain
				tempGlob := pathpkg.Join(tmpDir, ".tmp-*")
				tempFiles, _ := pathpkg.Glob(tempGlob)
				if len(tempFiles) > 0 {
					t.Errorf("Temp files remain after write: %v", tempFiles)
				}
			}
		})
	}
}

// TestOutputWriterBinary tests binary output (PDF, PNG)
func TestOutputWriterBinary(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("write PNG data", func(t *testing.T) {
		pngFile := pathpkg.Join(tmpDir, "output.png")
		writer := NewOutputWriter(pngFile, "png")

		// Fake PNG data (just for testing file operations)
		fakeData := []byte{0x89, 0x50, 0x4E, 0x47} // PNG magic number

		err := writer.WritePNG(fakeData)
		if err != nil {
			t.Errorf("WritePNG() error = %v", err)
			return
		}

		// Verify file exists and has correct data
		data, err := os.ReadFile(pngFile)
		if err != nil {
			t.Errorf("Failed to read PNG file: %v", err)
			return
		}

		if len(data) != len(fakeData) {
			t.Errorf("PNG file size mismatch. Expected: %d, Got: %d", len(fakeData), len(data))
		}
	})

	t.Run("write PDF data", func(t *testing.T) {
		pdfFile := pathpkg.Join(tmpDir, "output.pdf")
		writer := NewOutputWriter(pdfFile, "pdf")

		// Fake PDF data
		fakeData := []byte{0x25, 0x50, 0x44, 0x46} // PDF magic number

		err := writer.WritePDF(fakeData)
		if err != nil {
			t.Errorf("WritePDF() error = %v", err)
			return
		}

		// Verify file exists
		data, err := os.ReadFile(pdfFile)
		if err != nil {
			t.Errorf("Failed to read PDF file: %v", err)
			return
		}

		if len(data) != len(fakeData) {
			t.Errorf("PDF file size mismatch. Expected: %d, Got: %d", len(fakeData), len(data))
		}
	})
}

// TestOutputWriterAtomicWrites tests that writes are atomic
func TestOutputWriterAtomicWrites(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := pathpkg.Join(tmpDir, "atomic.txt")

	writer := NewOutputWriter(testFile, "text")
	content := "Atomic write test content"

	// Perform write
	err := writer.WriteHTML(content)
	if err != nil {
		t.Errorf("WriteHTML() error = %v", err)
		return
	}

	// Verify no temp files were left behind
	tempGlob := pathpkg.Join(tmpDir, ".tmp-*")
	tempFiles, err := pathpkg.Glob(tempGlob)
	if err != nil {
		t.Errorf("Failed to glob temp files: %v", err)
		return
	}

	if len(tempFiles) > 0 {
		t.Errorf("Temp files remain after write: %v", tempFiles)
	}

	// Verify file was created with correct content
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Errorf("Failed to read file: %v", err)
		return
	}

	// WriteString doesn't add newline for files
	if string(data) != content {
		t.Errorf("File content mismatch. Expected: %q, Got: %q", content, string(data))
	}
}
