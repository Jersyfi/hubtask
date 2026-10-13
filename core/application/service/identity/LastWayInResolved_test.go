// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// UC-ID-12 check 6 and UC-ID-11 check 8 under the installation's level: whether the password is
// still a way in is the rule in force - the installation's default and lock over the workspace's
// own layer - not the workspace's own setting. A workspace row stored before a lock or a default
// moved past it reads differently from the rule it is under, which is the case this holds.
func TestTheLastWayInGuardReadsTheResolvedWaysIn(t *testing.T) {
	methods := func(m ...string) *[]string { return &m }
	cases := []struct {
		name      string
		instance  domain.PolicyLayer
		workspace *[]string
		refused   bool
	}{
		{name: "a lock to providers alone, the workspace set nothing",
			instance: lockedTo(methods(domain.MethodOidc)), refused: true},
		{name: "a lock to providers alone over a workspace that kept the password before it",
			instance:  lockedTo(methods(domain.MethodOidc)),
			workspace: methods(domain.MethodDirect, domain.MethodOidc), refused: true},
		{name: "a lock that keeps the password over a workspace that dropped it before it",
			instance:  lockedTo(methods(domain.MethodDirect, domain.MethodOidc)),
			workspace: methods(domain.MethodOidc)},
		{name: "a default of providers alone, the workspace set nothing",
			instance: domain.PolicyLayer{Patch: domain.PolicyPatch{Methods: methods(domain.MethodOidc)}},
			refused:  true},
		{name: "a default of the password alone over an older workspace without it",
			instance:  domain.PolicyLayer{Patch: domain.PolicyPatch{Methods: methods(domain.MethodDirect)}},
			workspace: methods(domain.MethodOidc)},
		{name: "no level above, the workspace dropped the password",
			workspace: methods(domain.MethodOidc), refused: true},
		{name: "no level above, an empty list is the product default",
			workspace: methods()},
		{name: "no level above, nothing set"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, door := range []string{"switch", "removal"} {
				f := newOfferFixture(time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC))
				f.writer.Workspaces = f.workspaces
				f.writer.Instance = &instanceSettings{level: repository.InstanceLevel{Policy: tc.instance}}
				f.offer.Writer = f.writer
				f.workspaces.row.Settings.SignIn.Methods = tc.workspace
				id := f.own(t, true)

				var err error
				if door == "switch" {
					_, err = f.offer.Execute(t.Context(), providerActor(), id, false, "")
				} else {
					err = RemoveIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor(), id, "")
				}

				if tc.refused {
					refusedAsLastWayIn(t, err, "the "+door)
					if len(f.store.rows) != 1 || !f.store.rows[0].Enabled {
						t.Errorf("the %s changed the provider despite the refusal", door)
					}
					continue
				}
				if err != nil {
					t.Errorf("the %s was refused although the password is a way in: %v", door, err)
				}
			}
		})
	}
}

func lockedTo(methods *[]string) domain.PolicyLayer {
	return domain.PolicyLayer{
		Patch: domain.PolicyPatch{Methods: methods},
		Locks: map[domain.PolicySwitch]bool{domain.SwitchMethods: true},
	}
}
