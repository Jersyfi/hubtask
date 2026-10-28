// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package text_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/text"
)

// The vectors that matter for an address (i18n-l10n.md §7): a Unicode label becomes its
// Punycode, an already-encoded label stays, case is mapped, and what the DNS would refuse is
// refused here - a label of the wrong shape, a mixed-script label the lookup profile declines.
func TestDomainsAreBroughtToTheFormTheDNSHolds(t *testing.T) {
	for domain, want := range map[string]string{
		"müller.de":        "xn--mller-kva.de",
		"MÜLLER.de":        "xn--mller-kva.de",
		"xn--mller-kva.de": "xn--mller-kva.de",
		"example.org":      "example.org",
		"Example.ORG":      "example.org",
		"bücher.example":   "xn--bcher-kva.example",
		"почта.рф":         "xn--80a1acny.xn--p1ai",
		"例え.テスト":           "xn--r8jz45g.xn--zckzah",
	} {
		got, err := (text.Domains{}).ToASCII(domain)
		if err != nil {
			t.Errorf("%q: %v", domain, err)
			continue
		}
		if got != want {
			t.Errorf("%q encoded as %q, want %q", domain, got, want)
		}
	}
}

func TestWhatTheDNSWouldRefuseIsRefused(t *testing.T) {
	for _, domain := range []string{
		"-hyphen.example",    // a label may not start with a hyphen
		"xn--zzzzzz.example", // a Punycode label that decodes to nothing valid
	} {
		got, err := (text.Domains{}).ToASCII(domain)
		if err == nil {
			t.Errorf("%q was accepted as %q, want a refusal", domain, got)
		}
	}
}

// An apex is its own registered domain, also under a suffix of two labels; a public suffix lies in
// none (UC-INS-15 check 4).
func TestTheRegisteredDomainOfAHost(t *testing.T) {
	for host, want := range map[string]string{
		"tasks.acme.com":   "acme.com",
		"acme.com":         "acme.com",
		"tasks.acme.co.uk": "acme.co.uk",
		"acme.co.uk":       "acme.co.uk",
	} {
		if got, err := (text.Registry{}).RegisteredDomain(host); err != nil || got != want {
			t.Errorf("RegisteredDomain(%q) = (%q, %v), want %q", host, got, err, want)
		}
	}
	if _, err := (text.Registry{}).RegisteredDomain("co.uk"); err == nil {
		t.Error("a public suffix lies in a registered domain")
	}
}
