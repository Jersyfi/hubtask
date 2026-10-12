// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// What newcomers get (UC-ID-11 check 5): nothing, one role on the workspace, or one group - and only
// where a newcomer exists to get it.
func TestWhatNewcomersGetIsHeldToWhereItMeansSomething(t *testing.T) {
	group := shared.ID("01936f2a-7c1e-7000-8000-0000000002a1")
	cases := []struct {
		name         string
		installation bool
		mode         Provisioning
		role         string
		group        shared.ID
		wantCode     string
		wantRole     Role
	}{
		{name: "nothing, the default", mode: ProvisionDomains},
		{name: "a role under DOMAINS", mode: ProvisionDomains, role: "member", wantRole: RoleMember},
		{name: "a role under ANY", mode: ProvisionAny, role: "VIEWER", wantRole: RoleViewer},
		{name: "an administrator's role is an administrator's choice", mode: ProvisionAny, role: "ADMIN", wantRole: RoleAdmin},
		{name: "a group under ANY", mode: ProvisionAny, group: group},
		{name: "a role and a group", mode: ProvisionAny, role: "MEMBER", group: group,
			wantCode: "identity_provider.newcomers_one"},
		{name: "INVITED_ONLY creates nobody", mode: ProvisionInvitedOnly, role: "MEMBER",
			wantCode: "identity_provider.newcomers_not_created"},
		{name: "INVITED_ONLY, a group", mode: ProvisionInvitedOnly, group: group,
			wantCode: "identity_provider.newcomers_not_created"},
		{name: "OWNER", mode: ProvisionDomains, role: "OWNER",
			wantCode: "identity_provider.newcomers_owner"},
		{name: "not a role", mode: ProvisionDomains, role: "BOSS",
			wantCode: "identity_provider.newcomer_role_invalid"},
		{name: "the installation's row", installation: true, mode: ProvisionInvitedOnly, role: "MEMBER",
			wantCode: "identity_provider.newcomers_installation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := providerInput()
			in.Provisioning = string(tc.mode)
			in.AllowedEmailDomains = []string{"example.org"}
			in.NewcomerRole = tc.role
			in.NewcomerGroupID = tc.group
			if tc.installation {
				in.TenantID = ""
			}
			configured, err := NewIdentityProvider(in)
			if tc.wantCode != "" {
				if err == nil || shared.AsError(err).DetailCode != tc.wantCode {
					t.Fatalf("answered %v, want %s", err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("configuring: %v", err)
			}
			if configured.NewcomerRole != tc.wantRole || configured.NewcomerGroupID != tc.group {
				t.Errorf("newcomers = %q/%q, want %q/%q",
					configured.NewcomerRole, configured.NewcomerGroupID, tc.wantRole, tc.group)
			}
			if got := configured.GivesNewcomers(); got != (tc.wantRole != "" || !tc.group.IsZero()) {
				t.Errorf("GivesNewcomers = %v", got)
			}
		})
	}
}
