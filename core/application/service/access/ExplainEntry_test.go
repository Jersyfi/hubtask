// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// The explanation of one entry is the decision the person meets: for every case, ExplainEntry
// permits exactly what Authorize permits, and says why.
func TestTheExplanationOfAnEntryIsTheDecisionThePersonMeets(t *testing.T) {
	cases := []struct {
		name     string
		held     []identity.Membership
		does     service.ItemAction
		assignee shared.ID
		want     EntryReason
	}{
		{"a guest reads the entry shared with them", held(identity.ItemScope(itemID), identity.RoleGuest),
			service.ItemRead, "", ThroughShare},
		{"a guest sees the entry but cannot change it", held(identity.ItemScope(itemID), identity.RoleGuest),
			service.ItemChange, "", RoleRefuses},
		{"a contributor changes what is assigned to them", held(identity.CollectionScope(collectionID), identity.RoleContributor),
			service.ItemChange, accountID, AsAssignee},
		{"a contributor cannot change what is somebody else's", held(identity.CollectionScope(collectionID), identity.RoleContributor),
			service.ItemChange, otherAccount, NotAssignee},
		{"a member changes any entry by their role", held(identity.HubScope(hubID), identity.RoleMember),
			service.ItemChange, otherAccount, ThroughRole},
		{"somebody who holds nothing there", held(identity.CollectionScope(neighbourID), identity.RoleMember),
			service.ItemRead, "", NothingHeld},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authorize, _, _, _ := serviceWith(tc.held)
			request := itemRequest(tc.does, tc.assignee)
			refused := authorize.Authorize(context.Background(), actorWithScopes("items:write"), request)

			explained, err := authorize.ExplainEntry(context.Background(), actorWithScopes("items:write"), accountID,
				request.Path, request.On)
			if err != nil {
				t.Fatalf("explaining: %v", err)
			}
			if explained.Permitted != (refused == nil) {
				t.Fatalf("the explanation says permitted=%v, the decision refused with %v", explained.Permitted, refused)
			}
			if explained.Reason != tc.want {
				t.Errorf("reason %s, want %s", explained.Reason, tc.want)
			}
		})
	}
}
