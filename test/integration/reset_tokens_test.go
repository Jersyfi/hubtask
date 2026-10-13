// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// A password replaced - through the reset link or from the profile - against the real database:
// the sessions live in one table and the personal access tokens in another, so whether the second
// survives the end of the first is a property of the statements, which no fake of RevokeOthers can
// show. Each test has its own account, sessions and token, named by nobody else in the package.
var (
	resetTokensAccount  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000051c1")
	resetTokensPhone    = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000051d1")
	resetTokensTablet   = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000051d2")
	resetTokensPAT      = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000051e1")
	changeTokensAccount = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000052c1")
	changeTokensHere    = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000052d1")
	changeTokensPhone   = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000052d2")
	changeTokensPAT     = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000052e1")
)

const (
	resetTokensOld = "the passphrase somebody may know"
	resetTokensNew = "a brand new passphrase nobody knows"
)

// passwordWriterOver is the password writer as the composition root builds it, over this database.
func passwordWriterOver(ctx context.Context, t *testing.T) (identityservice.PasswordWriter, postgres.SignInRepository) {
	t.Helper()
	session, signIn, uow := realWriter(ctx, t)
	installation := secret.New(installationSecret)
	workspaces := postgres.NewWorkspaceSettingsRepository()
	passwords := identityservice.PasswordWriter{
		Session: session,
		Resolver: identityservice.SignInPolicyResolver{
			Workspaces: workspaces, Instance: postgres.NewInstanceSettingRepository(), UnitOfWork: uow,
		},
		Accounts: postgres.NewPasswordRepository(), Histories: postgres.NewPasswordRepository(),
		Pending: postgres.NewMfaRepository(
			security.NewPendingTokenHasher(installation), security.NewRecoveryCodeHasher(installation)),
		WaysIn: identityservice.WaysIn{
			Providers: postgres.NewIdentityProviderRepository(), Workspaces: workspaces,
			UnitOfWork: uow, Clock: clockadapter.System{},
		},
		Text: text.Composing{}, UnitOfWork: uow, Clock: clockadapter.System{}, IDs: session.IDs,
	}
	passwords.Session.Rule = passwords
	return passwords, signIn
}

