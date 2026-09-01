// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The OAuth2 provider of H-05, against the real boundary. Gate SG-3: one negative per port
// method.

func oauthStores(ctx context.Context, t *testing.T) (
	postgres.OauthClientRepository, postgres.OauthGrantRepository,
	postgres.OauthCodeRepository, persistence.UnitOfWork,
) {
	t.Helper()
	installation := secret.New(installationSecret)
	return postgres.NewOauthClientRepository(security.NewOauthClientSecretHasher(installation)),
		postgres.NewOauthGrantRepository(),
		postgres.NewOauthCodeRepository(security.NewOauthCodeHasher(installation)),
		postgres.NewUnitOfWork(appPool(ctx, t))
}

func seededClient(ctx context.Context, t *testing.T, uow persistence.UnitOfWork,
	clients postgres.OauthClientRepository, tenantID shared.ID, id shared.ID,
) identity.Token {
	t.Helper()
	presented, err := identity.NewOauthClientSecret(tenantID, sessionSecretOf(byte(id[len(id)-1])))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	inTenant(t, uow, tenantID, func(ctx context.Context) error {
		return clients.Insert(ctx, identity.OauthClient{
			ID: id, TenantID: tenantID, Name: "App " + id.String()[:8], Confidential: true,
			RedirectURIs: []string{"https://app.example/cb"}, CreatedAt: time.Now().UTC(),
		}, presented)
	})
	return presented
}

// Gate SG-3: OauthClients - Insert, List, Find, SecretMatches, Delete.
func TestAClientIsInvisibleAndUnusableFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	clients, _, _, uow := oauthStores(ctx, t)

	clientA := shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a01")
	secretA := seededClient(ctx, t, uow, clients, tenantA, clientA)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		found, err := clients.Find(ctx, clientA)
		if err != nil || found.Name == "" {
			t.Fatalf("own client (%+v, %v)", found, err)
		}
		matches, err := clients.SecretMatches(ctx, clientA, secret.New(secretA.Secret()))
		if err != nil || !matches {
			t.Fatalf("own secret (%v, %v)", matches, err)
		}
		matches, err = clients.SecretMatches(ctx, clientA, secret.New("hbt_ocs_wrong"))
		if err != nil || matches {
			t.Fatalf("a wrong secret matched (%v, %v)", matches, err)
		}
		return nil
	})

	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		if _, err := clients.Find(ctx, clientA); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another tenant's client answered %v, want not found", err)
		}
		if _, err := clients.SecretMatches(ctx, clientA, secret.New(secretA.Secret())); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another tenant's secret was comparable: %v", err)
		}
		listed, err := clients.List(ctx)
		if err != nil {
			t.Fatalf("listing: %v", err)
		}
		for _, client := range listed {
			if client.ID == clientA {
				t.Error("another tenant's client listed here")
			}
		}
		removed, err := clients.Delete(ctx, clientA)
		if err != nil || removed {
			t.Errorf("another tenant's client removed from here (%v, %v)", removed, err)
		}
		return nil
	})
}

