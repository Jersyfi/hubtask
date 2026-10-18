// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package plan

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// The provider's account link at a plan (multi-tenancy.md §4.2): set where the installation leaves
// it open, refused under its lock, and never locked by the plan - nothing below a plan sets it.
func TestAPlansAccountLink(t *testing.T) {
	link := func(url string, locked bool) identity.LegalLayer {
		return identity.LegalLayer{
			Links: identity.LegalLinks{}.With(identity.LinkAccount, url),
			Locks: map[identity.LegalLink]bool{identity.LinkAccount: locked},
		}
	}
	cases := []struct {
		name  string
		plan  identity.LegalLayer
		above identity.LegalLayer
		want  string
	}{
		{"its own where the installation leaves it open", link("https://shop.example/account", false), link("https://provider.example", false), ""},
		{"its own where the installation sets none", link("https://shop.example/account", false), identity.LegalLayer{}, ""},
		{"under the installation's lock", link("https://shop.example/account", false), link("https://provider.example", true), "admin.plan_under_installation_lock"},
		{"locked by the plan", link("https://shop.example/account", true), identity.LegalLayer{}, "admin.plan_account_link_not_lockable"},
		{"none of its own under a lock", identity.LegalLayer{}, link("https://provider.example", true), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Values{Legal: c.plan}.FitsUnder(Above{Legal: c.above})
			if got := detailOf(err); got != c.want {
				t.Fatalf("FitsUnder = %q (%v), want %q", got, err, c.want)
			}
		})
	}
}
