package security

import (
	"regexp"
	"testing"
)

func TestValidator_ValidateString(t *testing.T) {
	tests := []struct {
		name      string
		validator *Validator
		input     string
		wantErr   bool
	}{
		{
			name:      "valid string",
			validator: NewValidator(DefaultValidationConfig()),
			input:     "hello world",
			wantErr:   false,
		},
		{
			name:      "string too long",
			validator: NewValidator(ValidationConfig{MaxStringLength: 5}),
			input:     "this is too long",
			wantErr:   true,
		},
		{
			name: "blocked pattern",
			validator: NewValidator(ValidationConfig{
				MaxStringLength: 1000,
				BlockedPatterns: []*regexp.Regexp{regexp.MustCompile(`<script`)},
			}),
			input:   "<script>alert('xss')</script>",
			wantErr: true,
		},
		{
			name:      "invalid UTF-8",
			validator: NewValidator(DefaultValidationConfig()),
			input:     string([]byte{0xff, 0xfe, 0xfd}),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validator.ValidateString(tt.input, "test")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateString() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateURL(t *testing.T) {
	tests := []struct {
		name      string
		validator *Validator
		url       string
		wantErr   bool
	}{
		{
			name:      "valid https URL",
			validator: NewValidator(DefaultValidationConfig()),
			url:       "https://example.com",
			wantErr:   false,
		},
		{
			name:      "valid http URL",
			validator: NewValidator(DefaultValidationConfig()),
			url:       "http://example.com",
			wantErr:   false,
		},
		{
			name: "invalid scheme",
			validator: NewValidator(ValidationConfig{
				AllowedURLSchemes: []string{"https"},
				MaxStringLength:   1000,
			}),
			url:     "http://example.com",
			wantErr: true,
		},
		{
			name: "blocked domain",
			validator: NewValidator(ValidationConfig{
				AllowedURLSchemes: []string{"http", "https"},
				AllowedDomains:    []string{"example.com"},
				MaxStringLength:   1000,
			}),
			url:     "https://evil.com",
			wantErr: true,
		},
		{
			name:      "javascript URL",
			validator: NewValidator(DefaultValidationConfig()),
			url:       "javascript:alert('xss')",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validator.ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateFilePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid relative path",
			path:    "files/document.txt",
			wantErr: false,
		},
		{
			name:    "path traversal",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "sensitive directory",
			path:    "/etc/shadow",
			wantErr: true,
		},
		{
			name:    "windows system directory",
			path:    "C:\\Windows\\System32\\config",
			wantErr: true,
		},
	}

	validator := NewValidator(DefaultValidationConfig())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateFilePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantErr bool
	}{
		{
			name:    "safe command",
			cmd:     "ls -l",
			wantErr: false,
		},
		{
			name:    "command with pipe",
			cmd:     "ls | grep test",
			wantErr: true,
		},
		{
			name:    "command with semicolon",
			cmd:     "ls; rm -rf /",
			wantErr: true,
		},
		{
			name:    "rm -rf command",
			cmd:     "rm -rf /",
			wantErr: true,
		},
		{
			name:    "command substitution",
			cmd:     "echo $(cat /etc/passwd)",
			wantErr: true,
		},
		{
			name:    "fork bomb",
			cmd:     ":(){ :|:& };:",
			wantErr: true,
		},
	}

	validator := NewValidator(DefaultValidationConfig())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCommand(tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_SanitizeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean string",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "string with null bytes",
			input:    "hello\x00world",
			expected: "helloworld",
		},
		{
			name:     "string with control characters",
			input:    "hello\x01\x02world",
			expected: "helloworld",
		},
		{
			name:     "preserve newlines and tabs",
			input:    "hello\n\tworld",
			expected: "hello\n\tworld",
		},
	}

	validator := NewValidator(DefaultValidationConfig())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.SanitizeString(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestValidator_ValidateJSONDepth(t *testing.T) {
	tests := []struct {
		name    string
		data    interface{}
		maxDepth int
		wantErr bool
	}{
		{
			name:    "shallow object",
			data:    map[string]interface{}{"key": "value"},
			maxDepth: 5,
			wantErr: false,
		},
		{
			name: "nested object within limit",
			data: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": "value",
					},
				},
			},
			maxDepth: 5,
			wantErr: false,
		},
		{
			name: "nested object exceeds limit",
			data: map[string]interface{}{
				"l1": map[string]interface{}{
					"l2": map[string]interface{}{
						"l3": map[string]interface{}{
							"l4": map[string]interface{}{
								"l5": map[string]interface{}{
									"l6": "value",
								},
							},
						},
					},
				},
			},
			maxDepth: 3,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(ValidationConfig{
				MaxObjectDepth: tt.maxDepth,
			})

			err := validator.ValidateJSONDepth(tt.data, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateJSONDepth() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
