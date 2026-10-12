// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The register a person can read and safely change (migration 0135), as the application role, in a
// database of its own: the empty register of a private installation is the state under test, and
// the suite's shared database always holds several workspaces.

const (
	rosterTenant      = "019364de-0000-7000-8000-0000000135a0"
	rosterOther       = "019364de-0000-7000-8000-0000000135b0"
	rosterOwner       = "019364de-0000-7000-8000-0000000135a1"
	rosterColleague   = "019364de-0000-7000-8000-0000000135a2"
	rosterInvited     = "019364de-0000-7000-8000-0000000135a3"
	rosterDisabled    = "019364de-0000-7000-8000-0000000135a4"
	rosterMember      = "019364de-0000-7000-8000-0000000135a5"
	rosterService     = "019364de-0000-7000-8000-0000000135a6"
	rosterOtherOwner  = "019364de-0000-7000-8000-0000000135b1"
	rosterNobody      = "019364de-0000-7000-8000-0000000135ff"
	rosterOwnerGrant  = "019364de-0000-7000-8000-0000000135c1"
	rosterMemberGrant = "019364de-0000-7000-8000-0000000135c2"
	rosterOtherGrant  = "019364de-0000-7000-8000-0000000135c3"
)

// privateInstallation is one workspace, its owner, a colleague, an invited and a disabled account,
// a member who is not an owner, and a service account - and nobody in the register.
func privateInstallation(ctx context.Context, t *testing.T, name string) (admin, app *pgxpool.Pool) {
	t.Helper()
	admin, app = emptyInstallation(ctx, t, name)
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + rosterTenant + `', 'family', 'The Family')`,
		`INSERT INTO account (id, tenant_id, kind, email, sign_in_name, display_name, status) VALUES
		   ('` + rosterOwner + `', '` + rosterTenant + `', 'USER', 'jerome@family.example', NULL, 'Jerome', 'ACTIVE'),
		   ('` + rosterColleague + `', '` + rosterTenant + `', 'USER', 'ada@family.example', NULL, 'Ada', 'ACTIVE'),
		   ('` + rosterInvited + `', '` + rosterTenant + `', 'USER', 'bob@family.example', NULL, 'Bob', 'INVITED'),
		   ('` + rosterDisabled + `', '` + rosterTenant + `', 'USER', 'cy@family.example', NULL, 'Cy', 'DISABLED'),
		   ('` + rosterMember + `', '` + rosterTenant + `', 'USER', NULL, 'dana', 'Dana', 'ACTIVE'),
		   ('` + rosterService + `', '` + rosterTenant + `', 'SERVICE_ACCOUNT', NULL, NULL, 'The shop', 'ACTIVE')`,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES
		   ('` + rosterOwnerGrant + `', '` + rosterTenant + `', '` + rosterOwner + `', 'TENANT', 'OWNER'),
		   ('` + rosterMemberGrant + `', '` + rosterTenant + `', '` + rosterMember + `', 'TENANT', 'MEMBER')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	return admin, app
}

func operatorHeld(ctx context.Context, t *testing.T, app *pgxpool.Pool, account string) bool {
	t.Helper()
	var held bool
	if err := app.QueryRow(ctx, `SELECT is_operator($1)`, account).Scan(&held); err != nil {
		t.Fatalf("asking the register: %v", err)
	}
	return held
}

func enlist(ctx context.Context, t *testing.T, app *pgxpool.Pool, account, by string) string {
	t.Helper()
	var outcome string
	if err := app.QueryRow(ctx, `SELECT enlist_operator($1, $2)`, account, by).Scan(&outcome); err != nil {
		t.Fatalf("enlisting: %v", err)
	}
	return outcome
}

func rosterRows(ctx context.Context, t *testing.T, app *pgxpool.Pool, caller string) map[string][2]string {
	t.Helper()
	rows, err := app.Query(ctx, `SELECT account_id::text, display_name, coalesce(email, sign_in_name, ''),
		kind, workspace_slug, workspace_name FROM operator_roster($1)`, caller)
	if err != nil {
		t.Fatalf("reading the roster: %v", err)
	}
	defer rows.Close()
	named := map[string][2]string{}
	for rows.Next() {
		var id, display, address, kind, slug, workspace string
		if err := rows.Scan(&id, &display, &address, &kind, &slug, &workspace); err != nil {
			t.Fatalf("scanning the roster: %v", err)
		}
		if slug != "family" || workspace != "The Family" {
			t.Errorf("row %s names workspace %q (%q)", id, workspace, slug)
		}
		named[id] = [2]string{display, address}
	}
	return named
}

