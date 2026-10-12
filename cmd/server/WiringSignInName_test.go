// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// The session writer folds a typed sign-in name with its Text normaliser (ADR-0074). Every door is a
// copy of the writer, so the field has to be in the literal that builds it: a copy taken before a
// later assignment would compare names unfolded, and "Lena" would never find "lena".
func TestTheSessionWriterFoldsSignInNamesFromItsLiteral(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing main.go: %v", err)
	}
	carries, late := false, false
	ast.Inspect(parsed, func(node ast.Node) bool {
		statement, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, target := range statement.Lhs {
			if exprString(target) == "sessionWriter.Text" {
				late = true
			}
			if exprString(target) != "sessionWriter" || statement.Tok != token.DEFINE || i >= len(statement.Rhs) {
				continue
			}
			literal, ok := statement.Rhs[i].(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, element := range literal.Elts {
				if pair, ok := element.(*ast.KeyValueExpr); ok && exprString(pair.Key) == "Text" {
					carries = true
				}
			}
		}
		return true
	})
	if late {
		t.Error("sessionWriter.Text is assigned after the literal - the copies taken before miss it")
	}
	if !carries {
		t.Error("the session writer's literal carries no Text normaliser for sign-in names")
	}
}
