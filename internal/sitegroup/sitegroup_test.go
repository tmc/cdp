package sitegroup

import "testing"

func TestHost(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{host: "notebooklm.google.com", want: "notebooklm.google.com"},
		{host: "www.lesswrong.com", want: "www.lesswrong.com"},
		{host: "cdn.lesswrong.com", want: "cdn.lesswrong.com"},
		{host: "NoteBookLM.Google.com", want: "notebooklm.google.com"},
		{host: "example.com:443", want: "example.com"},
		{host: "127.0.0.1", want: "127.0.0.1"},
		{host: "[::1]", want: "::1"},
		{host: "localhost", want: "localhost"},
		{host: "", want: "unknown_domain"},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := Host(tt.host); got != tt.want {
				t.Errorf("Host(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}
