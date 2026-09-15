// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package i18n is the port for the declared exception to rule 8.
//
// The backend delivers no display text: an answer carries a code and its parameters, and whoever
// has a person in front of them builds the sentence (ADR-0011). i18n-l10n.md §1 names the
// exceptions - email, push, an ICS feed, a PDF export - and says the same thing about all of them:
// rendering goes through this port, in the locale of the **recipient**, never of whoever triggered
// it.
//
// The recipient's locale is the whole reason this is a port at all. An adapter could render into
// whatever language the process was started with and nobody would notice until the day two accounts
// in one workspace speak different languages - which is exactly the acceptance test C-09 carries.
// Making the locale an argument means there is no call that can forget it.
package i18n

// Renderer turns a message code into a sentence in one locale.
//
// It answers rather than failing. A code with no entry renders as itself and a placeholder with no
// parameter is left standing, which is the fallback i18n-l10n.md §3 prescribes: a code in a subject
// line is ugly, an empty subject line is a bug report nobody can act on. An adapter that refused
// would turn a missing translation into an undelivered email.
type Renderer interface {
	// Render builds the sentence. The locale is BCP 47 (`de`, `de-AT`, `pt-BR`); an empty one, or
	// one this installation has no catalogue for, falls back down the chain to the source language
	// (i18n-l10n.md §2).
	Render(locale, code string, params map[string]string) string
}

// LocaleInfo is one row of the manifest's `supported_locales` (i18n-l10n.md §2, §6): a locale
// this installation has a catalogue for, and the three facts a client needs before it has
// rendered anything - which way the script runs, the day the week starts, and the character
// between the integer and the fraction.
type LocaleInfo struct {
	// Tag is BCP 47, as the catalogue file is named.
	Tag string
	// Direction is `ltr` or `rtl`.
	Direction string
	// WeekStart is `MONDAY`, `SUNDAY` or `SATURDAY` - the account's own vocabulary, so that a
	// client compares the two without translating.
	WeekStart string
	// DecimalSeparator is `.` or `,` or the locale's own.
	DecimalSeparator string
}

// Locales answers which locales this installation serves. Derived from the catalogue files
// present rather than from a constant, which is what makes a new language a file and not a
// release (arc42 QS-08).
type Locales interface {
	// SupportedLocales answers one row per catalogue, the source language first.
	SupportedLocales() []LocaleInfo
}

// Negotiator reads what a request asked for, against the catalogues this installation has
// (i18n-l10n.md §2).
//
// A port for the reason Renderer is one: the matching is `golang.org/x/text/language`'s, which
// ADR-0056 confines to the i18n adapter, and the REST middleware that has the header in hand may
// not import it. What comes back is the client's own tag, not the catalogue's - `de-AT` when the
// client said `de-AT` and the installation has `de` - because the tag travels on the actor into
// places that want the region: an entry's content language, the week a person starts on.
type Negotiator interface {
	// Negotiate answers the tag from an Accept-Language header that this installation can serve
	// best, as the client wrote it: `fr, de;q=0.8` on an installation with German and no French
	// answers `de`. When nothing in the header is served, the client's first preference is
	// answered as it stands, so that an unserved language still names itself. An absent or
	// unreadable header answers "".
	Negotiate(acceptLanguage string) string
}
