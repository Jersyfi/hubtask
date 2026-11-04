// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"
	"testing"
)

// The support contact and the documentation resolve like the legal links - a workspace's own,
// unless the installation locked its - and never join the footer's four.
func TestTheHelpLinksResolveAndLockLikeTheLegalOnes(t *testing.T) {
	instance := LegalLayer{
		Links: LegalLinks{SupportURL: "mailto:help@host.example", DocsURL: "https://host.example/docs"},
		Locks: map[LegalLink]bool{LinkDocs: true},
	}
	workspace := LegalLayer{Links: LegalLinks{SupportURL: "https://acme.example/help", DocsURL: "https://acme.example/docs"}}

	resolved, locks := EffectiveLegal(instance, LegalLayer{}, workspace)

	if resolved.Of(LinkSupport) != "https://acme.example/help" {
		t.Errorf("the open support contact resolved to %q", resolved.Of(LinkSupport))
	}
	if resolved.Of(LinkDocs) != "https://host.example/docs" {
		t.Errorf("the locked documentation resolved to %q", resolved.Of(LinkDocs))
	}
	if locks[LinkDocs] != LockInstance || locks[LinkSupport] != LockNone {
		t.Errorf("the locks read %v", locks)
	}
	only, _ := EffectiveLegal(instance, LegalLayer{}, LegalLayer{})
	if only.Of(LinkSupport) != "mailto:help@host.example" {
		t.Errorf("the installation's support contact resolved to %q", only.Of(LinkSupport))
	}
	for _, name := range HelpLinkNames() {
		if slices.Contains(LegalLinkNames(), name) {
			t.Errorf("%s is drawn in the footer", name)
		}
		if !slices.Contains(LegalSettingNames(), name) {
			t.Errorf("%s is not set beside the legal links", name)
		}
	}
	if (LegalLinks{SupportURL: "mailto:help@host.example"}).IsEmpty() {
		t.Error("a support contact alone is not nothing to store")
	}
}

// A support contact may be an address to write to; every other link is a page.
func TestOnlyTheSupportContactTakesAMailAddress(t *testing.T) {
	cases := []struct {
		name  LegalLink
		raw   string
		valid bool
	}{
		{LinkSupport, "mailto:help@acme.example", true},
		{LinkSupport, "https://acme.example/help", true},
		{LinkSupport, "mailto:", false},
		{LinkSupport, "mailto:@", false},
		{LinkSupport, "mailto:help@", false},
		{LinkSupport, "javascript:alert(1)", false},
		{LinkDocs, "mailto:help@acme.example", false},
		{LinkImprint, "mailto:help@acme.example", false},
		{LinkDocs, "https://acme.example/docs", true},
	}
	for _, each := range cases {
		_, err := ValidLegalURL(each.name, each.raw)
		if (err == nil) != each.valid {
			t.Errorf("%s %q: valid %v, err %v", each.name, each.raw, each.valid, err)
		}
	}
}
