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
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// UC-ID-10 checks 1 and 6: a workspace without the password mails a CONNECT link, and the provider
// flow started from it carries the credential (ADR-0078 §1, migration 0117) - home to its own
// workspace and nowhere else (gate SG-3). Its own workspaces and accounts, named by nobody else in
// this shared database.
var (
	sc33TenantA   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a01")
	sc33TenantB   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a02")
	sc33AccountA  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a11")
	sc33AccountB  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a12")
	sc33Provider  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a21")
	sc33ConnectA  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a31")
	sc33ConnectB  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a32")
	sc33LinkA     = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a33")
	sc33FlowA     = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a41")
	sc33FlowPlain = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a42")
	sc33FlowB     = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a43")
)

// seedConnectTenants writes the two workspaces and one active account in each, and clears what an
// earlier run left of this file's rows so that its fixed tokens are fresh.
func seedConnectTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name)
		VALUES ($1, 'sc33-a', 'SC-33 A'), ($2, 'sc33-b', 'SC-33 B')
		ON CONFLICT (id) DO NOTHING`, sc33TenantA.String(), sc33TenantB.String()); err != nil {
		t.Fatalf("seeding the workspaces: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $2, 'USER', 'member-a@sc33.example', 'Member A', 'ACTIVE'),
		       ($3, $4, 'USER', 'member-b@sc33.example', 'Member B', 'ACTIVE')
		ON CONFLICT (id) DO NOTHING`,
		sc33AccountA.String(), sc33TenantA.String(),
		sc33AccountB.String(), sc33TenantB.String()); err != nil {
		t.Fatalf("seeding the accounts: %v", err)
	}
	for _, statement := range []string{
		`DELETE FROM oidc_flow WHERE tenant_id IN ($1, $2)`,
		`DELETE FROM auth_pending WHERE tenant_id IN ($1, $2)`,
	} {
		if _, err := admin.Exec(ctx, statement, sc33TenantA.String(), sc33TenantB.String()); err != nil {
			t.Fatalf("clearing an earlier run: %v", err)
		}
	}
}

// sc33Token mints a token of one workspace from fixed material.
func sc33Token(t *testing.T, tenantID shared.ID, seed byte, flow bool) domain.Token {
	t.Helper()
	material := make([]byte, domain.TokenSecretBytes)
	for i := range material {
		material[i] = seed + byte(i)
	}
	mint := domain.NewPendingToken
	if flow {
		mint = domain.NewOidcFlowState
	}
	token, err := mint(tenantID, material)
	if err != nil {
		t.Fatalf("minting a token: %v", err)
	}
	return token
}

// sc33Connect is a CONNECT credential of one account.
func sc33Connect(id, tenantID, accountID shared.ID, now time.Time) domain.PendingCredential {
	return domain.PendingCredential{
		ID: id, TenantID: tenantID, AccountID: accountID, Purpose: domain.PendingConnect,
		CreatedAt: now, ExpiresAt: now.Add(domain.ResetLifetime),
	}
}

// sc33Flow is a sign-in flow of one workspace carrying a pending credential, or none.
func sc33Flow(t *testing.T, id, tenantID, pendingID shared.ID, now time.Time) domain.OidcFlow {
	t.Helper()
	flow, err := domain.NewOidcFlow(domain.NewOidcFlowInput{
		ID: id, TenantID: tenantID, ProviderID: sc33Provider, Nonce: "the-nonce",
		Verifier: "v-verifier-that-is-long-enough-for-rfc-7636-000", Now: now,
		PendingID: pendingID,
	})
	if err != nil {
		t.Fatalf("building a flow: %v", err)
	}
	return flow
}

