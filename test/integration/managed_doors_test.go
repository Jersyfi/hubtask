// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/test/dbtest"
)

// The doors through the boundary answer on a database the operator brings (multi-tenancy.md §2.1).
// Every SECURITY DEFINER function that reads across workspaces runs as the owner, and FORCE binds
// an owner that is neither superuser nor BYPASSRLS: without the owner's own policies, sign-in found
// no workspace, the census counted nothing and the register named nobody. Each door is called the
// way the product calls it - as the application role, with no workspace set. The rows are seeded by
// the server's superuser, so that seeding asks nothing of the policies under test.
func TestTheDoorsAnswerOnADatabaseWithoutASuperuser(t *testing.T) {
	ctx := context.Background()
	const name = "hubtask_managed_doors"
	ownerDSN, appDSN := managedDatabase(ctx, t, name)
	if err := dbtest.Migrate(ctx, ownerDSN); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	seeder, err := url.Parse(dbtest.Start(t).AdminDSN)
	if err != nil {
		t.Fatalf("the shared DSN is not a URL: %v", err)
	}
	seeder.Path = "/" + name
	super, err := pgxpool.New(ctx, seeder.String())
	if err != nil {
		t.Fatalf("superuser pool: %v", err)
	}
	defer super.Close()

	const (
		tenantA  = "01936f2a-7c1e-7000-8000-000000145601"
		tenantB  = "01936f2a-7c1e-7000-8000-000000145602"
		ownerA   = "01936f2a-7c1e-7000-8000-000000145611"
		personB  = "01936f2a-7c1e-7000-8000-000000145612"
		provider = "01936f2a-7c1e-7000-8000-000000145621"
	)
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name, settings) VALUES
		 ('` + tenantA + `', 'door-a', 'Door A', '{"offered_providers": ["` + provider + `"]}'),
		 ('` + tenantB + `', 'door-b', 'Door B', '{}')`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status) VALUES
		 ('` + ownerA + `', '` + tenantA + `', 'USER', 'same@door.example', 'Ada', 'ACTIVE'),
		 ('` + personB + `', '` + tenantB + `', 'USER', 'same@door.example', 'Ben', 'ACTIVE')`,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES
		 ('01936f2a-7c1e-7000-8000-000000145631', '` + tenantA + `', '` + ownerA + `', 'TENANT', 'OWNER')`,
		`INSERT INTO identity_provider (id, tenant_id, issuer, client_id, client_secret_enc, client_secret_key_id, created_at)
		 VALUES ('` + provider + `', NULL, 'https://door.example/idp', 'hubtask', '\x00', 'k-door', now())`,
	} {
		if _, err := super.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	app, err := pgxpool.New(ctx, appDSN)
	if err != nil {
		t.Fatalf("application pool: %v", err)
	}
	defer app.Close()
	one := func(t *testing.T, query string, into any, args ...any) {
		t.Helper()
		if err := app.QueryRow(ctx, query, args...).Scan(into); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}

	t.Run("sign-in finds the workspace", func(t *testing.T) {
		var a, b *string
		one(t, `SELECT resolve_tenant('door-a')::text`, &a)
		one(t, `SELECT resolve_tenant('door-b')::text`, &b)
		if a == nil || *a != tenantA || b == nil || *b != tenantB {
			t.Errorf("resolve_tenant answered %v and %v", a, b)
		}
	})

	t.Run("a name on the receiving domain names its workspace", func(t *testing.T) {
		if _, err := super.Exec(ctx, `INSERT INTO mail_setting
			(tenant_id, enabled, name, updated_at) VALUES ('`+tenantB+`', true, 'door-b', now())`); err != nil {
			t.Fatalf("seeding the intake: %v", err)
		}
		var named *string
		one(t, `SELECT resolve_mail_workspace('DOOR-B')::text`, &named)
		if named == nil || *named != tenantB {
			t.Errorf("resolve_mail_workspace answered %v", named)
		}
		var taken bool
		one(t, `SELECT mail_workspace_name_taken('door-b', '`+tenantA+`'::uuid)`, &taken)
		if !taken {
			t.Error("another workspace's name read as free")
		}
	})

	t.Run("the operator's list and the census count both workspaces", func(t *testing.T) {
		var listed, active, accounts, workspaces int
		var unset bool
		one(t, `SELECT count(*) FROM admin_tenants()`, &listed)
		var queried int
		one(t, `SELECT count(*) FROM admin_tenants_query(NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 50)`, &queried)
		if queried != 2 {
			t.Errorf("admin_tenants_query found %d workspaces, want 2", queried)
		}
		one(t, `SELECT workspaces_active FROM instance_census()`, &active)
		one(t, `SELECT accounts_total FROM instance_census()`, &accounts)
		one(t, `SELECT workspaces FROM operator_register_state()`, &workspaces)
		one(t, `SELECT installation_unset()`, &unset)
		if listed != 2 || active != 2 || accounts != 2 || workspaces != 2 || unset {
			t.Errorf("admin_tenants %d, census active %d and accounts %d, register state %d, unset %t",
				listed, active, accounts, workspaces, unset)
		}
	})

	t.Run("a data subject is found in every workspace", func(t *testing.T) {
		var found int
		one(t, `SELECT count(*) FROM subject_tenants('SAME@door.example')`, &found)
		if found != 2 {
			t.Errorf("subject_tenants found %d workspaces, want 2", found)
		}
	})

	t.Run("the operator register reads accounts and memberships", func(t *testing.T) {
		var resolved *string
		one(t, `SELECT resolve_operator_account('door-b', 'same@door.example')::text`, &resolved)
		if resolved == nil || *resolved != personB {
			t.Errorf("resolve_operator_account answered %v", resolved)
		}
		var outcome string
		one(t, `SELECT enlist_operator($1, $1)`, &outcome, ownerA)
		if outcome != "ADDED" {
			t.Errorf("enlist_operator answered %q, want ADDED", outcome)
		}
		var isOperator, isNot bool
		var roster int
		one(t, `SELECT is_operator($1)`, &isOperator, ownerA)
		one(t, `SELECT is_operator($1)`, &isNot, personB)
		one(t, `SELECT count(*) FROM operator_roster($1) WHERE workspace_slug = 'door-a'`, &roster, ownerA)
		if !isOperator || isNot || roster != 1 {
			t.Errorf("is_operator %t for the operator and %t for another, roster %d", isOperator, isNot, roster)
		}
	})

	t.Run("a plan with a workspace on it is not removed", func(t *testing.T) {
		const plan = "01936f2a-7c1e-7000-8000-000000145641"
		for _, statement := range []string{
			`INSERT INTO plan (id, name, display_name, created_at, updated_at)
			 VALUES ('` + plan + `', 'door-family', 'Family', now(), now())`,
			`UPDATE tenant SET plan_id = '` + plan + `' WHERE id = '` + tenantB + `'`,
		} {
			if _, err := super.Exec(ctx, statement); err != nil {
				t.Fatalf("seeding: %v", err)
			}
		}
		var outcome string
		one(t, `SELECT remove_plan($1, now(), NULL)`, &outcome, plan)
		if outcome != "IN_USE" {
			t.Errorf("remove_plan answered %q for a plan a workspace is on, want IN_USE", outcome)
		}
	})

	t.Run("an offered provider is counted and moved", func(t *testing.T) {
		var offers int
		one(t, `SELECT count_provider_offers($1)`, &offers, provider)
		if offers != 1 {
			t.Errorf("count_provider_offers answered %d, want 1", offers)
		}
		if _, err := app.Exec(ctx, `SELECT move_provider_offer($1, 1)`, provider); err != nil {
			t.Fatalf("move_provider_offer: %v", err)
		}
		var moved int
		if err := super.QueryRow(ctx,
			`SELECT offered_workspaces FROM identity_provider WHERE id = $1`, provider).Scan(&moved); err != nil {
			t.Fatalf("reading the provider: %v", err)
		}
		if moved != 1 {
			t.Errorf("move_provider_offer left %d offered workspaces, want 1", moved)
		}
	})

	// Alert A-29's numbers: the installation's mail app counted across both workspaces - one
	// mailbox failing for two hours, one for ten minutes (inside the grace), one on the
	// administrator's own app and one switched off, neither of which is the installation's.
	t.Run("the installation's mail apps are counted across workspaces", func(t *testing.T) {
		mailbox := func(id, tenant, provider string, installation, enabled bool, failingFor string) string {
			failing := "NULL"
			if failingFor != "" {
				failing = "now() - interval '" + failingFor + "'"
			}
			return `INSERT INTO mailbox (id, tenant_id, protocol, address, host, port, username, folder,
				done_folder, enabled, secret_enc, secret_key_id, failing_since, auth, oauth_provider,
				oauth_token_url, oauth_installation, created_at)
				VALUES ('` + id + `', '` + tenant + `', 'IMAP', '` + id[len(id)-4:] + `@door.example',
				'imap.door.example', 993, 'u', 'INBOX', 'Hubtask', ` + boolSQL(enabled) + `, '\x00', 'k',
				` + failing + `, 'OAUTH', '` + provider + `', 'https://door.example/token', ` +
				boolSQL(installation) + `, now())`
		}
		for _, statement := range []string{
			mailbox("01936f2a-7c1e-7000-8000-000000145651", tenantA, "google", true, true, "2 hours"),
			mailbox("01936f2a-7c1e-7000-8000-000000145652", tenantB, "google", true, true, "10 minutes"),
			mailbox("01936f2a-7c1e-7000-8000-000000145653", tenantB, "google", false, true, "2 hours"),
			mailbox("01936f2a-7c1e-7000-8000-000000145654", tenantA, "microsoft", true, false, "2 hours"),
		} {
			if _, err := super.Exec(ctx, statement); err != nil {
				t.Fatalf("seeding a mailbox: %v", err)
			}
		}
		rows, err := app.Query(ctx, `SELECT provider, connected, failing FROM installation_mail_apps()`)
		if err != nil {
			t.Fatalf("installation_mail_apps: %v", err)
		}
		defer rows.Close()
		answered := map[string][2]int64{}
		for rows.Next() {
			var provider string
			var connected, failing int64
			if err := rows.Scan(&provider, &connected, &failing); err != nil {
				t.Fatalf("reading: %v", err)
			}
			answered[provider] = [2]int64{connected, failing}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("reading: %v", err)
		}
		if len(answered) != 1 || answered["google"] != [2]int64{2, 1} {
			t.Errorf("installation_mail_apps answered %v, want google connected 2, failing 1", answered)
		}
		var direct int
		one(t, `SELECT count(*) FROM mailbox`, &direct)
		if direct != 0 {
			t.Errorf("without a workspace the application read %d mailboxes", direct)
		}
	})

	// The SAML providers' certificate ends across both workspaces (identity.md §26.1): ended, within
	// a week, within the month, far off, and one switched off that nobody signs in through.
	t.Run("the SAML providers' certificate ends are counted across workspaces", func(t *testing.T) {
		saml := func(id, tenant, enabled, end string) string {
			return `INSERT INTO identity_provider (id, tenant_id, kind, issuer, client_id, client_secret_enc,
				client_secret_key_id, saml_sso_url, saml_certificate, enabled, saml_certificates_end, created_at)
				VALUES ('` + id + `', '` + tenant + `', 'SAML', 'https://idp.door.example/` + id[len(id)-4:] + `',
				'hubtask', '\x00', 'k', 'https://idp.door.example/sso', 'pem', ` + enabled + `, now() + interval '` + end + `', now())`
		}
		for _, statement := range []string{
			saml("01936f2a-7c1e-7000-8000-000000145671", tenantA, "true", "-1 day"),
			saml("01936f2a-7c1e-7000-8000-000000145672", tenantB, "true", "3 days"),
			saml("01936f2a-7c1e-7000-8000-000000145673", tenantB, "true", "20 days"),
			saml("01936f2a-7c1e-7000-8000-000000145674", tenantA, "true", "200 days"),
			saml("01936f2a-7c1e-7000-8000-000000145675", tenantA, "false", "-1 day"),
		} {
			if _, err := super.Exec(ctx, statement); err != nil {
				t.Fatalf("seeding a SAML provider: %v", err)
			}
		}
		var providers, within30, within7, ended int64
		if err := app.QueryRow(ctx, `SELECT providers, within_30_days, within_7_days, ended FROM saml_certificates_ending()`).
			Scan(&providers, &within30, &within7, &ended); err != nil {
			t.Fatalf("saml_certificates_ending: %v", err)
		}
		if providers != 4 || within30 != 2 || within7 != 1 || ended != 1 {
			t.Errorf("saml_certificates_ending answered %d, %d, %d, %d; want 4, 2, 1, 1", providers, within30, within7, ended)
		}
	})

	// The health report's backlogs (P-11): the outbox counted across workspaces through its door,
	// and a dead-lettered job among the figures, as the application role reads them.
	t.Run("the health report's backlogs count every workspace and the dead letters", func(t *testing.T) {
		event := func(id, tenant, dispatched string) string {
			return `INSERT INTO outbox_event (id, tenant_id, event_type, payload, actor_type, occurred_at, dispatched_at)
				VALUES ('` + id + `', '` + tenant + `', 'de.hubtask.test.v1', '{}', 'SYSTEM',
				now() - interval '30 seconds', ` + dispatched + `)`
		}
		for _, statement := range []string{
			event("01936f2a-7c1e-7000-8000-000000145671", tenantA, "NULL"),
			event("01936f2a-7c1e-7000-8000-000000145672", tenantB, "NULL"),
			event("01936f2a-7c1e-7000-8000-000000145673", tenantB, "now()"),
			`INSERT INTO job (id, tenant_id, kind, state, attempts, max_attempts, finished_at, last_error)
			 VALUES ('01936f2a-7c1e-7000-8000-000000145681', '` + tenantA + `', 'backup.run', 'DEAD_LETTER',
			 3, 3, now(), 'queue.lease_expired_exhausted')`,
			// Held uploads: one workspace's pass waiting two hours, another's dead-lettered.
			`INSERT INTO job (id, tenant_id, kind, state, attempts, max_attempts, dedupe_key, created_at)
			 VALUES ('01936f2a-7c1e-7000-8000-000000145682', '` + tenantA + `', 'media.scan', 'PENDING',
			 0, 8, 'media.scan:` + tenantA + `', now() - interval '2 hours')`,
			`INSERT INTO job (id, tenant_id, kind, state, attempts, max_attempts, dedupe_key, finished_at, last_error)
			 VALUES ('01936f2a-7c1e-7000-8000-000000145683', '` + tenantB + `', 'media.scan', 'DEAD_LETTER',
			 8, 8, 'media.scan:` + tenantB + `', now(), 'postgres.query_failed')`,
		} {
			if _, err := super.Exec(ctx, statement); err != nil {
				t.Fatalf("seeding: %v", err)
			}
		}

		read := postgres.Backlogs{UnitOfWork: postgres.NewUnitOfWork(app), Clock: clockadapter.System{}}.Read
		got, err := read(ctx)
		if err != nil {
			t.Fatalf("counting the backlogs: %v", err)
		}
		if got.OutboxPending != 2 || got.OutboxLagSeconds < 25 {
			t.Errorf("outbox pending %d, lag %.1fs; want 2 across both workspaces, about 30s",
				got.OutboxPending, got.OutboxLagSeconds)
		}
		if got.DeadLetterTotal != 2 {
			t.Errorf("dead letters = %d, want the two", got.DeadLetterTotal)
		}
		if got.HeldUploadWorkspaces != 2 || got.HeldUploadOldestSeconds < 7100 {
			t.Errorf("held uploads in %d workspaces, oldest %.0fs; want 2, about 7200s",
				got.HeldUploadWorkspaces, got.HeldUploadOldestSeconds)
		}
		var direct int
		one(t, `SELECT count(*) FROM outbox_event`, &direct)
		if direct != 0 {
			t.Errorf("without a workspace the application read %d outbox events", direct)
		}
	})

	// The backup warning (UC-BAK-01 check 11): workspaces without a target or a recent backup are
	// counted across workspaces through the door (0368), as the application role reads them.
	t.Run("the backup gaps count every workspace", func(t *testing.T) {
		gaps := func() (withoutTarget, withoutRecent int64) {
			t.Helper()
			if err := app.QueryRow(ctx, `SELECT without_target, without_recent FROM installation_backup_gaps(7)`).
				Scan(&withoutTarget, &withoutRecent); err != nil {
				t.Fatalf("installation_backup_gaps: %v", err)
			}
			return withoutTarget, withoutRecent
		}
		targetBefore, recentBefore := gaps()
		if targetBefore < 2 || recentBefore < 2 {
			t.Fatalf("before any backup %d without a target, %d without a recent backup; want both workspaces", targetBefore, recentBefore)
		}
		for _, statement := range []string{
			`INSERT INTO backup_target (id, tenant_id, name, kind) VALUES
			 ('01936f2a-7c1e-7000-8000-000000145691', '` + tenantA + `', 'Door A folder', 'LOCAL')`,
			`INSERT INTO backup_run (id, target_id, tenant_id, trigger, mode, status, finished_at) VALUES
			 ('01936f2a-7c1e-7000-8000-000000145692', '01936f2a-7c1e-7000-8000-000000145691', '` + tenantA + `',
			  'MANUAL', 'FULL', 'SUCCEEDED', now() - interval '2 days'),
			 ('01936f2a-7c1e-7000-8000-000000145693', '01936f2a-7c1e-7000-8000-000000145691', '` + tenantA + `',
			  'MANUAL', 'FULL', 'SUCCEEDED', now() - interval '9 days')`,
		} {
			if _, err := super.Exec(ctx, statement); err != nil {
				t.Fatalf("seeding: %v", err)
			}
		}
		if target, recent := gaps(); target != targetBefore-1 || recent != recentBefore-1 {
			t.Errorf("after A's backup %d without a target, %d without a recent backup; want %d and %d",
				target, recent, targetBefore-1, recentBefore-1)
		}
		var direct int
		one(t, `SELECT count(*) FROM backup_target`, &direct)
		if direct != 0 {
			t.Errorf("without a workspace the application read %d backup targets", direct)
		}
	})

	// The owner's policies name the owner and nobody else: the application, reading the same tables
	// directly with no workspace set, still sees nothing.
	t.Run("the application role is still bounded", func(t *testing.T) {
		var tenants, accounts, memberships, providers int
		one(t, `SELECT count(*) FROM tenant`, &tenants)
		one(t, `SELECT count(*) FROM account`, &accounts)
		one(t, `SELECT count(*) FROM membership`, &memberships)
		one(t, `SELECT count(*) FROM identity_provider WHERE tenant_id IS NOT NULL`, &providers)
		if tenants+accounts+memberships+providers != 0 {
			t.Errorf("without a workspace the application read %d tenants, %d accounts, %d memberships, %d providers",
				tenants, accounts, memberships, providers)
		}
	})
}

// boolSQL is a boolean as an SQL literal, for a seeding statement.
func boolSQL(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
