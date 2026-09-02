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
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The relying party's surface against the real boundary (H-04). Gate SG-3: one workspace's
// provider, its sealed secret, its sign-in flows and its people's provider subjects are all
// invisible and unusable next door.

var (
	idpTenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc01")
	idpTenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc02")
	idpAccount = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc11")
)

func seedIdentityProviderTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	statements := []string{
		`INSERT INTO tenant (id, slug, display_name)
		 VALUES ('` + idpTenantA.String() + `', 'idp-a', 'IdP A'),
		        ('` + idpTenantB.String() + `', 'idp-b', 'IdP B')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		 VALUES ('` + idpAccount.String() + `', '` + idpTenantA.String() + `', 'USER',
		         'ada@example.org', 'Ada', 'ACTIVE')
		 ON CONFLICT (id) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding the provider tenants: %v", err)
		}
	}
}

func TestOneWorkspacesProviderIsInvisibleNextDoor(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)

	providers := postgres.NewIdentityProviderRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()

	configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		TenantID: idpTenantA, Issuer: "https://login.a.example", ClientID: "hubtask-a",
		AllowedEmailDomains: []string{"a.example"}, Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the configuration: %v", err)
	}
	sealed := cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("A's sealed client secret")}

	// A configures its provider.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		stored, err := providers.Upsert(ctx, configured, sealed, now)
		if err != nil {
			t.Fatalf("writing A's provider: %v", err)
		}
		if stored.Issuer != "https://login.a.example" || stored.Version != 1 {
			t.Errorf("A's provider came back as %+v", stored)
		}
		return nil
	})

	// Gate SG-3: B sees no provider at all, and cannot delete one.
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		if _, err := providers.Find(ctx); err == nil {
			t.Error("A's provider is visible next door")
		} else if !isNotFound(err) {
			t.Errorf("B's read answered %v, want not found", err)
		}
		if _, _, err := providers.FindWithSecret(ctx); err == nil {
			t.Error("A's sealed secret is readable next door")
		}
		removed, err := providers.Delete(ctx)
		if err != nil {
			t.Fatalf("B's delete: %v", err)
		}
		if removed {
			t.Error("B deleted A's provider")
		}
		return nil
	})

	// And A still has it, secret and all.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		found, envelope, err := providers.FindWithSecret(ctx)
		if err != nil {
			t.Fatalf("reading A's provider back: %v", err)
		}
		if found.ClientID != "hubtask-a" {
			t.Errorf("A's client id is %q", found.ClientID)
		}
		if string(envelope.Ciphertext) != "A's sealed client secret" || envelope.KeyID != "k1" {
			t.Error("A's envelope did not come back as it was stored")
		}
		return nil
	})

	// The upsert replaces rather than duplicating, and the version rises.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		second := configured
		second.Issuer = "https://login.a2.example"
		stored, err := providers.Upsert(ctx, second, sealed, now.Add(time.Minute))
		if err != nil {
			t.Fatalf("reconfiguring: %v", err)
		}
		if stored.Version != 2 || stored.Issuer != "https://login.a2.example" {
			t.Errorf("the reconfiguration answered %+v", stored)
		}
		return nil
	})
	var rows int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM identity_provider WHERE tenant_id = $1`, idpTenantA.String(),
	).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("A holds %d provider rows (%v), want exactly one", rows, err)
	}
}

func TestOneWorkspacesSignInFlowsAndSubjectsStayHome(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)

	flows := postgres.NewOidcFlowRepository(security.NewOidcFlowHasher(secret.New(installationSecret)))
	external := postgres.NewExternalAccountRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()

	material := make([]byte, domain.TokenSecretBytes)
	for i := range material {
		material[i] = byte(i + 1)
	}
	state, err := domain.NewOidcFlowState(idpTenantA, material)
	if err != nil {
		t.Fatalf("minting a state: %v", err)
	}
	flow, err := domain.NewOidcFlow(domain.NewOidcFlowInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc21"), TenantID: idpTenantA,
		Nonce: "the-nonce", Verifier: "v-verifier-that-is-long-enough-for-rfc-7636-000", Now: now,
	})
	if err != nil {
		t.Fatalf("building a flow: %v", err)
	}

	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		return flows.Insert(ctx, flow, state)
	})

	// Gate SG-3: B cannot spend A's state, however exactly it presents it.
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		_, found, err := flows.Consume(ctx, state, now)
		if err != nil {
			t.Fatalf("B consuming A's state: %v", err)
		}
		if found {
			t.Error("B spent A's sign-in flow")
		}
		return nil
	})

	// A spends it once, and only once.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		consumed, found, err := flows.Consume(ctx, state, now)
		if err != nil || !found {
			t.Fatalf("A consuming its own state: (%v, %v)", found, err)
		}
		if consumed.Nonce != "the-nonce" {
			t.Errorf("the flow came back with nonce %q", consumed.Nonce)
		}
		return nil
	})
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		if _, found, _ := flows.Consume(ctx, state, now); found {
			t.Error("a spent state was consumed a second time")
		}
		return nil
	})

	// The subject seam: A links one of its people, and B finds nothing by that subject.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		linked, err := external.LinkSubject(ctx, idpAccount, "subject-in-a", now)
		if err != nil || !linked {
			t.Fatalf("linking A's account: (%v, %v)", linked, err)
		}
		return nil
	})
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		if _, err := external.FindBySubject(ctx, "subject-in-a"); err == nil {
			t.Error("B found A's account by its provider subject")
		}
		return nil
	})
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		found, err := external.FindBySubject(ctx, "subject-in-a")
		if err != nil {
			t.Fatalf("A finding its own account by subject: %v", err)
		}
		if found.ID != idpAccount {
			t.Errorf("the subject named %s", found.ID)
		}
		// A second link on the same account is refused: the column is written once, and an
		// account already bound is never quietly re-pointed.
		relinked, err := external.LinkSubject(ctx, idpAccount, "another-subject", now)
		if err != nil {
			t.Fatalf("re-linking: %v", err)
		}
		if relinked {
			t.Error("an account already bound to a subject was re-pointed at another")
		}
		return nil
	})
}