// A CONNECT link is a pending credential of its own purpose: found by the identifier a flow keeps, in
// its own workspace only (gate SG-3), and spent once.
func TestAConnectLinkIsFoundAtHomeAndSpentOnce(t *testing.T) {
	ctx := context.Background()
	seedConnectTenants(ctx, t)
	pending, uow := mfaStores(ctx, t)
	now := time.Now().UTC()

	connectA := sc33Token(t, sc33TenantA, 1, false)
	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		return pending.Insert(ctx, sc33Connect(sc33ConnectA, sc33TenantA, sc33AccountA, now), connectA)
	})

	// Gate SG-3: B asks for A's credential by its identifier and learns nothing.
	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: sc33TenantB}, func(ctx context.Context) error {
		_, err := pending.FindByID(ctx, sc33ConnectA)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("B reading A's CONNECT link by its identifier: %v, want not found", err)
	}

	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		found, err := pending.FindByID(ctx, sc33ConnectA)
		if err != nil {
			t.Fatalf("A reading its CONNECT link: %v", err)
		}
		if found.Credential.Purpose != domain.PendingConnect || found.Account.ID != sc33AccountA ||
			found.Credential.TenantID != sc33TenantA || found.Credential.Verify(now) != nil {
			t.Errorf("the CONNECT link came back as %+v", found.Credential)
		}
		return nil
	})

	// Spent once: the second spend finds nothing to spend, and the link is refused afterwards.
	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		first, err := pending.Consume(ctx, sc33ConnectA, now)
		if err != nil || !first {
			t.Fatalf("spending the link: (%v, %v)", first, err)
		}
		second, err := pending.Consume(ctx, sc33ConnectA, now)
		if err != nil || second {
			t.Errorf("the link was spent twice: (%v, %v)", second, err)
		}
		found, err := pending.FindByID(ctx, sc33ConnectA)
		if err != nil {
			return err
		}
		if found.Credential.Verify(now) == nil {
			t.Error("a spent CONNECT link still verifies")
		}
		return nil
	})

	// A newer link spends the earlier one (Supersede), and only in its own workspace.
	connectB := sc33Token(t, sc33TenantB, 61, false)
	inTenant(t, uow, sc33TenantB, func(ctx context.Context) error {
		return pending.Insert(ctx, sc33Connect(sc33ConnectB, sc33TenantB, sc33AccountB, now), connectB)
	})
	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		spent, err := pending.Supersede(ctx, sc33AccountB, domain.PendingConnect, now)
		if err != nil || spent != 0 {
			t.Errorf("A superseded B's link: (%d, %v)", spent, err)
		}
		return nil
	})
	inTenant(t, uow, sc33TenantB, func(ctx context.Context) error {
		spent, err := pending.Supersede(ctx, sc33AccountB, domain.PendingConnect, now)
		if err != nil || spent != 1 {
			t.Errorf("superseding B's own link: (%d, %v), want one", spent, err)
		}
		return nil
	})
}

// The proof a pending link carries travels with it into the second factor's step (migration 0117):
// written and read back as it was given, and a link without one reads as the password.
func TestAPendingLinkKeepsItsProof(t *testing.T) {
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

	presented := sc33Token(t, sc33TenantA, 121, false)
	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		return pending.Insert(ctx, domain.PendingCredential{
			ID: sc33LinkA, TenantID: sc33TenantA, AccountID: sc33AccountA, Purpose: domain.PendingTotp,
			CreatedAt: now, ExpiresAt: now.Add(domain.PendingLifetime),
			Link: &domain.LinkIntent{ProviderID: sc33Provider, Subject: "sub-a", Proof: domain.LinkProofMailbox},
		}, presented)
	})
	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		found, err := pending.FindByToken(ctx, presented)
		if err != nil {
			t.Fatalf("reading the link back: %v", err)
		}
		if found.Credential.Link == nil || found.Credential.Link.Proof != domain.LinkProofMailbox {
			t.Errorf("the link came back as %+v, want the mailbox proof", found.Credential.Link)
		}
		return nil
	})
}

// A sign-in flow started from a CONNECT link carries it back to the callback of its own workspace,
// and a workspace cannot bind its flow to another workspace's link (gate SG-3).
func TestAFlowCarriesItsConnectLinkHomeAndNowhereElse(t *testing.T) {
	ctx := context.Background()
	seedConnectTenants(ctx, t)
	pending, uow := mfaStores(ctx, t)
	flows := postgres.NewOidcFlowRepository(security.NewOidcFlowHasher(secret.New(installationSecret)))
	now := time.Now().UTC()

	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		return pending.Insert(ctx, sc33Connect(sc33ConnectA, sc33TenantA, sc33AccountA, now),
			sc33Token(t, sc33TenantA, 1, false))
	})

	connectState := sc33Token(t, sc33TenantA, 31, true)
	plainState := sc33Token(t, sc33TenantA, 91, true)
	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		if err := flows.Insert(ctx, sc33Flow(t, sc33FlowA, sc33TenantA, sc33ConnectA, now), connectState); err != nil {
			return err
		}
		return flows.Insert(ctx, sc33Flow(t, sc33FlowPlain, sc33TenantA, "", now), plainState)
	})

	// Gate SG-3: B names A's link on a flow of its own, and the key refuses it.
	if err := uow.Within(ctx, persistence.Scope{TenantID: sc33TenantB}, func(ctx context.Context) error {
		return flows.Insert(ctx, sc33Flow(t, sc33FlowB, sc33TenantB, sc33ConnectA, now),
			sc33Token(t, sc33TenantB, 151, true))
	}); err == nil {
		t.Error("B wrote a flow carrying A's CONNECT link")
	}

	// Gate SG-3: B cannot spend A's flow, and learns nothing of the link it carries.
	inTenant(t, uow, sc33TenantB, func(ctx context.Context) error {
		consumed, found, err := flows.Consume(ctx, connectState, now)
		if err != nil {
			t.Fatalf("B consuming A's state: %v", err)
		}
		if found || !consumed.PendingID.IsZero() {
			t.Error("B spent A's flow, or read the link it carries")
		}
		return nil
	})

	inTenant(t, uow, sc33TenantA, func(ctx context.Context) error {
		consumed, found, err := flows.Consume(ctx, connectState, now)
		if err != nil || !found {
			t.Fatalf("A consuming its connect flow: (%v, %v)", found, err)
		}
		if consumed.PendingID != sc33ConnectA || !consumed.InvitedAccountID.IsZero() {
			t.Errorf("the flow came back carrying %q, want the CONNECT link", consumed.PendingID)
		}
		// When it left for the provider, which a connection's sign-in has to follow (ADR-0078 §1).
		if consumed.CreatedAt.IsZero() || consumed.CreatedAt.Sub(now).Abs() > time.Second {
			t.Errorf("the flow came back started at %v, want %v", consumed.CreatedAt, now)
		}
		plain, found, err := flows.Consume(ctx, plainState, now)
		if err != nil || !found {
			t.Fatalf("A consuming its plain flow: (%v, %v)", found, err)
		}
		if !plain.PendingID.IsZero() {
			t.Errorf("a flow without a link came back carrying %q", plain.PendingID)
		}
		return nil
	})
}

