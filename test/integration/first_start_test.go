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
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
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

// UC-INS-01 checks 3, 4 and 6 against the real stores: one setup writes the workspace, its active
// owner with a password that verifies, the owner's register entry and the journal entry, spends the
// code, and answers a session that works; a second setup is refused.
func TestTheSetupMakesTheFirstWorkspaceAndItsOperatorInOneStep(t *testing.T) {
	ctx := context.Background()
	admin, app := emptyInstallation(ctx, t, "hubtask_first_start_setup")
	uow := postgres.NewUnitOfWork(app)

	writer, signIn, _ := realWriter(ctx, t)
	writer.UnitOfWork = uow
	writer.Multi = false
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	hybrid, err := clockadapter.NewHybridClock(clockadapter.System{}, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}
	codes := postgres.NewSetupCodeRepository(security.NewSetupCodeHasher(secret.New(installationSecret)))
	setup := identityservice.SetUpInstallation{
		Writer: writer, Codes: codes, Owners: signIn, Operators: postgres.NewOperatorRepository(),
		Journal: postgres.NewInstanceJournal(pageCursors()), Text: text.Composing{},
		Tenancy: env.TenancySingle,
		Seeder: adminservice.ProvisionTenant{
			Tenants: postgres.NewAdminTenantRepository(), Accounts: postgres.NewAccountRepository(),
			Grants: postgres.NewMembershipGrantRepository(pageCursors()), Containers: containerRepo(),
			Buckets: postgres.NewBucketRepository(), Labels: postgres.NewLabelRepository(),
			Events: postgres.NewOutbox(jobQueue(t)), Changes: postgres.NewChangeLog(),
			Audit: postgres.NewAuditSink(ids), Renderer: renderer, Text: text.Composing{},
			UnitOfWork: uow, Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
			Entropy: clockadapter.CryptoRandom{}, Tenancy: env.TenancySingle,
		},
	}

	code, err := identity.NewSetupCode([]byte{9, 8, 7, 6, 5, 4, 3, 2})
	if err != nil {
		t.Fatalf("minting a code: %v", err)
	}
	now := time.Now().UTC()
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		_, err := codes.Issue(ctx, ids.NewID(), code, now, now.Add(identity.SetupCodeLifetime))
		return err
	}); err != nil {
		t.Fatalf("issuing: %v", err)
	}

	command := identityservice.SetUpInstallationCommand{
		Code: code.Display(), DisplayName: "Family Winkel", OwnerEmail: "jerome@example.eu",
		OwnerDisplayName: "Jérôme", Password: secret.New("seven blue lanterns above the harbour"),
	}
	if _, err := setup.Execute(ctx, identityservice.SetUpInstallationCommand{
		Code: "AAAA-BBBB-CCCC", DisplayName: command.DisplayName, OwnerEmail: command.OwnerEmail,
		Password: command.Password,
	}); shared.AsError(err).DetailCode != "setup.code_wrong" {
		t.Fatalf("a wrong code answered %v", err)
	}

	pair, err := setup.Execute(ctx, command)
	if err != nil {
		t.Fatalf("the setup was refused: %v", err)
	}
	if pair.AccessToken.Reveal() == "" {
		t.Error("the setup answered no access token")
	}

	var (
		slug, status, kind string
		passwordHash       *string
		operators, events  int
		codesLeft          int
	)
	if err := admin.QueryRow(ctx, `
		SELECT t.slug, a.status::text, a.kind::text, a.password_hash,
		       (SELECT count(*) FROM operator o WHERE o.account_id = a.id AND o.added_by IS NULL),
		       (SELECT count(*) FROM instance_event e WHERE e.action = 'instance.set_up' AND e.tenant_id = t.id),
		       (SELECT count(*) FROM instance_setup_code)
		FROM tenant t JOIN account a ON a.tenant_id = t.id`).Scan(
		&slug, &status, &kind, &passwordHash, &operators, &events, &codesLeft); err != nil {
		t.Fatalf("reading what the setup wrote: %v", err)
	}
	if slug != "family-winkel" || status != "ACTIVE" || kind != "USER" {
		t.Errorf("workspace %q, owner %s %s", slug, status, kind)
	}
	if passwordHash == nil {
		t.Fatal("the owner has no password")
	}
	if same, err := writer.Passwords.Verify(*passwordHash, secret.New("seven blue lanterns above the harbour")); err != nil || !same {
		t.Errorf("the stored password does not verify: %v", err)
	}
	if operators != 1 || events != 1 || codesLeft != 0 {
		t.Errorf("register entries %d, journal entries %d, codes left %d; want 1, 1, 0", operators, events, codesLeft)
	}
	var hubs int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM container WHERE type = 'HUB'`).Scan(&hubs); err != nil || hubs != 1 {
		t.Errorf("the seeded hub: %d, %v", hubs, err)
	}

	if _, err := setup.Execute(ctx, command); shared.AsError(err).DetailCode != "setup.already_done" {
		t.Errorf("a second setup answered %v", err)
	}
}

// UC-INS-02 checks 4 and 5 and UC-INS-03 check 4 against the register's real doors: an entry whose
// account exists is added and journalled, one whose account does not waits and is named in the
// health report, a bare address finds the one workspace's account, the environment removes nobody,
// and two workspaces with an empty register are reported with the command.
func TestTheEnvironmentsOperatorsReachTheRealRegister(t *testing.T) {
	ctx := context.Background()
	admin, app := emptyInstallation(ctx, t, "hubtask_first_start_operators")
	uow := postgres.NewUnitOfWork(app)
	operators := postgres.NewOperatorRepository()
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES ('019364de-0000-7000-8000-00000005e761', 'acme', 'Acme')`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status) VALUES
		   ('019364de-0000-7000-8000-00000005e771', '019364de-0000-7000-8000-00000005e761', 'USER', 'ada@acme.example', 'Ada', 'ACTIVE')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	applying := adminservice.OperatorsFromEnvironment{
		Entries: []env.OperatorEntry{
			{Workspace: "acme", Email: "ADA@acme.example"},
			{Workspace: "acme", Email: "bob@acme.example"},
		},
		Operators: operators, Single: operators, Journal: postgres.NewInstanceJournal(pageCursors()),
		UnitOfWork: uow, Clock: clockadapter.System{}, IDs: ids,
	}

	waiting, err := applying.Apply(ctx)
	if err != nil {
		t.Fatalf("applying: %v", err)
	}
	if len(waiting) != 1 || waiting[0].Email != "bob@acme.example" {
		t.Errorf("waiting = %+v, want Bob", waiting)
	}
	var registered, journalled int
	if err := admin.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM operator WHERE account_id = '019364de-0000-7000-8000-00000005e771'),
		(SELECT count(*) FROM instance_event WHERE action = 'instance.operator_added' AND actor_label = 'HUBTASK_OPERATORS')`).
		Scan(&registered, &journalled); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if registered != 1 || journalled != 1 {
		t.Errorf("registered %d, journalled %d; want 1, 1", registered, journalled)
	}

	// A bare address finds the one workspace's account.
	var found shared.ID
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		var err error
		found, err = operators.ResolveSingle(ctx, "ada@acme.example")
		return err
	}); err != nil || found.String() != "019364de-0000-7000-8000-00000005e771" {
		t.Errorf("a bare address found %q, %v", found, err)
	}

	// The health report names Bob; with a second workspace and the register emptied, it names the
	// command too.
	warnings := adminservice.RegisterWarnings{State: operators, Environment: applying, UnitOfWork: uow}
	codes := map[string]string{}
	for _, warning := range warnings.Warnings(ctx) {
		codes[warning.Code] = warning.Params["entry"] + warning.Params["command"]
	}
	if codes["config.operator_pending"] != "acme:bob@acme.example" {
		t.Errorf("warnings = %v", codes)
	}
	if _, named := codes["config.operator_register_empty"]; named {
		t.Error("a register with an operator was reported empty")
	}
	for _, statement := range []string{
		`DELETE FROM operator`,
		`INSERT INTO tenant (id, slug, display_name) VALUES ('019364de-0000-7000-8000-00000005e762', 'bravo', 'Bravo')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	codes = map[string]string{}
	for _, warning := range warnings.Warnings(ctx) {
		codes[warning.Code] = warning.Params["command"]
	}
	if codes["config.operator_register_empty"] != adminservice.RestoreCommand {
		t.Errorf("two workspaces and an empty register: %v", codes)
	}
}
