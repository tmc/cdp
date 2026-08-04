// Package docscan reports the flags a command registers, so that tests can
// check the command's documentation against its actual surface.
//
// Documentation drifts silently: a flag is added, the doc comment is not.
// Scanning the source for flag registrations gives tests a list to compare
// against, without running the command or importing its package.
package docscan

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Flags returns the names of the flags registered in the Go package in dir,
// sorted. It recognizes the standard library forms
//
//	flag.String("name", ...)      flag.StringVar(&v, "name", ...)
//	fs.String("name", ...)        fs.StringVar(&v, "name", ...)
//	flag.Var(&v, "name", ...)     fs.Var(&v, "name", ...)
//
// where fs is any expression, so both the package-level [flag.CommandLine] and
// an explicit [flag.FlagSet] are covered. Test files are ignored.
func Flags(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	seen := make(map[string]bool)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if name, ok := flagName(call); ok {
				seen[name] = true
			}
			return true
		})
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// flagName reports the flag name registered by call, if call is a flag
// registration.
func flagName(call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	arg, ok := nameArg(sel.Sel.Name, call.Args)
	if !ok {
		return "", false
	}
	lit, ok := arg.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	name, err := strconv.Unquote(lit.Value)
	if err != nil || name == "" {
		return "", false
	}
	return name, true
}

// registrar describes a flag registration method: where the flag name sits in
// the argument list, and how many arguments the method takes. Var and the
// XxxVar forms take the value first; every other form takes the name first.
// The arity is checked so that unrelated methods that merely share a name,
// such as a String method on some other type, are not mistaken for flags.
type registrar struct {
	nameArg int
	args    int
}

var registrars = map[string]registrar{
	"Bool": {0, 3}, "BoolVar": {1, 4},
	"Duration": {0, 3}, "DurationVar": {1, 4},
	"Float64": {0, 3}, "Float64Var": {1, 4},
	"Int": {0, 3}, "IntVar": {1, 4},
	"Int64": {0, 3}, "Int64Var": {1, 4},
	"String": {0, 3}, "StringVar": {1, 4},
	"Uint": {0, 3}, "UintVar": {1, 4},
	"Uint64": {0, 3}, "Uint64Var": {1, 4},
	"Func": {0, 3}, "BoolFunc": {0, 3}, "TextVar": {1, 4},
	"Var": {1, 3},
}

// nameArg returns the argument holding the flag name for a call to method.
func nameArg(method string, args []ast.Expr) (ast.Expr, bool) {
	r, ok := registrars[method]
	if !ok || len(args) != r.args {
		return nil, false
	}
	return args[r.nameArg], true
}
