// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The version is the terms link's: from the level that set the link, under the link's lock.
func TestTheTermsVersionResolvesWithItsLink(t *testing.T) {
	const host, acme = "https://host.example/terms", "https://acme.example/terms"
	for _, tc := range []struct {
		name              string
		instance          LegalLayer
		workspace         LegalLayer
		wantURL, wantVer  string
		wantBy            TermsSetBy
		wantVersionLocked bool
	}{
		{name: "nothing set anywhere"},
		{
			name:     "the installation's terms",
			instance: LegalLayer{Links: LegalLinks{TermsURL: host, TermsVersion: "3"}},
			wantURL:  host, wantVer: "3", wantBy: TermsByInstallation,
		},
		{
			name:      "the workspace's own terms and version",
			instance:  LegalLayer{Links: LegalLinks{TermsURL: host, TermsVersion: "3"}},
			workspace: LegalLayer{Links: LegalLinks{TermsURL: acme, TermsVersion: "7"}},
			wantURL:   acme, wantVer: "7", wantBy: TermsByWorkspace,
		},
		{
			name:      "the workspace's own terms carry no version, the installation's does not lend one",
			instance:  LegalLayer{Links: LegalLinks{TermsURL: host, TermsVersion: "3"}},
			workspace: LegalLayer{Links: LegalLinks{TermsURL: acme}},
			wantURL:   acme, wantBy: TermsByWorkspace,
		},
		{
			name:      "a version stored without its own link does not relabel the installation's terms",
			instance:  LegalLayer{Links: LegalLinks{TermsURL: host, TermsVersion: "3"}},
			workspace: LegalLayer{Links: LegalLinks{TermsVersion: "99"}},
			wantURL:   host, wantVer: "3", wantBy: TermsByInstallation,
		},
		{
			name: "a locked installation link brings its version and its lock",
			instance: LegalLayer{
				Links: LegalLinks{TermsURL: host, TermsVersion: "3"},
				Locks: map[LegalLink]bool{LinkTerms: true},
			},
			workspace: LegalLayer{Links: LegalLinks{TermsURL: acme, TermsVersion: "7"}},
			wantURL:   host, wantVer: "3", wantBy: TermsByInstallation, wantVersionLocked: true,
		},
		{
			name: "a lock pinning no link pins no terms",
			instance: LegalLayer{
				Links: LegalLinks{TermsVersion: "3"},
				Locks: map[LegalLink]bool{LinkTerms: true},
			},
			workspace:         LegalLayer{Links: LegalLinks{TermsURL: acme, TermsVersion: "7"}},
			wantVersionLocked: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved, locks := EffectiveLegal(tc.instance, tc.workspace)
			terms := resolved.Terms()
			if terms.URL != tc.wantURL || terms.Version != tc.wantVer || terms.SetBy != tc.wantBy {
				t.Errorf("terms = %+v, want (%q, %q, %q)", terms, tc.wantURL, tc.wantVer, tc.wantBy)
			}
			if terms.IsSet() != (tc.wantURL != "") {
				t.Errorf("IsSet = %v", terms.IsSet())
			}
			if (locks[LinkTermsVersion] == LockInstance) != tc.wantVersionLocked {
				t.Errorf("the version's lock = %q", locks[LinkTermsVersion])
			}
			if got := LegalSourceOf(LinkTermsVersion, tc.instance, tc.workspace); got !=
				LegalSourceOf(LinkTerms, tc.instance, tc.workspace) {
				t.Errorf("the version's source %q is not the link's", got)
			}
		})
	}
}

// An agreement covers terms only while link and version are the ones agreed to (UC-ID-19 check 2).
func TestAnAgreementCoversExactlyTheTermsAgreedTo(t *testing.T) {
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	v3 := TermsInForce{URL: "https://host.example/terms", Version: "3", SetBy: TermsByInstallation}
	for _, tc := range []struct {
		name      string
		terms     TermsInForce
		agreement TermsAgreement
		want      bool
	}{
		{name: "no terms need no agreement", terms: TermsInForce{}, want: true},
		{name: "never agreed", terms: v3, want: false},
		{name: "agreed to this link and version", terms: v3,
			agreement: TermsAgreement{URL: v3.URL, Version: "3", AgreedAt: at}, want: true},
		{name: "agreed to an older version", terms: v3,
			agreement: TermsAgreement{URL: v3.URL, Version: "2", AgreedAt: at}, want: false},
		{name: "agreed to other terms with the same version", terms: v3,
			agreement: TermsAgreement{URL: "https://acme.example/terms", Version: "3", AgreedAt: at}, want: false},
		{name: "terms without a version, agreed", terms: TermsInForce{URL: v3.URL},
			agreement: TermsAgreement{URL: v3.URL, AgreedAt: at}, want: true},
		{name: "terms that gained a version", terms: v3,
			agreement: TermsAgreement{URL: v3.URL, AgreedAt: at}, want: false},
		{name: "a stored pair without a time is no agreement", terms: v3,
			agreement: TermsAgreement{URL: v3.URL, Version: "3"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.terms.AgreedBy(tc.agreement); got != tc.want {
				t.Errorf("AgreedBy = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestATermsVersionIsBoundedAndHasNoSecondLine(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
		refused         bool
	}{
		{name: "empty is no version", raw: "  ", want: ""},
		{name: "trimmed", raw: " 2026-11 ", want: "2026-11"},
		{name: "sixty-four characters", raw: strings.Repeat("ä", 64), want: strings.Repeat("ä", 64)},
		{name: "sixty-five characters", raw: strings.Repeat("a", 65), refused: true},
		{name: "a line break", raw: "3\nSigned in as admin", refused: true},
		{name: "a tab", raw: "3\t4", refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidTermsVersion(tc.raw)
			if tc.refused {
				var problem *shared.Error
				if !errors.As(err, &problem) || problem.DetailCode != "auth.terms_version_invalid" {
					t.Fatalf("err = %v, want auth.terms_version_invalid", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("(%q, %v), want %q", got, err, tc.want)
			}
		})
	}
	// The setting names route the version to this check, not to the link's.
	if _, err := ValidLegalURL(LinkTermsVersion, "3"); err != nil {
		t.Errorf("a version was judged as a link: %v", err)
	}
}
