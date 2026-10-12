// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

var (
	oaTenant       = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132c01")
	oaNobody       = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132c11")
	oaPrivateOnly  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132c12")
	oaEntryGuest   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132c13")
	oaGroupMember  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132c14")
	oaWorkspaceMem = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132c15")
	oaGroup        = shared.MustParseID("01936f2a-7c1e-7000-8000-000000132c21")
)

// `has_access` on `GET /accounts/me` is the full condition the newcomer notice needs (UC-ID-08
// check 2), and the same one the server puts on a private hub of one's own (UC-ID-16 check 1,
// ADR-0073 §2): every scope counts, directly or through a group. Where it is false the notice
// stands and no hub can be created; where it is true - a person whose only role is on their own
// private hub, a guest on one entry - the person may create a private hub, so *Create hub* stays.
// Through the registry as a request reaches it, against PostgreSQL as the application role.
func TestHasAccessIsExactlyWhatAPrivateHubOfOnesOwnNeeds(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	statements := []string{
		`INSERT INTO tenant (id, slug, display_name)
		 VALUES ('` + oaTenant.String() + `', 'own-access', 'Own access') ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status) VALUES
		 ('` + oaNobody.String() + `', '` + oaTenant.String() + `', 'USER', 'nobody@own-access.example', 'Nobody', 'ACTIVE'),
		 ('` + oaPrivateOnly.String() + `', '` + oaTenant.String() + `', 'USER', 'private@own-access.example', 'Private', 'ACTIVE'),
		 ('` + oaEntryGuest.String() + `', '` + oaTenant.String() + `', 'USER', 'guest@own-access.example', 'Guest', 'ACTIVE'),
		 ('` + oaGroupMember.String() + `', '` + oaTenant.String() + `', 'USER', 'grouped@own-access.example', 'Grouped', 'ACTIVE'),
		 ('` + oaWorkspaceMem.String() + `', '` + oaTenant.String() + `', 'USER', 'member@own-access.example', 'Member', 'ACTIVE')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO account_group (id, tenant_id, name)
		 VALUES ('` + oaGroup.String() + `', '` + oaTenant.String() + `', 'Readers') ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO account_group_member (tenant_id, group_id, account_id)
		 VALUES ('` + oaTenant.String() + `', '` + oaGroup.String() + `', '` + oaGroupMember.String() + `')
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO membership (id, tenant_id, account_id, group_id, scope_type, scope_id, role) VALUES
		 ('01936f2a-7c1e-7000-8000-000000132c31', '` + oaTenant.String() + `', '` + oaPrivateOnly.String() + `', NULL, 'HUB', '01936f2a-7c1e-7000-8000-000000132c41', 'OWNER'),
		 ('01936f2a-7c1e-7000-8000-000000132c32', '` + oaTenant.String() + `', '` + oaEntryGuest.String() + `', NULL, 'ITEM', '01936f2a-7c1e-7000-8000-000000132c42', 'GUEST'),
		 ('01936f2a-7c1e-7000-8000-000000132c33', '` + oaTenant.String() + `', NULL, '` + oaGroup.String() + `', 'COLLECTION', '01936f2a-7c1e-7000-8000-000000132c43', 'VIEWER'),
		 ('01936f2a-7c1e-7000-8000-000000132c34', '` + oaTenant.String() + `', '` + oaWorkspaceMem.String() + `', NULL, 'TENANT', NULL, 'MEMBER')
		 ON CONFLICT (id) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	authoriser := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork,
		Audit: postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed)), Clock: fixed,
	}
	registry, err := usecase.NewRegistry(nil, identityservice.GetOwnAccount{
		Accounts: postgres.NewAccountRepository(), UnitOfWork: unitOfWork,
		Access: postgres.NewMembershipRepository(),
	}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}

	cases := []struct {
		name    string
		account shared.ID
		access  bool
	}{
		{"nobody gave them anything", oaNobody, false},
		{"only their own private hub", oaPrivateOnly, true},
		{"a guest on one entry", oaEntryGuest, true},
		{"a role through a group", oaGroupMember, true},
		{"a role on the workspace", oaWorkspaceMem, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actor := appshared.ActorContext{
				Kind: appshared.ActorUser, TenantID: oaTenant, AccountID: tc.account, AccountName: "Person",
				Scopes: []string{"accounts:read"},
			}
			own, err := registry.Invoke(ctx, identityservice.GetOwnAccountName, actor, usecase.Input{})
			if err != nil {
				t.Fatalf("reading the own account: %v", err)
			}
			if own["has_access"] != tc.access {
				t.Errorf("has_access = %v, want %v", own["has_access"], tc.access)
			}
			refused := authoriser.AuthorizeOwnPrivateHub(ctx, actor, access.Request{}) != nil
			if refused == tc.access {
				t.Errorf("a private hub of one's own refused: %v, while has_access is %v", refused, tc.access)
			}
		})
	}
}
