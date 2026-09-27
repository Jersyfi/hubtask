// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package text is the port for the two things the domain has to do to text and may not do itself
// (i18n-l10n.md §5, §7): bring an input to its Unicode normal form, and bring the domain half of
// an address to the ASCII form a mail server sees. Both are golang.org/x libraries, and both are
// confined to their adapter by ADR-0056 - which is why a domain constructor takes one of these
// rather than calling a function.
package text

import "strings"

// DomainEncoder brings a domain name to the form the DNS holds it in (IDNA 2008, RFC 5891):
// `müller.de` becomes `xn--mller-kva.de`, and `example.org` stays as it is.
//
// One direction only. What is stored and compared is the ASCII form, so that two spellings of one
// mailbox are one row; what is shown is the client's business, and a client that wants the
// Unicode form has `URL.domainToUnicode`.
type DomainEncoder interface {
	// ToASCII answers the encoded domain, or an error for a label the profile refuses - an empty
	// label, a hyphen in the wrong place, a character no domain may carry.
	ToASCII(domain string) (string, error)
}

// Normalizer brings text to Unicode Normalization Form C (i18n-l10n.md §5): `e` followed by a
// combining acute becomes the one code point `é`, so that the two spellings a keyboard, a
// clipboard and a file system produce for one visible character are one string to the unique
// index, one token to the search document, and one comparison to the query language.
//
// The domain constructor of each text kind applies it on the way in - once, where the text is
// trimmed and bounded - and nothing stored is rewritten: rows written before M-07 stay as they
// were, which §5 says is the consequence and the owner decided (#577).
type Normalizer interface {
	// NFC answers the text in normal form C. Text that already is stays byte-identical.
	NFC(text string) string

	// NFKC answers the text in normal form KC - compatibility decomposition, then canonical
	// composition - for the one thing in this product that is a *secret* rather than content: a
	// password (ADR-0068 §7).
	//
	// Content is normalised with NFC, and the reason is the difference between the two: NFKC
	// rewrites `fi` into `fi`, `2` into `2` and a full-width letter into ASCII, which is exactly
	// what must not happen to a title somebody typed. A password is compared, never shown, and
	// the same word typed on two keyboards has to be one password - so the fold that would
	// destroy a title is what makes a password work. It is applied where the password is counted
	// and where it is hashed, so that what was counted is what is stored.
	NFKC(text string) string
}

// Composing is the Normalizer the tests use, here rather than in a helper package for the reason
// clock.Fixed is: every layer's tests need one, and a fake per package is a copy per package to
// get wrong. It composes the handful of pairs the tests write and leaves everything else alone -
// which is enough to prove that a constructor called it, and nothing else is what the tests claim.
type Composing struct{}

// NFC composes the Latin letters the tests decompose: a base letter followed by U+0301 (acute),
// U+0308 (diaeresis), U+0303 (tilde) or U+0327 (cedilla).
func (Composing) NFC(text string) string {
	return composing.Replace(text)
}

// NFKC does the same and folds the handful of compatibility characters the tests write, which is
// enough to prove that a caller applied the compatibility form rather than the canonical one.
func (Composing) NFKC(text string) string {
	return compatible.Replace(composing.Replace(text))
}

var composing = strings.NewReplacer(
	"e\u0301", "\u00e9", "E\u0301", "\u00c9", "a\u0301", "\u00e1", "o\u0301", "\u00f3", "u\u0301", "\u00fa",
	"a\u0308", "\u00e4", "o\u0308", "\u00f6", "u\u0308", "\u00fc", "A\u0308", "\u00c4", "O\u0308", "\u00d6", "U\u0308", "\u00dc",
	"n\u0303", "\u00f1", "c\u0327", "\u00e7",
)

// compatible folds the compatibility characters the tests use: a ligature, a superscript digit,
// and the full-width Latin letters.
var compatible = strings.NewReplacer(
	"\ufb01", "fi", "\ufb02", "fl", "\u00b2", "2", "\u00b3", "3",
	"\uff21", "A", "\uff41", "a", "\uff10", "0", "\uff3a", "Z", "\uff5a", "z",
)
