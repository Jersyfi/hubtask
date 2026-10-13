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
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The terms of use agreed to (UC-ID-19, migration 0140), against the real boundary.

// Gate SG-3: TermsAgreementOf and RecordTermsAgreement.
func TestATermsAgreementIsReadAndWrittenOnlyInItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	_, _, signIn, uow := sessionStores(ctx, t)
	at := time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)

	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		changed, err := signIn.RecordTermsAgreement(ctx, sessionAccountA, domain.TermsAgreement{
			URL: "https://b.example/terms", Version: "9", AgreedAt: at,
		})
		if err != nil || changed {
			t.Errorf("another tenant's account agreed from here (%v, %v)", changed, err)
		}
		if _, err := signIn.TermsAgreementOf(ctx, sessionAccountA); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another tenant's agreement answered %v, want not found", err)
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		// Reset first: the integration database is shared between runs.
		if _, err := signIn.RecordTermsAgreement(ctx, sessionAccountA, domain.TermsAgreement{
			URL: "https://a.example/terms", Version: "", AgreedAt: at.Add(-time.Hour),
		}); err != nil {
			t.Fatalf("recording: %v", err)
		}
		unversioned, err := signIn.TermsAgreementOf(ctx, sessionAccountA)
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if unversioned.Version != "" || unversioned.URL != "https://a.example/terms" {
			t.Errorf("an agreement without a version came back as %+v", unversioned)
		}

		changed, err := signIn.RecordTermsAgreement(ctx, sessionAccountA, domain.TermsAgreement{
			URL: "https://a.example/terms", Version: "3", AgreedAt: at,
		})
		if err != nil || !changed {
			t.Fatalf("recording (%v, %v)", changed, err)
		}
		got, err := signIn.TermsAgreementOf(ctx, sessionAccountA)
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if got.URL != "https://a.example/terms" || got.Version != "3" || !got.AgreedAt.Equal(at) {
			t.Errorf("the agreement came back as %+v", got)
		}
		return nil
	})

	// The stored shape, read as an operator would, not through the adapter that wrote it.
	var url, version string
	var agreedAt time.Time
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT terms_url, terms_version, terms_agreed_at FROM account WHERE id = $1`,
		sessionAccountA.String()).Scan(&url, &version, &agreedAt); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if url != "https://a.example/terms" || version != "3" || !agreedAt.Equal(at) {
		t.Errorf("the row holds (%q, %q, %v)", url, version, agreedAt)
	}
}

// The TERMS step's pending credential keeps the session it opens: how the sign-in proved the
// account, and the provider that did.
func TestATermsStepKeepsTheSessionItOpens(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)
	pending, uow := mfaStores(ctx, t)
	now := time.Now().UTC()

	presented, err := domain.NewPendingToken(idpTenantA, sessionSecretOf(0xE7))
	if err != nil {
		t.Fatalf("minting a pending token: %v", err)
	}
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		return pending.Insert(ctx, domain.PendingCredential{
			ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc71"), TenantID: idpTenantA,
			AccountID: idpAccount, Purpose: domain.PendingTerms,
			CreatedAt: now, ExpiresAt: now.Add(domain.PendingLifetime),
			SessionMethod: domain.SignedInWithOidc, SessionVia: idpRowA,
		}, presented)
	})
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		if _, err := pending.FindByToken(ctx, presented); err == nil {
			t.Error("B found A's terms step")
		}
		return nil
	})
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		lookup, err := pending.FindByToken(ctx, presented)
		if err != nil {
			t.Fatalf("A reading its terms step: %v", err)
		}
		got := lookup.Credential
		if got.Purpose != domain.PendingTerms || got.SessionMethod != domain.SignedInWithOidc ||
			got.SessionVia != idpRowA {
			t.Errorf("the terms step came back as %+v", got)
		}
		return nil
	})
}

// The agreement given on the invitation card rides on the provider flow to the arrival.
func TestAnInvitationFlowCarriesTheTermsAgreed(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)
	flows := postgres.NewOidcFlowRepository(security.NewOidcFlowHasher(secret.New(installationSecret)))
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()

	state, err := domain.NewOidcFlowState(idpTenantA, sessionSecretOf(0xE8))
	if err != nil {
		t.Fatalf("minting a state: %v", err)
	}
	flow, err := domain.NewOidcFlow(domain.NewOidcFlowInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc72"), TenantID: idpTenantA,
		ProviderID: idpRowA, InvitedAccountID: idpAccount,
		Nonce: "the-nonce", Verifier: "v-verifier-that-is-long-enough-for-rfc-7636-000", Now: now,
		Terms: domain.TermsAgreement{URL: "https://a.example/terms", Version: "3"},
	})
	if err != nil {
		t.Fatalf("building a flow: %v", err)
	}
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		return flows.Insert(ctx, flow, state)
	})
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		consumed, found, err := flows.Consume(ctx, state, now)
		if err != nil || !found {
			t.Fatalf("consuming (%v, %v)", found, err)
		}
		if consumed.Terms.URL != "https://a.example/terms" || consumed.Terms.Version != "3" {
			t.Errorf("the flow's agreement came back as %+v", consumed.Terms)
		}
		return nil
	})
}
