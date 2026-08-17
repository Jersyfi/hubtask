// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// guardedClientPackage is the one place allowed to build an HTTP client: it is the guarded one.
const guardedClientPackage = "infrastructure/httpclient"

// outboundExceptions are the files that may reach for the standard library's HTTP client
// directly, each for a reason that is written down at the call site.
//
// A list rather than a comment convention: an exception should be visible here, in one place,
// where adding one is a decision somebody makes rather than a nolint nobody reads.
var outboundExceptions = map[string]string{
	// The container health check calls the process's own /healthz on 127.0.0.1. GuardedClient
	// refuses loopback by design, which is exactly right for a webhook and exactly wrong here.
	"cmd/server/main.go": "the container health check against 127.0.0.1",
}

// forbiddenOutbound is what makes a call without the guard: the package-level helpers, the
// shared client, and the shared transport. Each of them dials whatever it is given, with no
// address check, no redirect budget, and - for the helpers - no context either.
var forbiddenOutbound = map[string]string{
	"Get":              "http.Get",
	"Post":             "http.Post",
	"PostForm":         "http.PostForm",
	"Head":             "http.Head",
	"DefaultClient":    "http.DefaultClient",
	"DefaultTransport": "http.DefaultTransport",
}

// TestOutboundHTTPGoesThroughTheGuardedClient checks rule 6 from CLAUDE.md (ADR-0015,
// security.md §T-07). An outbound call that skips GuardedClient skips the SSRF guard, the
// redirect budget, and the response size limit - and the first one to do so will be an adapter
// written in a hurry against a third-party API.
func TestOutboundHTTPGoesThroughTheGuardedClient(t *testing.T) {
	roots := []string{"../../core", "../../infrastructure", "../../presentation", "../../cmd"}

	forEachGoFile(t, roots, func(path string, f *ast.File, fset *token.FileSet) {
		relative := rel(path)
		if strings.Contains(filepath.ToSlash(filepath.Dir(path)), guardedClientPackage) {
			return
		}
		if _, allowed := outboundExceptions[relative]; allowed {
			return
		}

		ast.Inspect(f, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.SelectorExpr:
				if isHTTPPackage(node.X) {
					if name, forbidden := forbiddenOutbound[node.Sel.Name]; forbidden {
						t.Errorf("%s:%d: %s bypasses the SSRF guard - go through httpclient.GuardedClient (rule 6)",
							relative, fset.Position(node.Pos()).Line, name)
					}
				}
			case *ast.CompositeLit:
				// &http.Client{...}: its own client, its own dialler, and none of the checks.
				if sel, ok := node.Type.(*ast.SelectorExpr); ok && isHTTPPackage(sel.X) && sel.Sel.Name == "Client" {
					t.Errorf("%s:%d: a second http.Client - the guarded one is the only way out (rule 6)",
						relative, fset.Position(node.Pos()).Line)
				}
			}
			return true
		})
	})
}

// TestTheOutboundExceptionsStillExist keeps the list above honest. An exception for a file that
// has been deleted or cleaned up is a hole that stays open for the next file to be given that
// name.
func TestTheOutboundExceptionsStillExist(t *testing.T) {
	for file := range outboundExceptions {
		if !fileExists(filepath.Join("../..", file)) {
			t.Errorf("%s is on the exception list but does not exist - remove the entry", file)
		}
	}
}

// TestTheOutboundExceptionsAreActuallyReachable guards the lookup itself rather than the list.
// The exception above is keyed on the forward-slash form, while WalkDir hands out whatever the
// platform uses - backslashes on Windows. When rel does not normalise that, the lookup misses,
// the exception is not honoured, and the gate reports a rule violation that is not one. It did,
// until this test existed.
func TestTheOutboundExceptionsAreActuallyReachable(t *testing.T) {
	for file := range outboundExceptions {
		native := filepath.Join("../..", file)
		if got := rel(native); got != file {
			t.Errorf("rel(%q) = %q, want %q - the exception would never be found", native, got, file)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isHTTPPackage(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "http"
}
