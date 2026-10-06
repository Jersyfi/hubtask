// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The parameter names are the catalogue's. A sentence whose placeholder nobody fills is a sentence
// with a hole in it, and the catalogue spells the length's `minimum` and everything else's `count`.
func TestAViolationCarriesTheParameterItsSentenceTakes(t *testing.T) {
	policy := PasswordPolicy{MinLength: 20, MinDigits: 2, MinClasses: 3, MaxRepeat: 1}
	violations := LocalPasswordViolations(policy, text.Composing{}, "aa", PasswordContext{})

	want := map[PasswordRuleID]map[string]string{
		RuleMinLength:  {"minimum": "20"},
		RuleMinDigits:  {"count": "2"},
		RuleMinClasses: {"count": "3"},
		RuleMaxRepeat:  {"count": "1"},
	}
	if len(violations) != len(want) {
		t.Fatalf("violations %v, want %d of them", violations, len(want))
	}
	for _, violation := range violations {
		if !reflect.DeepEqual(violation.Params, want[violation.Rule]) {
			t.Errorf("%s carried %v, want %v", violation.Rule, violation.Params, want[violation.Rule])
		}
	}
}

// One field error per rule, all against the password itself: the field is the password, and the
// list under it is what says which rule refused (milestone decision 7).
func TestARefusedPasswordNamesEveryRuleAgainstTheOneField(t *testing.T) {
	err := PasswordRefused([]PasswordViolation{
		{Rule: RuleMinLength, Params: map[string]string{"minimum": "12"}},
		{Rule: RuleCommon},
	})
	fields := fieldsOf(t, err)
	if len(fields) != 2 {
		t.Fatalf("fields %v, want two", fields)
	}
	for _, field := range fields {
		if field.Path != "/password" {
			t.Errorf("field path %q, want /password", field.Path)
		}
	}
	if fields[0].Code != "auth.password_rule.min_length" || fields[1].Code != "auth.password_rule.common" {
		t.Errorf("codes %q and %q", fields[0].Code, fields[1].Code)
	}
	if PasswordRefused(nil) != nil {
		t.Error("no violations is no refusal")
	}
}

// The ceiling is not a policy switch: a paste of a whole document is a validation error, and it is
// reported alone rather than beside "at least twelve characters".
func TestTheCeilingIsItsOwnRefusal(t *testing.T) {
	long := make([]byte, MaxPasswordLength+1)
	for index := range long {
		long[index] = 'a'
	}
	err := CheckPasswordAgainst(DefaultSignInPolicy().Password, text.Composing{}, string(long),
		PasswordContext{})
	fields := fieldsOf(t, err)
	if len(fields) != 1 || fields[0].Code != "auth.password_too_long" {
		t.Fatalf("the ceiling answered %v", err)
	}
}

// CheckPassword keeps its name and its meaning for every caller that had one (ADR-0068 §2).
func TestCheckPasswordStillAnswersTheProductDefault(t *testing.T) {
	if err := CheckPassword("a long enough password"); err != nil {
		t.Errorf("a conforming password answered %v", err)
	}
	if CheckPassword("short") == nil {
		t.Error("a short password was accepted")
	}
}

// Context words: the product's name is always one, a short fragment never is, and the fold makes
// the comparison about what a password is rather than how it is spelled.
func TestContextWordsAreFlattenedAndBounded(t *testing.T) {
	words := ContextWords(text.Composing{}, PasswordContext{
		Email:         "anna@contoso.example",
		DisplayName:   "Anna Bo Berger",
		WorkspaceName: "Contoso Ltd",
		WorkspaceHost: "contoso.hubtask.example",
	})
	held := map[string]bool{}
	for _, word := range words {
		held[word] = true
	}
	// The address is folded like everything else, `@` into `a` among the rest: the rule is about
	// what a password is, not how it is spelled, and a comparison that kept the punctuation would
	// miss `anna(at)contoso.example`.
	for _, expected := range []string{"hubtask", "anna", "annaacontoso.example", "berger", "contoso"} {
		if !held[expected] {
			t.Errorf("%q is not a context word, want it in %v", expected, words)
		}
	}
	if held["bo"] || held["ltd"] {
		t.Errorf("a short fragment became a context word: %v", words)
	}
}

// fieldsOf reads the field errors off a refusal, which is what a client's list is built from.
func fieldsOf(t *testing.T, err error) []shared.FieldError {
	t.Helper()
	var refusal *shared.Error
	if !errors.As(err, &refusal) {
		t.Fatalf("not a domain error: %v", err)
	}
	return refusal.Fields
}
