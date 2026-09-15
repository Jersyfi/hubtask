// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"log/slog"
	"strings"
	"testing"
)

// The rows against the browser's own answers, recorded here as the source (M-05, ADR-0056): for
// each tag, `new Intl.Locale(tag).getWeekInfo().firstDay` (1 is Monday, 6 Saturday, 7 Sunday) and
// the `decimal` part of `new Intl.NumberFormat(tag).formatToParts(1.5)`, read from Node 24's ICU
// on 2026-09-15:
//
//	en 7 "."   zh-Hans 1 "."   hi 7 "."   es 1 ","   ar 6 "."   fr 1 ","
//	bn 7 "."   pt 7 ","        ru 1 ","   id 7 ","   de 1 ","
//
// A row that disagrees with this list is a row the client would contradict.
func TestTheLocaleTableAgreesWithIntl(t *testing.T) {
	intl := map[string]struct {
		firstDay int
		decimal  string
	}{
		"en": {7, "."}, "zh-hans": {1, "."}, "hi": {7, "."}, "es": {1, ","}, "ar": {6, "."},
		"fr": {1, ","}, "bn": {7, "."}, "pt": {7, ","}, "ru": {1, ","}, "id": {7, ","}, "de": {1, ","},
	}
	weekday := map[int]string{1: "MONDAY", 6: "SATURDAY", 7: "SUNDAY"}

	if len(intl) != len(weekAndDecimal) {
		t.Fatalf("the table has %d rows and the recorded Intl answers %d", len(weekAndDecimal), len(intl))
	}
	for tag, want := range intl {
		row, known := weekAndDecimal[tag]
		if !known {
			t.Errorf("%s: no row in the table", tag)
			continue
		}
		if row.week != weekday[want.firstDay] || row.decimal != want.decimal {
			t.Errorf("%s: the table says %s %q, Intl says %s %q", tag, row.week, row.decimal,
				weekday[want.firstDay], want.decimal)
		}
	}
}

// The direction is the script's, inferred where the tag does not state one, so that a language
// the table does not know is still told which way it runs.
func TestTheDirectionIsTheScripts(t *testing.T) {
	for tag, want := range map[string]string{
		"ar": "rtl", "ar-EG": "rtl", "he": "rtl", "fa": "rtl", "ur": "rtl", "dv": "rtl",
		"en": "ltr", "de": "ltr", "zh-Hans": "ltr", "hi": "ltr", "ja": "ltr",
	} {
		if got := localeInfo(strings.ToLower(tag)).Direction; got != want {
			t.Errorf("%s runs %s, want %s", tag, got, want)
		}
	}
}

// The manifest's rows: one per catalogue present, the source first, the tag as the file named
// it, and honest defaults for a locale the table does not know - plus the line in the log.
func TestSupportedLocalesIsOneRowPerCatalogue(t *testing.T) {
	embedded := directory(t, map[string]string{
		"en.json":      `{"a.one": "One"}`,
		"ar.json":      `{"a.one": "واحد"}`,
		"zh-Hans.json": `{"a.one": "一"}`,
		"cy.json":      `{"a.one": "Un"}`,
	})
	renderer, err := newRenderer(embedded, nil)
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}

	rows := renderer.SupportedLocales()
	want := []LocaleInfo{
		{Tag: "en", Direction: "ltr", WeekStart: "SUNDAY", DecimalSeparator: "."},
		{Tag: "ar", Direction: "rtl", WeekStart: "SATURDAY", DecimalSeparator: "."},
		{Tag: "cy", Direction: "ltr", WeekStart: "MONDAY", DecimalSeparator: "."},
		{Tag: "zh-Hans", Direction: "ltr", WeekStart: "MONDAY", DecimalSeparator: "."},
	}
	if len(rows) != len(want) {
		t.Fatalf("%d rows, want %d: %v", len(rows), len(want), rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}

	var log strings.Builder
	renderer.LogUnknownLocales(slog.New(slog.NewTextHandler(&log, nil)))
	if !strings.Contains(log.String(), "locale=cy") || strings.Contains(log.String(), "locale=ar") {
		t.Errorf("the log names the wrong locales:\n%s", log.String())
	}
}

// The locale's week, through the same matcher a message is rendered through: de-AT reads de's
// row, pt-BR reads pt's, and a tag that lands nowhere starts on Monday (M-06).
func TestWeekStartOfReadsTheRowTheTagLandsOn(t *testing.T) {
	embedded := directory(t, map[string]string{
		"en.json": `{"a.one": "One"}`,
		"de.json": `{"a.one": "Eins"}`,
		"ar.json": `{"a.one": "واحد"}`,
		"pt.json": `{"a.one": "Um"}`,
		"cy.json": `{"a.one": "Un"}`,
	})
	renderer, err := newRenderer(embedded, nil)
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}
	for locale, want := range map[string]string{
		"de": "MONDAY", "de-AT": "MONDAY", "ar": "SATURDAY", "ar-EG": "SATURDAY",
		"pt-BR": "SUNDAY", "en": "SUNDAY", "en-GB": "SUNDAY",
		"cy": "MONDAY", "fr": "MONDAY", "": "MONDAY", "nonsense tag": "MONDAY",
	} {
		if got := renderer.WeekStartOf(locale); got != want {
			t.Errorf("%q starts on %s, want %s", locale, got, want)
		}
	}
}
