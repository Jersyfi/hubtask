// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/text/language"
)

// This file mirrors apps/webapp/src/lib/i18n/format.test.ts case for case - the same patterns,
// the same parameters, the same sentences - so that the two renderers cannot drift apart
// unnoticed (M-02). A case added on one side is added on the other, in the same order.

func formatIn(t *testing.T, pattern string, params map[string]string, locale string) string {
	t.Helper()
	nodes, err := parseMessage(pattern)
	if err != nil {
		t.Fatalf("parsing %q: %v", pattern, err)
	}
	return render(nodes, params, language.Make(locale))
}

func formatEN(t *testing.T, pattern string, params map[string]string) string {
	t.Helper()
	return formatIn(t, pattern, params, "en")
}

func TestACodeWithParametersRendersInEnglish(t *testing.T) {
	if got := formatEN(t, "Hello {name}", map[string]string{"name": "Ada"}); got != "Hello Ada" {
		t.Errorf("%q", got)
	}
	got := formatEN(t, "{actor} completed {item_title}",
		map[string]string{"actor": "Anna", "item_title": "Review the quote"})
	if got != "Anna completed Review the quote" {
		t.Errorf("%q", got)
	}
}

func TestAMissingParameterLeavesItsPlaceholderStanding(t *testing.T) {
	const pattern = "Something went wrong on our side. Reference: {request_id}"
	if got := formatEN(t, pattern, nil); got != pattern {
		t.Errorf("%q", got)
	}
}

func TestThePluralCategoriesAreTheLocalesNotEnglishs(t *testing.T) {
	const polish = "{n, plural, one{# zadanie} few{# zadania} many{# zadań} other{# zadania}}"
	for n, want := range map[string]string{"1": "1 zadanie", "3": "3 zadania", "5": "5 zadań", "22": "22 zadania"} {
		if got := formatIn(t, polish, map[string]string{"n": n}, "pl"); got != want {
			t.Errorf("pl %s: %q, want %q", n, got, want)
		}
	}

	// Arabic has all six, and `zero` is a category rather than the number nought - which is the
	// distinction a hand-rolled n == 0 check gets wrong.
	const arabic = "{n, plural, zero{صفر} one{واحد} two{اثنان} few{قليل} many{كثير} other{آخر}}"
	for n, want := range map[string]string{"0": "صفر", "2": "اثنان", "11": "كثير"} {
		if got := formatIn(t, arabic, map[string]string{"n": n}, "ar"); got != want {
			t.Errorf("ar %s: %q, want %q", n, got, want)
		}
	}
}

func TestAnExactMatchWinsOverItsCategoryAndAnOffsetMovesTheNumber(t *testing.T) {
	const pattern = "{n, plural, =0{Nobody has read it} one{# person has read it} other{# people have read it}}"
	for n, want := range map[string]string{"0": "Nobody has read it", "1": "1 person has read it", "7": "7 people have read it"} {
		if got := formatEN(t, pattern, map[string]string{"n": n}); got != want {
			t.Errorf("%s: %q, want %q", n, got, want)
		}
	}

	const withOffset = "{n, plural, offset:1 one{you and # other} other{you and # others}}"
	for n, want := range map[string]string{"2": "you and 1 other", "4": "you and 3 others"} {
		if got := formatEN(t, withOffset, map[string]string{"n": n}); got != want {
			t.Errorf("%s: %q, want %q", n, got, want)
		}
	}
}

func TestSelectordinalUsesTheOrdinalRulesWhichAreADifferentSet(t *testing.T) {
	const pattern = "{n, selectordinal, one{#st} two{#nd} few{#rd} other{#th}} attempt"
	for n, want := range map[string]string{"1": "1st attempt", "2": "2nd attempt", "11": "11th attempt", "22": "22nd attempt"} {
		if got := formatEN(t, pattern, map[string]string{"n": n}); got != want {
			t.Errorf("%s: %q, want %q", n, got, want)
		}
	}
}

func TestSelectChoosesByValueAndFallsBackToOther(t *testing.T) {
	const pattern = "{scope, select, hub{the hub} collection{the collection} other{this}}"
	if got := formatEN(t, pattern, map[string]string{"scope": "hub"}); got != "the hub" {
		t.Errorf("%q", got)
	}
	if got := formatEN(t, pattern, map[string]string{"scope": "work_package"}); got != "this" {
		t.Errorf("%q", got)
	}
	if got := formatEN(t, pattern, nil); got != "this" {
		t.Errorf("%q", got)
	}
}

func TestBranchesHoldWholeMessagesNotOnlyText(t *testing.T) {
	const nested = "{n, plural, one{{actor} added # label} other{{actor} added # labels}}"
	if got := formatEN(t, nested, map[string]string{"n": "1", "actor": "Anna"}); got != "Anna added 1 label" {
		t.Errorf("%q", got)
	}
	if got := formatEN(t, nested, map[string]string{"n": "3", "actor": "Anna"}); got != "Anna added 3 labels" {
		t.Errorf("%q", got)
	}
}

// The one case the two files differ on, deliberately: the client groups digits with
// Intl.NumberFormat, and this side writes a number as it was given (no number symbols here,
// milestone-0.8.0.md decision 8). The client's case is `1,234 tasks`.
func TestANumberIsWrittenAsItWasGiven(t *testing.T) {
	if got := formatEN(t, "{count} tasks", map[string]string{"count": "1234"}); got != "1234 tasks" {
		t.Errorf("%q", got)
	}
	if got := formatIn(t, "{n, plural, one{# Aufgabe} other{# Aufgaben}}", map[string]string{"n": "1234"}, "de"); got != "1234 Aufgaben" {
		t.Errorf("%q", got)
	}
}

func TestApostrophesFollowICUWhichIsTheRuleEnglishTripsOver(t *testing.T) {
	for pattern, want := range map[string]string{
		"the item's owner":      "the item's owner",
		"'{'not an argument'}'": "{not an argument}",
		"it''s done":            "it's done",
	} {
		if got := formatEN(t, pattern, nil); got != want {
			t.Errorf("%q rendered %q, want %q", pattern, got, want)
		}
	}
}

func TestSyntaxThisRendererDoesNotImplementIsRefusedByName(t *testing.T) {
	for pattern, want := range map[string]string{
		"{n, number}":                       "`{n, number}` is not implemented",
		"{d, date, short}":                  "`{d, date}` is not implemented",
		"{n, plural, one{#}}":               "no `other` branch",
		"{n, plural, singular{#} other{#}}": "not a CLDR plural category",
		"{name":                             "not closed",
		"a } stray":                         "with no `{` before it",
	} {
		_, err := parseMessage(pattern)
		var syntaxErr *MessageSyntaxError
		if !errors.As(err, &syntaxErr) {
			t.Errorf("%q: accepted, want a refusal", pattern)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q: refused with %q, want it to name %q", pattern, err, want)
		}
	}
}

// What the port hands over is strings; a plural's operand that is not a number leaves the
// placeholder standing, like an absent one, rather than choosing a branch at random.
func TestAPluralOperandThatIsNotANumberLeavesThePlaceholder(t *testing.T) {
	const pattern = "{n, plural, one{# item} other{# items}}"
	if got := formatEN(t, pattern, map[string]string{"n": "many"}); got != "{n}" {
		t.Errorf("%q", got)
	}
	if got := formatEN(t, pattern, map[string]string{"n": "1.5"}); got != "1.5 items" {
		t.Errorf("%q", got)
	}
}
