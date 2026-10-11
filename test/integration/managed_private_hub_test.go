// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// managedOn answers the product's default: managed accounts allowed.
type managedOn struct{}

func (managedOn) Resolve(context.Context, shared.ID) (identityservice.ResolvedPolicy, error) {
	return identityservice.ResolvedPolicy{
		Effective: identity.Effective(identity.PolicyLayer{}, identity.PolicyLayer{}, identity.PolicyLayer{}),
	}, nil
}

func managedCreation(ctx context.Context, t *testing.T) identityservice.CreateManagedAccount {
	t.Helper()
	entropy := clockadapter.CryptoRandom{}
	passwords, err := crypto.NewPasswords(entropy)
	if err != nil {
		t.Fatalf("building the hasher: %v", err)
	}
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	sink := postgres.NewAuditSink(ids)
	authoriser := realAuthoriser(ctx, t)
	return identityservice.CreateManagedAccount{
		Managed: postgres.NewAccountRepository(), Authorizer: authoriser,
		Grant: identityservice.GrantMembership{
			Grants: postgres.NewMembershipGrantRepository(pageCursors()), Accounts: postgres.NewAccountRepository(),
			Groups: postgres.NewGroupRepository(pageCursors()), Hubs: postgres.NewHubLockRepository(),
			Authorizer: authoriser, Audit: sink, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		},
		Rule: managedOn{}, Passwords: passwords, Entropy: entropy,
		Audit: sink, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
	}
}

// UC-ID-20 check 1 at a private hub (ADR-0073 §1): the workspace's owner, who holds nothing in the
// hub, is answered as if it were not there - and no account is left behind - while a shared hub
// beside it takes the new person.
func TestAManagedAccountIsNotPlacedInAPrivateHubByOneOutsideIt(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	create := managedCreation(ctx, t)
	owner := actorIn(w.tenant, w.administrator, "members:write")

	hidden := freshSignInName(t)
	_, err := create.Execute(ctx, owner, identityservice.CreateManagedAccountCommand{
		DisplayName: "Lena", SignInName: hidden, Role: identity.RoleContributor, Scope: identity.HubScope(w.hub),
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("placing in a private hub from outside answered %v, want not found", err)
	}
	var left int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM account WHERE tenant_id = $1 AND sign_in_name = $2`,
		w.tenant.String(), hidden).Scan(&left); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if left != 0 {
		t.Errorf("the refused creation left %d account(s) behind", left)
	}

	created, err := create.Execute(ctx, owner, identityservice.CreateManagedAccountCommand{
		DisplayName: "Lena", SignInName: freshSignInName(t), Role: identity.RoleContributor,
		Scope: identity.HubScope(w.sharedHub),
	})
	if err != nil || created.Membership.Scope.ID != w.sharedHub || created.StartPassword.IsEmpty() {
		t.Fatalf("placing in the shared hub answered %+v, %v", created.Membership, err)
	}
}
