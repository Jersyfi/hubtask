// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/base32"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// SC-22 (#1117), against the real database: a wrong proof at a second-factor door advances the
// attempt ledger. The real unit of work rolls back everything a refusing transaction wrote, so a
// failure recorded inside it never landed - which no service test could show, because their unit of
// work did not roll back. The step-up's wrong password is the door walked here: the same helper
// settles the refusal at every door, and the service tests (SecondFactorLedger_test.go) hold each
// door to it against a unit of work that now rolls back the ledger as this one does.
var (
	ledgerAccount = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000022c1")
	ledgerSession = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000022d1")
)

func TestAWrongStepUpPasswordAdvancesTheLedger(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	passwords, err := crypto.NewPasswords(clockadapter.CryptoRandom{})
	if err != nil {
		t.Fatalf("the hasher: %v", err)
	}
	hash, err := passwords.Hash(secret.New("the right passphrase"))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status)
		VALUES ($1, $2, 'USER', 'ledger@example.org', 'Ledger', $3, 'ACTIVE')
		ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash`,
		ledgerAccount.String(), tenantA.String(), hash); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		VALUES ($1, $2, $3, now(), now() + interval '30 days')
		ON CONFLICT (id) DO UPDATE SET revoked_at = NULL`,
		ledgerSession.String(), tenantA.String(), ledgerAccount.String()); err != nil {
		t.Fatalf("seeding the session: %v", err)
	}
	sessions, _, signIn, uow := sessionStores(ctx, t)
	subject := "stepup:" + ledgerAccount.String()
	inTenant(t, uow, tenantA, func(ctx context.Context) error { return signIn.Clear(ctx, subject) })

	writer := identityservice.SessionWriter{
		Accounts: signIn, Sessions: sessions, Attempts: signIn, Passwords: passwords,
		Audit: postgres.NewAuditSink(generator{t}), UnitOfWork: uow, Clock: clockadapter.System{},
	}
	actor := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantA, AccountID: ledgerAccount, TokenID: ledgerSession,
	}

	for want := 1; want <= 2; want++ {
		if _, err := (identityservice.StepUp{Writer: writer}).Execute(ctx, actor,
			identityservice.StepUpCommand{Password: secret.New("a wrong passphrase")}); err == nil {
			t.Fatal("a wrong password proved a step-up")
		}
		if err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
			standing, err := signIn.Find(ctx, subject)
			if err != nil {
				return err
			}
			if standing.Failures != want {
				t.Errorf("after %d wrong passwords the ledger stands at %d", want, standing.Failures)
			}
			return nil
		}); err != nil {
			t.Fatalf("reading the ledger: %v", err)
		}
	}
}

// The other doors, walked with the real services over the real stores: the enrolment's
// confirmation, the sign-in's second step (a code and a recovery code), the step-up (a code and a
// recovery code) and the replacement's confirmation. The provider's step-up and the LINK step start
// at a provider this database cannot stand in for; the service tests hold those two.
var (
	doorsAccount = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000022c2")
	doorsSession = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000022d2")
)

// realWriter is the session writer as the composition root builds it, over this database.
func realWriter(ctx context.Context, t *testing.T) (identityservice.SessionWriter, postgres.SignInRepository, persistence.UnitOfWork) {
	t.Helper()
	installation := secret.New(installationSecret)
	sessions, refresh, signIn, uow := sessionStores(ctx, t)
	passwords, err := crypto.NewPasswords(clockadapter.CryptoRandom{})
	if err != nil {
		t.Fatalf("the hasher: %v", err)
	}
	mfa := postgres.NewMfaRepository(
		security.NewPendingTokenHasher(installation), security.NewRecoveryCodeHasher(installation))
	return identityservice.SessionWriter{
		Accounts: signIn, Sessions: sessions, Refresh: refresh, Attempts: signIn, Tenants: signIn,
		Passwords: passwords, Signer: security.NewSessionTokenIssuer(installation),
		Audit: postgres.NewAuditSink(generator{t}), UnitOfWork: uow,
		Clock: clockadapter.System{}, IDs: clockadapter.NewUUIDv7(clockadapter.System{}),
		Entropy: clockadapter.CryptoRandom{}, Multi: true,
		Enrollments: mfa, Recovery: mfa, Pending: mfa, Policy: mfa,
		Encryptor:   ring(t, keyOne()),
		Memberships: postgres.NewMembershipRepository(),
		People:      postgres.NewAccountRepository(),
		StepUps:     postgres.NewStepUpRepository(security.NewStepUpTokenHasher(installation)),
		Issuer:      "Hubtask",
	}, signIn, uow
}

