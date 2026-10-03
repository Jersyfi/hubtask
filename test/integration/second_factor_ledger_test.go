// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
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
