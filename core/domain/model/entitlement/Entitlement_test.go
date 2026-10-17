// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package entitlement_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A plan can leave out exactly three features, and none of what may never be switched off
// (UC-INS-13 checks 3 and 5).
func TestTheSetIsExactlyThreeAndNamesNothingThatMayNeverBeSwitchedOff(t *testing.T) {
	want := []entitlement.Feature{
		entitlement.AutomationRules, entitlement.Webhooks, entitlement.OwnSignInProviders,
	}
	if got := entitlement.Features(); !slices.Equal(got, want) {
		t.Fatalf("Features() = %v, want %v", got, want)
	}

	for _, never := range []entitlement.Feature{
		"EXPORT", "DELETION", "ACCESS_REQUESTS", "LANGUAGE", "TIME_ZONE", "ACCESSIBILITY",
		"REST", "MCP", "HUBCTL", "TOKENS", "CALENDAR_FEED", "", "automation_rules",
	} {
		if never.Valid() {
			t.Errorf("%q is a feature a plan can leave out", never)
		}
	}
}

func TestWhatIsLeftOutIsRefusedAndTheRestIncluded(t *testing.T) {
	cases := []struct {
		name     string
		leftOut  entitlement.LeftOut
		feature  entitlement.Feature
		included bool
	}{
		{"no plan includes everything", nil, entitlement.AutomationRules, true},
		{"empty includes everything", entitlement.LeftOut{}, entitlement.Webhooks, true},
		{"left out", entitlement.LeftOut{entitlement.Webhooks}, entitlement.Webhooks, false},
		{"another left out", entitlement.LeftOut{entitlement.Webhooks}, entitlement.AutomationRules, true},
		{"all left out", entitlement.Features(), entitlement.OwnSignInProviders, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.leftOut.Includes(tc.feature); got != tc.included {
				t.Fatalf("Includes = %v, want %v", got, tc.included)
			}
			err := tc.leftOut.Require(tc.feature)
			if tc.included {
				if err != nil {
					t.Fatalf("Require refused an included feature: %v", err)
				}
				return
			}
			if !errors.Is(err, shared.ErrForbidden) {
				t.Fatalf("Require = %v, want forbidden", err)
			}
			refusal := shared.AsError(err)
			if refusal.DetailCode != entitlement.NotIncludedCode || refusal.Params["feature"] != string(tc.feature) {
				t.Fatalf("refusal = %s %v", refusal.DetailCode, refusal.Params)
			}
		})
	}
}

func TestOrderedIsTheDomainsOrderEachOnce(t *testing.T) {
	got := entitlement.Ordered([]entitlement.Feature{
		entitlement.OwnSignInProviders, entitlement.AutomationRules, entitlement.OwnSignInProviders, "UNKNOWN",
	})
	want := entitlement.LeftOut{entitlement.AutomationRules, entitlement.OwnSignInProviders}
	if !slices.Equal(got, want) {
		t.Fatalf("Ordered = %v, want %v", got, want)
	}
}