// UC-INS-05 checks 1, 3, 4 and 5 against the functions themselves.
func TestTheRegisterNamesItsRowsAndAddingCannotLockTheOwnerOut(t *testing.T) {
	ctx := context.Background()
	admin, app := privateInstallation(ctx, t, "hubtask_operator_roster")

	if !operatorHeld(ctx, t, app, rosterOwner) {
		t.Fatal("the owner of the only workspace is not the operator of an empty register")
	}

	// Only an active account can be added (check 4), and nothing is written for one that is not.
	for _, account := range []string{rosterInvited, rosterDisabled} {
		if got := enlist(ctx, t, app, account, rosterOwner); got != "NOT_ACTIVE" {
			t.Errorf("enlisting %s answered %q", account, got)
		}
	}
	if got := enlist(ctx, t, app, rosterNobody, rosterOwner); got != "UNKNOWN" {
		t.Errorf("enlisting nobody answered %q", got)
	}

	// The first addition brings the owner along (check 3).
	if got := enlist(ctx, t, app, rosterColleague, rosterOwner); got != "ADDED_WITH_CALLER" {
		t.Fatalf("the first addition answered %q", got)
	}
	if !operatorHeld(ctx, t, app, rosterOwner) || !operatorHeld(ctx, t, app, rosterColleague) {
		t.Fatal("after the first addition the owner or the colleague is not an operator")
	}
	// From then on only the account named.
	if got := enlist(ctx, t, app, rosterService, rosterColleague); got != "ADDED" {
		t.Errorf("a later addition answered %q", got)
	}
	if got := enlist(ctx, t, app, rosterService, rosterColleague); got != "PRESENT" {
		t.Errorf("adding an operator again answered %q", got)
	}

	// The roster names its rows, never a workspace's other accounts (check 1).
	named := rosterRows(ctx, t, app, rosterColleague)
	if len(named) != 3 {
		t.Fatalf("the roster has %d rows, want three: %v", len(named), named)
	}
	if named[rosterOwner] != [2]string{"Jerome", "jerome@family.example"} ||
		named[rosterColleague] != [2]string{"Ada", "ada@family.example"} ||
		named[rosterService][0] != "The shop" {
		t.Errorf("the roster names %v", named)
	}
	if _, listed := named[rosterMember]; listed {
		t.Error("the roster named an account the register does not hold")
	}

	// Removing tells "not there" from "the last one" (check 5).
	dismiss := func(account string) string {
		var outcome string
		if err := app.QueryRow(ctx, `SELECT dismiss_operator($1)`, account).Scan(&outcome); err != nil {
			t.Fatalf("dismissing: %v", err)
		}
		return outcome
	}
	if got := dismiss(rosterMember); got != "ABSENT" {
		t.Errorf("removing an account not in the register answered %q", got)
	}
	if got := dismiss(rosterService); got != "REMOVED" {
		t.Errorf("removing the service account answered %q", got)
	}
	if got := dismiss(rosterColleague); got != "REMOVED" {
		t.Errorf("removing the colleague answered %q", got)
	}
	if got := dismiss(rosterOwner); got != "LAST" {
		t.Errorf("removing the last operator answered %q", got)
	}

	// The application role still reads nothing of the table itself.
	if _, err := app.Exec(ctx, `SELECT count(*) FROM operator`); err == nil {
		t.Error("the application role reads the register directly")
	}
	_ = admin
}

