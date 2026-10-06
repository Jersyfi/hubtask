// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// SC-32: a sign-in flow started from an invitation's own link carries the invited account
// (ADR-0078 §1, migration 0116) - back to the callback of its own workspace and to no other, and
// only an account of the workspace the flow belongs to (gate SG-3). Its own workspaces and
// accounts, named by nobody else in this shared database.
var (
	sc32TenantA  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000032a01")
	sc32TenantB  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000032a02")
	sc32InvitedA = shared.MustParseID("01936f2a-7c1e-7000-8000-000000032a11")
	sc32InvitedB = shared.MustParseID("01936f2a-7c1e-7000-8000-000000032a12")
	sc32Provider = shared.MustParseID("01936f2a-7c1e-7000-8000-000000032a21")
)

// seedSecondProofTenants writes the two workspaces and one invited account in each.
func seedSecondProofTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name)
		VALUES ($1, 'sc32-a', 'SC-32 A'), ($2, 'sc32-b', 'SC-32 B')
		ON CONFLICT (id) DO NOTHING`, sc32TenantA.String(), sc32TenantB.String()); err != nil {
		t.Fatalf("seeding the workspaces: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $2, 'USER', 'invited-a@sc32.example', 'Invited A', 'INVITED'),
		       ($3, $4, 'USER', 'invited-b@sc32.example', 'Invited B', 'INVITED')
		ON CONFLICT (id) DO UPDATE SET status = 'INVITED'`,
		sc32InvitedA.String(), sc32TenantA.String(),
		sc32InvitedB.String(), sc32TenantB.String()); err != nil {
		t.Fatalf("seeding the invited accounts: %v", err)
	}
}

// sc32Flow builds a flow of one workspace, from fixed material so that a re-run meets its own row.
func sc32Flow(t *testing.T, id, tenantID, invited shared.ID, seed byte) (domain.OidcFlow, domain.Token) {
	t.Helper()
	material := make([]byte, domain.TokenSecretBytes)
	for i := range material {
		material[i] = seed + byte(i)
	}
	state, err := domain.NewOidcFlowState(tenantID, material)
	if err != nil {
		t.Fatalf("minting a state: %v", err)
	}
	flow, err := domain.NewOidcFlow(domain.NewOidcFlowInput{
		ID: id, TenantID: tenantID, ProviderID: sc32Provider, Nonce: "the-nonce",
		Verifier: "v-verifier-that-is-long-enough-for-rfc-7636-000", Now: time.Now().UTC(),
		InvitedAccountID: invited,
	})
	if err != nil {
		t.Fatalf("building a flow: %v", err)
	}
	return flow, state
}

func TestAFlowCarriesItsInvitationHomeAndNowhereElse(t *testing.T) {
	ctx := context.Background()
	seedSecondProofTenants(ctx, t)
	admin := adminPool(ctx, t)
	// An earlier run's flows, so that the fixed states below are fresh.
	if _, err := admin.Exec(ctx, `DELETE FROM oidc_flow WHERE tenant_id IN ($1, $2)`,
		sc32TenantA.String(), sc32TenantB.String()); err != nil {
		t.Fatalf("clearing an earlier run's flows: %v", err)
	}

	flows := postgres.NewOidcFlowRepository(security.NewOidcFlowHasher(secret.New(installationSecret)))
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()

	invitedFlow, invitedState := sc32Flow(t,
		shared.MustParseID("01936f2a-7c1e-7000-8000-000000032a31"), sc32TenantA, sc32InvitedA, 1)
	plainFlow, plainState := sc32Flow(t,
		shared.MustParseID("01936f2a-7c1e-7000-8000-000000032a32"), sc32TenantA, "", 61)
	inTenant(t, uow, sc32TenantA, func(ctx context.Context) error {
		if err := flows.Insert(ctx, invitedFlow, invitedState); err != nil {
			return err
		}
		return flows.Insert(ctx, plainFlow, plainState)
	})

	// Gate SG-3: B names A's invited account on a flow of its own, and the key refuses it - a
	// workspace cannot bind its sign-in to another workspace's invitation.
	foreignFlow, foreignState := sc32Flow(t,
		shared.MustParseID("01936f2a-7c1e-7000-8000-000000032a33"), sc32TenantB, sc32InvitedA, 121)
	if err := uow.Within(ctx, persistence.Scope{TenantID: sc32TenantB}, func(ctx context.Context) error {
		return flows.Insert(ctx, foreignFlow, foreignState)
	}); err == nil {
		t.Error("B wrote a flow carrying A's invited account")
	}

	// Gate SG-3: B cannot spend A's flow, and learns nothing of the invitation it carries.
	inTenant(t, uow, sc32TenantB, func(ctx context.Context) error {
		consumed, found, err := flows.Consume(ctx, invitedState, now)
		if err != nil {
			t.Fatalf("B consuming A's state: %v", err)
		}
		if found || !consumed.InvitedAccountID.IsZero() {
			t.Error("B spent A's flow, or read the invitation it carries")
		}
		return nil
	})

	// A spends both: the one started from the invitation carries the account, the other none.
	inTenant(t, uow, sc32TenantA, func(ctx context.Context) error {
		consumed, found, err := flows.Consume(ctx, invitedState, now)
		if err != nil || !found {
			t.Fatalf("A consuming its invited flow: (%v, %v)", found, err)
		}
		if consumed.InvitedAccountID != sc32InvitedA {
			t.Errorf("the flow came back carrying %q, want the invited account", consumed.InvitedAccountID)
		}
		plain, found, err := flows.Consume(ctx, plainState, now)
		if err != nil || !found {
			t.Fatalf("A consuming its plain flow: (%v, %v)", found, err)
		}
		if !plain.InvitedAccountID.IsZero() {
			t.Errorf("a flow without an invitation came back carrying %q", plain.InvitedAccountID)
		}
		return nil
	})
}
