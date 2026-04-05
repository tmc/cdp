//go:generate cp -r ../../extension/coverage/ extension_bundle/coverage/

package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

//go:embed all:extension_bundle/coverage
var bundledExtensions embed.FS

// extractBundledExtensions extracts embedded extensions to ~/.cdp/extensions/.
// It only re-extracts when the embedded version differs from the on-disk
// .version file. Returns the base directory containing extracted extensions.
func extractBundledExtensions() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	base := filepath.Join(home, ".cdp", "extensions")

	entries, err := fs.ReadDir(bundledExtensions, "extension_bundle")
	if err != nil {
		return base, fmt.Errorf("read embedded bundle: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name() // e.g. "coverage"
		destDir := filepath.Join(base, name)

		// Check version stamp.
		embeddedVersion, err := fs.ReadFile(bundledExtensions, "extension_bundle/"+name+"/manifest.json")
		if err != nil {
			continue
		}
		versionFile := filepath.Join(destDir, ".embedded_version")
		existing, _ := os.ReadFile(versionFile)
		if string(existing) == string(embeddedVersion) {
			log.Printf("bundled extension %s: up to date", name)
			continue
		}

		// Extract all files.
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return base, fmt.Errorf("mkdir %s: %w", destDir, err)
		}
		prefix := "extension_bundle/" + name
		err = fs.WalkDir(bundledExtensions, prefix, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(prefix, path)
			dest := filepath.Join(destDir, rel)
			if d.IsDir() {
				return os.MkdirAll(dest, 0755)
			}
			data, err := fs.ReadFile(bundledExtensions, path)
			if err != nil {
				return err
			}
			return os.WriteFile(dest, data, 0644)
		})
		if err != nil {
			return base, fmt.Errorf("extract %s: %w", name, err)
		}

		// Write version stamp.
		os.WriteFile(versionFile, embeddedVersion, 0644)
		log.Printf("bundled extension %s: extracted to %s", name, destDir)
	}

	return base, nil
}
