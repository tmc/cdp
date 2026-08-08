// Package cdpproto records the DevTools protocol surface this tree is built
// against, so that tests can catch two kinds of silent drift.
//
// The first is a method name that does not exist. Most protocol calls go
// through the generated cdproto bindings and fail to compile when they are
// wrong, but a call made by raw string does not: it type checks, builds, and
// fails at runtime with "-32601 method not found" against every browser, on
// every machine, forever. [Names] and [Valid] let a test reject those names
// before they ship.
//
// The second is an upgrade that changes the protocol without anyone noticing.
// The name list in methods.txt is generated from cdproto and committed, so
// bumping cdproto and running go generate produces a reviewable diff of
// exactly which commands and events appeared or disappeared. That diff is the
// prompt to update the tools, docs, and skills that describe the surface.
//
// Regenerate with:
//
//	go generate ./internal/cdpproto
package cdpproto

import (
	_ "embed"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:generate go run ./gen -o methods.txt

//go:embed methods.txt
var methods string

var names = func() map[string]bool {
	m := make(map[string]bool)
	for _, line := range strings.Split(methods, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			m[line] = true
		}
	}
	return m
}()

// Names returns every protocol command and event name known to the cdproto
// version this tree depends on, sorted.
func Names() []string {
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Valid reports whether name is a protocol command or event.
func Valid(name string) bool { return names[name] }

// A Ref is a protocol method name written as a string literal in Go source.
type Ref struct {
	Name string // the method name, such as "Page.startScreencast"
	File string // the file containing it
	Line int
}

// Refs returns the protocol method names appearing as string literals in the
// Go package in dir, sorted by position. Test files are skipped, because their
// literals are overwhelmingly documentation and filenames — "Domain.method" in
// a help message, "README.md" in a path — which have the shape of a method
// name without being one.
//
// Refs finds names by shape rather than by call site, because the calls that
// need checking are the ones that do not go through a generated binding, and
// those take the name as an ordinary argument.
func Refs(dir string) ([]Ref, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		return nil, err
	}
	var refs []Ref
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil || !isMethodName(s) {
					return true
				}
				refs = append(refs, Ref{
					Name: s,
					File: filepath.Base(path),
					Line: fset.Position(lit.Pos()).Line,
				})
				return true
			})
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].File != refs[j].File {
			return refs[i].File < refs[j].File
		}
		return refs[i].Line < refs[j].Line
	})
	return refs, nil
}

func isMethodName(s string) bool {
	domain, member, ok := strings.Cut(s, ".")
	if !ok || domain == "" || member == "" || strings.Contains(member, ".") {
		return false
	}
	if !isUpper(rune(domain[0])) || !isLower(rune(member[0])) {
		return false
	}
	return isAlnum(domain) && isAlnum(member)
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }

func isAlnum(s string) bool {
	for _, r := range s {
		if !isUpper(r) && !isLower(r) && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}
