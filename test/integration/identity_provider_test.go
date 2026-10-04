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
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The relying party's surface against the real boundary (H-04). Gate SG-3: one workspace's
// provider, its sealed secret, its sign-in flows and its people's provider subjects are all
// invisible and unusable next door.

var (
	idpTenantA  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc01")
	idpTenantB  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc02")
	idpAccount  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc11")
	idpRowA     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc31")
	idpRowASec  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc32")
	idpRowShare = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc33")
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
		ID: idpRowA, TenantID: idpTenantA, Issuer: "https://login.a.example", ClientID: "hubtask-a",
		AllowedEmailDomains: []string{"a.example"}, Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the configuration: %v", err)
	}
	sealed := cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("A's sealed client secret")}

	// A configures its provider.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		stored, err := providers.Insert(ctx, configured, sealed)
		if err != nil {
			t.Fatalf("writing A's provider: %v", err)
		}
		if stored.Issuer != "https://login.a.example" || stored.Version != 1 {
			t.Errorf("A's provider came back as %+v", stored)
		}
		if stored.Installation() {
			t.Error("a workspace's row came back as the installation's")
		}
		return nil
	})

	// Gate SG-3: B sees no provider at all, and cannot change or delete one.
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		listed, err := providers.List(ctx)
		if err != nil {
			t.Fatalf("B's list: %v", err)
		}
		if len(listed) != 0 {
			t.Errorf("B sees %d providers, want none of A's", len(listed))
		}
		if _, err := providers.Find(ctx, idpRowA); err == nil {
			t.Error("A's provider is visible next door")
		} else if !isNotFound(err) {
			t.Errorf("B's read answered %v, want not found", err)
		}
		if _, _, err := providers.FindWithSecret(ctx, idpRowA); err == nil {
			t.Error("A's sealed secret is readable next door")
		}
		stolen := configured
		stolen.ClientID = "hubtask-b"
		if _, found, err := providers.Update(ctx, stolen, nil, now); err != nil || found {
			t.Errorf("B rewrote A's provider: (%v, %v)", found, err)
		}
		if _, found, err := providers.Reconfigure(ctx, stolen, nil, now); err != nil || found {
			t.Errorf("B reconfigured A's provider: (%v, %v)", found, err)
		}
		removed, err := providers.Delete(ctx, idpRowA, now)
		if err != nil {
			t.Fatalf("B's delete: %v", err)
		}
		if removed {
			t.Error("B deleted A's provider")
		}
		return nil
	})

	// And A still has it, secret and all - unchanged by what B tried.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		found, envelope, err := providers.FindWithSecret(ctx, idpRowA)
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

	// Plural: a second provider on the same workspace, and both are in force.
	second, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: idpRowASec, TenantID: idpTenantA, Issuer: "https://login.a2.example",
		ClientID: "hubtask-a2", Enabled: true, Position: 1, Now: now,
	})
	if err != nil {
		t.Fatalf("building the second configuration: %v", err)
	}
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		if _, err := providers.Insert(ctx, second, sealed); err != nil {
			t.Fatalf("writing A's second provider: %v", err)
		}
		listed, err := providers.List(ctx)
		if err != nil {
			t.Fatalf("A's list: %v", err)
		}
		if len(listed) != 2 {
			t.Fatalf("A sees %d providers, want both", len(listed))
		}
		counted, err := providers.Count(ctx)
		if err != nil || counted != 2 {
			t.Errorf("A counts %d providers (%v)", counted, err)
		}
		return nil
	})

	// The update replaces in place, keeps the sealed secret when none is offered, and the version
	// rises. Two rows stay two rows.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		renamed := second
		renamed.DisplayName = "The other directory"
		stored, found, err := providers.Update(ctx, renamed, nil, now.Add(time.Minute))
		if err != nil || !found {
			t.Fatalf("reconfiguring: (%v, %v)", found, err)
		}
		if stored.Version != 2 || stored.DisplayName != "The other directory" {
			t.Errorf("the reconfiguration answered %+v", stored)
		}
		_, envelope, err := providers.FindWithSecret(ctx, idpRowASec)
		if err != nil {
			t.Fatalf("reading the secret back: %v", err)
		}
		if string(envelope.Ciphertext) != "A's sealed client secret" {
			t.Error("an update without a secret lost the one that was sealed")
		}
		return nil
	})

	// Reconfigure is the form's write (ADR-0076 §5): every field but the switch, which the statement
	// keeps as the row holds it whatever the configuration carries.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		formed := second
		formed.DisplayName = "Named on the form"
		formed.Enabled = false
		stored, found, err := providers.Reconfigure(ctx, formed, nil, now.Add(2*time.Minute))
		if err != nil || !found {
			t.Fatalf("reconfiguring without the switch: (%v, %v)", found, err)
		}
		if !stored.Enabled || stored.DisplayName != "Named on the form" || stored.Version != 3 {
			t.Errorf("the form's write answered %+v, want the switch kept on and the name written", stored)
		}
		return nil
	})
	var rows int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM identity_provider WHERE tenant_id = $1`, idpTenantA.String(),
	).Scan(&rows); err != nil || rows != 2 {
		t.Errorf("A holds %d provider rows (%v), want two", rows, err)
	}
}

// The rule migration 0103 exists for, and the one this file has to name: a row that belongs to no
// workspace is readable by every workspace and writable by none of them.
//
// Both halves are the point. The read has to work, because the sign-in card cannot draw a button
// for a provider it cannot see; the write has to fail, because the row is the installation's and a
// workspace that could switch it off could lock every other workspace out of its own way in.
func TestTheInstallationsProviderIsReadByEveryWorkspaceAndWrittenByNone(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)

	providers := postgres.NewIdentityProviderRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()

	// A `GOOGLE` row rather than a `GENERIC` one, and the reason is ADR-0071 §2 rather than
	// convenience: a provider whose addresses this installation cannot vouch for may never be an
	// installation provider, because that row reaches every workspace. What this test is about is
	// the policy — read by all, written by none — so it uses a kind the policy lets exist.
	offered, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: idpRowShare, Issuer: "https://accounts.google.com", ClientID: "hubtask-platform",
		Kind: string(domain.KindGoogle), Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the installation's configuration: %v", err)
	}
	if !offered.Installation() {
		t.Fatal("a provider with no workspace does not call itself the installation's")
	}

	// Written under the scope that has no tenant. The insert names `current_tenant_id()`, which is
	// NULL there - so the level is the scope's answer and not a field a caller sent.
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		_, err := providers.Insert(ctx, offered,
			cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("the platform's secret")})
		return err
	}); err != nil {
		t.Fatalf("writing the installation's provider: %v", err)
	}

	for _, tenant := range []shared.ID{idpTenantA, idpTenantB} {
		inTenant(t, uow, tenant, func(ctx context.Context) error {
			found, err := providers.Find(ctx, idpRowShare)
			if err != nil {
				t.Fatalf("%s reading the installation's provider: %v", tenant, err)
			}
			if !found.Installation() {
				t.Errorf("%s reads the row as its own", tenant)
			}

			// And cannot touch it. Neither the update nor the delete matches a row, because the
			// write policy compares the tenant and this row has none.
			stolen := offered
			stolen.Enabled = false
			if _, written, err := providers.Update(ctx, stolen, nil, now); err != nil || written {
				t.Errorf("%s switched the installation's provider off: (%v, %v)", tenant, written, err)
			}
			if removed, err := providers.Delete(ctx, idpRowShare, now); err != nil || removed {
				t.Errorf("%s deleted the installation's provider: (%v, %v)", tenant, removed, err)
			}

			// Nor is it counted against the workspace's own bound: it is not theirs to be limited
			// by.
			counted, err := providers.Count(ctx)
			if err != nil {
				t.Fatalf("counting: %v", err)
			}
			for _, row := range mustList(ctx, t, providers) {
				if row.ID == idpRowShare && counted > 0 && tenant == idpTenantB {
					t.Error("the installation's row was counted against a workspace's bound")
				}
			}
			return nil
		})
	}

	// It is still enabled, and the census the re-seal reads finds it under the scope that owns it.
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		listed, err := providers.List(ctx)
		if err != nil {
			return err
		}
		for _, row := range listed {
			if row.ID == idpRowShare && !row.Enabled {
				t.Error("a workspace switched the installation's provider off after all")
			}
		}
		sealedRows, err := providers.ListSealed(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, row := range sealedRows {
			found = found || row.ProviderID == idpRowShare
		}
		if !found {
			t.Error("the re-seal's own read does not see the installation's row")
		}
		return nil
	}); err != nil {
		t.Fatalf("reading the installation's level: %v", err)
	}
}

// mustList is the list or the end of the test.
func mustList(
	ctx context.Context, t *testing.T, providers postgres.IdentityProviderRepository,
) []domain.IdentityProvider {
	t.Helper()
	listed, err := providers.List(ctx)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	return listed
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
		ProviderID: idpRowA,
		Nonce:      "the-nonce", Verifier: "v-verifier-that-is-long-enough-for-rfc-7636-000", Now: now,
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
		// Which provider it left through, so the exchange is signed with the right secret.
		if consumed.ProviderID != idpRowA {
			t.Errorf("the flow came back naming provider %s", consumed.ProviderID)
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
		linked, err := external.LinkSubject(ctx, idpRowA, idpAccount, "subject-in-a", now)
		if err != nil || !linked {
			t.Fatalf("linking A's account: (%v, %v)", linked, err)
		}
		return nil
	})
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		if _, err := external.FindBySubject(ctx, idpRowA, "subject-in-a"); err == nil {
			t.Error("B found A's account by its provider subject")
		}
		// Gate SG-3: ProvidersOf.
		if providers, err := external.ProvidersOf(ctx, idpAccount); err != nil || len(providers) != 0 {
			t.Errorf("B read the providers of A's account: (%v, %v)", providers, err)
		}
		return nil
	})
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		providers, err := external.ProvidersOf(ctx, idpAccount)
		if err != nil || len(providers) != 1 || providers[0] != idpRowA {
			t.Errorf("A's account is connected to %v (%v), want the one provider", providers, err)
		}
		return nil
	})
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		found, err := external.FindBySubject(ctx, idpRowA, "subject-in-a")
		if err != nil {
			t.Fatalf("A finding its own account by subject: %v", err)
		}
		if found.ID != idpAccount {
			t.Errorf("the subject named %s", found.ID)
		}
		// A second link on the same account *at the same provider* is refused: the primary key is
		// (workspace, account, provider), so an account already bound there is never quietly
		// re-pointed.
		relinked, err := external.LinkSubject(ctx, idpRowA, idpAccount, "another-subject", now)
		if err != nil {
			t.Fatalf("re-linking: %v", err)
		}
		if relinked {
			t.Error("an account already bound to a subject was re-pointed at another")
		}
		// At a *second* provider it is a second link, which is the whole point of the plural: one
		// person, two ways in.
		alsoLinked, err := external.LinkSubject(ctx, idpRowASec, idpAccount, "subject-at-the-other", now)
		if err != nil || !alsoLinked {
			t.Fatalf("linking the same account at a second provider: (%v, %v)", alsoLinked, err)
		}
		if _, err := external.FindBySubject(ctx, idpRowASec, "subject-at-the-other"); err != nil {
			t.Errorf("the second provider's link does not answer: %v", err)
		}
		// And the first provider does not answer the second's subject.
		if _, err := external.FindBySubject(ctx, idpRowA, "subject-at-the-other"); err == nil {
			t.Error("one provider answered another provider's subject")
		}
		// The account holds an identity now, which is a credential ADR-0071's addendum protects.
		held, err := external.HasIdentity(ctx, idpAccount)
		if err != nil || !held {
			t.Errorf("A's linked account holds no identity: (%v, %v)", held, err)
		}
		return nil
	})

	// Gate SG-3 for HasIdentity: B cannot learn that A's account signs in through a provider.
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		held, err := external.HasIdentity(ctx, idpAccount)
		if err != nil {
			t.Fatalf("B asking about A's account: %v", err)
		}
		if held {
			t.Error("B learnt that A's account holds a provider identity")
		}
		return nil
	})
	// A provider arrival waiting for the account's own proof (migration 0110): the pending row
	// carries which provider and which subject it will connect, reads them back in A, and is
	// nothing in B.
	pending, _ := mfaStores(ctx, t)
	presented, err := domain.NewPendingToken(idpTenantA, sessionSecretOf(0xE2))
	if err != nil {
		t.Fatalf("minting a pending token: %v", err)
	}
	waiting := domain.PendingCredential{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc31"), TenantID: idpTenantA,
		AccountID: idpAccount, Purpose: domain.PendingLink,
		CreatedAt: now, ExpiresAt: now.Add(domain.PendingLifetime),
		Link: &domain.LinkIntent{ProviderID: idpRowA, Subject: "waiting-subject"},
	}
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		return pending.Insert(ctx, waiting, presented)
	})
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		if _, err := pending.FindByToken(ctx, presented); err == nil {
			t.Error("B found A's pending link")
		}
		return nil
	})
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		lookup, err := pending.FindByToken(ctx, presented)
		if err != nil {
			t.Fatalf("A reading its pending link: %v", err)
		}
		link := lookup.Credential.Link
		if lookup.Credential.Purpose != domain.PendingLink || link == nil ||
			link.ProviderID != idpRowA || link.Subject != "waiting-subject" {
			t.Errorf("the pending link came back as %+v", lookup.Credential)
		}
		return nil
	})
}
