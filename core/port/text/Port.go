// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package text is the port for the two things the domain has to do to text and may not do itself
// (i18n-l10n.md §5, §7): bring an input to its Unicode normal form, and bring the domain half of
// an address to the ASCII form a mail server sees. Both are golang.org/x libraries, and both are
// confined to their adapter by ADR-0056 - which is why a domain constructor takes one of these
// rather than calling a function.
package text

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
