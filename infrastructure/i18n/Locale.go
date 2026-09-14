// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"log/slog"

	"golang.org/x/text/language"

	port "github.com/Jersyfi/hubtask/core/port/i18n"
)

// LocaleInfo is the port's row, answered from the catalogues present and the table below.
type LocaleInfo = port.LocaleInfo

var _ port.Locales = Renderer{}

// The writing direction is the script's, and the script is what `language.Tag.Script()` infers
// for a tag that does not state one - which is why this is a list of scripts and not of
// languages: a twelfth language written right to left is covered by the script it uses.
var rightToLeft = map[string]bool{
	"Arab": true, "Hebr": true, "Syrc": true, "Thaa": true, "Nkoo": true,
	"Adlm": true, "Rohg": true, "Samr": true, "Mand": true,
}

// weekAndDecimal is the table behind week_start and decimal_separator: the `1.0` locale set,
// the ten most spoken languages by total speakers and German (milestone-0.8.0.md, decision 3).
//
// A table rather than a dependency, on purpose: golang.org/x/text exposes no week data and no
// number symbols as a public API, and the CLDR archive it can parse is not something a server
// should carry for eleven rows (ADR-0056). The values are what a browser's Intl answers for the
// same tags - `new Intl.Locale(tag).getWeekInfo().firstDay` and the `decimal` part of
// `Intl.NumberFormat(tag).formatToParts(1.5)` - recorded in Locale_test.go as the source, so that
// the row and the browser cannot disagree unnoticed. The week is CLDR's for the language's
// default region: `en` is the United States and starts on Sunday, `pt` is Brazil; a regional
// catalogue such as `en-GB.json` needs a row of its own, and until it has one is answered with the
// defaults below and a line in the log.
var weekAndDecimal = map[string]struct{ week, decimal string }{
	"en":      {"SUNDAY", "."},
	"zh-hans": {"MONDAY", "."},
	"hi":      {"SUNDAY", "."},
	"es":      {"MONDAY", ","},
	"ar":      {"SATURDAY", "."},
	"fr":      {"MONDAY", ","},
	"bn":      {"SUNDAY", "."},
	"pt":      {"SUNDAY", ","},
	"ru":      {"MONDAY", ","},
	"id":      {"SUNDAY", ","},
	"de":      {"MONDAY", ","},
}

// The answer for a locale the table does not know: honest defaults, and a line in the log at
// start saying the table lacks a row, so that the twelfth language is a row rather than a
// surprise.
const (
	defaultWeekStart        = "MONDAY"
	defaultDecimalSeparator = "."
)

// SupportedLocales answers one row per catalogue present, in the order Locales() gives them.
func (r Renderer) SupportedLocales() []LocaleInfo {
	rows := make([]LocaleInfo, 0, len(r.catalogues))
	for _, tag := range r.Locales() {
		rows = append(rows, localeInfo(tag))
	}
	return rows
}

// localeInfo builds one row. The tag is lower-cased as Locales() keys it; the manifest carries it
// as the file was named, which language.Tag.String() restores (`zh-hans` → `zh-Hans`).
func localeInfo(tag string) LocaleInfo {
	parsed := language.Make(tag)
	info := LocaleInfo{
		Tag:              parsed.String(),
		Direction:        "ltr",
		WeekStart:        defaultWeekStart,
		DecimalSeparator: defaultDecimalSeparator,
	}
	if script, _ := parsed.Script(); rightToLeft[script.String()] {
		info.Direction = "rtl"
	}
	if row, known := weekAndDecimal[tag]; known {
		info.WeekStart, info.DecimalSeparator = row.week, row.decimal
	}
	return info
}

// LogUnknownLocales says, once at start, which catalogues present have no row in the table. A
// log line rather than a refusal: the defaults are honest, and a translation nobody can enable
// because a table lacks a row would be the wrong kind of strictness.
func (r Renderer) LogUnknownLocales(logger *slog.Logger) {
	for _, tag := range r.Locales() {
		if _, known := weekAndDecimal[tag]; !known {
			logger.Warn("a catalogue has no locale metadata row; week start and decimal separator are defaults",
				slog.String("locale", tag))
		}
	}
}
