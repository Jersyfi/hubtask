// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package plan

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func number(value int) *int { return &value }

func sources(value integration.AiSources) *integration.AiSources { return &value }

func floor(value integration.AiMinJurisdiction) *integration.AiMinJurisdiction { return &value }

func detailOf(err error) string {
	var refused *shared.Error
	if errors.As(err, &refused) {
		return refused.DetailCode
	}
	return ""
}

func TestAShortNameIsALabel(t *testing.T) {
	for name, valid := range map[string]bool{
		"family": true, "b2b-business": true, "p1": true, "0-free": true,
		"": false, "f": false, "Family": false, "-family": false, "family plan": false,
		"a2345678901234567890123456789012345678901": false,
	} {
		if got := ValidName(name) == nil; got != valid {
			t.Errorf("ValidName(%q) valid = %t, want %t", name, got, valid)
		}
	}
}

func TestADisplayNameIsBounded(t *testing.T) {
	long := make([]rune, MaxDisplayNameLength+1)
	for i := range long {
		long[i] = 'x'
	}
	for name, valid := range map[string]bool{"Family": true, "": false, string(long): false} {
		if got := ValidDisplayName(name) == nil; got != valid {
			t.Errorf("ValidDisplayName(%d runes) valid = %t, want %t", len([]rune(name)), got, valid)
		}
	}
}

// What a plan may decide against the installation (multi-tenancy.md §4.2): under a lock nothing,
// where open only tighter - a limit excepted, which a plan may raise where it is open.
func TestAPlanFitsUnderTheInstallation(t *testing.T) {
	lockedLength := identity.PolicyLayer{
		Patch: identity.PolicyPatch{MinLength: number(14)},
		Locks: map[identity.PolicySwitch]bool{identity.SwitchMinLength: true},
	}
	openLength := identity.PolicyLayer{Patch: identity.PolicyPatch{MinLength: number(14)}}

	cases := []struct {
		name   string
		values Values
		above  Above
		want   string
	}{
		{name: "nothing decided fits anything", values: Values{}, above: Above{Policy: lockedLength}},
		{
			name:   "a stricter password where it is open",
			values: Values{Policy: identity.PolicyLayer{Patch: identity.PolicyPatch{MinLength: number(16)}}},
			above:  Above{Policy: openLength},
		},
		{
			name:   "a looser password where it is open",
			values: Values{Policy: identity.PolicyLayer{Patch: identity.PolicyPatch{MinLength: number(12)}}},
			above:  Above{Policy: openLength},
			want:   "admin.plan_looser_than_installation",
		},
		{
			name:   "even a stricter password under the installation's lock",
			values: Values{Policy: identity.PolicyLayer{Patch: identity.PolicyPatch{MinLength: number(20)}}},
			above:  Above{Policy: lockedLength},
			want:   "admin.plan_under_installation_lock",
		},
		{
			name: "a higher limit where it is open",
			values: Values{Quotas: identity.QuotaDefaults{
				Limits: map[string]int64{"items": 100000}, Locks: map[string]bool{},
			}},
			above: Above{Quotas: identity.QuotaDefaults{Limits: map[string]int64{"items": 1000}}},
		},
		{
			name: "any limit under the installation's lock",
			values: Values{Quotas: identity.QuotaDefaults{
				Limits: map[string]int64{"items": 10}, Locks: map[string]bool{},
			}},
			above: Above{Quotas: identity.QuotaDefaults{
				Limits: map[string]int64{"items": 1000}, Locks: map[string]bool{"items": true},
			}},
			want: "admin.plan_under_installation_lock",
		},
		{
			name:   "a link the installation locks",
			values: Values{Legal: identity.LegalLayer{Links: identity.LegalLinks{}.With(identity.LinkTerms, "https://b2b.example/terms")}},
			above: Above{Legal: identity.LegalLayer{
				Links: identity.LegalLinks{}.With(identity.LinkTerms, "https://provider.example/terms"),
				Locks: map[identity.LegalLink]bool{identity.LinkTerms: true},
			}},
			want: "admin.plan_under_installation_lock",
		},
		{
			name:   "the terms' version follows its link's lock",
			values: Values{Legal: identity.LegalLayer{Links: identity.LegalLinks{}.With(identity.LinkTermsVersion, "2026-10")}},
			above: Above{Legal: identity.LegalLayer{
				Locks: map[identity.LegalLink]bool{identity.LinkTerms: true},
			}},
			want: "admin.plan_under_installation_lock",
		},
		{
			name:   "a version without a terms link of the plan's own",
			values: Values{Legal: identity.LegalLayer{Links: identity.LegalLinks{}.With(identity.LinkTermsVersion, "2026-10")}},
			want:   "auth.terms_version_without_link",
		},
		{
			name:   "an AI rule narrower than the installation's",
			values: Values{Ai: integration.AiLayer{Sources: sources(integration.AiSourcesOffered)}},
			above:  Above{Ai: integration.AiLayer{Sources: sources(integration.AiSourcesEither)}},
		},
		{
			name:   "an AI rule wider than the installation's",
			values: Values{Ai: integration.AiLayer{Sources: sources(integration.AiSourcesEither)}},
			above:  Above{Ai: integration.AiLayer{Sources: sources(integration.AiSourcesOwn)}},
			want:   "admin.plan_looser_than_installation",
		},
		{
			name:   "a lower floor for an own model than the installation's",
			values: Values{Ai: integration.AiLayer{MinJurisdiction: floor(integration.AiMinAny)}},
			above:  Above{Ai: integration.AiLayer{MinJurisdiction: floor(integration.AiMinEEA)}},
			want:   "admin.plan_looser_than_installation",
		},
		{
			name:   "a higher floor",
			values: Values{Ai: integration.AiLayer{MinJurisdiction: floor(integration.AiMinSelfHosted)}},
			above:  Above{Ai: integration.AiLayer{MinJurisdiction: floor(integration.AiMinEEA)}},
		},
		{
			name:   "a feature outside the closed set",
			values: Values{LeftOut: []Feature{"EXPORT"}},
			want:   "admin.plan_feature_unknown",
		},
		{
			name:   "the three features left out",
			values: Values{LeftOut: Features()},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.values.FitsUnder(c.above)
			if got := detailOf(err); got != c.want {
				t.Errorf("FitsUnder = %v (%q), want %q", err, got, c.want)
			}
		})
	}
}

// The set is closed: what no plan may switch off is no entry of it (UC-INS-13 check 5).
func TestNothingAPersonIsOwedIsAFeature(t *testing.T) {
	for _, owed := range []Feature{"EXPORT", "DELETION", "ACCESS_REQUESTS", "LANGUAGE", "TIME_ZONE", "ACCESSIBILITY", "API", "MCP"} {
		if owed.Valid() {
			t.Errorf("%s is a feature a plan could leave out", owed)
		}
	}
	if len(Features()) != 3 {
		t.Errorf("the set holds %d features, want the three of multi-tenancy.md §4.2", len(Features()))
	}
}

func TestAPlanIncludesWhatItDoesNotLeaveOut(t *testing.T) {
	family := Plan{Values: Values{LeftOut: []Feature{FeatureWebhooks}}}
	if family.Includes(FeatureWebhooks) || !family.Includes(FeatureAutomationRules) {
		t.Error("Includes does not answer the plan's left-out features")
	}
}
