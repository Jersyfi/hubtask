// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"testing"

	usecases "github.com/Jersyfi/hubtask/core/application/catalogue"
)

// privilegedUseCases is security.md §5's list, as far as its operations exist in this build: the
// use cases that must demand a fresh step-up, and why.
//
// The map and the descriptors' own StepUp declarations are reconciled in both directions, so the
// next privileged action ships with a declaration or fails this build - and a declaration nobody
// meant cannot appear without being named here (identity.md §16.3).
var privilegedUseCases = map[string]string{
	"ConfigureIdentityProvider":                "which provider may vouch for this workspace's people is a way in (identity.md §16.3)",
	"ConfigureFirstIdentityProvider":           "which provider may vouch for this workspace's people is a way in (identity.md §16.3)",
	"RemoveIdentityProvider":                   "which provider may vouch for this workspace's people is a way in (identity.md §16.3)",
	"OfferIdentityProvider":                    "which provider may vouch for this workspace's people is a way in (identity.md §16.3)",
	"ConfigureInstanceIdentityProvider":        "a way in offered to every workspace (identity.md §16.3)",
	"RemoveInstanceIdentityProvider":           "a way in offered to every workspace (identity.md §16.3)",
	"StartRestore":                             "the destructive restore modes replace data (backup-restore.md §8.3)",
	"GrantMembership":                          "granting OWNER is changing the OWNER role",
	"RevokeMembership":                         "revoking an OWNER membership is changing the OWNER role",
	"CreateAccessToken":                        "an admin-scoped token reaches the control plane",
	"RequestTenantDeletion":                    "it is the request that ends a workspace (multi-tenancy.md §5)",
	"OpenTenantPassword":                       "it widens the way into somebody else's workspace (ADR-0078 §3)",
	"DisableTotp":                              "a stolen session removing the second factor is the attack the factor exists against (ADR-0075 §3)",
	"StartAuthenticatorReplacement":            "replacing the second factor is the same power as removing it (identity.md §8)",
	"WithdrawInstanceIdentityProvider":         "ends a way in every workspace is offered; Withdraw now is the answer to a compromised provider (ADR-0076 §3)",
	"CancelInstanceIdentityProviderWithdrawal": "offers a way in to every workspace again (ADR-0076 §2)",
}

func TestEveryPrivilegedOperationDeclaresItsStepUp(t *testing.T) {
	declared := map[string]bool{}
	for _, descriptor := range usecases.Descriptors() {
		if descriptor.StepUp != "" {
			declared[descriptor.Name] = true
		}
		if privilegedUseCases[descriptor.Name] != "" && descriptor.StepUp == "" {
			t.Errorf("%s is a privileged operation (%s) and declares no step-up - "+
				"a list in a document is how the next privileged action ships without one",
				descriptor.Name, privilegedUseCases[descriptor.Name])
		}
	}
	for name := range declared {
		if privilegedUseCases[name] == "" {
			t.Errorf("%s declares a step-up and is not in the privileged list - "+
				"add it there with its reason, so the two cannot drift apart", name)
		}
	}
	if len(declared) == 0 {
		t.Fatal("nothing declares a step-up at all - the declaration no longer reaches the catalogue")
	}
}