func TestAWrongProofAdvancesTheLedgerAtEveryDoorItCanBeWalkedTo(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	writer, signIn, uow := realWriter(ctx, t)
	hash, err := writer.Passwords.Hash(secret.New("the right passphrase"))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `DELETE FROM account_mfa WHERE account_id = $1`, doorsAccount.String()); err != nil {
		t.Fatalf("clearing an earlier run's factor: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status)
		VALUES ($1, $2, 'USER', 'doors@example.org', 'Doors', $3, 'ACTIVE')
		ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash`,
		doorsAccount.String(), tenantA.String(), hash); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		VALUES ($1, $2, $3, now(), now() + interval '30 days')
		ON CONFLICT (id) DO UPDATE SET revoked_at = NULL`,
		doorsSession.String(), tenantA.String(), doorsAccount.String()); err != nil {
		t.Fatalf("seeding the session: %v", err)
	}
	mfaSubject, stepUpSubject := "mfa:"+doorsAccount.String(), "stepup:"+doorsAccount.String()
	for _, subject := range []string{mfaSubject, stepUpSubject, "account:doors@example.org"} {
		inTenant(t, uow, tenantA, func(ctx context.Context) error { return signIn.Clear(ctx, subject) })
	}
	ledger := func(subject string) int {
		t.Helper()
		var failures int
		inTenant(t, uow, tenantA, func(ctx context.Context) error {
			standing, err := signIn.Find(ctx, subject)
			failures = standing.Failures
			return err
		})
		return failures
	}
	actor := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantA, AccountID: doorsAccount, TokenID: doorsSession,
		Scopes: []string{"account:read"},
	}
	wrong := func(raw []byte) string { return domain.TotpCode(raw, domain.TotpStep(time.Now())+1000) }

	// The enrolment's confirmation.
	minted, err := identityservice.EnrollTotp{Writer: writer}.Execute(ctx, actor, identityservice.EnrollTotpCommand{})
	if err != nil {
		t.Fatalf("enrolling: %v", err)
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(minted.Secret.Reveal())
	if err != nil {
		t.Fatalf("reading the secret: %v", err)
	}
	if _, err := (identityservice.ConfirmTotp{Writer: writer}).Execute(ctx, actor,
		identityservice.ConfirmTotpCommand{Code: wrong(raw)}); err == nil {
		t.Fatal("a wrong code armed the factor")
	}
	if got := ledger(mfaSubject); got != 1 {
		t.Errorf("after a wrong confirmation the ledger stands at %d, want 1", got)
	}
	if _, err := (identityservice.ConfirmTotp{Writer: writer}).Execute(ctx, actor,
		identityservice.ConfirmTotpCommand{Code: domain.TotpCode(raw, domain.TotpStep(time.Now()))}); err != nil {
		t.Fatalf("confirming: %v", err)
	}

	// The sign-in's second step: a wrong code, then a wrong recovery code.
	signedIn, err := identityservice.SignIn{Writer: writer}.Execute(ctx, identityservice.SignInCommand{
		Email: "doors@example.org", Password: secret.New("the right passphrase"), TenantHeader: tenantA.String(),
	})
	if err != nil || signedIn.Challenge == nil {
		t.Fatalf("no second step: (%+v, %v)", signedIn, err)
	}
	before := ledger(mfaSubject)
	for i, cmd := range []identityservice.CompleteSignInCommand{
		{PendingToken: signedIn.Challenge.Token, Code: wrong(raw)},
		{PendingToken: signedIn.Challenge.Token, RecoveryCode: secret.New("AAAA-BBBB-CCCC-DDDD")},
	} {
		if _, _, err := (identityservice.CompleteSignIn{Writer: writer}).Execute(ctx, cmd); err == nil {
			t.Fatal("a wrong proof completed the sign-in")
		}
		if got := ledger(mfaSubject); got != before+i+1 {
			t.Errorf("after second-step refusal %d the ledger stands at %d, want %d", i+1, got, before+i+1)
		}
	}

	// The step-up: a wrong code, then a wrong recovery code.
	before = ledger(stepUpSubject)
	for i, cmd := range []identityservice.StepUpCommand{
		{Code: wrong(raw)}, {RecoveryCode: secret.New("AAAA-BBBB-CCCC-DDDD")},
	} {
		if _, err := (identityservice.StepUp{Writer: writer}).Execute(ctx, actor, cmd); err == nil {
			t.Fatal("a wrong proof proved a step-up")
		}
		if got := ledger(stepUpSubject); got != before+i+1 {
			t.Errorf("after step-up refusal %d the ledger stands at %d, want %d", i+1, got, before+i+1)
		}
	}

	// The replacement's confirmation: begun behind a password step-up, refused with a wrong code.
	inTenant(t, uow, tenantA, func(ctx context.Context) error { return signIn.Clear(ctx, stepUpSubject) })
	grant, err := identityservice.StepUp{Writer: writer}.Execute(ctx, actor,
		identityservice.StepUpCommand{Password: secret.New("the right passphrase")})
	if err != nil {
		t.Fatalf("stepping up: %v", err)
	}
	if _, err := (identityservice.StartAuthenticatorReplacement{Writer: writer}).Execute(ctx, actor,
		identityservice.StartAuthenticatorReplacementCommand{StepUpToken: grant.Token.Reveal()}); err != nil {
		t.Fatalf("beginning a replacement: %v", err)
	}
	before = ledger(mfaSubject)
	if _, err := (identityservice.ConfirmAuthenticatorReplacement{Writer: writer}).Execute(ctx, actor,
		identityservice.ConfirmAuthenticatorReplacementCommand{Code: wrong(raw)}); err == nil {
		t.Fatal("a wrong code confirmed the replacement")
	}
	if got := ledger(mfaSubject); got != before+1 {
		t.Errorf("after a wrong replacement code the ledger stands at %d, want %d", got, before+1)
	}
}
