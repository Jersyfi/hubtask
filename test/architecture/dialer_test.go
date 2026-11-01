// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// dialerExceptions are the files that may build a net.Dialer or tls.Dialer themselves. Every
// other non-HTTP protocol dials through httpclient.GuardedDialer, so the allowlist, resolution
// and the dial-time address check cannot be half copied (rule 6, T-07).
var dialerExceptions = map[string]string{
	"infrastructure/clamd/Scanner.go":     "the unix socket of a local scanner; its TCP path is guarded",
	"infrastructure/mail/SMTP.go":         "the operator-configured mail relay, the trust class of the database DSN",
	"infrastructure/storage/S3Storage.go": "the operator-configured object storage endpoint",
	"cmd/restore-drill/kube.go":           "the pod's own API server, reached through the injected address",
}

// TestNonHTTPDialsGoThroughTheGuardedDialer extends rule 6 to every socket the process opens
// to an address somebody configured: a fifth protocol adapter that forgot one of the three
// checks would otherwise pass every gate.
func TestNonHTTPDialsGoThroughTheGuardedDialer(t *testing.T) {
	roots := []string{"../../core", "../../infrastructure", "../../presentation", "../../cmd"}

	forEachGoFile(t, roots, func(path string, f *ast.File, fset *token.FileSet) {
		relative := rel(path)
		if strings.Contains(filepath.ToSlash(filepath.Dir(path)), guardedClientPackage) {
			return
		}
		if _, allowed := dialerExceptions[relative]; allowed {
			return
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Dialer" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && (pkg.Name == "net" || pkg.Name == "tls") {
				t.Errorf("%s:%d: %s.Dialer skips the outbound guard - dial through httpclient.GuardedDialer (rule 6)",
					relative, fset.Position(sel.Pos()).Line, pkg.Name)
			}
			return true
		})
	})
}

// TestTheDialerExceptionsStillExist keeps the list above honest.
func TestTheDialerExceptionsStillExist(t *testing.T) {
	for file := range dialerExceptions {
		if !fileExists(filepath.Join("../..", file)) {
			t.Errorf("%s is on the dialer exception list but does not exist - remove the entry", file)
		}
	}
}
