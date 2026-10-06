// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// SC-32, against the real database: a provider arrival this workspace turns away is in the trail.
// The refusal used to be appended inside the arrival's transaction, which the refusal itself rolls
// back - so `identity_provider.provider_refused` was never stored, and every service test read it
// because their unit of work kept what a failing transaction wrote. The arrival here is an invited
// person coming without the invitation's link, at a provider that is not authoritative for the
// address: refused, nothing connected, nothing activated, and the refusal stored.
//
// The provider is the one thing this database cannot stand in for, so the relying party is a stub
// that answers the identity a provider would have verified. Everything else is the real service
// over the real stores.
var (
	sc32RefusalProvider = shared.MustParseID("01936f2a-7c1e-7000-8000-000000032a22")
	sc32RefusalInvited  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000032a13")
)

// stubRelying answers one verified identity, for every exchange.
type stubRelying struct{ identity identityprovider.Identity }

func (stubRelying) Check(context.Context, string) error { return nil }

func (stubRelying) AuthorizationURL(
	context.Context, identityprovider.Config, identityprovider.Authorization,
) (string, error) {
	return "https://sc32-refusal.example/authorize", nil
}

func (s stubRelying) Exchange(
	context.Context, identityprovider.Config, identityprovider.Exchange,
) (identityprovider.Identity, error) {
	return s.identity, nil
}

func TestARefusedProviderArrivalIsStoredInTheTrail(t *testing.T) {
	ctx := context.Background()
	seedSecondProofTenants(ctx, t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $2, 'USER', 'refused-invitee@sc32.example', 'Refused', 'INVITED')
		ON CONFLICT (id) DO UPDATE SET status = 'INVITED'`,
		sc32RefusalInvited.String(), sc32TenantA.String()); err != nil {
		t.Fatalf("seeding the invited account: %v", err)
	}

	writer, _, uow := realWriter(ctx, t)
	// A key nobody else in this database names, so no rotation drill meets this row's wrapping.
	writer.Encryptor = ring(t, crypto.KeyMaterial{
		ID: "k32refusal", Material: secret.New("sc32-refusal-key-not-a-real-secret"),
	})
	providers := postgres.NewIdentityProviderRepository()

	configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: sc32RefusalProvider, TenantID: sc32TenantA, Issuer: "https://sc32-refusal.example",
		ClientID: "hubtask", Provisioning: string(domain.ProvisionAny), Enabled: true, Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	inTenant(t, uow, sc32TenantA, func(ctx context.Context) error {
		if _, err := providers.Find(ctx, sc32RefusalProvider); err == nil {
			return nil // an earlier run in this database already configured it
		}
		sealed, err := writer.Encryptor.Seal(ctx, secret.New("s3cr3t"),
			identityservice.ClientSecretPurpose(sc32TenantA))
		if err != nil {
			return err
		}
		_, err = providers.Insert(ctx, configured, sealed)
		return err
	})

	oidc := identityservice.OidcWriter{
		Session:   writer,
		Providers: providers,
		Flows:     postgres.NewOidcFlowRepository(security.NewOidcFlowHasher(secret.New(installationSecret))),
		External:  postgres.NewExternalAccountRepository(),
		Accounts:  postgres.NewAccountRepository(),
		Relying: stubRelying{identity: identityprovider.Identity{
			Subject: "sc32-refused-subject", Email: "refused-invitee@sc32.example",
			EmailVerified: true, DisplayName: "Refused",
		}},
		RedirectURL: "https://hubtask.example/auth/callback",
	}

	refusals := func() int {
		t.Helper()
		var counted int
		if err := admin.QueryRow(ctx, `
			SELECT count(*) FROM audit_log
			WHERE tenant_id = $1 AND action = 'identity.provider_refused'`,
			sc32TenantA.String()).Scan(&counted); err != nil {
			t.Fatalf("counting the refusals: %v", err)
		}
		return counted
	}
	before := refusals()

	started, err := identityservice.StartOidcSignIn{Writer: oidc}.Execute(ctx,
		identityservice.StartOidcSignInCommand{
			ProviderID: sc32RefusalProvider, TenantHeader: sc32TenantA.String(),
		})
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	_, err = identityservice.CompleteOidcSignIn{Writer: oidc}.Execute(ctx,
		identityservice.CompleteOidcSignInCommand{Code: "the-code", State: started.State})
	if shared.AsError(err).DetailCode != "identity_provider.invitation_needs_link" {
		t.Fatalf("the arrival without the link answered %v", err)
	}

	if after := refusals(); after != before+1 {
		t.Errorf("the trail holds %d refusals after the arrival, want %d", after, before+1)
	}
	var status string
	var connected int
	if err := admin.QueryRow(ctx, `
		SELECT a.status, (SELECT count(*) FROM account_identity WHERE account_id = a.id)
		FROM account a WHERE a.id = $1`, sc32RefusalInvited.String()).Scan(&status, &connected); err != nil {
		t.Fatalf("reading the account back: %v", err)
	}
	if status != "INVITED" || connected != 0 {
		t.Errorf("the refused arrival left the account %s with %d connections", status, connected)
	}
}
