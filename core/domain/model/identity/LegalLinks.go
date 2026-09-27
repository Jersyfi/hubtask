// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"net/url"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The links a sign-in screen may be obliged to show, and who decides them (the concept's §10,
// SI-12).
//
// Two audiences on one installation, which is the whole reason these are settings rather than
// build-time text. A provider serving consumers has to show an imprint and a privacy notice on the
// page where somebody signs in, and has to be the one who decides what they point at - so it locks
// them. A provider serving companies hands each workspace its own, because the controller of that
// workspace's data is the company and not the host. Both are one product.
//
// And a private installation owes nobody an imprint: with nothing set at either level, the footer
// has no line at all rather than a line pointing nowhere.

// LegalLink names one of the four, in the spelling the contract uses.
type LegalLink string

const (
	LinkImprint       LegalLink = "imprint_url"
	LinkPrivacy       LegalLink = "privacy_url"
	LinkTerms         LegalLink = "terms_url"
	LinkAccessibility LegalLink = "accessibility_url"
)

// LegalLinkNames is the four, in the order a footer draws them.
func LegalLinkNames() []LegalLink {
	return []LegalLink{LinkImprint, LinkPrivacy, LinkTerms, LinkAccessibility}
}

// MaxLegalURLLength bounds a stored link. Long enough for any real address, short enough that the
// column is not a place to put a document.
const MaxLegalURLLength = 2000

// LegalLinks is the four as one value. An empty string is "no link", which is a legitimate answer
// and not a missing one.
type LegalLinks struct {
	ImprintURL       string
	PrivacyURL       string
	TermsURL         string
	AccessibilityURL string
}

// Of answers one link by name.
func (l LegalLinks) Of(name LegalLink) string {
	switch name {
	case LinkImprint:
		return l.ImprintURL
	case LinkPrivacy:
		return l.PrivacyURL
	case LinkTerms:
		return l.TermsURL
	case LinkAccessibility:
		return l.AccessibilityURL
	}
	return ""
}

// With answers the links with one of them replaced.
func (l LegalLinks) With(name LegalLink, value string) LegalLinks {
	switch name {
	case LinkImprint:
		l.ImprintURL = value
	case LinkPrivacy:
		l.PrivacyURL = value
	case LinkTerms:
		l.TermsURL = value
	case LinkAccessibility:
		l.AccessibilityURL = value
	}
	return l
}

// IsEmpty reports whether there is nothing to draw at all - which is what a private installation
// answers, and the reason the footer can have no line rather than an empty one.
func (l LegalLinks) IsEmpty() bool {
	for _, name := range LegalLinkNames() {
		if l.Of(name) != "" {
			return false
		}
	}
	return true
}

// LegalLayer is one level's say about the four: what it set, and which of them it fixed.
type LegalLayer struct {
	Links LegalLinks
	Locks map[LegalLink]bool
}

// EffectiveLegal resolves workspace -> instance -> nothing.
//
// A locked link is the instance's whatever the workspace holds, for Effective's reason: resolving
// has to answer something for a row written before the lock landed, and refusing is the writer's
// job where there is a field to name.
func EffectiveLegal(instance, workspace LegalLayer) (LegalLinks, map[LegalLink]LockOrigin) {
	resolved := LegalLinks{}
	locks := map[LegalLink]LockOrigin{}

	for _, name := range LegalLinkNames() {
		if instance.Locks[name] {
			locks[name] = LockInstance
			resolved = resolved.With(name, instance.Links.Of(name))
			continue
		}
		if value := workspace.Links.Of(name); value != "" {
			resolved = resolved.With(name, value)
			continue
		}
		resolved = resolved.With(name, instance.Links.Of(name))
	}
	return resolved, locks
}

// ValidLegalURL bounds and checks one link.
//
// Absolute, and `http`/`https` only: a link a sign-in screen renders is a link somebody clicks, and
// `javascript:` in an administrator's hands is a cross-site script in everybody else's. The empty
// string passes, because "no link" is an answer.
func ValidLegalURL(name LegalLink, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	refused := func(code string) error {
		return shared.ErrValidation.
			WithDetail(code).
			WithFields(shared.FieldError{Path: "/sign_in_policy/" + string(name), Code: code})
	}
	if len(trimmed) > MaxLegalURLLength {
		return "", refused("auth.legal_url_too_long")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "", refused("auth.legal_url_invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", refused("auth.legal_url_scheme")
	}
	return trimmed, nil
}
