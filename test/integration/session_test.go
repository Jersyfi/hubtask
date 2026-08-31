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

// The sign-in surface of H-01, against the real boundary. Gate SG-3: one negative per port
// method - a session, a refresh token, an attempt row and a redemption token of one tenant are
// invisible and unusable in another, proved against row level security rather than asserted.

var (
	sessionAccountA = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000c1")
	sessionAccountB = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000c2")
	invitedA        = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000c3")
	invitedB        = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000c4")
	sessionA        = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000d1")
	sessionB        = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000d2")
	refreshA        = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000e1")
	refreshB        = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000e2")
)

func sessionSecretOf(seed byte) []byte {
	material := make([]byte, identity.TokenSecretBytes)
	for i := range material {
		material[i] = seed
	}
	return material
}

// sessionFixtures seeds two tenants, an active and an invited account in each, one live session
// with one live refresh token in each, and a redemption token on each invited account.
func sessionFixtures(ctx context.Context, t *testing.T) (tokenA, tokenB, redeemA identity.Token) {
	t.Helper()
	admin := adminPool(ctx, t)
	installation := secret.New(installationSecret)
	refreshHasher := security.NewSessionRefreshHasher(installation)
	redemptionHasher := security.NewRedemptionTokenHasher(installation)

	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone)
		VALUES ($1, 'tenant-a', 'A', 'de', 'Europe/Berlin'), ($2, 'tenant-b', 'B', 'en', 'UTC')
		ON CONFLICT (id) DO NOTHING`, tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding tenants: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status)
		VALUES
			($1, $5, 'USER', 'a@example.org', 'A', '$argon2id$v=19$m=64,t=1,p=1$c2FsdA$aGFzaA', 'ACTIVE'),
			($2, $6, 'USER', 'b@example.org', 'B', NULL, 'ACTIVE'),
			($3, $5, 'USER', 'invited-a@example.org', 'IA', NULL, 'INVITED'),
			($4, $6, 'USER', 'invited-b@example.org', 'IB', NULL, 'INVITED')
		ON CONFLICT (id) DO NOTHING`,
		sessionAccountA.String(), sessionAccountB.String(),
		invitedA.String(), invitedB.String(),
		tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding accounts: %v", err)
	}

	var err error
	tokenA, err = identity.NewRefreshToken(tenantA, sessionSecretOf(0xA1))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	tokenB, err = identity.NewRefreshToken(tenantB, sessionSecretOf(0xB1))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	redeemA, err = identity.NewRedemptionToken(tenantA, sessionSecretOf(0xC1))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	redeemB, err := identity.NewRedemptionToken(tenantB, sessionSecretOf(0xC2))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}

	if _, err := admin.Exec(ctx, `
		INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		VALUES
			($1, $3, $5, now(), now() + interval '30 days'),
			($2, $4, $6, now(), now() + interval '30 days')
		ON CONFLICT (id) DO NOTHING`,
		sessionA.String(), sessionB.String(),
		tenantA.String(), tenantB.String(),
		sessionAccountA.String(), sessionAccountB.String()); err != nil {
		t.Fatalf("seeding sessions: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO session_refresh_token (id, tenant_id, session_id, token_hash, created_at, expires_at)
		VALUES
			($1, $3, $5, $7, now(), now() + interval '30 days'),
			($2, $4, $6, $8, now(), now() + interval '30 days')
		ON CONFLICT (id) DO NOTHING`,
		refreshA.String(), refreshB.String(),
		tenantA.String(), tenantB.String(),
		sessionA.String(), sessionB.String(),
		refreshHasher.Hash(tokenA.Secret()), refreshHasher.Hash(tokenB.Secret())); err != nil {
		t.Fatalf("seeding refresh tokens: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		UPDATE account SET redemption_token_hash = CASE id
			WHEN $1::uuid THEN $3::bytea
			WHEN $2::uuid THEN $4::bytea
		END,
		redemption_expires_at = now() + interval '14 days'
		WHERE id IN ($1::uuid, $2::uuid)`,
		invitedA.String(), invitedB.String(),
		redemptionHasher.Hash(redeemA.Secret()), redemptionHasher.Hash(redeemB.Secret())); err != nil {
		t.Fatalf("seeding redemption tokens: %v", err)
	}
	return tokenA, tokenB, redeemA
}