// The password switch's count (ADR-0078 §1): the workspace's active people that no provider in the
// list signs in - connected, invited and service accounts aside - and none of the workspace next
// door (gate SG-3).
func TestTheCountIsTheWorkspacesUnconnectedPeopleOnly(t *testing.T) {
	ctx := context.Background()
	seedConnectTenants(ctx, t)
	admin := adminPool(ctx, t)
	var (
		connected = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a51")
		invited   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a52")
		service   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a53")
		disabled  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a54")
		other     = shared.MustParseID("01936f2a-7c1e-7000-8000-000000033a22")
	)
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $5, 'USER', 'connected@sc33.example', 'Connected', 'ACTIVE'),
		       ($2, $5, 'USER', 'invited@sc33.example', 'Invited', 'INVITED'),
		       ($3, $5, 'SERVICE_ACCOUNT', NULL, 'Robot', 'ACTIVE'),
		       ($4, $5, 'USER', 'disabled@sc33.example', 'Disabled', 'DISABLED')
		ON CONFLICT (id) DO NOTHING`,
		connected.String(), invited.String(), service.String(), disabled.String(),
		sc33TenantA.String()); err != nil {
		t.Fatalf("seeding the accounts: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO identity_provider (id, tenant_id, issuer, client_id, client_secret_enc,
		                               client_secret_key_id, created_at)
		VALUES ($1, $3, 'https://sc33.example/idp', 'hubtask', '\x00', 'k-sc33', now()),
		       ($2, $3, 'https://sc33.example/other', 'hubtask', '\x00', 'k-sc33', now())
		ON CONFLICT (id) DO NOTHING`, sc33Provider.String(), other.String(), sc33TenantA.String()); err != nil {
		t.Fatalf("seeding the providers: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account_identity (tenant_id, account_id, provider_id, subject, linked_at)
		VALUES ($1, $2, $3, 'sc33-connected', now()), ($1, $4, $5, 'sc33-elsewhere', now())
		ON CONFLICT DO NOTHING`, sc33TenantA.String(), connected.String(), sc33Provider.String(),
		sc33AccountA.String(), other.String()); err != nil {
		t.Fatalf("seeding the identities: %v", err)
	}

	external := postgres.NewExternalAccountRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	count := func(tenantID shared.ID, providers ...shared.ID) int {
		t.Helper()
		var counted int
		if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
			n, err := external.CountWithoutIdentityAt(ctx, providers)
			counted = n
			return err
		}); err != nil {
			t.Fatalf("counting: %v", err)
		}
		return counted
	}

	// Switched on: the one provider. Member A's identity is at the other one, which is not a way in
	// here, so A counts; the connected account does not; invited, service and disabled never do.
	if got := count(sc33TenantA, sc33Provider); got != 1 {
		t.Errorf("A's count with its provider = %d, want 1", got)
	}
	// Both switched on: nobody active is without one.
	if got := count(sc33TenantA, sc33Provider, other); got != 0 {
		t.Errorf("A's count with both providers = %d, want 0", got)
	}
	// None switched on: every active person.
	if got := count(sc33TenantA); got != 2 {
		t.Errorf("A's count with no provider = %d, want 2", got)
	}
	// Gate SG-3: B counts its own one member and none of A's, whichever providers it names.
	if got := count(sc33TenantB, sc33Provider, other); got != 1 {
		t.Errorf("B's count = %d, want its own one member", got)
	}
}
