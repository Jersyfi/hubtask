// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// installationSecret stands in for HUBTASK_SECRET_KEY. Long enough to pass the configuration's
// own minimum, and a constant here because the test is about the boundary, not about the key.
const installationSecret = "test-only-installation-secret-not-a-real-one"

var (
	accountA = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000aa")
	accountB = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000bb")
	tokenIDA = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000a1")
	tokenIDB = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000b1")
)

func tokenFixtures(ctx context.Context, t *testing.T) (identity.Token, identity.Token) {
	t.Helper()
	admin := adminPool(ctx, t)
	hasher := security.NewTokenHasher(secret.New(installationSecret))

	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone)
		VALUES ($1, 'tenant-a', 'A', 'de', 'Europe/Berlin'), ($2, 'tenant-b', 'B', 'en', 'UTC')
		ON CONFLICT (id) DO NOTHING`, tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding tenants: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, display_name, locale, status)
		VALUES ($1, $3, 'USER', 'A', 'fr', 'ACTIVE'), ($2, $4, 'SERVICE_ACCOUNT', 'B', NULL, 'ACTIVE')
		ON CONFLICT (id) DO NOTHING`,
		accountA.String(), accountB.String(), tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding accounts: %v", err)
	}

	tokenA := mintFor(t, tenantA, 1)
	tokenB := mintFor(t, tenantB, 2)

	if _, err := admin.Exec(ctx, `
		INSERT INTO access_token
			(id, tenant_id, account_id, name, token_hash, token_prefix, scopes, expires_at)
		VALUES
			($1, $3, $5, 'a', $7, 'hbt_pat_', ARRAY['items:read'],  now() + interval '30 days'),
			($2, $4, $6, 'b', $8, 'hbt_pat_', ARRAY['items:write'], now() + interval '30 days')
		ON CONFLICT (id) DO NOTHING`,
		tokenIDA.String(), tokenIDB.String(),
		tenantA.String(), tenantB.String(),
		accountA.String(), accountB.String(),
		hasher.Hash(tokenA.Secret()), hasher.Hash(tokenB.Secret())); err != nil {
		t.Fatalf("seeding tokens: %v", err)
	}
	return tokenA, tokenB
}

func mintFor(t *testing.T, tenant shared.ID, seed byte) identity.Token {
	t.Helper()
	material := make([]byte, identity.TokenSecretBytes)
	for i := range material {
		material[i] = seed
	}
	token, err := identity.NewToken(tenant, material)
	if err != nil {
		t.Fatalf("minting a token: %v", err)
	}
	return token
}

func tokenRepository(ctx context.Context, t *testing.T) (repository.AccessTokens, persistence.UnitOfWork) {
	t.Helper()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	return postgres.NewAccessTokenRepository(security.NewTokenHasher(secret.New(installationSecret))), uow
}

func TestATokenIsFoundInItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	tokenA, _ := tokenFixtures(ctx, t)
	repo, uow := tokenRepository(ctx, t)

	var credential repository.Credential
	err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		var err error
		credential, err = repo.FindByToken(ctx, tokenA)
		return err
	})
	if err != nil {
		t.Fatalf("the token was not found in its own tenant: %v", err)
	}

	if credential.Token.ID != tokenIDA {
		t.Errorf("token id = %q, want %q", credential.Token.ID, tokenIDA)
	}
	if credential.Token.TenantID != tenantA || credential.Account.ID != accountA {
		t.Errorf("credential = %+v", credential)
	}
	if len(credential.Token.Scopes) != 1 || credential.Token.Scopes[0] != "items:read" {
		t.Errorf("scopes = %v", credential.Token.Scopes)
	}
	if credential.Token.ExpiresAt.IsZero() || !credential.Token.RevokedAt.IsZero() {
		t.Errorf("expires %v, revoked %v", credential.Token.ExpiresAt, credential.Token.RevokedAt)
	}
	if credential.Account.Kind != identity.AccountUser || credential.Account.Status != identity.AccountActive {
		t.Errorf("account = %+v", credential.Account)
	}
	if credential.Account.Locale != "fr" {
		t.Errorf("account locale = %q", credential.Account.Locale)
	}
	if credential.TenantLocale != "de" || credential.TenantTimeZone != "Europe/Berlin" {
		t.Errorf("tenant defaults = %q / %q", credential.TenantLocale, credential.TenantTimeZone)
	}
}

