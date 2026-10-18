// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"
	"testing"
)

// The provider's account link resolves installation → plan, never through a workspace, and is
// none of the links the footer, the sign-in card or a workspace's own policy carries (UC-INS-13
// check 11).
func TestTheAccountLinkResolvesFromTheInstallationAndThePlan(t *testing.T) {
	level := func(url string, locked bool) LegalLayer {
		return LegalLayer{Links: LegalLinks{}.With(LinkAccount, url), Locks: map[LegalLink]bool{LinkAccount: locked}}
	}
	const installation, plan = "https://provider.example/account", "https://shop.example/family"
	cases := []struct {
		name             string
		instance, planOf LegalLayer
		want             string
	}{
		{"nothing set anywhere", LegalLayer{}, LegalLayer{}, ""},
		{"the installation's", level(installation, false), LegalLayer{}, installation},
		{"the plan's over an open installation's", level(installation, false), level(plan, false), plan},
		{"the plan's where the installation sets none", LegalLayer{}, level(plan, false), plan},
		{"the installation's lock binds the plan", level(installation, true), level(plan, false), installation},
		{"a lock without a value leaves none", level("", true), level(plan, false), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EffectiveAccountLink(c.instance, c.planOf); got != c.want {
				t.Fatalf("EffectiveAccountLink = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTheAccountLinkIsNoneOfTheWorkspacesLinks(t *testing.T) {
	if slices.Contains(LegalLinkNames(), LinkAccount) || slices.Contains(LegalSettingNames(), LinkAccount) {
		t.Fatal("the account link is among the links a workspace, the footer or the sign-in card carries")
	}
	if !slices.Contains(LevelLegalSettingNames(), LinkAccount) {
		t.Fatal("the installation and a plan cannot set the account link")
	}
	if (LegalLinks{}).With(LinkAccount, "https://x.example").IsEmpty() != true {
		t.Error("an account link alone draws a footer")
	}
	declaration := DeclarationOfLink(LinkAccount)
	if declaration.Kind != KindLink || declaration.May != MayPlanReplace {
		t.Errorf("the account link is declared %+v", declaration)
	}
}
