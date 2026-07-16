// Package lint contains static source checks enforced as ordinary tests
// (P2-3): they run with the default `go test ./...` target and fail CI on
// violation.
package lint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// forbiddenLevels are the zerolog level methods that must not appear in
// designated hot-path functions. Debug/Trace are permitted (disabled at
// production level, zero-cost when disabled); Fatal/Panic are permitted
// (terminal, cannot recur at event rate).
var forbiddenLevels = map[string]bool{
	"Info":  true,
	"Warn":  true,
	"Error": true,
}

// hotFunctions maps repo-relative files to the function/method names that
// form the hot path: the dispatch loop, ring/arena operations, and
// per-message adapter parse paths. See core/logger/doc.go for the rule.
var hotFunctions = map[string][]string{
	"core/msgbus/eventbus.go": {
		"Dispatch", "Poll", "Allocate", "Publish", "Cancel", "Release",
		"reserveSlow", "publishSlow",
	},
	"core/msgbus/msgbus.go": {
		"Dispatch", "Poll", "Allocate", "Publish", "Cancel", "Send", "AllocateCmd",
	},
	"adapter/binance/dataclient.go": {
		"processMessage", "processStreamMessage", "processDepthUpdate",
		"processTrade", "getPrecision", "appendPriceLevels", "OnMessage",
	},
	"adapter/bybit/dataclient.go": {
		"processMessage", "processOrderbook", "processDepthSnapshot",
		"processDepthUpdate", "processTrade", "processTradeItem",
		"getPrecision", "appendPriceLevels", "OnMessage",
	},
}

// loggerFreePackages are directories whose non-test files must not import
// any logging package at all (the innermost hot layer).
var loggerFreePackages = []string{
	"core/mem",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller path")
	}
	// file = <root>/internal/lint/hotpath_test.go
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

// TestHotPathLogging rejects Info/Warn/Error log calls inside designated
// hot-path functions. High-frequency conditions must be atomic counters
// reported by the msgbus observer goroutine, not inline log statements.
func TestHotPathLogging(t *testing.T) {
	root := repoRoot(t)

	for relFile, funcs := range hotFunctions {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filepath.Join(root, relFile), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", relFile, err)
		}

		designated := make(map[string]bool, len(funcs))
		for _, name := range funcs {
			designated[name] = true
		}
		found := make(map[string]bool, len(funcs))

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !designated[fn.Name.Name] {
				continue
			}
			found[fn.Name.Name] = true
			ast.Inspect(fn, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || !forbiddenLevels[sel.Sel.Name] {
					return true
				}
				if !isLoggerExpr(sel.X) {
					return true
				}
				pos := fset.Position(sel.Pos())
				t.Errorf("%s:%d: log().%s() in hot-path function %s (use a counter + observer, or Debug; see core/logger/doc.go)",
					relFile, pos.Line, sel.Sel.Name, fn.Name.Name)
				return true
			})
		}

		// Guard the guard: renaming a designated function must not silently
		// drop it from coverage.
		for name := range designated {
			if !found[name] {
				t.Errorf("%s: designated hot-path function %s not found; update internal/lint/hotpath_test.go", relFile, name)
			}
		}
	}
}

// isLoggerExpr reports whether expr produces a zerolog logger/event:
// log(), logger.Get(), or a chained zerolog builder call rooted in one.
func isLoggerExpr(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name == "log"
	case *ast.SelectorExpr:
		if ident, ok := fun.X.(*ast.Ident); ok && ident.Name == "logger" && fun.Sel.Name == "Get" {
			return true
		}
		// Chained builder, e.g. log().Warn().Str(...): recurse into the root.
		return isLoggerExpr(fun.X)
	}
	return false
}

// TestLoggerFreePackages asserts the innermost hot-path packages import no
// logging facility at all.
func TestLoggerFreePackages(t *testing.T) {
	root := repoRoot(t)

	for _, relDir := range loggerFreePackages {
		files, err := filepath.Glob(filepath.Join(root, relDir, "*.go"))
		if err != nil {
			t.Fatalf("glob %s: %v", relDir, err)
		}
		if len(files) == 0 {
			t.Fatalf("%s: no Go files found; update internal/lint/hotpath_test.go", relDir)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if strings.Contains(path, "zerolog") || strings.HasSuffix(path, "core/logger") {
					t.Errorf("%s imports %s: %s must stay logger-free (P2-3)", file, path, relDir)
				}
			}
		}
	}
}
