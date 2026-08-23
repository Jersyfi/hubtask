// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

// TestEveryItemWriteNamesItsSubject is what makes the per-role suite in test/security sufficient
// (gate SG-5, C-04).
//
// The narrowings live in one decision point, and a use case reaches them by naming the entry it is
// writing. A use case that forgets to name one is not refused and not caught by its own test: it
// gets the unqualified permission, which is exactly the state C-04 was opened to end. Nothing but
// reading the source catches that, because the field is optional by construction - a container
// request must not carry it.
//
// The rule: every access.Request literal whose TargetType is the item target names a subject.
func TestEveryItemWriteNamesItsSubject(t *testing.T) {
	forEachGoFile(t, []string{"../../core/application"}, func(path string, f *ast.File, fset *token.FileSet) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}

		ast.Inspect(f, func(n ast.Node) bool {
			literal, ok := n.(*ast.CompositeLit)
			if !ok || !isAccessRequest(literal) {
				return true
			}
			if targetTypeOf(literal) != "itemTarget" || keyed(literal, "On") {
				return true
			}

			t.Errorf("%s:%d: a permission question about an entry does not name it - "+
				"set On, or the matrix's qualifiers are silently skipped (C-04, ADR-0005)",
				rel(path), fset.Position(literal.Pos()).Line)
			return true
		})
	})
}

// TestTheNarrowingIsNotDecidedOutsideTheApplicationLayer is the other half of SG-5: the decision
// may be consulted anywhere and made in one place. An adapter that read the per-entry matrix would
// be an adapter deciding whether somebody may write, which is the arrangement ADR-0005 forbids.
func TestTheNarrowingIsNotDecidedOutsideTheApplicationLayer(t *testing.T) {
	decisions := []string{"AllowsItemAction", "ItemAccessOf", "AllowsItem"}

	forEachGoFile(t, []string{"../../infrastructure", "../../presentation", "../../cmd"},
		func(path string, f *ast.File, fset *token.FileSet) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			// The REST layer renders the matrix into the capability manifest, which is describing
			// it rather than deciding from it: nothing there refuses anybody.
			if strings.HasSuffix(rel(path), "presentation/rest/MetaController.go") {
				return
			}

			ast.Inspect(f, func(n ast.Node) bool {
				selector, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				for _, decision := range decisions {
					if selector.Sel.Name == decision {
						t.Errorf("%s:%d: %s is called outside the application layer - "+
							"authorisation is decided there and nowhere else (rule 2, ADR-0005)",
							rel(path), fset.Position(selector.Pos()).Line, decision)
					}
				}
				return true
			})
		})
}

// isAccessRequest reports whether the literal builds an access.Request.
func isAccessRequest(literal *ast.CompositeLit) bool {
	selector, ok := literal.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "access" && selector.Sel.Name == "Request"
}

// targetTypeOf reads the identifier the literal's TargetType field was set to, or the empty string
// when it was set to something other than a plain name.
func targetTypeOf(literal *ast.CompositeLit) string {
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok || key.Name != "TargetType" {
			continue
		}
		if value, ok := pair.Value.(*ast.Ident); ok {
			return value.Name
		}
	}
	return ""
}

// keyed reports whether the literal sets the named field at all.
func keyed(literal *ast.CompositeLit, field string) bool {
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := pair.Key.(*ast.Ident); ok && key.Name == field {
			return true
		}
	}
	return false
}
