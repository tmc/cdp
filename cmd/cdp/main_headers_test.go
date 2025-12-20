package main

import (
	"testing"
)

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    stringSlice
		expected map[string]interface{}
	}{
		{
			name:  "single header",
			input: stringSlice{"Authorization: Bearer token"},
			expected: map[string]interface{}{
				"Authorization": "Bearer token",
			},
		},
		{
			name: "multiple headers",
			input: stringSlice{
				"Authorization: Bearer token123",
				"Content-Type: application/json",
				"X-Test: value",
			},
			expected: map[string]interface{}{
				"Authorization": "Bearer token123",
				"Content-Type":  "application/json",
				"X-Test":        "value",
			},
		},
		{
			name:     "empty input",
			input:    stringSlice{},
			expected: map[string]interface{}{},
		},
		{
			name: "header with spaces",
			input: stringSlice{
				"  Authorization  :  Bearer token  ",
			},
			expected: map[string]interface{}{
				"Authorization": "Bearer token",
			},
		},
		{
			name: "value with colons",
			input: stringSlice{
				"Authorization: Bearer abc:def:ghi",
			},
			expected: map[string]interface{}{
				"Authorization": "Bearer abc:def:ghi",
			},
		},
		{
			name: "invalid headers (no colon)",
			input: stringSlice{
				"InvalidHeader",
				"Valid: value",
			},
			expected: map[string]interface{}{
				"Valid": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseHeaders(tt.input)

			// Check length
			if len(result) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d", len(result), len(tt.expected))
				return
			}

			// Check each key-value pair
			for key, expectedValue := range tt.expected {
				actualValue, ok := result[key]
				if !ok {
					t.Errorf("missing key: %s", key)
					continue
				}
				if actualValue != expectedValue {
					t.Errorf("value mismatch for key %s: got %v, want %v", key, actualValue, expectedValue)
				}
			}
		})
	}
}

func TestParseHeadersEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    stringSlice
		expected map[string]interface{}
	}{
		{
			name:     "empty header name",
			input:    stringSlice{": value"},
			expected: map[string]interface{}{},
		},
		{
			name: "header with numbers",
			input: stringSlice{
				"X-API-Version: 1.2.3",
				"Content-Length: 1024",
			},
			expected: map[string]interface{}{
				"X-API-Version": "1.2.3",
				"Content-Length": "1024",
			},
		},
		{
			name: "common authentication headers",
			input: stringSlice{
				"Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
				"X-API-Key: abc123def456",
			},
			expected: map[string]interface{}{
				"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
				"X-API-Key":     "abc123def456",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseHeaders(tt.input)

			for key, expectedValue := range tt.expected {
				actualValue, ok := result[key]
				if !ok {
					t.Errorf("missing key: %s", key)
					continue
				}
				if actualValue != expectedValue {
					t.Errorf("value mismatch for key %s: got %v, want %v", key, actualValue, expectedValue)
				}
			}

			if len(result) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d", len(result), len(tt.expected))
			}
		})
	}
}
