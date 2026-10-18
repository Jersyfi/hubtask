// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The provider's account link makes the round trip through the installation's and a plan's rows,
// and resolves for a workspace in its own scope: its plan's over the installation's, the
// installation's where it is on no plan or on a plan without one (UC-INS-13 check 11).
func TestTheAccountLinkResolvesFromStoredLevels(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	instance := postgres.NewInstanceSettingRepository()
	plans := postgres.NewPlanRepository()

	// The installation's level is everybody's: read it, extend it, and put it back afterwards.
	var before identityrepo.InstanceLevel
	inSystem(ctx, t, uow, func(ctx context.Context) error {
		var err error
		before, err = instance.Read(ctx)
		return err
	})
	t.Cleanup(func() {
		inSystem(ctx, t, uow, func(ctx context.Context) error { return instance.Write(ctx, before, "", time.Now()) })
	})
	const installation, family = "https://provider.example/account", "https://shop.example/family"
	extended := before
	extended.Legal.Links = extended.Legal.Links.With(identity.LinkAccount, installation)
	inSystem(ctx, t, uow, func(ctx context.Context) error { return instance.Write(ctx, extended, "", time.Now()) })

	withLink := newPlan(t, planName("acct"))
	withLink.Values.Legal = identity.LegalLayer{
		Links: identity.LegalLinks{}.With(identity.LinkAccount, family),
		Locks: map[identity.LegalLink]bool{},
	}
	without := newPlan(t, planName("noacct"))
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, withLink, "") })
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, without, "") })

	onFamily, onOther, onNone := seedWorkspace(ctx, t, "acctfam"), seedWorkspace(ctx, t, "acctoth"), seedWorkspace(ctx, t, "acctnone")
	for workspace, on := range map[shared.ID]shared.ID{onFamily: withLink.ID, onOther: without.ID} {
		if _, err := admin.Exec(ctx, `UPDATE tenant SET plan_id = $1 WHERE id = $2`, on.String(), workspace.String()); err != nil {
			t.Fatal(err)
		}
	}

	resolver := identityservice.SignInPolicyResolver{
		Workspaces: postgres.NewWorkspaceSettingsRepository(), Instance: instance, UnitOfWork: uow,
	}
	for workspace, want := range map[shared.ID]string{onFamily: family, onOther: installation, onNone: installation} {
		got, err := resolver.AccountLink(ctx, workspace)
		if err != nil || got != want {
			t.Errorf("the workspace %s resolved %q (%v), want %q", workspace, got, err, want)
		}
	}

	// The plan's row reads back as it was written.
	var read identityrepo.InstanceLevel
	inSystem(ctx, t, uow, func(ctx context.Context) error {
		var err error
		read, err = instance.Read(ctx)
		return err
	})
	if read.Legal.Links.AccountURL != installation {
		t.Errorf("the installation's link read back %q", read.Legal.Links.AccountURL)
	}
}
