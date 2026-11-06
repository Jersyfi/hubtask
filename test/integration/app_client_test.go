// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The person of this test alone, so no other test's listing of appPerson meets its rows.
var (
	appPerson        = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000017a0")
	appPersonSession = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000017a1")
)

// The installed apps' sign-in over the real stores (identity.md §15.3, ADR-0096; UC-SYN-07 check 3):
// the consent of the built-in client writes a row of its own, the code is exchanged with PKCE for a
// leashed pair, the grant lists with its platform, and the app's sign-out withdraws it - from its own
// workspace only (gate SG-3 for Sessions.RevokeAppGrant).
func TestTheAppClientsCodeExchangesForALeashedPair(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	session, _, uow := realWriter(ctx, t)
	clients, grants, codes, _ := oauthStores(ctx, t)
	writer := identityservice.OauthWriter{
		Session: session, Clients: clients, Grants: grants, Codes: codes,
		KnownScopes:   []string{"items:read", "items:write"},
		SessionScopes: []string{"items:read", "items:write", "admin:tenants"},
	}

	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $2, 'USER', 'app-person@example.org', 'L', 'ACTIVE')
		ON CONFLICT (id) DO NOTHING`, appPerson.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the person: %v", err)
	}

	// The person's own session in the browser, which the consent lands on.
	now := time.Now().UTC()
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if _, err := session.Sessions.FindForAuth(ctx, appPersonSession); err == nil {
			return nil
		}
		return session.Sessions.Insert(ctx, identity.Session{
			ID: appPersonSession, TenantID: tenantA, AccountID: appPerson,
			CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		})
	})
	person := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantA, AccountID: appPerson,
		TokenID: appPersonSession, Scopes: []string{"accounts:read"},
	}

	verifier := strings.Repeat("k", 64)
	digest := sha256.Sum256([]byte(verifier))
	redirect := "http://127.0.0.1:51734/oauth"
	minted, err := identityservice.AuthorizeOauthClient{Writer: writer}.Execute(ctx, person,
		identityservice.AuthorizeOauthClientCommand{
			ClientID: identity.AppClientID, RedirectURI: redirect,
			Challenge: base64.RawURLEncoding.EncodeToString(digest[:]), Method: "S256",
			Device: "LENA-LAPTOP", Platform: "windows",
		})
	if err != nil {
		t.Fatalf("consenting: %v", err)
	}

	pair, err := identityservice.ExchangeOauthCode{Writer: writer}.Execute(ctx, identityservice.ExchangeOauthCodeCommand{
		Code: minted.Code, RedirectURI: redirect, ClientID: identity.AppClientID, Verifier: verifier,
	})
	if err != nil || pair.RefreshToken.IsEmpty() {
		t.Fatalf("exchanging: %v", err)
	}
	if _, err := (identityservice.ExchangeOauthCode{Writer: writer}).Execute(ctx, identityservice.ExchangeOauthCodeCommand{
		Code: minted.Code, RedirectURI: redirect, ClientID: identity.AppClientID, Verifier: verifier,
	}); err == nil {
		t.Fatal("the code exchanged twice")
	}

	var appSession shared.ID
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		listings, err := grants.ListForAccount(ctx, appPerson)
		if err != nil {
			return err
		}
		found := false
		for _, listing := range listings {
			if listing.AppPlatform == identity.AppWindows && listing.ClientName == "LENA-LAPTOP" {
				found = true
				if len(listing.Grant.Scopes) != 2 {
					t.Errorf("the grant carries %v, want a session's consentable scopes", listing.Grant.Scopes)
				}
			}
		}
		if !found {
			t.Fatalf("the app's grant is not listed: %+v", listings)
		}
		registry, err := clients.List(ctx)
		if err != nil {
			return err
		}
		for _, client := range registry {
			if client.IsApp() || client.Name == "LENA-LAPTOP" {
				t.Errorf("the registry lists a consent's row: %+v", client)
			}
		}
		return nil
	})
	var raw string
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT s.id::text FROM session s JOIN oauth_grant g ON g.id = s.grant_id
		JOIN oauth_client c ON c.id = g.client_id
		WHERE s.account_id = $1 AND c.app_platform = 'windows' AND s.revoked_at IS NULL
		ORDER BY s.created_at DESC LIMIT 1`, appPerson.String()).Scan(&raw); err == nil {
		appSession = shared.ID(raw)
	}
	if appSession.IsZero() {
		t.Fatal("no session under the app's grant")
	}

	// Another workspace cannot withdraw it.
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		grantID, _, err := session.Sessions.RevokeAppGrant(ctx, appSession, appPerson, now)
		if err != nil || !grantID.IsZero() {
			t.Errorf("another workspace withdrew the app's grant (%v, %v)", grantID, err)
		}
		return nil
	})

	// The app signs out: its session ends, and the grant with it.
	app := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantA, AccountID: appPerson,
		TokenID: appSession, Scopes: []string{"accounts:read", "accounts:write"},
	}
	if err := (identityservice.RevokeSession{Writer: session}).Execute(ctx, app, appSession); err != nil {
		t.Fatalf("signing the app out: %v", err)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		listings, err := grants.ListForAccount(ctx, appPerson)
		if err != nil {
			return err
		}
		for _, listing := range listings {
			if listing.AppPlatform != "" {
				t.Errorf("the app's grant outlived its sign-out: %+v", listing)
			}
		}
		return nil
	})
}
