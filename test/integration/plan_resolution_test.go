// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A workspace carries its plan's sign-in and legal layers, read in its own scope; a workspace on no
// plan, or on another, never reads this one's (a cross-tenant negative test).
func TestAWorkspaceReadsItsOwnPlan(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	plans := postgres.NewPlanRepository()
	workspaces := postgres.NewWorkspaceSettingsRepository()
	family := newPlan(t, planName("res"))
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, family, "") })

	onPlan, onNone := seedWorkspace(ctx, t, "onplan"), seedWorkspace(ctx, t, "onnone")
	if _, err := admin.Exec(ctx, `UPDATE tenant SET plan_id = $1 WHERE id = $2`, family.ID.String(), onPlan.String()); err != nil {
		t.Fatal(err)
	}

	read := func(id shared.ID) identity.Workspace {
		t.Helper()
		var found identity.Workspace
		if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: id}, func(ctx context.Context) error {
			var err error
			found, err = workspaces.Find(ctx)
			return err
		}); err != nil {
			t.Fatalf("reading the workspace: %v", err)
		}
		return found
	}

	held := read(onPlan)
	if held.Plan.Name != family.Name || held.Plan.DisplayName != "Family" ||
		held.Plan.Policy.Patch.MinLength == nil || *held.Plan.Policy.Patch.MinLength != 16 ||
		!held.Plan.Policy.Locks[identity.SwitchMinLength] ||
		held.Plan.Legal.Links.Of(identity.LinkTerms) != "https://provider.example/family-terms" {
		t.Errorf("the workspace on the plan read %+v", held.Plan)
	}
	if other := read(onNone); other.Plan.Name != "" || other.Plan.Policy.Patch.MinLength != nil {
		t.Errorf("a workspace on no plan read %+v", other.Plan)
	}
}

// identity.md §18: an invite-only offer a plan's lock on the ways in turns on counts every live
// workspace on that plan, so the installation cannot remove a provider a plan still mandates.
func TestAPlansMandateCountsItsWorkspaces(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	plans := postgres.NewPlanRepository()
	providers := postgres.NewIdentityProviderRepository()

	methods := []string{identity.MethodOidc}
	mandating := newPlan(t, planName("mandate"))
	mandating.Values.Policy = identity.PolicyLayer{
		Patch: identity.PolicyPatch{Methods: &methods},
		Locks: map[identity.PolicySwitch]bool{identity.SwitchMethods: true},
	}
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, mandating, "") })

	offerID := freshID(t)
	offer, err := identity.NewIdentityProvider(identity.NewIdentityProviderInput{
		ID: offerID, Issuer: "https://id.mandate.example", ClientID: "hubtask-plan",
		Provisioning: string(identity.ProvisionInvitedOnly), Enabled: true, Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("building the offer: %v", err)
	}
	inSystem(ctx, t, uow, func(ctx context.Context) error {
		_, err := providers.Insert(ctx, offer, cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("s")})
		return err
	})
	t.Cleanup(func() {
		_, _ = admin.Exec(context.WithoutCancel(ctx), `DELETE FROM identity_provider WHERE id = $1`, offerID.String())
	})

	count := func() int {
		t.Helper()
		var counted int
		inSystem(ctx, t, uow, func(ctx context.Context) error {
			found, err := providers.Find(ctx, offerID)
			counted = found.OfferedWorkspaces
			return err
		})
		return counted
	}

	onPlan := seedWorkspace(ctx, t, "mandated")
	if got := count(); got != 0 {
		t.Fatalf("before any workspace is on the plan the count is %d", got)
	}
	if _, err := admin.Exec(ctx, `UPDATE tenant SET plan_id = $1 WHERE id = $2`, mandating.ID.String(), onPlan.String()); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 1 {
		t.Errorf("a workspace on the mandating plan is counted %d times, want 1", got)
	}
	if _, err := admin.Exec(ctx, `UPDATE tenant SET plan_id = NULL WHERE id = $1`, onPlan.String()); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 0 {
		t.Errorf("a workspace moved off the plan is still counted: %d", got)
	}
}