func sessionStores(ctx context.Context, t *testing.T) (
	postgres.SessionRepository, postgres.RefreshTokenRepository,
	postgres.SignInRepository, persistence.UnitOfWork,
) {
	t.Helper()
	installation := secret.New(installationSecret)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	return postgres.NewSessionRepository(),
		postgres.NewRefreshTokenRepository(security.NewSessionRefreshHasher(installation)),
		postgres.NewSignInRepository(
			security.NewRedemptionTokenHasher(installation),
			security.NewAuthAttemptHasher(installation)),
		uow
}

func inTenant(t *testing.T, uow persistence.UnitOfWork, tenant shared.ID, fn func(context.Context) error) {
	t.Helper()
	if err := uow.Within(context.Background(), persistence.Scope{TenantID: tenant}, fn); err != nil {
		t.Fatalf("transaction: %v", err)
	}
}

// ============================ Sessions (repository.Sessions) ============================

func TestASessionIsFoundInItsOwnTenantAndNotAnothers(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	sessions, _, _, uow := sessionStores(ctx, t)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		credential, err := sessions.FindForAuth(ctx, sessionA)
		if err != nil {
			t.Fatalf("own session not found: %v", err)
		}
		if credential.Session.AccountID != sessionAccountA || credential.Account.DisplayName != "A" {
			t.Errorf("credential = %+v", credential)
		}
		if credential.TenantLocale == "" || credential.TenantTimeZone == "" {
			// The value itself depends on which fixture seeded the shared tenant first; what
			// matters is that the locale chain travels in the one round trip.
			t.Errorf("tenant defaults did not travel: %q %q", credential.TenantLocale, credential.TenantTimeZone)
		}
		return nil
	})

	// Gate SG-3: FindForAuth.
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if _, err := sessions.FindForAuth(ctx, sessionB); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another tenant's session answered %v, want not found", err)
		}
		return nil
	})
}

// Gate SG-3: Insert.
func TestASessionCannotBeOpenedForAnotherTenantsAccount(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	sessions, _, _, uow := sessionStores(ctx, t)

	err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		return sessions.Insert(ctx, identity.Session{
			ID:        shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000d9"),
			TenantID:  tenantA,
			AccountID: sessionAccountB,
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
		})
	})
	if err == nil {
		t.Fatal("a session was opened for another tenant's account")
	}
}

// Gate SG-3: ForAccount.
func TestSessionsAreListedPerTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	sessions, _, _, uow := sessionStores(ctx, t)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		own, err := sessions.ForAccount(ctx, sessionAccountA, time.Now())
		if err != nil || len(own) != 1 {
			t.Fatalf("own listing = %v, %v; want the one session", own, err)
		}
		foreign, err := sessions.ForAccount(ctx, sessionAccountB, time.Now())
		if err != nil || len(foreign) != 0 {
			t.Errorf("another tenant's account listed %d sessions from here", len(foreign))
		}
		return nil
	})
}

// Gate SG-3: TouchLastSeen and Extend.
func TestBookkeepingWritesStayInsideTheTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	sessions, _, _, uow := sessionStores(ctx, t)
	admin := adminPool(ctx, t)

	at := time.Now().Add(time.Minute).UTC()
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if err := sessions.TouchLastSeen(ctx, sessionB, at); err != nil {
			t.Fatalf("touching: %v", err)
		}
		if err := sessions.Extend(ctx, sessionB, at.Add(time.Hour)); err != nil {
			t.Fatalf("extending: %v", err)
		}
		return nil
	})

	var lastSeen *time.Time
	if err := admin.QueryRow(ctx,
		`SELECT last_seen_at FROM session WHERE id = $1`, sessionB.String(),
	).Scan(&lastSeen); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if lastSeen != nil {
		t.Error("a write from another tenant reached the row")
	}
}

