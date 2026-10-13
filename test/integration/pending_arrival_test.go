// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A provider's newcomer waiting on the TERMS step (migration 0142), against the real boundary: the
// row carries the arrival and no account, only FindArrivalByToken reads it, and only in its own
// workspace.

var (
	arrivalPendingA  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000141a01")
	arrivalPendingAc = shared.MustParseID("01936f2a-7c1e-7000-8000-000000141a02")
)

// Gate SG-3: FindArrivalByToken.
func TestANewcomersTermsStepCarriesTheArrivalAndNoAccount(t *testing.T) {
	ctx := context.Background()
	seedConnectTenants(ctx, t)
	pending, uow := mfaStores(ctx, t)
	now := time.Now().UTC()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO identity_provider (id, tenant_id, issuer, client_id, client_secret_enc,
		                               client_secret_key_id, created_at)
		VALUES ($1, $2, 'https://sc33.example/idp', 'hubtask', '\x00', 'k-sc33', now())
		ON CONFLICT (id) DO NOTHING`, sc33Provider.String(), sc33TenantA.String()); err != nil {
		t.Fatalf("seeding the provider: %v", err)
	}

	arrival := domain.ArrivalIntent{
		ProviderID: sc33Provider, Subject: "sub-newcomer", Email: "nora@sc33.example",
		DisplayName: "Nora", Directory: "dir-1",
	}
	newcomer := sc33Token(t, sc33TenantA, 141, false)
	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		return pending.Insert(ctx, domain.PendingCredential{
			ID: arrivalPendingA, TenantID: sc33TenantA, Purpose: domain.PendingTerms,
			CreatedAt: now, ExpiresAt: now.Add(domain.PendingLifetime),
			SessionMethod: domain.SignedInWithOidc, SessionVia: sc33Provider, Arrival: &arrival,
		}, newcomer)
	})
	// A row with an account, beside it: the new read never answers it.
	withAccount := sc33Token(t, sc33TenantA, 142, false)
	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		return pending.Insert(ctx, domain.PendingCredential{
			ID: arrivalPendingAc, TenantID: sc33TenantA, AccountID: sc33AccountA, Purpose: domain.PendingTerms,
			CreatedAt: now, ExpiresAt: now.Add(domain.PendingLifetime),
		}, withAccount)
	})

	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		found, err := pending.FindArrivalByToken(ctx, newcomer)
		if err != nil {
			t.Fatalf("reading the arrival back: %v", err)
		}
		if found.Credential.Arrival == nil || *found.Credential.Arrival != arrival {
			t.Errorf("the arrival came back as %+v, want %+v", found.Credential.Arrival, arrival)
		}
		if !found.Credential.AccountID.IsZero() || !found.Account.ID.IsZero() {
			t.Errorf("a newcomer's step names an account: %+v", found)
		}
		if found.Credential.SessionVia != sc33Provider || found.TenantStatus != domain.TenantActive {
			t.Errorf("the step's session and workspace came back as %+v", found)
		}
		// The steps that read FindByToken never meet a credential without an account.
		if _, err := pending.FindByToken(ctx, newcomer); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("FindByToken answered a newcomer's row: %v", err)
		}
		if _, err := pending.FindArrivalByToken(ctx, withAccount); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("FindArrivalByToken answered a row with an account: %v", err)
		}
		return nil
	})

	// Another workspace reads nothing, even with the secret rewritten to name it.
	rewritten := sc33Token(t, sc33TenantB, 141, false)
	inTenant(t, uow, sc33TenantB, func(ctx context.Context) error {
		if _, err := pending.FindArrivalByToken(ctx, rewritten); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another workspace read the arrival: %v", err)
		}
		return nil
	})

	// The checks hold the shape: no arrival on a row with an account, no row without either.
	if _, err := admin.Exec(ctx, `
		UPDATE auth_pending SET arrival_email = 'x@sc33.example' WHERE id = $1`,
		arrivalPendingAc.String()); err == nil {
		t.Error("a row with an account took an arrival")
	}
	if _, err := admin.Exec(ctx, `
		UPDATE auth_pending SET purpose = 'TOTP' WHERE id = $1`, arrivalPendingA.String()); err == nil {
		t.Error("a row without an account became another step")
	}
}
