// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/identity"
)

// The writers are values, so a copy taken before the sign-in path learns the rule never learns it.
// The password writer's copy (which a reset opens its session through) and the provider's copy
// (which a provider sign-in and the LINK step open theirs through) were both taken before, so a
// reset read the old boolean instead of the rule and a provider session answered to no session
// bound in production - while every test, which wires its own fixtures, passed (SC-03, SC-06).
func TestTheRuleReachesEveryWriterThatOpensASession(t *testing.T) {
	session := identity.SessionWriter{}
	passwords := identity.PasswordWriter{Session: session}
	oidc := identity.OidcWriter{Session: session}

	teachTheRule(&session, &passwords, &oidc)

	for name, rule := range map[string]identity.SignInRuleReader{
		"the sign-in path":         session.Rule,
		"the reset's session copy": passwords.Session.Rule,
		"the provider's copy":      oidc.Session.Rule,
	} {
		if rule == nil {
			t.Errorf("%s opens sessions without the rule", name)
		}
	}
}

// The step-up at the provider (ADR-0075 §2) is the same trap the other way round: every verifier a
// privileged operation holds is a copy of the session writer, so the stores PROVIDER needs have to
// be in the writer's own literal. Assigned after it, the copies taken in between would neither name
// PROVIDER in a refusal nor prove it - and a provider-only administrator would meet a dialog with
// nothing in it on exactly the operations whose copy came first.
func TestTheProviderStepUpIsInTheSessionWritersOwnLiteral(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing main.go: %v", err)
	}
	var literal *ast.CompositeLit
	late := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch statement := node.(type) {
		case *ast.AssignStmt:
			for i, target := range statement.Lhs {
				name := exprString(target)
				if name == "sessionWriter.StepUpProviders" {
					late = true
				}
				if name == "sessionWriter" && statement.Tok == token.DEFINE && i < len(statement.Rhs) {
					if found, ok := statement.Rhs[i].(*ast.CompositeLit); ok {
						literal = found
					}
				}
			}
		}
		return true
	})
	if late {
		t.Error("sessionWriter.StepUpProviders is assigned after the literal - the copies taken before miss it")
	}
	if literal == nil {
		t.Fatal("main.go builds no `sessionWriter := identity.SessionWriter{...}` literal")
	}
	stores := map[string]bool{}
	for _, element := range literal.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok || exprString(field.Key) != "StepUpProviders" {
			continue
		}
		inner, ok := field.Value.(*ast.CompositeLit)
		if !ok {
			t.Fatal("StepUpProviders is not built in place")
		}
		for _, store := range inner.Elts {
			if pair, ok := store.(*ast.KeyValueExpr); ok {
				stores[exprString(pair.Key)] = true
			}
		}
	}
	for _, want := range []string{"Providers", "Flows", "External", "Workspaces", "Relying", "RedirectURL"} {
		if !stores[want] {
			t.Errorf("the session writer's literal gives the step-up at the provider no %s", want)
		}
	}
}

func exprString(expression ast.Expr) string {
	switch e := expression.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	default:
		return ""
	}
}

// The rule teachTheRule hands out is the password writer as main.go builds it, so a field the
// sign-in's verdict needs has to be in that literal - a test fixture that wires its own never shows
// it missing. WaysIn is ADR-0076 §4's fallback: without it the card offers the password back and the
// trail never records who signed in through it.
func TestThePasswordWriterCarriesTheWaysIn(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	found, carries := false, false
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		selector, ok := literal.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "PasswordWriter" {
			return true
		}
		found = true
		for _, element := range literal.Elts {
			if pair, ok := element.(*ast.KeyValueExpr); ok {
				if key, ok := pair.Key.(*ast.Ident); ok && key.Name == "WaysIn" {
					carries = true
				}
			}
		}
		return true
	})
	if !found {
		t.Fatal("main.go builds no identity.PasswordWriter")
	}
	if !carries {
		t.Error("main.go builds the password writer without WaysIn: no workspace falls back to the password")
	}
}
