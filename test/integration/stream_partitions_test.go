// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	streamsrepo "github.com/Jersyfi/hubtask/core/application/repository/streams"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/test/dbtest"
)

// The stream partitions of H-09, against the real boundary: the conversion rehearsed over
// existing data with the old binary's statements, the duty creating and repairing, the default
// catching, and the retention dropping - with the evidence's counts.

// The rolling-update rehearsal: a database migrated to the state BEFORE the conversion is
// populated the way a running installation is, the conversion runs, and then the statements the
// OLD binary would still be issuing - plain inserts, reads and updates by id - are executed as
// raw SQL against the new schema. deployment.md §5's constraint, proved rather than argued.
func TestTheConversionCarriesExistingDataAndTheOldStatements(t *testing.T) {
	ctx := context.Background()
	pool, dsn := databaseMigratedTo(ctx, t, "stream_rehearsal", 67)

	seed := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, sql)
		}
	}
	tenant := "01936f2a-7c1e-7000-8000-00000000d001"
	seed(`INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'rehearsal', 'Rehearsal')`, tenant)
	seed(`INSERT INTO account (id, tenant_id, kind, display_name, status)
	      VALUES ('01936f2a-7c1e-7000-8000-00000000d002', $1, 'USER', 'R', 'ACTIVE')`, tenant)
	seed(`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
	      VALUES ('01936f2a-7c1e-7000-8000-00000000d003', $1, 'HUB', 'R hub', 'a0',
	              '01936f2a-7c1e-7000-8000-00000000d002')`, tenant)
	seed(`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
	      VALUES ('01936f2a-7c1e-7000-8000-00000000d004', $1, 'COLLECTION',
	              '01936f2a-7c1e-7000-8000-00000000d003', 'R collection', 'a0',
	              '01936f2a-7c1e-7000-8000-00000000d002')`, tenant)
	seed(`INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key, created_by)
	      VALUES ('01936f2a-7c1e-7000-8000-00000000d005', $1,
	              '01936f2a-7c1e-7000-8000-00000000d004', 'TASK',
	              '/01936f2a-7c1e-7000-8000-00000000d005/', 1, 'R task', 'a0',
	              '01936f2a-7c1e-7000-8000-00000000d002')`, tenant)
	seed(`INSERT INTO automation_rule (id, tenant_id, scope_type, name, run_as, trigger, actions, created_by)
	      VALUES ('01936f2a-7c1e-7000-8000-00000000d006', $1, 'TENANT', 'R rule',
	              '01936f2a-7c1e-7000-8000-00000000d002', '{"kind":"EVENT"}'::jsonb, '[]'::jsonb,
	              '01936f2a-7c1e-7000-8000-00000000d002')`, tenant)
	// The three streams, populated as a running system leaves them.
	seed(`INSERT INTO activity_entry (id, tenant_id, item_id, actor_type, verb, occurred_at)
	      VALUES ('01936f2a-7c1e-7000-8000-00000000d011', $1,
	              '01936f2a-7c1e-7000-8000-00000000d005', 'USER', 'item.created', now() - interval '3 days')`,
		tenant)
	seed(`INSERT INTO outbox_event (id, tenant_id, event_type, payload, actor_type, occurred_at)
	      VALUES ('01936f2a-7c1e-7000-8000-00000000d012', $1, 'de.hubtask.test.v1', '{}'::jsonb,
	              'USER', now() - interval '3 days')`, tenant)
	seed(`INSERT INTO rule_run (id, tenant_id, rule_id, status, started_at)
	      VALUES ('01936f2a-7c1e-7000-8000-00000000d013', $1,
	              '01936f2a-7c1e-7000-8000-00000000d006', 'RUNNING', now() - interval '3 days')`, tenant)

	// The history tables must not be rewritten: the attach is a metadata act, and the filenode
	// is the witness (the search migration's own probe).
	filenodes := map[string]uint32{}
	for _, table := range streamsrepo.Tables() {
		var node uint32
		if err := pool.QueryRow(ctx, `SELECT pg_relation_filenode($1::regclass)`, table).Scan(&node); err != nil {
			t.Fatalf("reading %s's filenode: %v", table, err)
		}
		filenodes[table] = node
	}

	if err := dbtest.Migrate(ctx, dsn); err != nil {
		t.Fatalf("the conversion failed over existing data: %v", err)
	}

	for _, table := range streamsrepo.Tables() {
		var node uint32
		if err := pool.QueryRow(ctx,
			`SELECT pg_relation_filenode(($1 || '_history')::regclass)`, table).Scan(&node); err != nil {
			t.Fatalf("reading %s_history's filenode: %v", table, err)
		}
		if node != filenodes[table] {
			t.Errorf("%s was rewritten during the conversion (filenode %d became %d)",
				table, filenodes[table], node)
		}
		var count int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE tenant_id = $1`, tenant).Scan(&count); err != nil {
			t.Fatalf("counting %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("%s lost its row in the conversion (%d)", table, count)
		}
	}

	// The old binary's statements, verbatim shapes: insert with an explicit timestamp, read and
	// update by id alone. All still work through the partitioned parents.
	seed(`INSERT INTO activity_entry (id, tenant_id, item_id, actor_type, verb, occurred_at)
	      VALUES ('01936f2a-7c1e-7000-8000-00000000d021', $1,
	              '01936f2a-7c1e-7000-8000-00000000d005', 'USER', 'item.completed', now())`, tenant)
	seed(`INSERT INTO outbox_event (id, tenant_id, event_type, payload, actor_type, occurred_at)
	      VALUES ('01936f2a-7c1e-7000-8000-00000000d022', $1, 'de.hubtask.test.v1', '{}'::jsonb,
	              'USER', now())`, tenant)
	seed(`UPDATE outbox_event SET dispatched_at = now()
	      WHERE id = ANY(ARRAY['01936f2a-7c1e-7000-8000-00000000d022']::uuid[])`)
	seed(`UPDATE rule_run SET status = 'SUCCEEDED', finished_at = now()
	      WHERE id = '01936f2a-7c1e-7000-8000-00000000d013'`)
	var found int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_event WHERE id = '01936f2a-7c1e-7000-8000-00000000d022'
		   AND dispatched_at IS NOT NULL`).Scan(&found); err != nil || found != 1 {
		t.Errorf("the old binary's update by id did not land (%d, %v)", found, err)
	}

	// The default partition catches an out-of-range row rather than erroring: a timestamp far
	// past every created month.
	seed(`INSERT INTO outbox_event (id, tenant_id, event_type, payload, actor_type, occurred_at)
	      VALUES ('01936f2a-7c1e-7000-8000-00000000d023', $1, 'de.hubtask.test.v1', '{}'::jsonb,
	              'USER', now() + interval '20 years')`, tenant)
	var caught int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_event_default`).Scan(&caught); err != nil || caught != 1 {
		t.Errorf("the default partition caught %d rows (%v), want the far-future one", caught, err)
	}
}

// The duty, E-09's precedent tests over the generalised function: create with policy and full
// grant, repair what an operator broke, and answer the empty name for a month the default
// already holds.
func TestTheStreamDutyCreatesAndRepairs(t *testing.T) {
	ctx := context.Background()
	dbtest.Start(t)
	admin := adminPool(ctx, t)
	partitions := postgres.NewStreamPartitionRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	future := time.Now().UTC().AddDate(0, 7, 0)
	var name string
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		ensured, err := partitions.Ensure(ctx, "outbox_event", future)
		name = ensured
		return err
	}); err != nil {
		t.Fatalf("ensuring: %v", err)
	}
	if !strings.HasPrefix(name, "outbox_event_") {
		t.Fatalf("the duty answered %q", name)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP TABLE IF EXISTS `+name)
	})

	// Break it the way an operator with too much power would, then repair.
	if _, err := admin.Exec(ctx, `DROP POLICY tenant_isolation ON `+name); err != nil {
		t.Fatalf("breaking the policy: %v", err)
	}
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		_, err := partitions.Ensure(ctx, "outbox_event", future)
		return err
	}); err != nil {
		t.Fatalf("repairing: %v", err)
	}
	var policies int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM pg_policies WHERE tablename = $1 AND policyname = 'tenant_isolation'`,
		name).Scan(&policies); err != nil || policies != 1 {
		t.Errorf("the repair left %d policies (%v)", policies, err)
	}

	// An unknown table refuses - the closed set holds.
	err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		_, err := partitions.Ensure(ctx, "tenant", future)
		return err
	})
	if err == nil {
		t.Error("the duty accepted a table outside the closed set")
	}
}

// The retention half: an aged-out month falls whole, with its row count; a tenant's longer
// configured retention holds it back; the default partition never falls.
//
// In its own database: pre-conversion months live inside the open-ended history partition, so
// the first droppable month only exists once the history itself has aged out - this test drops
// the (empty, fresh-install) history first, exactly what a converted installation experiences a
// retention period after go-live.
func TestAnAgedOutMonthFallsWholeAndConfiguredRetentionHoldsIt(t *testing.T) {
	ctx := context.Background()
	pool, _ := databaseMigratedTo(ctx, t, "stream_aged", 68)

	tenant := "01936f2a-7c1e-7000-8000-00000000d101"
	if _, err := pool.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'aged', 'Aged')`, tenant); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	// The empty history out of the way, an old month can exist as its own partition.
	if _, err := pool.Exec(ctx,
		`ALTER TABLE outbox_event DETACH PARTITION outbox_event_history`); err != nil {
		t.Fatalf("detaching the history: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP TABLE outbox_event_history`); err != nil {
		t.Fatalf("dropping the history: %v", err)
	}

	old := time.Now().UTC().AddDate(0, -3, 0)
	var name string
	if err := pool.QueryRow(ctx,
		`SELECT ensure_stream_partition('outbox_event', date_trunc('month', $1::timestamptz)::date)`,
		old).Scan(&name); err != nil || name == "" {
		t.Fatalf("ensuring the old month (%q, %v)", name, err)
	}
	inMonth := time.Date(old.Year(), old.Month(), 15, 12, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_event (id, tenant_id, event_type, payload, actor_type, occurred_at, dispatched_at)
		VALUES ('01936f2a-7c1e-7000-8000-00000000d111', $1, 'de.hubtask.test.v1', '{}'::jsonb, 'USER', $2, $2),
		       ('01936f2a-7c1e-7000-8000-00000000d112', $1, 'de.hubtask.test.v1', '{}'::jsonb, 'USER', $2, $2)`,
		tenant, inMonth); err != nil {
		t.Fatalf("seeding the aged rows: %v", err)
	}

	// A tenant configured to keep events for a year holds the month back for everybody.
	if _, err := pool.Exec(ctx, `
		INSERT INTO retention_policy (tenant_id, data_kind, retain_days)
		VALUES ($1, 'OUTBOX_EVENT', 365)`, tenant); err != nil {
		t.Fatalf("configuring the long retention: %v", err)
	}
	rows, err := pool.Query(ctx, `SELECT dropped, rows_removed FROM drop_stream_partition('outbox_event', 7)`)
	if err != nil {
		t.Fatalf("dropping under the long retention: %v", err)
	}
	held := 0
	for rows.Next() {
		held++
	}
	rows.Close()
	if held != 0 {
		t.Fatalf("a month fell although a tenant keeps events for a year")
	}

	// The configuration gone, the month falls whole - counted for the evidence.
	if _, err := pool.Exec(ctx,
		`DELETE FROM retention_policy WHERE tenant_id = $1 AND data_kind = 'OUTBOX_EVENT'`,
		tenant); err != nil {
		t.Fatalf("clearing the retention: %v", err)
	}
	var fellName string
	var fellRows int64
	if err := pool.QueryRow(ctx,
		`SELECT dropped, rows_removed FROM drop_stream_partition('outbox_event', 7)`,
	).Scan(&fellName, &fellRows); err != nil {
		t.Fatalf("dropping: %v", err)
	}
	if fellName != name || fellRows != 2 {
		t.Fatalf("dropped %s with %d rows, want %s with 2", fellName, fellRows, name)
	}

	// Gone from the catalogue, default still standing.
	var remains int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_class WHERE relname = $1`, name).Scan(&remains); err != nil || remains != 0 {
		t.Errorf("the partition still exists (%d, %v)", remains, err)
	}
	var defaults int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_class WHERE relname = 'outbox_event_default'`).Scan(&defaults); err != nil || defaults != 1 {
		t.Errorf("the default partition went with it (%d, %v)", defaults, err)
	}
}
