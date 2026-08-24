// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"strings"
	"unicode/utf8"
)

// SubjectMaxLength is where a subject line is cut, in code points.
//
// An entry's title may be 500 code points (work.MaxItemTitleLength) and a subject line that long is
// a subject line no mail client shows and some servers refuse. Cut rather than refused: the message
// is still worth sending, and the entry it is about is one link away.
const SubjectMaxLength = 120

// Ellipsis marks a subject that was cut. One code point rather than three dots, so that the cut
// reads as a cut rather than as somebody's punctuation.
const Ellipsis = "…"

// Message is what a channel is asked to deliver: message codes and a closed set of parameters.
//
// The closed set is the enforcement of data-protection.md §9, and it is why this is a struct rather
// than a map. "Never the note body" written as a rule is a habit somebody breaks in a hurry; written
// as a type with no field a note fits in, it is a compile error. The three things a message may
// carry are who did it, what it was called and where to look, and nothing here can be extended by a
// caller passing an extra key.
//
// The codes are keys under `email.*` in locales/en.json (i18n-l10n.md §3). Rendering them is the
// declared exception to rule 8 and happens in the recipient's locale (§1) - which is why this type
// carries codes and parameters rather than sentences: the language is not decided here.
type Message struct {
	// SubjectCode and BodyCode are the two halves of an email. Separate keys rather than one
	// message split on a newline, because a translator works on a subject line differently from a
	// paragraph and a channel that has no subject - push, one day - uses only one of them.
	SubjectCode string
	BodyCode    string
	// ActorName is who caused it, as their display name. Empty where nobody did.
	ActorName string
	// Title is the entry's title, and it is empty whenever the recipient's preference says so or
	// there is no entry. Never a note, never a comment body: there is no field for either.
	Title string
	// Link is where to look. An absolute URL built from the configured public base, so that a mail
	// client has something to click - a relative path in an email is a dead link.
	Link string
}

// Params is the message's parameters as the i18n port takes them.
//
// Only the ones that have a value: a placeholder with no parameter is left standing by the
// catalogue (infrastructure/i18n), which is how a missing value shows up as a missing value rather
// than as a gap nobody notices. A message whose title was withheld therefore uses a body code that
// has no `{title}` in it rather than relying on this to blank one.
func (m Message) Params() map[string]string {
	params := make(map[string]string, 3)
	if m.ActorName != "" {
		params["actor"] = m.ActorName
	}
	if m.Title != "" {
		params["title"] = m.Title
	}
	if m.Link != "" {
		params["link"] = m.Link
	}
	return params
}

// WithTitle attaches the entry's title, cut to a subject's length, unless the decision withheld it.
//
// The decision is taken rather than a bool, so that the one call that could put a title into a
// message is the one that has the decision in its hand. A caller holding only a title cannot set
// the field: it is set here or it is empty.
func (m Message) WithTitle(title string, decision Decision) Message {
	if !decision.IncludeTitle {
		return m
	}
	m.Title = TrimSubject(title)
	return m
}

// TrimSubject cuts a title to SubjectMaxLength code points and marks the cut.
//
// Code points rather than bytes, for the reason every length in this codebase counts them (I-W7):
// cutting UTF-8 by byte produces a replacement character, and a subject line ending in one looks
// like a bug in the sender rather than a long title.
func TrimSubject(title string) string {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) <= SubjectMaxLength {
		return title
	}

	runes := []rune(title)
	// One code point short of the limit, so that the ellipsis fits inside it rather than pushing
	// the result one past the bound it was cut to.
	return strings.TrimRight(string(runes[:SubjectMaxLength-1]), " ") + Ellipsis
}