// seedPasswordAccount puts an account with a password, live sessions and one personal access token
// into tenant A, replacing whatever an earlier run left of them, and answers the token.
func seedPasswordAccount(
	ctx context.Context, t *testing.T, passwords identityservice.PasswordWriter,
	account shared.ID, email string, tokenID shared.ID, tokenSeed byte, sessions ...shared.ID,
) string {
	t.Helper()
	admin := adminPool(ctx, t)
	hash, err := passwords.Session.Passwords.Hash(secret.New(resetTokensOld))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if _, err := admin.Exec(ctx, `DELETE FROM account_mfa WHERE account_id = $1`, account.String()); err != nil {
		t.Fatalf("clearing an earlier run's factor: %v", err)
	}
	if _, err := admin.Exec(ctx, `DELETE FROM access_token WHERE id = $1`, tokenID.String()); err != nil {
		t.Fatalf("clearing an earlier run's token: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, password_set_at, status)
		VALUES ($1, $2, 'USER', $3, 'Token Keeper', $4, now() - interval '30 days', 'ACTIVE')
		ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash,
		  password_set_at = EXCLUDED.password_set_at, status = 'ACTIVE'`,
		account.String(), tenantA.String(), email, hash); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	for _, id := range sessions {
		if _, err := admin.Exec(ctx, `
			INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
			VALUES ($1, $2, $3, now(), now() + interval '30 days')
			ON CONFLICT (id) DO UPDATE SET revoked_at = NULL`,
			id.String(), tenantA.String(), account.String()); err != nil {
			t.Fatalf("seeding a session: %v", err)
		}
	}
	presented := mintFor(t, tenantA, tokenSeed)
	tokens := postgres.NewAccessTokenRepository(security.NewTokenHasher(secret.New(installationSecret)))
	//nolint:contextcheck // the helper opens a transaction of its own, as every test of this package does
	inTenant(t, passwords.UnitOfWork, tenantA, func(ctx context.Context) error {
		return tokens.Insert(ctx, minted(tokenID, tenantA, account, "the nightly export"), presented)
	})
	return presented.Secret()
}

// tokenStillWorks presents the token on the path every request takes.
func tokenStillWorks(ctx context.Context, t *testing.T, passwords identityservice.PasswordWriter, presented string) error {
	t.Helper()
	_, err := identityservice.AuthenticateToken{
		Tokens:     postgres.NewAccessTokenRepository(security.NewTokenHasher(secret.New(installationSecret))),
		UnitOfWork: passwords.UnitOfWork, Clock: clockadapter.System{},
	}.Execute(ctx, identityservice.AuthenticateTokenCommand{Credential: presented})
	return err
}

// sessionOpen reads one session back the way authentication does.
func sessionOpen(ctx context.Context, t *testing.T, passwords identityservice.PasswordWriter, id shared.ID) bool {
	t.Helper()
	var open bool
	//nolint:contextcheck // the helper opens a transaction of its own, as every test of this package does
	inTenant(t, passwords.UnitOfWork, tenantA, func(ctx context.Context) error {
		credential, err := passwords.Session.Sessions.FindForAuth(ctx, id)
		if err != nil {
			return err
		}
		open = credential.Session.Verify(time.Now()) == nil
		return nil
	})
	return open
}

// UC-ID-04 checks 6 and 7: the request and the reset, through the real services over the real
// stores. Every session the account had ends and its personal access token still authenticates; the
// trail holds the request and the change, and neither the address nor either password is anywhere
// in the stored rows.
func TestAResetEndsTheSessionsKeepsTheTokensAndTrailsNoAddress(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	passwords, signIn := passwordWriterOver(ctx, t)
	const address = "reset-tokens@example.org"
	presented := seedPasswordAccount(ctx, t, passwords, resetTokensAccount, address,
		resetTokensPAT, 0x51, resetTokensPhone, resetTokensTablet)
	since := time.Now().Add(-time.Second)

	if err := (identityservice.ForgetPassword{Writer: passwords, Tenants: signIn, Multi: true}).Execute(ctx,
		identityservice.ForgetPasswordCommand{Email: address, TenantHeader: tenantA.String()}); err != nil {
		t.Fatalf("asking for a link: %v", err)
	}
	link, err := identityservice.MintResetToken{Writer: passwords}.MintResetToken(ctx, tenantA, resetTokensAccount)
	if err != nil || link.Token.IsEmpty() {
		t.Fatalf("minting the link: %+v (%v)", link, err)
	}
	result, err := identityservice.ResetPassword{Writer: passwords}.Execute(ctx, identityservice.ResetPasswordCommand{
		Token: link.Token, Password: secret.New(resetTokensNew), TenantHeader: tenantA.String(),
	})
	if err != nil || result.Pair == nil {
		t.Fatalf("resetting: %+v (%v)", result, err)
	}

	// Check 6.
	for _, id := range []shared.ID{resetTokensPhone, resetTokensTablet} {
		if sessionOpen(ctx, t, passwords, id) {
			t.Errorf("session %s is still open after the reset", id)
		}
	}
	if err := tokenStillWorks(ctx, t, passwords, presented); err != nil {
		t.Errorf("the personal access token stopped working with the reset: %v", err)
	}

	// Check 7, on the stored rows.
	rows, err := adminPool(ctx, t).Query(ctx, `
		SELECT action, row_to_json(audit_log)::text FROM audit_log
		WHERE tenant_id = $1 AND (actor_id = $2 OR target_id = $2) AND occurred_at >= $3`,
		tenantA.String(), resetTokensAccount.String(), since)
	if err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var action, stored string
		if err := rows.Scan(&action, &stored); err != nil {
			t.Fatalf("reading a trail row: %v", err)
		}
		seen[action] = true
		for _, leaked := range []string{address, "reset-tokens", resetTokensOld, resetTokensNew} {
			if strings.Contains(strings.ToLower(stored), leaked) {
				t.Errorf("the %s entry carries %q: %s", action, leaked, stored)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	for _, want := range []string{"auth.password_reset_requested", "account.password_changed"} {
		if !seen[want] {
			t.Errorf("the trail holds %v, want %s", seen, want)
		}
	}
}

// UC-ID-05 check 4: a password changed from the profile ends the other sessions, keeps this one, and
// leaves the personal access token authenticating.
func TestAPasswordChangeLeavesThePersonalAccessTokensWorking(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	passwords, _ := passwordWriterOver(ctx, t)
	presented := seedPasswordAccount(ctx, t, passwords, changeTokensAccount, "change-tokens@example.org",
		changeTokensPAT, 0x52, changeTokensHere, changeTokensPhone)
	// The step-up's own proof is tested at length elsewhere; this is about what the change does once
	// it is let through.
	passwords.StepUp = provenStepUp{}

	if err := (identityservice.ChangePassword{Writer: passwords}).Execute(ctx, appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantA, AccountID: changeTokensAccount, TokenID: changeTokensHere,
	}, identityservice.ChangePasswordCommand{
		Password: secret.New(resetTokensNew), StepUpToken: "proven",
	}); err != nil {
		t.Fatalf("changing the password: %v", err)
	}

	if !sessionOpen(ctx, t, passwords, changeTokensHere) {
		t.Error("the session that changed the password ended with it")
	}
	if sessionOpen(ctx, t, passwords, changeTokensPhone) {
		t.Error("another session survived the change")
	}
	if err := tokenStillWorks(ctx, t, passwords, presented); err != nil {
		t.Errorf("the personal access token stopped working with the change: %v", err)
	}
}
