// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"errors"
	"regexp"
	"testing"

	"golang.org/x/text/language"
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

// Every message in every embedded catalogue is inside the subset the renderer implements - the
// same subset the client implements, so a construct outside it refuses the file when the
// catalogue is loaded rather than printing braces at a recipient (i18n-l10n.md §3, M-02). The
// refusal is heard here, at build time; an operator's file meets it at start.
func TestEveryEmbeddedCatalogueLoadsAndIsWithinTheSubset(t *testing.T) {
	catalogues, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("a catalogue the binary carries does not load: %v", err)
	}
	if len(catalogues) < 2 {
		t.Fatalf("%d catalogues embedded, want the source and German at least", len(catalogues))
	}
}

func TestAMessageOutsideTheSubsetRefusesTheCatalogue(t *testing.T) {
	for _, message := range []string{
		`{"a.date": "Due {at, date, short}"}`,
		`{"a.number": "{n, number} entries"}`,
		`{"a.open": "An open {brace"}`,
		`{"a.stray": "a } stray"}`,
		`{"a.plural": "{n, plural, one{# item}}"}`,
	} {
		_, err := load([]byte(message), language.English)
		var syntaxErr *MessageSyntaxError
		if !errors.As(err, &syntaxErr) {
			t.Errorf("%s: loaded, want a refusal naming the construct (got %v)", message, err)
		}
	}
}

// The subset has no plural, and the catalogue must not fake one either. "schedule(s)" is a
// plural written by hand, read as a hedge by a person and as nothing by a translator; the shape the
// file uses instead puts the number last, after a colon ("Days left: {days}."), where it is right
// for one and for many. Found on the runs screen (issue 547) and once more beside it.
func TestTheSourceCatalogueDoesNotHedgePlurals(t *testing.T) {
	catalogue, err := LoadEnglish()
	if err != nil {
		t.Fatalf("loading the source catalogue: %v", err)
	}
	hedge := regexp.MustCompile(`[a-z]\(s\)`)
	for code, message := range catalogue.messages {
		if hedge.MatchString(message.pattern) {
			t.Errorf("%s hedges a plural with \"(s)\": %q - put the count last, after a colon", code, message.pattern)
		}
	}
}

func TestMessageRendering(t *testing.T) {
	catalogue, err := load([]byte(`{
		"a.plain":     "Nothing to fill in.",
		"a.one":       "The variable {variable} is not set.",
		"a.two":       "{variable} must be at least {minimum} characters long.",
		"a.repeated":  "{value} and {value} again.",
		"a.adjacent":  "{first}{second}",
		"a.untouched": "A literal {variable} with no parameters.",
		"a.plural":    "{count, plural, one{# entry} other{# entries}}"
	}`), language.English)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

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
		{"a plural", "a.plural", map[string]string{"count": "3"}, "3 entries", true},
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
	if _, err := load([]byte(`{"a.code": 7}`), language.English); err == nil {
		t.Error("a catalogue whose values are not strings loaded without complaint")
	}
}
