// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
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

// The identifiers the write side of G-01 uses. Distinct from the fixtures above, so that a test
// which mints can run beside one that reads without either seeing the other's rows.
var (
	mintedIDA = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000c1")
	mintedIDB = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000c2")
)

func minted(id, tenant, account shared.ID, name string) identity.AccessToken {
	return identity.AccessToken{
		ID: id, TenantID: tenant, AccountID: account, Name: name,
		Scopes:    []string{"items:read"},
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second),
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
}

// The cross-tenant negative for Insert, which gate SG-3 requires of every new repository method.
//
// The insert takes its tenant from current_tenant_id() rather than from the row it is handed, so
// the boundary shows up as the composite foreign key refusing: an account of tenant A does not
// exist in tenant B, whatever the caller claims.
func TestATokenCannotBeMintedForAnotherTenantsAccount(t *testing.T) {
	ctx := context.Background()
	tokenFixtures(ctx, t)
	repo, uow := tokenRepository(ctx, t)
	presented := mintFor(t, tenantB, 9)

	err := uow.Within(ctx, persistence.Scope{TenantID: tenantB}, func(ctx context.Context) error {
		return repo.Insert(ctx, minted(mintedIDA, tenantA, accountA, "a smuggled token"), presented)
	})
	if err == nil {
		t.Fatal("a token was minted for an account of another tenant")
	}
	if rows := countTokens(ctx, t, mintedIDA); rows != 0 {
		t.Fatalf("the refused mint left %d rows behind", rows)
	}
}

// Find, ListForAccount and Revoke, each from the wrong tenant. Row level security is what makes
// them pass: none of the three queries carries a tenant condition of its own.
func TestTheWriteSideIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	tokenFixtures(ctx, t)
	repo, uow := tokenRepository(ctx, t)

	// One real row in tenant A to be invisible.
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		return repo.Insert(ctx, minted(mintedIDB, tenantA, accountA, "a's own"), mintFor(t, tenantA, 8))
	}); err != nil {
		t.Fatalf("seeding the row to hide: %v", err)
	}

	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantB}, func(ctx context.Context) error {
		_, err := repo.Find(ctx, mintedIDB)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("Find: error = %v, want not found", err)
	}

	var listed []identity.AccessToken
	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantB}, func(ctx context.Context) error {
		var err error
		listed, err = repo.ListForAccount(ctx, accountA)
		return err
	}); err != nil {
		t.Fatalf("listing from the wrong tenant failed for the wrong reason: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("ListForAccount answered %d of another tenant's tokens", len(listed))
	}

	// A revocation from the wrong tenant reaches no row - and reports no error, because "nothing
	// updated" is what a policy looks like from outside. What matters is that it changed nothing.
	var changed bool
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantB}, func(ctx context.Context) error {
		var err error
		changed, err = repo.Revoke(ctx, mintedIDB, time.Now().UTC())
		return err
	}); err != nil {
		t.Fatalf("the cross-tenant revocation failed for the wrong reason: %v", err)
	}
	if changed {
		t.Fatal("another tenant revoked a credential")
	}
	if revoked := readRevokedAt(ctx, t, mintedIDB); !revoked.IsZero() {
		t.Fatalf("another tenant stamped revoked_at: %v", revoked)
	}
}