// Gate SG-3: Revoke and RevokeAll.
func TestRevocationStaysInsideTheTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	sessions, _, _, uow := sessionStores(ctx, t)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		changed, err := sessions.Revoke(ctx, sessionB, sessionAccountB, time.Now())
		if err != nil {
			t.Fatalf("revoking: %v", err)
		}
		if changed {
			t.Error("another tenant's session was revoked from here")
		}
		ended, err := sessions.RevokeAll(ctx, sessionAccountB, time.Now())
		if err != nil {
			t.Fatalf("revoking all: %v", err)
		}
		if ended != 0 {
			t.Errorf("%d of another tenant's sessions ended from here", ended)
		}
		return nil
	})
}

func TestASessionRevokesOnceAndItsPairRefuses(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	sessions, _, _, uow := sessionStores(ctx, t)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		changed, err := sessions.Revoke(ctx, sessionA, sessionAccountA, time.Now())
		if err != nil || !changed {
			t.Fatalf("revoking (%v, %v), want the first withdrawal to write", changed, err)
		}
		changed, err = sessions.Revoke(ctx, sessionA, sessionAccountA, time.Now())
		if err != nil || changed {
			t.Fatalf("revoking twice (%v, %v), want nothing to change", changed, err)
		}

		credential, err := sessions.FindForAuth(ctx, sessionA)
		if err != nil {
			t.Fatalf("reading back: %v", err)
		}
		if verifyErr := credential.Session.Verify(time.Now()); verifyErr == nil {
			t.Error("a revoked session still verifies - its pair would keep answering")
		}
		return nil
	})
}

// ====================== Refresh tokens (repository.RefreshTokens) ======================

// Gate SG-3: FindByToken, and the rewritten-tenant probe the PAT suite runs.
func TestARefreshTokenQuotingTheWrongTenantFindsNothing(t *testing.T) {
	ctx := context.Background()
	tokenA, _, _ := sessionFixtures(ctx, t)
	_, refresh, _, uow := sessionStores(ctx, t)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		credential, err := refresh.FindByToken(ctx, tokenA)
		if err != nil {
			t.Fatalf("own token not found: %v", err)
		}
		if credential.Session.ID != sessionA || credential.Account.ID != sessionAccountA {
			t.Errorf("credential = %+v", credential)
		}
		return nil
	})

	// The same secret rewritten to name tenant B: the hash covers the whole string, so it
	// matches nothing at all - not "matches and is refused", matches nothing.
	rewritten, err := identity.NewRefreshToken(tenantB, sessionSecretOf(0xA1))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		if _, err := refresh.FindByToken(ctx, rewritten); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("a rewritten token answered %v, want not found", err)
		}
		return nil
	})
}

// Gate SG-3: Insert.
func TestARefreshTokenCannotJoinAnotherTenantsSession(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	_, refresh, _, uow := sessionStores(ctx, t)

	presented, err := identity.NewRefreshToken(tenantA, sessionSecretOf(0xA9))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	err = uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		return refresh.Insert(ctx, identity.RefreshToken{
			ID:        shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000e9"),
			TenantID:  tenantA,
			SessionID: sessionB,
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
		}, presented)
	})
	if err == nil {
		t.Fatal("a refresh token was chained to another tenant's session")
	}
}

