// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

// PG-4: `PERSONAL_CONTENT` does not appear in logs, metrics, traces, audit `changes` or error
// responses (data-protection.md §10, rule 10).
//
// It is checked by the *name of the attribute* rather than by the value, because a value is not
// there to be read at build time and a name is. Every one of these carries a key that says what it
// is: `slog.String("title", …)`, `attribute.String("email", …)`, `WithParams(map[string]string{"body":
// …})`. A key from the content vocabulary is either a leak or a name that should have been chosen
// more carefully - and both are worth stopping, because the second becomes the first when somebody
// fills it in later.
//
// The audit `changes` half of §10 is PG-1's: a change carries a classification, and the masking
// derived from it decides what is written.

// contentKeys are the attribute names that carry what somebody wrote or who they are. Deliberately
// short: a list that tried to be exhaustive would be a list nobody keeps up, and these are the
// names a person reaches for when they are about to log the wrong thing.
var contentKeys = map[string]string{
	"title":        "an entry's title is PERSONAL_CONTENT",
	"notes":        "the notes of an entry are PERSONAL_CONTENT",
	"body":         "a comment's body is PERSONAL_CONTENT",
	"comment":      "a comment is PERSONAL_CONTENT",
	"description":  "a description is PERSONAL_CONTENT",
	"email":        "an address is PERSONAL_BASIC",
	"display_name": "a name is PERSONAL_BASIC",
	"actor_label":  "the actor's label is PERSONAL_BASIC and belongs in the trail alone",
	"subject":      "a subject line is PERSONAL_CONTENT",
	"raw_body":     "an intake body is PERSONAL_CONTENT",
	"password":     "a credential is SECRET",
	"token":        "a credential is SECRET",
	"secret":       "a credential is SECRET",
	"credential":   "a credential is SECRET",
}

// carriers are the calls whose first string argument is an attribute name. Three families: the
// structured logger, the metric and trace attributes, and the problem document's parameters.
var carriers = map[string]bool{
	"slog.String": true, "slog.Any": true, "slog.Int": true, "slog.Bool": true,
	"attribute.String": true, "attribute.Int": true, "attribute.Bool": true,
}

func TestPG4NoContentReachesALogAMetricOrAnError(t *testing.T) {
	checked := 0

	forEachGoFile(t, func(path string, file *ast.File, fset *token.FileSet) {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || !carriers[pkg.Name+"."+selector.Sel.Name] {
				return true
			}

			checked++
			key, ok := stringLiteral(call.Args[0])
			if !ok {
				return true
			}
			if reason, forbidden := contentKeys[key]; forbidden {
				t.Errorf("%s: %q reaches a log, a metric or a trace (PG-4) - %s",
					at(path, fset, call), key, reason)
			}
			return true
		})
	})

	if checked == 0 {
		t.Fatal("PG-4 found no attribute at all - the pattern no longer matches")
	}
	t.Logf("PG-4 checked %d attributes", checked)
}

// The problem document is the other place a value leaves: `WithParams` renders into the message a
// client is shown, and a message code with a title in its parameters is content on the wire.
func TestPG4NoContentReachesAProblemDocument(t *testing.T) {
	forEachGoFile(t, func(path string, file *ast.File, fset *token.FileSet) {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "WithParams" {
				return true
			}

			literal, ok := call.Args[0].(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, element := range literal.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := stringLiteral(pair.Key)
				if !ok {
					continue
				}
				if reason, forbidden := contentKeys[key]; forbidden {
					t.Errorf("%s: %q is a parameter of a problem document (PG-4) - %s",
						at(path, fset, pair), key, reason)
				}
			}
			return true
		})
	})
}

func stringLiteral(expr ast.Expr) (string, bool) {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	return strings.Trim(literal.Value, "`\""), true
}
