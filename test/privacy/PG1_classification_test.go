// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"go/ast"
	"go/token"
	"testing"
)

// PG-1: every field with personal content has a classification; unclassified fields fail the build
// (data-protection.md §10, ADR-0018 decision 1).
//
// The place a field's classification is *acted on* in this build is the audit trail: `audit.Change`
// carries one, and the masking derived from it decides whether a value is written in clear text,
// as a fingerprint, or not at all. A change built without one used to fall through to OPEN, which
// is the direction that cannot be taken back - a title written into the trail is a copy no deletion
// reaches (audit.md §4).
//
// So this is what PG-1 checks: no `Change` literal anywhere in the source omits `Classification`.
// It is a build-time rule rather than a review habit, which is the whole point of a gate; the port
// masks an unclassified field at run time as well, for the case where something builds one from a
// variable.
func TestPG1EveryRecordedChangeIsClassified(t *testing.T) {
	found := 0

	forEachGoFile(t, func(path string, file *ast.File, fset *token.FileSet) {
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok || !isAuditChange(literal.Type) {
				return true
			}
			found++

			for _, element := range literal.Elts {
				if pair, ok := element.(*ast.KeyValueExpr); ok {
					if key, ok := pair.Key.(*ast.Ident); ok && key.Name == "Classification" {
						return true
					}
				}
			}
			t.Errorf("%s: a change is written into the audit trail without a classification "+
				"(PG-1). Name one: audit.Open for a value the trail may carry, audit.Sensitive "+
				"for a person's, audit.Secret for a credential",
				at(path, fset, literal))
			return true
		})
	})

	// A gate that matched nothing is a gate that is not looking where it thinks it is.
	t.Logf("PG-1 checked %d recorded changes", found)
	if found == 0 {
		t.Fatal("PG-1 found no audit change at all - the pattern no longer matches")
	}
}

// isAuditChange recognises the port's type however the importing package spells it: `audit.Change`
// where the port is imported plainly, `port.Change` where the importing package is itself called
// audit, and `Change` inside the port.
func isAuditChange(expr ast.Expr) bool {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name == "Change"
	case *ast.SelectorExpr:
		if pkg, ok := typed.X.(*ast.Ident); ok {
			return typed.Sel.Name == "Change" && (pkg.Name == "audit" || pkg.Name == "port")
		}
	}
	return false
}
