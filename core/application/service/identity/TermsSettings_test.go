// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// legalResolved is the part of a resolution applyLegal reads, built the way the resolver builds it.
func legalResolved(instance domain.LegalLayer, own domain.LegalLinks) ResolvedPolicy {
	legal, locks := domain.EffectiveLegal(instance, domain.LegalLayer{Links: own})
	return ResolvedPolicy{Legal: legal, LegalLock: locks, InstanceLegal: instance}
}

// A workspace sets a version only beside a terms link of its own, never on the installation's
// terms, and its version leaves with its link (UC-ID-19, identity.md §18).
func TestAWorkspacesTermsVersionGoesWithItsOwnLink(t *testing.T) {
	const host, acme = "https://host.example/terms", "https://acme.example/terms"
	installation := domain.LegalLayer{Links: domain.LegalLinks{TermsURL: host, TermsVersion: "3"}}
	locked := domain.LegalLayer{
		Links: domain.LegalLinks{TermsURL: host, TermsVersion: "3"},
		Locks: map[domain.LegalLink]bool{domain.LinkTerms: true},
	}

	for _, tc := range []struct {
		name        string
		instance    domain.LegalLayer
		own         domain.LegalLinks
		sent        map[domain.LegalLink]string
		wantLink    string
		wantVersion string
		wantMoved   []string
		refusal     string
	}{
		{
			name: "its own link with a version, in one save", instance: installation,
			sent:     map[domain.LegalLink]string{domain.LinkTerms: acme, domain.LinkTermsVersion: "1"},
			wantLink: acme, wantVersion: "1", wantMoved: []string{"terms_url", "terms_version"},
		},
		{
			name: "its own link with the installation's version number", instance: installation,
			sent:     map[domain.LegalLink]string{domain.LinkTerms: acme, domain.LinkTermsVersion: "3"},
			wantLink: acme, wantVersion: "3", wantMoved: []string{"terms_url", "terms_version"},
		},
		{
			name: "a new version of its own terms", instance: installation,
			own:      domain.LegalLinks{TermsURL: acme, TermsVersion: "1"},
			sent:     map[domain.LegalLink]string{domain.LinkTerms: acme, domain.LinkTermsVersion: "2"},
			wantLink: acme, wantVersion: "2", wantMoved: []string{"terms_version"},
		},
		{
			name: "the form sent back unchanged", instance: installation,
			own:      domain.LegalLinks{TermsURL: acme, TermsVersion: "1"},
			sent:     map[domain.LegalLink]string{domain.LinkTerms: acme, domain.LinkTermsVersion: "1"},
			wantLink: acme, wantVersion: "1",
		},
		{
			name: "the installation's version sent back with its terms", instance: installation,
			sent: map[domain.LegalLink]string{domain.LinkTerms: host, domain.LinkTermsVersion: "3"},
		},
		{
			name: "a version on the installation's terms", instance: installation,
			sent:    map[domain.LegalLink]string{domain.LinkTermsVersion: "4"},
			refusal: "auth.terms_version_without_link",
		},
		{
			name: "giving up its own terms takes their version along", instance: installation,
			own:       domain.LegalLinks{TermsURL: acme, TermsVersion: "1"},
			sent:      map[domain.LegalLink]string{domain.LinkTerms: ""},
			wantMoved: []string{"terms_url", "terms_version"},
		},
		{
			name: "the installation's lock holds the version too", instance: locked,
			sent:    map[domain.LegalLink]string{domain.LinkTermsVersion: "4"},
			refusal: "auth.policy_locked",
		},
		{
			name: "the locked version sent back unchanged", instance: locked,
			sent: map[domain.LegalLink]string{domain.LinkTerms: host, domain.LinkTermsVersion: "3"},
		},
		{
			name: "a version with a second line", instance: installation,
			sent:    map[domain.LegalLink]string{domain.LinkTerms: acme, domain.LinkTermsVersion: "1\n2"},
			refusal: "auth.terms_version_invalid",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			links, moved, err := WorkspaceWriter{}.applyLegal(legalResolved(tc.instance, tc.own), tc.own, tc.sent)
			if tc.refusal != "" {
				var problem *shared.Error
				if !errors.As(err, &problem) || problem.DetailCode != tc.refusal {
					t.Fatalf("err = %v, want %s", err, tc.refusal)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if links.TermsURL != tc.wantLink || links.TermsVersion != tc.wantVersion {
				t.Errorf("stored (%q, %q), want (%q, %q)",
					links.TermsURL, links.TermsVersion, tc.wantLink, tc.wantVersion)
			}
			fields := make([]string, 0, len(moved))
			for _, change := range moved {
				fields = append(fields, change.Field)
			}
			if len(fields) != len(tc.wantMoved) {
				t.Fatalf("moved %v, want %v", fields, tc.wantMoved)
			}
			for i := range fields {
				if fields[i] != tc.wantMoved[i] {
					t.Errorf("moved %v, want %v", fields, tc.wantMoved)
				}
			}
		})
	}
}

// The sign-in rules answer the terms' version and who set them, and nothing where no terms are set
// (UC-ID-19 checks 3 and 5).
func TestTheSignInRulesAnswerTheTermsInForce(t *testing.T) {
	handler, workspace, instance := newRulesFixture()

	none, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if legal := legalOutput(none.Legal); len(legal) != 0 {
		t.Errorf("no terms answered %v", legal)
	}

	instance.level.Legal = domain.LegalLayer{
		Links: domain.LegalLinks{TermsURL: "https://host.example/terms", TermsVersion: "3"},
	}
	byInstallation, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	legal := legalOutput(byInstallation.Legal)
	if legal["terms_version"] != "3" || legal["terms_set_by"] != "INSTALLATION" {
		t.Errorf("the installation's terms answered %v", legal)
	}

	workspace.row.Settings.Legal = domain.LegalLinks{TermsURL: "https://acme.example/terms"}
	byWorkspace, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	legal = legalOutput(byWorkspace.Legal)
	if _, held := legal["terms_version"]; held || legal["terms_set_by"] != "WORKSPACE" {
		t.Errorf("the workspace's terms without a version answered %v", legal)
	}
}
