// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

var viewerID = shared.MustParseID("0192f000-0000-7000-8000-0000000000ef")

// UC-ID-29 check 2: a view shows only what both the person and the viewer may read - a hub the
// person reads and the viewer does not is refused to the view, and the trail names the viewer.
func TestAViewShowsOnlyWhatBothMayRead(t *testing.T) {
	read := Request{
		Permission: service.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)},
		Action:     "container.read", TokenScope: "containers:read", TargetType: "container", TargetID: hubID,
	}
	cases := []struct {
		name   string
		viewer []identity.Membership
		allow  bool
	}{
		{"the viewer reads it too", []identity.Membership{{AccountID: viewerID, Scope: identity.TenantScope(), Role: identity.RoleAdmin}}, true},
		{"the viewer reads nothing there", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			authorize, store, trail, _ := serviceWith(nil)
			store.byAccount = map[shared.ID][]identity.Membership{
				accountID: {{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleMember}},
				viewerID:  c.viewer,
			}
			view := actorWithScopes("containers:read")
			view.ViewedBy = viewerID

			err := authorize.Authorize(context.Background(), view, read)
			allowed, permitsErr := authorize.Permits(context.Background(), view, read)
			reach, reachErr := authorize.ReachInto(context.Background(), view, read, hubID)
			if c.allow {
				if err != nil || permitsErr != nil || !allowed || reachErr != nil || !reach.All {
					t.Fatalf("refused: %v %v %v %v %+v", err, allowed, permitsErr, reachErr, reach)
				}
				return
			}
			if !errors.Is(err, shared.ErrForbidden) || allowed || !errors.Is(reachErr, shared.ErrForbidden) {
				t.Fatalf("the view read past its viewer: %v %v %v", err, allowed, reachErr)
			}
			if len(trail.entries) == 0 || deniedBy(trail.entries[0]) != "viewer" {
				t.Fatalf("entries = %+v", trail.entries)
			}
		})
	}
}
