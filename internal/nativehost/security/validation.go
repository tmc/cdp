package security

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ValidationConfig holds validation configuration
type ValidationConfig struct {
	MaxMessageSize    int
	MaxStringLength   int
	MaxArrayLength    int
	MaxObjectDepth    int
	AllowedURLSchemes []string
	AllowedDomains    []string // Empty means allow all
	BlockedPatterns   []*regexp.Regexp
}

// DefaultValidationConfig returns default validation configuration
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		MaxMessageSize:    1024 * 1024, // 1MB
		MaxStringLength:   10000,
		MaxArrayLength:    1000,
		MaxObjectDepth:    10,
		AllowedURLSchemes: []string{"http", "https"},
		AllowedDomains:    []string{}, // Allow all by default
		BlockedPatterns:   []*regexp.Regexp{},
	}
}

// StrictValidationConfig returns a more restrictive configuration
func StrictValidationConfig() ValidationConfig {
	return ValidationConfig{
		MaxMessageSize:    512 * 1024, // 512KB
		MaxStringLength:   5000,
		MaxArrayLength:    500,
		MaxObjectDepth:    5,
		AllowedURLSchemes: []string{"https"}, // Only HTTPS
		AllowedDomains:    []string{},
		BlockedPatterns: []*regexp.Regexp{
			regexp.MustCompile(`<script`),
			regexp.MustCompile(`javascript:`),
			regexp.MustCompile(`on\w+\s*=`), // Event handlers
		},
	}
}

// Validator handles input validation and sanitization
type Validator struct {
	config ValidationConfig
}

// NewValidator creates a new validator
func NewValidator(config ValidationConfig) *Validator {
	return &Validator{
		config: config,
	}
}

// ValidateString validates a string input
func (v *Validator) ValidateString(s string, fieldName string) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("%s: invalid UTF-8 encoding", fieldName)
	}

	if len(s) > v.config.MaxStringLength {
		return fmt.Errorf("%s: exceeds maximum length %d", fieldName, v.config.MaxStringLength)
	}

	// Check for blocked patterns
	for _, pattern := range v.config.BlockedPatterns {
		if pattern.MatchString(s) {
			return fmt.Errorf("%s: contains blocked pattern", fieldName)
		}
	}

	return nil
}

// ValidateURL validates a URL
func (v *Validator) ValidateURL(urlStr string) error {
	if err := v.ValidateString(urlStr, "url"); err != nil {
		return err
	}

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Check scheme
	schemeAllowed := false
	for _, scheme := range v.config.AllowedURLSchemes {
		if parsed.Scheme == scheme {
			schemeAllowed = true
			break
		}
	}
	if !schemeAllowed {
		return fmt.Errorf("URL scheme %s not allowed", parsed.Scheme)
	}

	// Check domain if restrictions are configured
	if len(v.config.AllowedDomains) > 0 {
		domainAllowed := false
		for _, domain := range v.config.AllowedDomains {
			if strings.HasSuffix(parsed.Host, domain) {
				domainAllowed = true
				break
			}
		}
		if !domainAllowed {
			return fmt.Errorf("domain %s not allowed", parsed.Host)
		}
	}

	return nil
}

// SanitizeString removes potentially dangerous characters
func (v *Validator) SanitizeString(s string) string {
	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")

	// Remove control characters except newline, tab, carriage return
	result := strings.Builder{}
	for _, r := range s {
		if r == '\n' || r == '\t' || r == '\r' || (r >= 32 && r < 127) || r >= 128 {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// ValidateMessageType checks if a message type is valid
func (v *Validator) ValidateMessageType(msgType string) error {
	if err := v.ValidateString(msgType, "message type"); err != nil {
		return err
	}

	// Message type should be alphanumeric with underscores
	if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(msgType) {
		return fmt.Errorf("invalid message type format: %s", msgType)
	}

	return nil
}

// ValidateID checks if an ID is valid
func (v *Validator) ValidateID(id string) error {
	if err := v.ValidateString(id, "id"); err != nil {
		return err
	}

	// ID should be alphanumeric with dashes
	if !regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`).MatchString(id) {
		return fmt.Errorf("invalid ID format: %s", id)
	}

	return nil
}

// ValidateJSONDepth checks object nesting depth
func (v *Validator) ValidateJSONDepth(data interface{}, currentDepth int) error {
	if currentDepth > v.config.MaxObjectDepth {
		return fmt.Errorf("JSON nesting exceeds maximum depth %d", v.config.MaxObjectDepth)
	}

	switch val := data.(type) {
	case map[string]interface{}:
		for _, item := range val {
			if err := v.ValidateJSONDepth(item, currentDepth+1); err != nil {
				return err
			}
		}
	case []interface{}:
		if len(val) > v.config.MaxArrayLength {
			return fmt.Errorf("array length %d exceeds maximum %d", len(val), v.config.MaxArrayLength)
		}
		for _, item := range val {
			if err := v.ValidateJSONDepth(item, currentDepth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

// ValidateCommand validates a command string for execution
func (v *Validator) ValidateCommand(cmd string) error {
	if err := v.ValidateString(cmd, "command"); err != nil {
		return err
	}

	// Block dangerous command patterns
	dangerousPatterns := []string{
		";", "|", "&", "`", "$(",
		"rm -rf", "dd if=", "mkfs",
		":(){ :|:& };:", // Fork bomb
	}

	cmdLower := strings.ToLower(cmd)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(cmdLower, pattern) {
			return fmt.Errorf("command contains dangerous pattern: %s", pattern)
		}
	}

	return nil
}

// ValidateFilePath validates a file path
func (v *Validator) ValidateFilePath(path string) error {
	if err := v.ValidateString(path, "file path"); err != nil {
		return err
	}

	// Block path traversal
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal not allowed")
	}

	// Block absolute paths to sensitive directories
	sensitiveDirs := []string{
		"/etc/", "/sys/", "/proc/", "/dev/",
		"C:\\Windows\\System32", "C:\\Windows\\SysWOW64",
	}

	for _, dir := range sensitiveDirs {
		if strings.HasPrefix(path, dir) {
			return fmt.Errorf("access to sensitive directory not allowed: %s", dir)
		}
	}

	return nil
}