// The cross-tenant negative test the security gate requires for every repository method
// (security.md §6, CLAUDE.md "the loop", step 5). Row level security is what makes it pass: the
// query carries no tenant condition of its own.
func TestATokenIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	tokenA, tokenB := tokenFixtures(ctx, t)
	repo, uow := tokenRepository(ctx, t)

	cases := map[string]struct {
		scope shared.ID
		token identity.Token
	}{
		"tenant B looking for A's token": {tenantB, tokenA},
		"tenant A looking for B's token": {tenantA, tokenB},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: c.scope}, func(ctx context.Context) error {
				_, err := repo.FindByToken(ctx, c.token)
				return err
			})
			if !errors.Is(err, shared.ErrNotFound) {
				t.Fatalf("error = %v, want not found - the tenant boundary did not hold", err)
			}
		})
	}
}

// The same method again, this time proving the boundary holds against the token itself rather
// than against the transaction: a credential rewritten to name another tenant must find nothing.
func TestARewrittenTenantFindsNothing(t *testing.T) {
	ctx := context.Background()
	tokenA, _ := tokenFixtures(ctx, t)
	repo, uow := tokenRepository(ctx, t)

	rewritten, err := identity.ParseToken(
		"hbt_pat_" + stripHyphens(tenantB.String()) + "_" + secretHalf(tokenA))
	if err != nil {
		t.Fatalf("the rewritten credential does not parse: %v", err)
	}

	err = uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantB}, func(ctx context.Context) error {
		_, err := repo.FindByToken(ctx, rewritten)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error = %v, want not found - the hash does not cover the tenant half", err)
	}
}

func TestTouchLastUsedStaysInsideTheTenant(t *testing.T) {
	ctx := context.Background()
	tokenA, _ := tokenFixtures(ctx, t)
	repo, uow := tokenRepository(ctx, t)
	used := time.Now().UTC().Truncate(time.Second)

	// From the wrong tenant the update reaches no row - and reports no error, because "nothing
	// updated" is what a policy looks like from the outside. What matters is that the value
	// does not change.
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantB}, func(ctx context.Context) error {
		return repo.TouchLastUsed(ctx, tokenIDA, used)
	}); err != nil {
		t.Fatalf("the cross-tenant touch failed for the wrong reason: %v", err)
	}
	if lastUsed := readLastUsed(ctx, t); !lastUsed.IsZero() {
		t.Fatalf("another tenant wrote last_used_at: %v", lastUsed)
	}

	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		return repo.TouchLastUsed(ctx, tokenIDA, used)
	}); err != nil {
		t.Fatalf("the touch failed in its own tenant: %v", err)
	}
	if lastUsed := readLastUsed(ctx, t); lastUsed.IsZero() {
		t.Error("last_used_at was not written in the token's own tenant")
	}

	// And the credential reads back what the repository wrote.
	var credential repository.Credential
	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		var err error
		credential, err = repo.FindByToken(ctx, tokenA)
		return err
	}); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if credential.Token.LastUsedAt.IsZero() {
		t.Error("the last use did not survive the round trip")
	}
}

// A repository called outside a unit of work must fail rather than reach for the pool - a query
// on the pool would run with no tenant context at all (CLAUDE.md rule 3).
func TestTheRepositoryRefusesToRunOutsideATransaction(t *testing.T) {
	ctx := context.Background()
	tokenA, _ := tokenFixtures(ctx, t)
	repo, _ := tokenRepository(ctx, t)

	if _, err := repo.FindByToken(ctx, tokenA); err == nil {
		t.Fatal("the repository answered without a transaction")
	}
}

func readLastUsed(ctx context.Context, t *testing.T) time.Time {
	t.Helper()
	var lastUsed *time.Time
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT last_used_at FROM access_token WHERE id = $1`, tokenIDA.String()).Scan(&lastUsed); err != nil {
		t.Fatalf("reading last_used_at: %v", err)
	}
	if lastUsed == nil {
		return time.Time{}
	}
	return *lastUsed
}

func stripHyphens(s string) string {
	out := make([]byte, 0, len(s))
	for i := range len(s) {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

// secretHalf is everything after the tenant part of a presented token.
func secretHalf(token identity.Token) string {
	raw := token.Secret()
	return raw[len(raw)-43:]
}
