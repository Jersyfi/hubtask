// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Terms of use, agreed and agreed again (UC-ID-19, identity.md §4.2 and §12).
//
// Hubtask holds no text of the terms: the terms are the link the workspace or the installation set
// (LegalLinks.TermsURL) and, where they gave one, a version beside it. What a person agrees to is
// that pair, so an agreement is current exactly while the stored link and version equal the ones in
// force - changing either asks everybody once more, which the settings screen says beside both.
// A link without a version is terms all the same: where a terms link is set, nobody has agreed yet.

// MaxTermsVersionLength bounds a version. Room for "2026-11" or "Version 4.1 of 1 November", and
// no room for a document.
const MaxTermsVersionLength = 64

// TermsSetBy says which level set the terms in force, which is whom a person who does not agree
// can ask.
type TermsSetBy string

const (
	TermsByInstallation TermsSetBy = "INSTALLATION"
	TermsByWorkspace    TermsSetBy = "WORKSPACE"
)

func termsSetByOf(source PolicySource) TermsSetBy {
	if source == SourceWorkspace {
		return TermsByWorkspace
	}
	return TermsByInstallation
}

// TermsInForce is what a person is asked to agree to. The zero value is "no terms".
type TermsInForce struct {
	URL     string
	Version string
	SetBy   TermsSetBy
}

// Terms answers the terms in force among resolved links.
func (l LegalLinks) Terms() TermsInForce {
	if l.TermsURL == "" {
		return TermsInForce{}
	}
	return TermsInForce{URL: l.TermsURL, Version: l.TermsVersion, SetBy: l.TermsSetBy}
}

// IsSet reports whether there are terms to agree to at all (UC-ID-19 check 5).
func (t TermsInForce) IsSet() bool { return t.URL != "" }

// Matches reports whether a link and a version a person was shown are these terms.
func (t TermsInForce) Matches(url, version string) bool {
	return t.URL == url && t.Version == version
}

// AgreedBy reports whether an agreement covers these terms: no terms need none, and otherwise the
// agreement must be to exactly this link and version.
func (t TermsInForce) AgreedBy(agreement TermsAgreement) bool {
	if !t.IsSet() {
		return true
	}
	return agreement.Agreed() && t.Matches(agreement.URL, agreement.Version)
}

// TermsAgreement is a person's latest agreement: the terms' link and version, and when. The zero
// value is "never agreed".
type TermsAgreement struct {
	URL      string
	Version  string
	AgreedAt time.Time
}

// Agreed reports whether the person ever agreed to anything.
func (a TermsAgreement) Agreed() bool { return !a.AgreedAt.IsZero() }

// ValidTermsVersion bounds and checks a version as an administrator typed it. The empty string
// passes: terms may carry no version.
//
// No control character, because the version is drawn on a sign-in card and written to the trail,
// and a line break in either is a forged second line.
func ValidTermsVersion(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if utf8.RuneCountInString(trimmed) > MaxTermsVersionLength ||
		strings.IndexFunc(trimmed, unicode.IsControl) >= 0 {
		return "", shared.ErrValidation.
			WithDetail("auth.terms_version_invalid").
			WithFields(shared.FieldError{
				Path: "/sign_in_policy/" + string(LinkTermsVersion), Code: "auth.terms_version_invalid",
			})
	}
	return trimmed, nil
}

// TermsVersionWithoutLink is the refusal of a version a level sets beside no terms link of its own.
func TermsVersionWithoutLink() error {
	return shared.ErrValidation.
		WithDetail("auth.terms_version_without_link").
		WithFields(shared.FieldError{
			Path: "/sign_in_policy/" + string(LinkTermsVersion), Code: "auth.terms_version_without_link",
		})
}