// Gate SG-3: Rotate - and the once-only property the reuse detection stands on.
func TestARefreshTokenRotatesOnceAndOnlyInItsTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	_, refresh, _, uow := sessionStores(ctx, t)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		changed, err := refresh.Rotate(ctx, refreshB, time.Now())
		if err != nil || changed {
			t.Errorf("another tenant's token rotated from here (%v, %v)", changed, err)
		}
		changed, err = refresh.Rotate(ctx, refreshA, time.Now())
		if err != nil || !changed {
			t.Fatalf("rotating (%v, %v), want the first exchange to write", changed, err)
		}
		changed, err = refresh.Rotate(ctx, refreshA, time.Now())
		if err != nil || changed {
			t.Errorf("rotating twice (%v, %v) - reuse detection has nothing to stand on", changed, err)
		}
		return nil
	})
}

// ====================== Sign-in accounts (repository.SignInAccounts) ======================

// Gate SG-3: FindForSignIn.
func TestAnAddressSignsInOnlyInItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	_, _, signIn, uow := sessionStores(ctx, t)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		own, err := signIn.FindForSignIn(ctx, "A@Example.ORG")
		if err != nil {
			t.Fatalf("own address not found: %v", err)
		}
		if own.PasswordHash.IsEmpty() {
			t.Error("the stored hash did not travel")
		}
		if _, err := signIn.FindForSignIn(ctx, "b@example.org"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another tenant's address answered %v, want not found", err)
		}
		return nil
	})
}

// Gate SG-3: SetRedemptionToken and Redeem.
func TestRedemptionWritesStayInsideTheTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	_, _, signIn, uow := sessionStores(ctx, t)

	presented, err := identity.NewRedemptionToken(tenantA, sessionSecretOf(0xC9))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		changed, err := signIn.SetRedemptionToken(
			ctx, invitedB, presented, time.Now().Add(time.Hour), time.Now())
		if err != nil || changed {
			t.Errorf("another tenant's invitation was re-minted from here (%v, %v)", changed, err)
		}
		changed, err = signIn.Redeem(ctx, invitedB, "$argon2id$hash", time.Now())
		if err != nil || changed {
			t.Errorf("another tenant's invitation was redeemed from here (%v, %v)", changed, err)
		}
		return nil
	})
}

// Gate SG-3: FindByRedemptionToken - and redemption's once-only property end to end.
func TestARedemptionTokenWorksExactlyOnceInItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	_, _, redeemA := sessionFixtures(ctx, t)
	_, _, signIn, uow := sessionStores(ctx, t)

	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		// The token names tenant A; under B's context the hash matches nothing.
		if _, err := signIn.FindByRedemptionToken(ctx, redeemA); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another tenant's redemption token answered %v, want not found", err)
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		found, err := signIn.FindByRedemptionToken(ctx, redeemA)
		if err != nil {
			t.Fatalf("own token not found: %v", err)
		}
		if found.Account.ID != invitedA || found.Account.Status != identity.AccountInvited {
			t.Errorf("found = %+v", found.Account)
		}
		if found.ExpiresAt.IsZero() {
			t.Error("the expiry did not travel")
		}

		changed, err := signIn.Redeem(ctx, invitedA, "$argon2id$first", time.Now())
		if err != nil || !changed {
			t.Fatalf("redeeming (%v, %v), want the first redemption to write", changed, err)
		}
		// The token died with the redemption: a second presentation finds nothing.
		if _, err := signIn.FindByRedemptionToken(ctx, redeemA); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("a redeemed token still answers %v", err)
		}
		changed, err = signIn.Redeem(ctx, invitedA, "$argon2id$second", time.Now())
		if err != nil || changed {
			t.Errorf("a second redemption wrote (%v, %v)", changed, err)
		}
		return nil
	})
}

// ====================== The attempt ledger (repository.AuthAttempts) ======================

