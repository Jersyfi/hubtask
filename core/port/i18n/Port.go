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
