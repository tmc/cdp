package docscan

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFlags(t *testing.T) {
	const src = `package main

import "flag"

var fs = flag.NewFlagSet("x", flag.ExitOnError)

var v stringSlice

func main() {
	flag.String("pkg-string", "", "")
	flag.BoolVar(&b, "pkg-boolvar", false, "")
	flag.Var(&v, "pkg-var", "")
	flag.Func("pkg-func", "", nil)
	fs.StringVar(&s, "set-stringvar", "", "")
	fs.Duration("set-duration", 0, "")
	c.fs.IntVar(&n, "nested-intvar", 0, "")

	notAFlag.String("ignored")
	flag.Parse()
	fmt.Println("also-ignored")
}
`
	dir := t.TempDir()
	write(t, filepath.Join(dir, "main.go"), src)
	// Registrations in test files must not count.
	write(t, filepath.Join(dir, "main_test.go"), `package main

import "flag"

func init() { flag.String("test-only", "", "") }
`)

	got, err := Flags(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"nested-intvar", "pkg-boolvar", "pkg-func", "pkg-string",
		"pkg-var", "set-duration", "set-stringvar",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Flags() = %q, want %q", got, want)
	}
}

func TestFlagsError(t *testing.T) {
	if _, err := Flags(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("Flags() on a missing directory succeeded, want error")
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o666); err != nil {
		t.Fatal(err)
	}
}
