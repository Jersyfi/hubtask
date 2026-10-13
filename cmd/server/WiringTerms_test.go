// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// The TERMS step (UC-ID-19) is the step-up's trap again: every writer that opens a session - the
// reset's, the provider's, the second factor's - is a copy of the session writer, so the terms in
// force and the agreements have to be in its own literal. Assigned after it, a copy taken in between
// would open sessions without ever asking for the terms.
func TestTheTermsAreInTheSessionWritersOwnLiteral(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing main.go: %v", err)
	}
	var literal *ast.CompositeLit
	late := map[string]bool{}
	ast.Inspect(parsed, func(node ast.Node) bool {
		statement, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, target := range statement.Lhs {
			switch name := exprString(target); name {
			case "sessionWriter.TermsIn", "sessionWriter.Terms":
				late[name] = true
			case "sessionWriter":
				if statement.Tok == token.DEFINE && i < len(statement.Rhs) {
					if found, ok := statement.Rhs[i].(*ast.CompositeLit); ok {
						literal = found
					}
				}
			}
		}
		return true
	})
	for name := range late {
		t.Errorf("%s is assigned after the literal - the copies taken before miss it", name)
	}
	if literal == nil {
		t.Fatal("main.go builds no `sessionWriter := identity.SessionWriter{...}` literal")
	}
	held := map[string]bool{}
	for _, element := range literal.Elts {
		if field, ok := element.(*ast.KeyValueExpr); ok {
			held[exprString(field.Key)] = true
		}
	}
	for _, want := range []string{"TermsIn", "Terms"} {
		if !held[want] {
			t.Errorf("the session writer's literal holds no %s: no sign-in would meet the terms", want)
		}
	}
}
