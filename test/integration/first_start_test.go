// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The first start's functions (migration 0128), as the application role, in a database of their
// own: the suite's shared database always holds workspaces, and "empty" is the state under test.

func emptyInstallation(ctx context.Context, t *testing.T, name string) (admin, app *pgxpool.Pool) {
	t.Helper()
	adminDSN, appDSN := recoveredCluster(ctx, t, name)
	var err error
	if admin, err = pgxpool.New(ctx, adminDSN); err != nil {
		t.Fatalf("connecting as the superuser: %v", err)
	}
	t.Cleanup(admin.Close)
	if app, err = pgxpool.New(ctx, appDSN); err != nil {
		t.Fatalf("connecting as the application role: %v", err)
	}
	t.Cleanup(app.Close)
	return admin, app
}

func TestASetupCodeIsIssuedOnlyOnAnEmptyInstallationAndClaimedOnce(t *testing.T) {
	ctx := context.Background()
	admin, app := emptyInstallation(ctx, t, "hubtask_first_start_claim")
	now := time.Now().UTC()

	var unset bool
	if err := app.QueryRow(ctx, `SELECT installation_unset()`).Scan(&unset); err != nil || !unset {
		t.Fatalf("a migrated database is not unset: %v %v", unset, err)
	}

	// Two processes, two codes: both land.
	issue := func(id string, hash []byte, issued time.Time) bool {
		var landed bool
		if err := app.QueryRow(ctx, `SELECT issue_setup_code($1, $2, $3, $4)`,
			id, hash, issued, issued.Add(30*time.Minute)).Scan(&landed); err != nil {
			t.Fatalf("issuing: %v", err)
		}
		return landed
	}
	if !issue("019364de-0000-7000-8000-00000005e701", []byte("first"), now) ||
		!issue("019364de-0000-7000-8000-00000005e702", []byte("second"), now) {
		t.Fatal("a code was not issued on an empty installation")
	}

	// The application role reads nothing of the table itself.
	if _, err := app.Exec(ctx, `SELECT count(*) FROM instance_setup_code`); err == nil {
		t.Error("the application role reads the setup codes directly")
	}

	claim := func(hash []byte, at time.Time) string {
		var answer string
		if err := app.QueryRow(ctx, `SELECT claim_setup_code($1, $2)`, hash, at).Scan(&answer); err != nil {
			t.Fatalf("claiming: %v", err)
		}
		return answer
	}
	if got := claim([]byte("nobody's"), now); got != "WRONG" {
		t.Errorf("a wrong code answered %q", got)
	}
	if got := claim([]byte("first"), now.Add(31*time.Minute)); got != "EXPIRED" {
		t.Errorf("an expired code answered %q", got)
	}

	// The claim and the first workspace in one transaction, as the setup makes them.
	tx, err := app.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning: %v", err)
	}
	var answer string
	if err := tx.QueryRow(ctx, `SELECT claim_setup_code($1, $2)`, []byte("second"), now).Scan(&answer); err != nil || answer != "CLAIMED" {
		t.Fatalf("the valid code answered %q, %v", answer, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rolling back: %v", err)
	}
	// Rolled back with the setup that failed: the code is still there to be used.
	if got := claim([]byte("second"), now); got != "CLAIMED" {
		t.Fatalf("a claim rolled back with its setup spent the code: %q", got)
	}
	// A claim outside a setup spends every code, so the first one is gone too.
	if got := claim([]byte("first"), now); got != "WRONG" {
		t.Errorf("a code survived another's claim: %q", got)
	}

	// From the first workspace on, nothing is issued and nothing is claimed.
	if _, err := admin.Exec(ctx, `INSERT INTO tenant (id, slug, display_name)
		VALUES ('019364de-0000-7000-8000-00000005e710', 'first', 'First')`); err != nil {
		t.Fatalf("seeding a workspace: %v", err)
	}
	if issue("019364de-0000-7000-8000-00000005e703", []byte("third"), now) {
		t.Error("a code was issued on an installation with a workspace")
	}
	if got := claim([]byte("third"), now); got != "ALREADY_DONE" {
		t.Errorf("a claim after setup answered %q", got)
	}
	if err := app.QueryRow(ctx, `SELECT installation_unset()`).Scan(&unset); err != nil || unset {
		t.Errorf("an installation with a workspace reads unset: %v %v", unset, err)
	}
}

