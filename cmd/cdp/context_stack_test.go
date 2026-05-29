package main

import (
	"path/filepath"
	"testing"
)

func TestContextStackOutputDir(t *testing.T) {
	got := contextStackOutputDir("/tmp/out", []string{"login", "submit"})
	want := filepath.Join("/tmp/out", "login", "submit")
	if got != want {
		t.Fatalf("contextStackOutputDir = %q, want %q", got, want)
	}
	if got := contextStackOutputDir("/tmp/out", nil); got != "/tmp/out" {
		t.Fatalf("root output dir = %q, want /tmp/out", got)
	}
}

func TestContextStackDisplay(t *testing.T) {
	if got := contextStackDisplay(nil); got != "(root)" {
		t.Fatalf("root display = %q", got)
	}
	if got := contextStackDisplay([]string{"login", "submit"}); got != "login/submit" {
		t.Fatalf("nested display = %q", got)
	}
}

func TestContextStackParentTag(t *testing.T) {
	if got := contextStackParentTag(nil); got != "" {
		t.Fatalf("root parent tag = %q", got)
	}
	if got := contextStackParentTag([]string{"login", "submit"}); got != "submit" {
		t.Fatalf("parent tag = %q", got)
	}
}
