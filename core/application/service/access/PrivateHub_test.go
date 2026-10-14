// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// The manifest's question and the create's refusal are one question: a person holding a role
// anywhere may own a private hub of their own, a service account never, and asking records nothing.
func TestMayOwnPrivateHubAnswersTheCreatesQuestionWithoutRecording(t *testing.T) {
	cases := []struct {
		name         string
		actor        appshared.ActorContext
		holdsNothing bool
		want         bool
	}{
		{name: "a person holding a role", actor: actorWithScopes("containers:write"), want: true},
		{name: "a person holding nothing", actor: actorWithScopes("containers:write"), holdsNothing: true, want: false},
		{name: "a service account holding a role", actor: appshared.ActorContext{
			Kind: appshared.ActorServiceAccount, TenantID: tenantID, AccountID: accountID, Scopes: []string{"containers:write"},
		}, want: false},
		{name: "anonymous", actor: appshared.Anonymous("en", "UTC"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authorize, store, trail, _ := serviceWith([]identity.Membership{
				{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleViewer},
			})
			store.holdsNothing = tc.holdsNothing

			got, err := authorize.MayOwnPrivateHub(t.Context(), tc.actor)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("MayOwnPrivateHub = %v, want %v", got, tc.want)
			}
			if len(trail.entries) != 0 {
				t.Errorf("asking wrote %d audit entries, want none", len(trail.entries))
			}
			// The create asks the same and agrees.
			refused := authorize.AuthorizeOwnPrivateHub(t.Context(), tc.actor, request()) != nil
			if refused == tc.want {
				t.Errorf("the create refused = %v where the manifest answered %v", refused, tc.want)
			}
		})
	}
}
