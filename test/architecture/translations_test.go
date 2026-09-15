// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

// What a second catalogue is held to (M-03, i18n-l10n.md §3): for every catalogue that is not
// the source, a key the source does not have fails - a translation of a message that was renamed
// or removed is a translation of nothing - and the arguments a translation takes are exactly the
// ones its source takes: fewer is a value that was lost, more is a placeholder nobody fills. A key
// the source has and the translation lacks is reported by family and does not fail, because a
// half-translated file is the normal state of a translation and the fallback is a feature.
//
// Every message parsing under the subset, and no key written twice, are the loader's own
// refusals: LoadEmbedded fails the whole gate before a comparison is made.
func TestEveryTranslationIsHeldToTheSource(t *testing.T) {
	catalogues, err := i18n.LoadEmbedded()
	if err != nil {
		t.Fatalf("a catalogue the binary carries does not load: %v", err)
	}
	source := catalogues[i18n.SourceLocale]

	for _, tag := range sortedTags(catalogues) {
		if tag == i18n.SourceLocale {
			continue
		}
		translation := catalogues[tag]
		report := coverageOf(source, translation)

		for _, code := range report.unknown {
			t.Errorf("%s.json translates %s, which the source catalogue does not have", tag, code)
		}
		for _, mismatch := range report.mismatched {
			t.Errorf("%s.json: %s takes {%s}, the source takes {%s}", tag, mismatch.code,
				strings.Join(mismatch.got, "} {"), strings.Join(mismatch.want, "} {"))
		}
		t.Logf("%s.json: %d of %d keys translated (%s)", tag, report.translated, source.Len(),
			report.families())
	}
}

type argumentMismatch struct {
	code      string
	got, want []string
}

type coverage struct {
	translated int
	unknown    []string
	mismatched []argumentMismatch
	// byFamily counts translated and total per first key segment, for the report.
	byFamily map[string][2]int
}

// families renders the per-family line: complete families by name, partial ones with their
// count, missing ones left out - the report is what a translator reads next.
func (c coverage) families() string {
	names := make([]string, 0, len(c.byFamily))
	for name, counts := range c.byFamily {
		if counts[0] > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		counts := c.byFamily[name]
		if counts[0] == counts[1] {
			parts = append(parts, name)
		} else {
			parts = append(parts, fmt.Sprintf("%s %d/%d", name, counts[0], counts[1]))
		}
	}
	return strings.Join(parts, ", ")
}

// coverageOf compares one translation against the source, key by key.
func coverageOf(source, translation i18n.Catalogue) coverage {
	report := coverage{byFamily: map[string][2]int{}}
	for _, code := range source.Codes() {
		family, _, _ := strings.Cut(code, ".")
		counts := report.byFamily[family]
		counts[1]++
		if translation.Has(code) {
			counts[0]++
		}
		report.byFamily[family] = counts
	}
	for _, code := range translation.Codes() {
		pattern, _ := translation.Pattern(code)
		original, known := source.Pattern(code)
		if !known {
			report.unknown = append(report.unknown, code)
			continue
		}
		report.translated++
		got, _ := i18n.Arguments(pattern)
		want, _ := i18n.Arguments(original)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			report.mismatched = append(report.mismatched, argumentMismatch{code: code, got: got, want: want})
		}
	}
	return report
}

func sortedTags(catalogues map[string]i18n.Catalogue) []string {
	tags := make([]string, 0, len(catalogues))
	for tag := range catalogues {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}