// Gate SG-3: OauthGrants and OauthCodes, and the once-only properties end to end.
func TestGrantsAndCodesAreBoundToTheirTenantAndBurnOnce(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	clients, grants, codes, uow := oauthStores(ctx, t)

	clientA := shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a02")
	seededClient(ctx, t, uow, clients, tenantA, clientA)

	grantID := shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a03")
	now := time.Now().UTC()
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		live, err := grants.Upsert(ctx, identity.OauthGrant{
			ID: grantID, TenantID: tenantA, AccountID: sessionAccountA, ClientID: clientA,
			Scopes: []string{"items:read"}, CreatedAt: now,
		})
		if err != nil || live != grantID {
			t.Fatalf("consenting (%v, %v)", live, err)
		}
		// The fresh consent replaces the scopes on the same live grant.
		live, err = grants.Upsert(ctx, identity.OauthGrant{
			ID: shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a04"),
			TenantID: tenantA, AccountID: sessionAccountA, ClientID: clientA,
			Scopes: []string{"items:read", "items:write"}, CreatedAt: now,
		})
		if err != nil || live != grantID {
			t.Fatalf("the second consent made a second grant (%v, %v)", live, err)
		}
		return nil
	})

	// Gate SG-3: the grant is invisible and immovable from another tenant.
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		if _, err := grants.Find(ctx, grantID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another tenant's grant answered %v", err)
		}
		listings, err := grants.ListForAccount(ctx, sessionAccountA)
		if err != nil || len(listings) != 0 {
			t.Errorf("another tenant's grants listed here: %v, %v", listings, err)
		}
		changed, err := grants.Revoke(ctx, grantID, sessionAccountA, now)
		if err != nil || changed {
			t.Errorf("another tenant's grant revoked from here (%v, %v)", changed, err)
		}
		if ended, err := grants.RevokeSessions(ctx, grantID, now); err != nil || ended != 0 {
			t.Errorf("another tenant's sessions ended from here (%d, %v)", ended, err)
		}
		return nil
	})

	// A code: minted in A, invisible under a rewritten tenant, burned exactly once.
	presented, err := identity.NewOauthCode(tenantA, sessionSecretOf(0xF1))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		return codes.Insert(ctx, identity.OauthCode{
			ID: shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a05"),
			TenantID: tenantA, ClientID: clientA, AccountID: sessionAccountA, GrantID: grantID,
			Challenge: "c", RedirectURI: "https://app.example/cb",
			CreatedAt: now, ExpiresAt: now.Add(2 * time.Minute),
		}, presented)
	})

	rewritten, err := identity.NewOauthCode(tenantB, sessionSecretOf(0xF1))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		if _, consumed, err := codes.Consume(ctx, rewritten, now); err != nil || consumed {
			t.Errorf("a rewritten code consumed (%v, %v)", consumed, err)
		}
		return nil
	})
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		code, consumed, err := codes.Consume(ctx, presented, now)
		if err != nil || !consumed {
			t.Fatalf("the first exchange (%v, %v)", consumed, err)
		}
		if code.GrantID != grantID || code.ClientID != clientA {
			t.Errorf("code %+v", code)
		}
		if _, consumed, err := codes.Consume(ctx, presented, now); err != nil || consumed {
			t.Fatalf("a code burned twice (%v, %v)", consumed, err)
		}
		return nil
	})

	// Revoking the grant ends the sessions it leashed, and the leashed session's scopes travel
	// through authentication.
	sessions := postgres.NewSessionRepository()
	leashed := shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a06")
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		session := identity.Session{
			ID: leashed, TenantID: tenantA, AccountID: sessionAccountA,
			CreatedAt: now, ExpiresAt: now.Add(time.Hour),
			GrantID: grantID, Scopes: []string{"items:read"},
		}
		if err := sessions.Insert(ctx, session); err != nil {
			t.Fatalf("inserting the leashed session: %v", err)
		}
		credential, err := sessions.FindForAuth(ctx, leashed)
		if err != nil {
			t.Fatalf("reading it back: %v", err)
		}
		if credential.ClientID != clientA || len(credential.Session.Scopes) != 1 {
			t.Fatalf("the leash did not travel: %+v", credential)
		}

		changed, err := grants.Revoke(ctx, grantID, sessionAccountA, now)
		if err != nil || !changed {
			t.Fatalf("revoking (%v, %v)", changed, err)
		}
		ended, err := grants.RevokeSessions(ctx, grantID, now)
		if err != nil || ended != 1 {
			t.Fatalf("ending the leashed sessions (%d, %v)", ended, err)
		}
		credential, err = sessions.FindForAuth(ctx, leashed)
		if err != nil {
			t.Fatalf("reading back: %v", err)
		}
		if verifyErr := credential.Session.Verify(time.Now()); verifyErr == nil {
			t.Error("a session under a revoked grant still verifies")
		}
		return nil
	})
}
