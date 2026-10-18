// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// UC-INS-10 check 5: every door that creates a person is wired with the people limit. A port left
// nil enforces none, so a door the composition root forgot would admit anybody - the trap of a
// late field on a copied literal. Read from the composition root's own source, literal by literal.
func TestEveryDoorThatCreatesAPersonIsWiredWithThePeopleLimit(t *testing.T) {
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the composition root: %v", err)
	}
	files := token.NewFileSet()
	doors := map[string]int{"InviteAccount": 0, "RenewInvitation": 0, "CreateManagedAccount": 0, "OidcWriter": 0}
	inspect := func(node ast.Node) bool {
		literal, isLiteral := node.(*ast.CompositeLit)
		if !isLiteral {
			return true
		}
		selector, isSelector := literal.Type.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		if _, isDoor := doors[selector.Sel.Name]; !isDoor {
			return true
		}
		for _, element := range literal.Elts {
			if field, isField := element.(*ast.KeyValueExpr); isField {
				if key, isIdent := field.Key.(*ast.Ident); isIdent && key.Name == "People" {
					if value, isIdent := field.Value.(*ast.Ident); isIdent && value.Name == "nil" {
						break
					}
					doors[selector.Sel.Name]++
					return true
				}
			}
		}
		t.Errorf("a %s literal is built without People", selector.Sel.Name)
		return true
	}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(files, name, nil, 0)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		ast.Inspect(file, inspect)
	}
	for door, wired := range doors {
		if wired == 0 {
			t.Errorf("no %s is wired with the people limit", door)
		}
	}
}
