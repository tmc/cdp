package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func contextStackOutputDir(base string, stack []string) string {
	dir := base
	for _, name := range stack {
		dir = filepath.Join(dir, name)
	}
	return dir
}

func contextStackDisplay(stack []string) string {
	if len(stack) == 0 {
		return "(root)"
	}
	return strings.Join(stack, "/")
}

func contextStackParentTag(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	return stack[len(stack)-1]
}

// validPathSegment checks a caller-supplied name that will be joined onto a
// directory. Tool names become files under -tools-dir and context names become
// directories under -output-dir; filepath.Join resolves ".." rather than
// rejecting it, so an unchecked name like "../../x" escapes the directory it
// was meant to stay in, and every one of those call sites runs MkdirAll on the
// result. A name is an identifier, not a path: it must be exactly one segment.
func validPathSegment(kind, name string) error {
	if name == "" {
		return fmt.Errorf("%s is required", kind)
	}
	if name == "." || name == ".." || filepath.Base(name) != name {
		return fmt.Errorf("invalid %s %q: must be a single path segment", kind, name)
	}
	return nil
}
