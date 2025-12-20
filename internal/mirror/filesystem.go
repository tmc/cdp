package mirror

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// PathGenerator handles local path generation for mirrored resources.
type PathGenerator struct {
	outputDir         string
	noHostDirectories bool
	noDirectories     bool
	forceDirectories  bool
	cutDirs           int
}

// NewPathGenerator creates a new path generator with the given options.
func NewPathGenerator(outputDir string, opts *MirrorOptions) *PathGenerator {
	pg := &PathGenerator{
		outputDir: outputDir,
	}

	if opts != nil {
		pg.noHostDirectories = opts.NoHostDirectories
		pg.noDirectories = opts.NoDirectories
		pg.forceDirectories = opts.ForceDirectories
		pg.cutDirs = opts.CutDirs
	}

	return pg
}

// GenerateLocalPath generates a local file path for a given URL.
func (pg *PathGenerator) GenerateLocalPath(urlStr string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	var pathParts []string

	// Add host directory unless disabled
	if !pg.noHostDirectories {
		host := sanitizeFilename(parsedURL.Host)
		if host != "" {
			pathParts = append(pathParts, host)
		}
	}

	// Process the URL path
	urlPath := parsedURL.Path
	if urlPath == "" || urlPath == "/" {
		urlPath = "/index.html"
	}

	// Split path into components
	pathComponents := strings.Split(strings.TrimPrefix(urlPath, "/"), "/")

	// Apply cutDirs if specified
	if pg.cutDirs > 0 && len(pathComponents) > pg.cutDirs {
		pathComponents = pathComponents[pg.cutDirs:]
	}

	// Handle directory options
	if pg.noDirectories {
		// Flatten all components into a single filename
		filename := strings.Join(pathComponents, "_")
		pathParts = append(pathParts, sanitizeFilename(filename))
	} else if pg.forceDirectories || len(pathComponents) > 1 {
		// Create directory structure
		for i, component := range pathComponents {
			sanitized := sanitizeFilename(component)
			if i == len(pathComponents)-1 && sanitized == "" {
				// Last component is empty (trailing slash), add index.html
				sanitized = "index.html"
			}
			pathParts = append(pathParts, sanitized)
		}
	} else {
		// Single component
		pathParts = append(pathParts, sanitizeFilename(pathComponents[0]))
	}

	// Construct final path
	localPath := filepath.Join(append([]string{pg.outputDir}, pathParts...)...)

	// Ensure the path has a proper extension for HTML content
	if !hasExtension(localPath) && !strings.HasSuffix(localPath, "/") {
		localPath = filepath.Join(localPath, "index.html")
	}

	return localPath, nil
}

// CreateDirectory creates all necessary directories for a local path.
func (pg *PathGenerator) CreateDirectory(localPath string) error {
	dir := filepath.Dir(localPath)
	return os.MkdirAll(dir, 0755)
}

// CreateMirrorStructure creates the base directory structure for a mirror.
func CreateMirrorStructure(baseDir string) error {
	dirs := []string{
		baseDir,
		filepath.Join(baseDir, "static", "js"),
		filepath.Join(baseDir, "static", "css"),
		filepath.Join(baseDir, "static", "images"),
		filepath.Join(baseDir, "static", "fonts"),
		filepath.Join(baseDir, "data", "api"),
		filepath.Join(baseDir, "data", "websockets"),
		filepath.Join(baseDir, "screenshots"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// sanitizeFilename removes or replaces characters that are invalid in filenames.
func sanitizeFilename(filename string) string {
	// Replace path separators and other invalid characters
	replacements := map[rune]string{
		'/':  "_",
		'\\': "_",
		':':  "_",
		'*':  "_",
		'?':  "_",
		'"':  "_",
		'<':  "_",
		'>':  "_",
		'|':  "_",
		'\x00': "_",
	}

	var result strings.Builder
	for _, ch := range filename {
		if replacement, ok := replacements[ch]; ok {
			result.WriteString(replacement)
		} else {
			result.WriteRune(ch)
		}
	}

	sanitized := result.String()

	// Remove leading/trailing dots and spaces
	sanitized = strings.Trim(sanitized, ". ")

	// Limit length (filesystem limit is typically 255 bytes)
	if len(sanitized) > 200 {
		sanitized = sanitized[:200]
	}

	// If empty after sanitization, use a default
	if sanitized == "" {
		sanitized = "file"
	}

	return sanitized
}

// hasExtension checks if a path has a file extension.
func hasExtension(path string) bool {
	base := filepath.Base(path)
	return strings.Contains(base, ".")
}

// GetContentTypeDirectory returns the appropriate subdirectory based on content type.
func GetContentTypeDirectory(contentType string) string {
	contentType = strings.ToLower(strings.Split(contentType, ";")[0])
	contentType = strings.TrimSpace(contentType)

	switch {
	case strings.HasPrefix(contentType, "text/css"):
		return "static/css"
	case strings.HasPrefix(contentType, "text/javascript"), strings.HasPrefix(contentType, "application/javascript"):
		return "static/js"
	case strings.HasPrefix(contentType, "image/"):
		return "static/images"
	case strings.HasPrefix(contentType, "font/"), strings.Contains(contentType, "font"):
		return "static/fonts"
	case strings.HasPrefix(contentType, "video/"), strings.HasPrefix(contentType, "audio/"):
		return "static/media"
	case strings.HasPrefix(contentType, "application/json"):
		return "data/api"
	default:
		return "static"
	}
}

// GenerateAssetPath generates a local path for an asset based on its content type and URL.
func (pg *PathGenerator) GenerateAssetPath(urlStr, contentType string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Get content-type specific subdirectory
	subdir := GetContentTypeDirectory(contentType)

	// Extract filename from URL
	filename := filepath.Base(parsedURL.Path)
	if filename == "" || filename == "/" {
		filename = "index.html"
	}
	filename = sanitizeFilename(filename)

	// Construct path
	localPath := filepath.Join(pg.outputDir, subdir, filename)

	// Handle collisions by adding a counter
	if _, err := os.Stat(localPath); err == nil {
		ext := filepath.Ext(filename)
		base := strings.TrimSuffix(filename, ext)
		for i := 1; ; i++ {
			localPath = filepath.Join(pg.outputDir, subdir, fmt.Sprintf("%s_%d%s", base, i, ext))
			if _, err := os.Stat(localPath); os.IsNotExist(err) {
				break
			}
		}
	}

	return localPath, nil
}

// GetRelativePath returns a relative path from one file to another.
func GetRelativePath(from, to string) (string, error) {
	fromDir := filepath.Dir(from)
	relPath, err := filepath.Rel(fromDir, to)
	if err != nil {
		return "", fmt.Errorf("failed to compute relative path: %w", err)
	}

	// Convert to forward slashes for URLs
	relPath = filepath.ToSlash(relPath)
	return relPath, nil
}

// EnsureIndexHTML ensures that a directory has an index.html file.
func EnsureIndexHTML(dir string) error {
	indexPath := filepath.Join(dir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		// Create a minimal index.html
		content := []byte("<!DOCTYPE html>\n<html>\n<head><title>Index</title></head>\n<body><h1>Index</h1></body>\n</html>")
		if err := os.WriteFile(indexPath, content, 0644); err != nil {
			return fmt.Errorf("failed to create index.html: %w", err)
		}
	}
	return nil
}