// The round trip the three management use cases depend on: what is written comes back, the
// revocation writes once, and the plaintext is nowhere in the row.
func TestAMintedTokenComesBackAndRevokesOnce(t *testing.T) {
	ctx := context.Background()
	tokenFixtures(ctx, t)
	repo, uow := tokenRepository(ctx, t)

	presented := mintFor(t, tenantA, 7)
	row := minted(mintedIDA, tenantA, accountA, "the nightly export")

	err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		return repo.Insert(ctx, row, presented)
	})
	if err != nil {
		t.Fatalf("minting failed: %v", err)
	}

	var found identity.AccessToken
	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		var err error
		found, err = repo.Find(ctx, mintedIDA)
		return err
	}); err != nil {
		t.Fatalf("reading it back failed: %v", err)
	}
	if found.Name != row.Name || len(found.Scopes) != 1 || found.Scopes[0] != "items:read" {
		t.Errorf("read back %+v", found)
	}
	if !found.ExpiresAt.Equal(row.ExpiresAt) {
		t.Errorf("expiry = %v, want %v", found.ExpiresAt, row.ExpiresAt)
	}

	// The credential presented is the credential that authenticates, through the hash and nothing
	// else - which is also the proof that Insert and FindByToken agree about what is stored.
	var credential repository.Credential
	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		var err error
		credential, err = repo.FindByToken(ctx, presented)
		return err
	}); err != nil {
		t.Fatalf("the minted credential does not authenticate: %v", err)
	}
	if credential.Token.ID != mintedIDA {
		t.Errorf("the credential found %s", credential.Token.ID)
	}

	// The plaintext is in no column. The row is what a stolen dump would contain, so this is the
	// test that says the dump is worth nothing (rule 10, security.md §8).
	if stored := dumpToken(ctx, t, mintedIDA); strings.Contains(stored, presented.Secret()) {
		t.Fatalf("the credential is in the row: %s", stored)
	}

	at := time.Now().UTC().Truncate(time.Second)
	var first, second bool
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		var err error
		if first, err = repo.Revoke(ctx, mintedIDA, at); err != nil {
			return err
		}
		second, err = repo.Revoke(ctx, mintedIDA, at.Add(time.Hour))
		return err
	}); err != nil {
		t.Fatalf("revoking failed: %v", err)
	}
	if !first || second {
		t.Errorf("revoked twice reported %v then %v, want true then false", first, second)
	}
	// The moment somebody pulled it is the one an auditor asks about, so the second call must not
	// overwrite it.
	if stamped := readRevokedAt(ctx, t, mintedIDA); !stamped.Equal(at) {
		t.Errorf("revoked_at = %v, want the first withdrawal at %v", stamped, at)
	}
}

// The account listing is bounded by the tenant like everything else, and by the kind: a person is
// found by name, and a machine is found in the list of machines.
func TestServiceAccountsAreListedPerTenant(t *testing.T) {
	ctx := context.Background()
	tokenFixtures(ctx, t)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	accounts := postgres.NewAccountRepository()

	// The fixture puts the one service account in tenant B and a person in tenant A.
	var inB, inA []identity.Account
	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantB}, func(ctx context.Context) error {
		var err error
		inB, err = accounts.ListOfKind(ctx, identity.AccountServiceAccount)
		return err
	}); err != nil {
		t.Fatalf("listing in B failed: %v", err)
	}
	if len(inB) != 1 || inB[0].ID != accountB {
		t.Fatalf("tenant B listed %+v", inB)
	}

	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		var err error
		inA, err = accounts.ListOfKind(ctx, identity.AccountServiceAccount)
		return err
	}); err != nil {
		t.Fatalf("listing in A failed: %v", err)
	}
	if len(inA) != 0 {
		t.Fatalf("tenant A saw %d of tenant B's service accounts", len(inA))
	}
}

func countTokens(ctx context.Context, t *testing.T, id shared.ID) int {
	t.Helper()
	var rows int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM access_token WHERE id = $1`, id.String()).Scan(&rows); err != nil {
		t.Fatalf("counting tokens: %v", err)
	}
	return rows
}

func readRevokedAt(ctx context.Context, t *testing.T, id shared.ID) time.Time {
	t.Helper()
	var revoked *time.Time
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT revoked_at FROM access_token WHERE id = $1`, id.String()).Scan(&revoked); err != nil {
		t.Fatalf("reading revoked_at: %v", err)
	}
	if revoked == nil {
		return time.Time{}
	}
	return *revoked
}

// dumpToken renders every column of the row as text - which is what a stolen backup is.
func dumpToken(ctx context.Context, t *testing.T, id shared.ID) string {
	t.Helper()
	var dumped string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT access_token::text FROM access_token WHERE id = $1`, id.String()).Scan(&dumped); err != nil {
		t.Fatalf("dumping the row: %v", err)
	}
	return dumped
}
