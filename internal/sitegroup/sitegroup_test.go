package sitegroup

import "testing"

func TestRegistrableDomain(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{host: "www.lesswrong.com", want: "lesswrong.com"},
		{host: "cdn.lesswrong.com", want: "lesswrong.com"},
		{host: "foo.bar.co.uk", want: "bar.co.uk"},
		{host: "example.com:443", want: "example.com"},
		{host: "127.0.0.1", want: "127.0.0.1"},
		{host: "[::1]", want: "::1"},
		{host: "localhost", want: "localhost"},
		{host: "", want: "unknown_domain"},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := RegistrableDomain(tt.host); got != tt.want {
				t.Errorf("RegistrableDomain(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}
