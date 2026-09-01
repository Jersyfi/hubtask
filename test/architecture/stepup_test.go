// SPDX-License-Identifier: BUSL-1.1
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
// meant cannot appear without being named here (H-03).
var privilegedUseCases = map[string]string{
	"StartRestore":          "the destructive restore modes replace data (backup-restore.md §8.3)",
	"GrantMembership":       "granting OWNER is changing the OWNER role",
	"RevokeMembership":      "revoking an OWNER membership is changing the OWNER role",
	"CreateAccessToken":     "an admin-scoped token reaches the control plane",
	"RequestTenantDeletion": "it is the request that ends a workspace (multi-tenancy.md §5, H-06)",
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
