package main

import (
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