// Review F2: the roster answers no row to anybody the register does not name - a member, the owner
// of another workspace - whatever they hold in their own workspace.
func TestTheRosterAnswersNothingToSomebodyItDoesNotName(t *testing.T) {
	ctx := context.Background()
	admin, app := privateInstallation(ctx, t, "hubtask_operator_roster_closed")
	if got := enlist(ctx, t, app, rosterColleague, rosterOwner); got != "ADDED_WITH_CALLER" {
		t.Fatalf("seeding the register answered %q", got)
	}
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + rosterOther + `', 'acme', 'Acme')`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status) VALUES
		   ('` + rosterOtherOwner + `', '` + rosterOther + `', 'USER', 'olga@acme.example', 'Olga', 'ACTIVE')`,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES
		   ('` + rosterOtherGrant + `', '` + rosterOther + `', '` + rosterOtherOwner + `', 'TENANT', 'OWNER')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding a second workspace: %v", err)
		}
	}

	for _, caller := range []string{rosterMember, rosterOtherOwner, rosterNobody} {
		if named := rosterRows(ctx, t, app, caller); len(named) != 0 {
			t.Errorf("%s, not an operator, read %d rows of the register", caller, len(named))
		}
	}
	if named := rosterRows(ctx, t, app, rosterOwner); len(named) != 2 {
		t.Errorf("an operator read %d rows, want two", len(named))
	}
}

// Two first additions at once: the lock makes the second wait, and it then finds the register no
// longer empty - one of them brings the caller along, not both.
func TestOfTwoFirstAdditionsAtOnceOneBringsTheCaller(t *testing.T) {
	ctx := context.Background()
	_, app := privateInstallation(ctx, t, "hubtask_operator_roster_race")

	first, err := app.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning: %v", err)
	}
	defer func() { _ = first.Rollback(ctx) }()
	var answer string
	if err := first.QueryRow(ctx, `SELECT enlist_operator($1, $2)`, rosterColleague, rosterOwner).Scan(&answer); err != nil || answer != "ADDED_WITH_CALLER" {
		t.Fatalf("the first addition answered %q, %v", answer, err)
	}

	second := make(chan string, 1)
	go func() {
		var got string
		if err := app.QueryRow(ctx, `SELECT enlist_operator($1, $2)`, rosterService, rosterOwner).Scan(&got); err != nil {
			got = "error: " + err.Error()
		}
		second <- got
	}()
	select {
	case got := <-second:
		t.Fatalf("the second addition did not wait for the first: %q", got)
	case <-time.After(300 * time.Millisecond):
	}
	if err := first.Commit(ctx); err != nil {
		t.Fatalf("committing the first addition: %v", err)
	}
	if got := <-second; got != "ADDED" {
		t.Errorf("the second addition answered %q, want ADDED", got)
	}
}

// UC-INS-05 checks 3 and 7 through the use case and the real stores: the owner adds a colleague to
// an empty register, both rows are journalled, and the owner is still an operator afterwards.
func TestAddingTheFirstColleagueKeepsTheOwnerAnOperator(t *testing.T) {
	ctx := context.Background()
	_, app := privateInstallation(ctx, t, "hubtask_operator_roster_use_case")
	uow := postgres.NewUnitOfWork(app)
	operators := postgres.NewOperatorRepository()
	writer := adminservice.InstanceWriter{
		Settings: postgres.NewInstanceSettingRepository(), Operators: operators,
		Journal: postgres.NewInstanceJournal(pageCursors()), UnitOfWork: uow,
		Clock: clockadapter.System{}, IDs: clockadapter.NewUUIDv7(clockadapter.System{}),
	}
	owner := appshared.ActorContext{
		TenantID: shared.ID(rosterTenant), AccountID: shared.ID(rosterOwner), AccountName: "Jerome",
		Kind: appshared.ActorUser, Scopes: []string{"admin:tenants"},
	}

	if err := (adminservice.AddOperator{Writer: writer}).
		ExecuteByAddress(ctx, owner, "family", "ada@family.example"); err != nil {
		t.Fatalf("adding the first colleague: %v", err)
	}
	register, err := (adminservice.ListOperators{Writer: writer}).Execute(ctx, owner)
	if err != nil {
		t.Fatalf("the owner's next request was refused: %v", err)
	}
	if len(register) != 2 {
		t.Fatalf("the register holds %d rows, want the colleague and the owner", len(register))
	}
	for _, row := range register {
		if row.DisplayName == "" || row.WorkspaceSlug != "family" {
			t.Errorf("a row reads %+v", row)
		}
	}

	var journalled int
	if err := app.QueryRow(ctx, `SELECT count(*) FROM instance_event WHERE action = 'instance.operator_added'`).
		Scan(&journalled); err != nil {
		t.Fatalf("reading the journal: %v", err)
	}
	if journalled != 2 {
		t.Errorf("%d additions journalled, want two", journalled)
	}

	// An invited account is refused through the use case as well.
	err = (adminservice.AddOperator{Writer: writer}).ExecuteByAddress(ctx, owner, "family", "bob@family.example")
	if shared.AsError(err).DetailCode != "admin.operator_not_active" {
		t.Errorf("adding an invited account answered %v", err)
	}
}

// UC-INS-04 check 1, the "only" half: with an empty register the owner of the only workspace is the
// operator and nobody else is - not a member, not an account without a role, not a service account;
// and once a second workspace exists, an empty register names nobody, so neither owner reaches it.
func TestAnEmptyRegisterNamesTheOnlyWorkspacesOwnerAndNobodyElse(t *testing.T) {
	ctx := context.Background()
	admin, app := privateInstallation(ctx, t, "hubtask_operator_only_owner")

	if !operatorHeld(ctx, t, app, rosterOwner) {
		t.Fatal("the owner of the only workspace does not reach the installation")
	}
	for _, other := range []string{rosterMember, rosterColleague, rosterService, rosterNobody} {
		if operatorHeld(ctx, t, app, other) {
			t.Errorf("%s, not the owner, reaches the installation through an empty register", other)
		}
	}

	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + rosterOther + `', 'acme', 'Acme')`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status) VALUES
		   ('` + rosterOtherOwner + `', '` + rosterOther + `', 'USER', 'olga@acme.example', 'Olga', 'ACTIVE')`,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES
		   ('` + rosterOtherGrant + `', '` + rosterOther + `', '` + rosterOtherOwner + `', 'TENANT', 'OWNER')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding a second workspace: %v", err)
		}
	}
	for _, owner := range []string{rosterOwner, rosterOtherOwner} {
		if operatorHeld(ctx, t, app, owner) {
			t.Errorf("%s, the owner of one of two workspaces, reaches the installation through an empty register", owner)
		}
	}
}
