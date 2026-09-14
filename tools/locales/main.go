// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command locales reports how complete each translation is (M-03, ADR-0055).
//
// One command rather than a diff: a contributor runs it before a pull request, and the number it
// prints is the number the gate in test/architecture computes - both read the catalogues through
// the same loader, so the report cannot say one thing and the gate another. What is wrong with a
// translation - an unknown key, a placeholder that does not match, a message outside the subset -
// is the gate's to refuse; this only says how far a translation has got and where.
//
// It reads the embedded catalogues, which is what ships. A file laid over them through
// HUBTASK_LOCALE_DIR is an installation's and not the repository's, and is not reported here.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

func main() {
	catalogues, err := i18n.LoadEmbedded()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	source := catalogues[i18n.SourceLocale]
	fmt.Printf("%s.json  %d keys (the source)\n", i18n.SourceLocale, source.Len())

	tags := make([]string, 0, len(catalogues))
	for tag := range catalogues {
		if tag != i18n.SourceLocale {
			tags = append(tags, tag)
		}
	}
	sort.Strings(tags)
	if len(tags) == 0 {
		fmt.Println("no translation yet - a translation is locales/<tag>.json (CONTRIBUTING.md)")
		return
	}

	for _, tag := range tags {
		report(tag, source, catalogues[tag])
	}
}

// report prints one translation: the total, then every family of the source with how many of
// its keys the translation carries - complete families first, then partial, then the missing
// ones, so that the next thing to translate is the last thing on the screen.
func report(tag string, source, translation i18n.Catalogue) {
	type family struct {
		name        string
		have, total int
	}
	byName := map[string]*family{}
	translated := 0
	for _, code := range source.Codes() {
		name, _, _ := strings.Cut(code, ".")
		f := byName[name]
		if f == nil {
			f = &family{name: name}
			byName[name] = f
		}
		f.total++
		if translation.Has(code) {
			f.have++
			translated++
		}
	}
	families := make([]*family, 0, len(byName))
	for _, f := range byName {
		families = append(families, f)
	}
	sort.Slice(families, func(i, j int) bool {
		ci, cj := completeness(families[i].have, families[i].total), completeness(families[j].have, families[j].total)
		if ci != cj {
			return ci > cj
		}
		return families[i].name < families[j].name
	})

	fmt.Printf("\n%s.json  %d of %d keys (%d%%)\n", tag, translated, source.Len(),
		100*translated/max(source.Len(), 1))
	for _, f := range families {
		state := "missing"
		switch {
		case f.have == f.total:
			state = "complete"
		case f.have > 0:
			state = "partial"
		}
		fmt.Printf("  %-22s %4d / %-4d %s\n", f.name, f.have, f.total, state)
	}
}

func completeness(have, total int) int {
	switch {
	case have == total:
		return 2
	case have > 0:
		return 1
	default:
		return 0
	}
}
