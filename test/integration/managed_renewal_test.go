// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-ID-20 check 5, against PostgreSQL as the application's role: a new start password lands as a
// hash with its mark, every session of the account ends, every unfinished step is spent, and the
// trail records it - for a managed account only.

// plainHasher stands for Argon2id: what is tested is where the hash lands, not how it is made.
type plainHasher struct{}

func (plainHasher) Hash(password secret.Secret) (string, error) {
	return "hash:" + password.Reveal(), nil
}

func (plainHasher) Verify(stored string, password secret.Secret) (bool, error) {
	return stored == "hash:"+password.Reveal(), nil
}

func (plainHasher) VerifyDecoy(secret.Secret) {}

func renewal(ctx context.Context, t *testing.T) identityservice.IssueStartPassword {
	t.Helper()
	return identityservice.IssueStartPassword{
		Accounts: postgres.NewAccountRepository(), StartPasswords: postgres.NewAccountRepository(),
		Sessions: postgres.NewSessionRepository(), Authorizer: allowAll{}, StepUp: provenStepUp{},
		Passwords: plainHasher{}, Entropy: clockadapter.CryptoRandom{},
		Audit: postgres.NewAuditSink(generator{t}), UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock: clockadapter.System{},
	}
}

func administratorOf(tenant shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000035a1"),
		AccountName: "Anna", Kind: shared.ActorUser,
	}
}

func seedSessionAndStep(ctx context.Context, t *testing.T, tenant, account shared.ID) {
	t.Helper()
	admin := adminPool(ctx, t)
	for range 2 {
		if _, err := admin.Exec(ctx, `
			INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
			VALUES ($1, $2, $3, now(), now() + interval '30 days')`,
			freshID(t).String(), tenant.String(), account.String()); err != nil {
			t.Fatalf("seeding a session: %v", err)
		}
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO auth_pending (id, tenant_id, account_id, token_hash, purpose, created_at, expires_at)
		VALUES ($1, $2, $3, $4, 'PASSWORD', now(), now() + interval '5 minutes')`,
		freshID(t).String(), tenant.String(), account.String(), []byte(freshID(t).String())); err != nil {
		t.Fatalf("seeding a change step: %v", err)
	}
}

func TestANewStartPasswordEndsEverythingTheOldOneOpened(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	lena := managedIn(t, tenantA, freshSignInName(t))
	seedSessionAndStep(ctx, t, tenantA, lena.ID)

	issued, err := renewal(ctx, t).Execute(ctx, administratorOf(tenantA), lena.ID, "proof")
	if err != nil {
		t.Fatalf("renewing: %v", err)
	}
	if issued.Account.SignInName != lena.SignInName {
		t.Errorf("the answer names %q", issued.Account.SignInName)
	}

	admin := adminPool(ctx, t)
	var (
		hash   string
		marked *time.Time
	)
	if err := admin.QueryRow(ctx, `SELECT password_hash, start_password_at FROM account WHERE id = $1`,
		lena.ID.String()).Scan(&hash, &marked); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if hash != "hash:"+issued.StartPassword.Reveal() || marked == nil || !marked.After(created) {
		t.Errorf("the row holds %q, %v", hash, marked)
	}
	var live, open, entries int
	if err := admin.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM session WHERE account_id = $1 AND revoked_at IS NULL),
		(SELECT count(*) FROM auth_pending WHERE account_id = $1 AND consumed_at IS NULL),
		(SELECT count(*) FROM audit_log WHERE target_id = $1 AND action = 'account.start_password_issued')`,
		lena.ID.String()).Scan(&live, &open, &entries); err != nil {
		t.Fatalf("reading what is left: %v", err)
	}
	if live != 0 || open != 0 || entries != 1 {
		t.Errorf("%d sessions live, %d steps open, %d trail entries; want 0, 0, 1", live, open, entries)
	}
}

// An account with an address recovers through its mailbox: the statement itself refuses to renew
// it, whatever a caller read before.
func TestTheStoreRenewsNoAccountWithAnAddress(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	lena := managedIn(t, tenantA, freshSignInName(t))
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE account SET email = $2 WHERE id = $1`,
		lena.ID.String(), freshSignInName(t)+"@example.org"); err != nil {
		t.Fatalf("giving the account an address: %v", err)
	}
	var renewed bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		renewed, err = postgres.NewAccountRepository().IssueStartPassword(ctx, lena.ID, "hash:x", time.Now())
		return err
	}); err != nil {
		t.Fatalf("renewing: %v", err)
	}
	if renewed {
		t.Error("an account with an address was given a start password")
	}
	_, err := renewal(ctx, t).Execute(ctx, administratorOf(tenantA), lena.ID, "proof")
	if shared.AsError(err) == nil || shared.AsError(err).DetailCode != "accounts.not_managed" {
		t.Errorf("the use case answered %v", err)
	}
}

// The cross-tenant negative test for both new store methods: another workspace's managed account is
// neither renewed nor are its steps spent.
func TestANewStartPasswordStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	theirs := managedIn(t, tenantB, freshSignInName(t))
	seedSessionAndStep(ctx, t, tenantB, theirs.ID)

	var (
		renewed bool
		spent   int
	)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		store := postgres.NewAccountRepository()
		var err error
		if renewed, err = store.IssueStartPassword(ctx, theirs.ID, "hash:x", time.Now()); err != nil {
			return err
		}
		spent, err = store.SpendPending(ctx, theirs.ID, time.Now())
		return err
	}); err != nil {
		t.Fatalf("reaching across: %v", err)
	}
	if renewed || spent != 0 {
		t.Errorf("another workspace's account: renewed %v, %d steps spent", renewed, spent)
	}
	_, err := renewal(ctx, t).Execute(ctx, administratorOf(tenantA), theirs.ID, "proof")
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the use case answered %v, want not found", err)
	}
}
