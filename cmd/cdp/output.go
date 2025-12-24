package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// OutputWriter handles writing output to files or stdout
type OutputWriter struct {
	filepath string
	format   string
}

// NewOutputWriter creates a new OutputWriter
func NewOutputWriter(path, format string) *OutputWriter {
	return &OutputWriter{
		filepath: path,
		format:   format,
	}
}

// WriteHTML writes HTML content
func (w *OutputWriter) WriteHTML(content string) error {
	return w.writeBytes([]byte(content))
}

// WritePNG writes PNG data
func (w *OutputWriter) WritePNG(data []byte) error {
	return w.writeBytes(data)
}

// WritePDF writes PDF data
func (w *OutputWriter) WritePDF(data []byte) error {
	return w.writeBytes(data)
}

// writeBytes writes data to the destination
func (w *OutputWriter) writeBytes(data []byte) error {
	// If filepath is empty or "-", write to stdout
	if w.filepath == "" || w.filepath == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}

	// Atomic write: write to temp file then rename
	dir := filepath.Dir(w.filepath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name()) // Clean up if something goes wrong before rename

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write data: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpFile.Name(), w.filepath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