// Gate SG-3: Find, Record and Clear.
func TestTheAttemptLedgerIsPerTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	_, _, signIn, uow := sessionStores(ctx, t)

	subject := "account:somebody@example.org"
	at := time.Now().UTC().Truncate(time.Second)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		return signIn.Record(ctx, subject, repository.AuthAttempt{
			Failures: 5, LastFailureAt: at, LockedUntil: at.Add(time.Minute),
		})
	})

	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		standing, err := signIn.Find(ctx, subject)
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if standing.Failures != 0 {
			t.Errorf("another tenant's failures visible here: %+v", standing)
		}
		// Clearing from the wrong tenant clears nothing.
		return signIn.Clear(ctx, subject)
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		standing, err := signIn.Find(ctx, subject)
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if standing.Failures != 5 || !standing.LockedUntil.After(at) {
			t.Errorf("the ledger lost its standing: %+v", standing)
		}
		return signIn.Clear(ctx, subject)
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		standing, err := signIn.Find(ctx, subject)
		if err != nil || standing.Failures != 0 {
			t.Errorf("the slate was not wiped: %+v, %v", standing, err)
		}
		return nil
	})
}

// ====================== Tenant resolution (repository.TenantDirectory) ======================

// Resolve runs under the installation scope - the caller has no tenant yet, which is the whole
// point - and answers one identifier or none, never a listing.
func TestResolveTenantAnswersASlugAndRefusesTheRest(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	_, _, signIn, uow := sessionStores(ctx, t)

	err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		id, err := signIn.Resolve(ctx, "tenant-a")
		if err != nil {
			t.Fatalf("resolving a slug: %v", err)
		}
		if id != tenantA {
			t.Errorf("resolved %v, want tenant A", id)
		}
		if _, err := signIn.Resolve(ctx, "no-such-workspace"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("an unknown slug answered %v, want not found", err)
		}
		// Two tenants exist, so the single-mode question has no one answer and is refused
		// rather than guessed.
		if _, err := signIn.Resolve(ctx, ""); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the empty slug answered %v on a two-tenant installation, want not found", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}
}

// ====================== The retention sweep (lifecycle.ExpiredSessions) ======================

// Gate SG-3: DeleteExpired and CountExpired - the sweep is bounded by the tenant of the running
// transaction, and only ever takes a session that is already over.
func TestTheSessionSweepStaysInsideTheTenantAndTakesOnlyTheOver(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	sessions, _, _, uow := sessionStores(ctx, t)
	admin := adminPool(ctx, t)

	overA := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f1")
	overB := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f2")
	idleButLive := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f3")
	if _, err := admin.Exec(ctx, `
		INSERT INTO session (id, tenant_id, account_id, created_at, last_seen_at, expires_at, revoked_at)
		VALUES
			($1, $4, $6, now() - interval '90 days', now() - interval '60 days', now() - interval '30 days', NULL),
			($2, $5, $7, now() - interval '90 days', now() - interval '60 days', now() - interval '30 days', NULL),
			($3, $4, $6, now() - interval '90 days', now() - interval '60 days', now() + interval '10 days', NULL)
		ON CONFLICT (id) DO NOTHING`,
		overA.String(), overB.String(), idleButLive.String(),
		tenantA.String(), tenantB.String(),
		sessionAccountA.String(), sessionAccountB.String()); err != nil {
		t.Fatalf("seeding expired sessions: %v", err)
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		due, err := sessions.CountExpired(ctx, cutoff, 100)
		if err != nil {
			t.Fatalf("counting: %v", err)
		}
		if due != 1 {
			t.Errorf("%d due, want tenant A's one over session - not B's, not the live one", due)
		}
		removed, err := sessions.DeleteExpired(ctx, cutoff, 100)
		if err != nil {
			t.Fatalf("sweeping: %v", err)
		}
		if removed != 1 {
			t.Errorf("removed %d, want exactly tenant A's over session", removed)
		}
		return nil
	})

	var left int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM session WHERE id IN ($1, $2)`,
		overB.String(), idleButLive.String()).Scan(&left); err != nil {
		t.Fatalf("counting the survivors: %v", err)
	}
	if left != 2 {
		t.Errorf("%d survivors, want B's row and the still-live one untouched", left)
	}
}
