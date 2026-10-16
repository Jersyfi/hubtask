// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// A passkey counts as a second factor wherever turning the app off and the codes ask (identity.md
// §23.2), and both read it from the session writer - of which every door holds a copy. A field set
// after the literal reaches none of the copies taken before (known traps: writer copies).
func TestThePasskeysAreInTheSessionWritersOwnLiteral(t *testing.T) {
	fields, late := literalOf(t, "sessionWriter")
	if late["sessionWriter.Keys"] {
		t.Error("sessionWriter.Keys is assigned after the literal - the copies taken before miss it")
	}
	if !fields["Keys"] {
		t.Error("the session writer's literal counts no passkeys")
	}
}

// Adding and removing a passkey tell the person (ADR-0078 §6) through the writer's own WayIn, which
// only its literal can carry to the use cases built from it.
func TestThePasskeyWriterTellsOfAChangedWayIn(t *testing.T) {
	fields, late := literalOf(t, "passkeyWriter")
	if !fields["WayIn"] || late["passkeyWriter.WayIn"] {
		t.Error("the passkey writer's literal tells nobody of a passkey added or removed")
	}
}

// literalOf answers the keys of the composite literal `name := …{…}` in main.go, and every field of
// it assigned anywhere afterwards.
func literalOf(t *testing.T, name string) (fields, late map[string]bool) {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing main.go: %v", err)
	}
	fields, late = map[string]bool{}, map[string]bool{}
	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		statement, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, target := range statement.Lhs {
			if selector := exprString(target); selector != name {
				late[selector] = true
				continue
			}
			if statement.Tok != token.DEFINE || i >= len(statement.Rhs) {
				continue
			}
			literal, ok := statement.Rhs[i].(*ast.CompositeLit)
			if !ok {
				continue
			}
			found = true
			for _, element := range literal.Elts {
				if pair, ok := element.(*ast.KeyValueExpr); ok {
					fields[exprString(pair.Key)] = true
				}
			}
		}
		return true
	})
	if !found {
		t.Fatalf("main.go builds no `%s := …{…}` literal", name)
	}
	return fields, late
}