// Two setups at once with two valid codes: the advisory lock makes the second wait for the first,
// and it then finds a workspace.
func TestOfTwoSetupsAtOnceTheFirstWins(t *testing.T) {
	ctx := context.Background()
	_, app := emptyInstallation(ctx, t, "hubtask_first_start_race")
	now := time.Now().UTC()
	for i, hash := range []string{"one", "two"} {
		if _, err := app.Exec(ctx, `SELECT issue_setup_code($1, $2, $3, $4)`,
			[]string{"019364de-0000-7000-8000-00000005e721", "019364de-0000-7000-8000-00000005e722"}[i],
			[]byte(hash), now, now.Add(30*time.Minute)); err != nil {
			t.Fatalf("issuing: %v", err)
		}
	}

	first, err := app.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning: %v", err)
	}
	defer func() { _ = first.Rollback(ctx) }()
	var answer string
	if err := first.QueryRow(ctx, `SELECT claim_setup_code($1, $2)`, []byte("one"), now).Scan(&answer); err != nil || answer != "CLAIMED" {
		t.Fatalf("the first claim answered %q, %v", answer, err)
	}
	// The setup's own write, in its own tenant's scope as the real one runs.
	if _, err := first.Exec(ctx, `SELECT set_config('app.tenant_id', '019364de-0000-7000-8000-00000005e730', true)`); err != nil {
		t.Fatalf("scoping: %v", err)
	}
	if _, err := first.Exec(ctx, `INSERT INTO tenant (id, slug, display_name)
		VALUES ('019364de-0000-7000-8000-00000005e730', 'winner', 'Winner')`); err != nil {
		t.Fatalf("the first setup's workspace: %v", err)
	}

	second := make(chan string, 1)
	go func() {
		var got string
		if err := app.QueryRow(ctx, `SELECT claim_setup_code($1, $2)`, []byte("two"), now).Scan(&got); err != nil {
			got = "error: " + err.Error()
		}
		second <- got
	}()

	select {
	case got := <-second:
		t.Fatalf("the second claim did not wait for the first: %q", got)
	case <-time.After(300 * time.Millisecond):
	}
	if err := first.Commit(ctx); err != nil {
		t.Fatalf("committing the first setup: %v", err)
	}
	if got := <-second; got != "ALREADY_DONE" {
		t.Errorf("the second setup answered %q", got)
	}
}

// The census leaves a workspace that holds nobody but a service account out of the customer
// counts and says how many there are; the register's state counts every workspace, as
// `is_operator` does.
func TestTheCensusDoesNotCountAServiceOnlyWorkspace(t *testing.T) {
	ctx := context.Background()
	admin, app := emptyInstallation(ctx, t, "hubtask_first_start_census")
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES
		   ('019364de-0000-7000-8000-00000005e741', 'operator', 'Operator'),
		   ('019364de-0000-7000-8000-00000005e742', 'family', 'Family')`,
		`INSERT INTO account (id, tenant_id, kind, display_name, status) VALUES
		   ('019364de-0000-7000-8000-00000005e751', '019364de-0000-7000-8000-00000005e741', 'SERVICE_ACCOUNT', 'Bootstrap', 'ACTIVE')`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status) VALUES
		   ('019364de-0000-7000-8000-00000005e752', '019364de-0000-7000-8000-00000005e742', 'USER', 'eva@family.example', 'Eva', 'ACTIVE')`,
		`INSERT INTO operator (tenant_id, account_id) VALUES
		   ('019364de-0000-7000-8000-00000005e741', '019364de-0000-7000-8000-00000005e751')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	var active, suspended, pending, accountsActive, accountsTotal, serviceOnly int64
	if err := app.QueryRow(ctx, `SELECT workspaces_active, workspaces_suspended,
		workspaces_pending_deletion, accounts_active, accounts_total, workspaces_service_only
		FROM instance_census()`).Scan(&active, &suspended, &pending, &accountsActive, &accountsTotal, &serviceOnly); err != nil {
		t.Fatalf("reading the census: %v", err)
	}
	if active != 1 || suspended != 0 || pending != 0 || serviceOnly != 1 {
		t.Errorf("census = %d active, %d suspended, %d pending, %d service-only; want 1, 0, 0, 1",
			active, suspended, pending, serviceOnly)
	}
	if accountsTotal != 2 {
		t.Errorf("accounts = %d, want both", accountsTotal)
	}

	var workspaces, operators int64
	if err := app.QueryRow(ctx, `SELECT workspaces, operators FROM operator_register_state()`).
		Scan(&workspaces, &operators); err != nil {
		t.Fatalf("reading the register's state: %v", err)
	}
	if workspaces != 2 || operators != 1 {
		t.Errorf("register state = %d workspaces, %d operators; want 2, 1", workspaces, operators)
	}
}
