// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import (
	"strings"
	"testing"
)

func TestLanguageTagAcceptsWhatBCP47Writes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"a language", "de", "de"},
		{"a language and a region", "de-AT", "de-AT"},
		{"a language and a script", "zh-Hans", "zh-Hans"},
		{"a numeric region", "es-419", "es-419"},
		{"surrounded by space, as a form field arrives", "  pt-BR  ", "pt-BR"},
		// Not stated is a state, and the one every entry starts in.
		{"nothing at all", "", ""},
		{"nothing but space", "   ", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tag, ok := LanguageTag(test.raw)
			if !ok {
				t.Fatalf("%q was refused", test.raw)
			}
			if tag != test.want {
				t.Errorf("%q became %q, want %q", test.raw, tag, test.want)
			}
		})
	}
}

func TestLanguageTagRefusesWhatIsNotATag(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"a sentence", "German, mostly"},
		{"a digit in the language subtag", "d3"},
		{"an empty subtag", "de-"},
		{"a subtag beyond eight characters", "de-Latinised"},
		{"punctuation", "de_AT"},
		{"longer than any tag anybody writes", strings.Repeat("de-", 20)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := LanguageTag(test.raw); ok {
				t.Errorf("%q was accepted", test.raw)
			}
		})
	}
}

// The check is structural and nothing else. Which languages this product has translations for is a
// catalogue, and which ones its PostgreSQL can index is the installation's - a tag that is neither
// is still a tag, and refusing it here would deny a workspace the right to say what it writes in.
func TestALanguageNobodySupportsIsStillAWellFormedTag(t *testing.T) {
	for _, tag := range []string{"cy", "kl", "haw"} {
		if _, ok := LanguageTag(tag); !ok {
			t.Errorf("%q was refused for being unsupported rather than for being malformed", tag)
		}
	}
}
