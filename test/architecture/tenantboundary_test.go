// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

// Rule 3 does not bend, and E-10 is where it would have.
//
// An access request has to produce a copy of somebody's data across every workspace they are a
// member of - the one operation in this system that legitimately crosses the tenant boundary
// (data-protection.md §4). The way it is *not* done is a repository method that takes a tenant as
// an argument: one such method is a query that runs outside the transaction's own scope, and every
// other method beside it becomes an invitation to add a second.
//
// What is done instead: one function that answers tenant *identifiers* and nothing else, and then
// one ordinary transaction per tenant, under that tenant's own context, through these very ports.
// This gate is what keeps the difference.
func TestNoRepositoryMethodTakesATenant(t *testing.T) {
	forEachGoFile(t, []string{"../../core/application/repository"},
		func(path string, file *ast.File, fset *token.FileSet) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}

			ast.Inspect(file, func(node ast.Node) bool {
				method, ok := node.(*ast.Field)
				if !ok {
					return true
				}
				signature, ok := method.Type.(*ast.FuncType)
				if !ok || signature.Params == nil {
					return true
				}

				for _, parameter := range signature.Params.List {
					for _, name := range parameter.Names {
						if !namesATenant(name.Name) {
							continue
						}
						t.Errorf("%s:%d: %s takes %q - a repository method never takes a tenant, "+
							"because the transaction's own scope is what decides it (rule 3, ADR-0010)",
							rel(path), fset.Position(name.Pos()).Line, methodName(method), name.Name)
					}
				}
				return true
			})
		})
}

// namesATenant is deliberately generous: `tenant`, `tenantID`, `forTenant` are all the same
// mistake, and a parameter that has to be spelled around to get past this gate is one somebody
// will notice while spelling it.
func namesATenant(parameter string) bool {
	return strings.Contains(strings.ToLower(parameter), "tenant")
}

func methodName(method *ast.Field) string {
	if len(method.Names) == 0 {
		return "an embedded method"
	}
	return method.Names[0].Name
}
