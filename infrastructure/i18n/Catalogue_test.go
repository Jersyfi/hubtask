// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"regexp"
	"strings"
	"testing"
)

func TestTheSourceCatalogueLoads(t *testing.T) {
	catalogue, err := LoadEnglish()
	if err != nil {
		t.Fatalf("loading the source catalogue: %v", err)
	}

	// One code from each family the catalogue serves, so that a wholesale rename shows up here.
	for _, code := range []string{"errors.not_found", "access.credential_required", "config.db_dsn_missing"} {
		if !catalogue.Has(code) {
			t.Errorf("the source catalogue does not know %s", code)
		}
	}
	if catalogue.Has("_comment") {
		t.Error("the note to the translators is offered as a message")
	}
}

// The renderer implements the simple-argument subset of ICU MessageFormat. This is what keeps
// that honest: the day a message needs a plural or a select, this test fails and whoever adds it
// has to teach the renderer rather than watch it print braces at a user.
func TestTheSourceCatalogueStaysWithinTheSubset(t *testing.T) {
	catalogue, err := LoadEnglish()
	if err != nil {
		t.Fatalf("loading the source catalogue: %v", err)
	}

	// A simple argument is `{name}` and nothing else - no comma, no nested brace, no format style.
	simpleArgument := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for code, message := range catalogue.messages {
		rest := message
		for {
			_, after, found := strings.Cut(rest, "{")
			if !found {
				break
			}
			name, tail, closed := strings.Cut(after, "}")
			if !closed {
				t.Errorf("%s: an unterminated { in %q", code, message)
				break
			}
			if !simpleArgument.MatchString(name) {
				t.Errorf("%s: %q is not a simple argument - the renderer would print it as text", code, "{"+name+"}")
			}
			rest = tail
		}
	}
}

func TestMessageRendering(t *testing.T) {
	catalogue := Catalogue{messages: map[string]string{
		"a.plain":     "Nothing to fill in.",
		"a.one":       "The variable {variable} is not set.",
		"a.two":       "{variable} must be at least {minimum} characters long.",
		"a.repeated":  "{value} and {value} again.",
		"a.unclosed":  "An open {brace",
		"a.adjacent":  "{first}{second}",
		"a.untouched": "A literal {variable} with no parameters.",
	}}

	for _, tc := range []struct {
		name   string
		code   string
		params map[string]string
		want   string
		known  bool
	}{
		{"a message without placeholders", "a.plain", nil, "Nothing to fill in.", true},
		{"one placeholder", "a.one", map[string]string{"variable": "HUBTASK_DB_DSN"},
			"The variable HUBTASK_DB_DSN is not set.", true},
		{"two placeholders", "a.two", map[string]string{"variable": "KEY", "minimum": "32"},
			"KEY must be at least 32 characters long.", true},
		{"the same placeholder twice", "a.repeated", map[string]string{"value": "x"}, "x and x again.", true},
		{"a parameter nobody asked for", "a.one", map[string]string{"variable": "K", "spare": "s"},
			"The variable K is not set.", true},
		{"a placeholder with no parameter", "a.two", map[string]string{"variable": "KEY"},
			"KEY must be at least {minimum} characters long.", true},
		{"an unterminated brace", "a.unclosed", map[string]string{"brace": "x"}, "An open {brace", true},
		{"adjacent placeholders", "a.adjacent", map[string]string{"first": "1", "second": "2"}, "12", true},
		{"no parameters at all", "a.untouched", nil, "A literal {variable} with no parameters.", true},
		{"an unknown code renders as itself", "a.missing", nil, "a.missing", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, known := catalogue.Message(tc.code, tc.params)
			if got != tc.want {
				t.Errorf("message %q, want %q", got, tc.want)
			}
			if known != tc.known {
				t.Errorf("known %v, want %v", known, tc.known)
			}
		})
	}
}

func TestABrokenCatalogueIsAnError(t *testing.T) {
	if _, err := load([]byte(`{"a.code": 7}`)); err == nil {
		t.Error("a catalogue whose values are not strings loaded without complaint")
	}
}
