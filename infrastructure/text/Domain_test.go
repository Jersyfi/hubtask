// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package text_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/text"
)

// The vectors that matter for an address (M-10, i18n-l10n.md §7): a Unicode label becomes its
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
