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
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// ADR-0075 §2 (SC-16): a step-up at the provider is a sign-in flow bound to the session that asked.
// Its state finishes that session's step-up and nothing else - not another session's, not a
// sign-in - and a sign-in flow finishes no step-up. Gate SG-3: none of it reaches another workspace.

var (
	stepUpFlowSession = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd01")
	stepUpFlowOther   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd02")
)

func stepUpFlowState(t *testing.T, seed byte) domain.Token {
	t.Helper()
	material := make([]byte, domain.TokenSecretBytes)
	for i := range material {
		material[i] = seed
	}
	state, err := domain.NewOidcFlowState(idpTenantA, material)
	if err != nil {
		t.Fatalf("minting a state: %v", err)
	}
	return state
}

func TestAStepUpFlowFinishesOnlyItsOwnSessionsStepUp(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)
	admin := adminPool(ctx, t)
	for _, id := range []shared.ID{stepUpFlowSession, stepUpFlowOther} {
		if _, err := admin.Exec(ctx, `
			INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
			VALUES ($1, $2, $3, now(), now() + interval '1 day') ON CONFLICT (id) DO NOTHING`,
			id.String(), idpTenantA.String(), idpAccount.String()); err != nil {
			t.Fatalf("seeding a session: %v", err)
		}
	}

	flows := postgres.NewOidcFlowRepository(security.NewOidcFlowHasher(secret.New(installationSecret)))
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()

	open := func(id shared.ID, session shared.ID, state domain.Token) {
		flow, err := domain.NewOidcFlow(domain.NewOidcFlowInput{
			ID: id, TenantID: idpTenantA, ProviderID: idpRowA, SessionID: session,
			Nonce: "the-nonce", Verifier: "v-verifier-that-is-long-enough-for-rfc-7636-000", Now: now,
		})
		if err != nil {
			t.Fatalf("building a flow: %v", err)
		}
		inTenant(t, uow, idpTenantA, func(ctx context.Context) error { return flows.Insert(ctx, flow, state) })
	}
	stepUpState := stepUpFlowState(t, 0x51)
	signInState := stepUpFlowState(t, 0x52)
	open(shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd11"), stepUpFlowSession, stepUpState)
	open(shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd12"), shared.ID(""), signInState)

	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		// A sign-in callback does not finish a step-up flow ...
		if _, found, err := flows.Consume(ctx, stepUpState, now); err != nil || found {
			t.Errorf("a sign-in spent a step-up flow: (%v, %v)", found, err)
		}
		// ... another session does not finish this session's step-up ...
		if _, found, err := flows.ConsumeForStepUp(ctx, stepUpState, stepUpFlowOther, now); err != nil || found {
			t.Errorf("another session spent this session's step-up flow: (%v, %v)", found, err)
		}
		// ... and a sign-in flow finishes no step-up.
		if _, found, err := flows.ConsumeForStepUp(ctx, signInState, stepUpFlowSession, now); err != nil || found {
			t.Errorf("a step-up spent a sign-in flow: (%v, %v)", found, err)
		}
		return nil
	})

	// Gate SG-3: B cannot spend A's step-up flow, however exactly it presents the session.
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		if _, found, err := flows.ConsumeForStepUp(ctx, stepUpState, stepUpFlowSession, now); err != nil || found {
			t.Errorf("B spent A's step-up flow: (%v, %v)", found, err)
		}
		return nil
	})

	// Its own session spends it, once, and learns which provider it went to.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		consumed, found, err := flows.ConsumeForStepUp(ctx, stepUpState, stepUpFlowSession, now)
		if err != nil || !found {
			t.Fatalf("the session spending its own step-up flow: (%v, %v)", found, err)
		}
		if consumed.ProviderID != idpRowA || consumed.SessionID != stepUpFlowSession || consumed.Nonce != "the-nonce" {
			t.Errorf("the flow came back as %+v", consumed)
		}
		if _, found, _ := flows.ConsumeForStepUp(ctx, stepUpState, stepUpFlowSession, now); found {
			t.Error("a spent step-up flow was spent a second time")
		}
		// The sign-in flow is still a sign-in's to spend.
		if _, found, err := flows.Consume(ctx, signInState, now); err != nil || !found {
			t.Errorf("the sign-in flow was not a sign-in's: (%v, %v)", found, err)
		}
		return nil
	})
}
