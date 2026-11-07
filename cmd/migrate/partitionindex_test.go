// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/Jersyfi/hubtask/db"
)

func TestAPartitionIndexNameStaysWithinPostgresLimit(t *testing.T) {
	short := partitionIndexName("audit_log_2026_08", "audit_request_idx")
	if short != "audit_log_2026_08_audit_request_idx" {
		t.Errorf("short name = %q", short)
	}
	long := partitionIndexName(strings.Repeat("p", 40), strings.Repeat("i", 40))
	if len(long) > 63 {
		t.Errorf("long name has %d bytes", len(long))
	}
	if long == partitionIndexName(strings.Repeat("p", 40), strings.Repeat("i", 41)) {
		t.Error("two different long names collide")
	}
}

func TestAPartitionIndexBuildIsConcurrentOnThePartition(t *testing.T) {
	build, err := partitionIndexBuild(missingPartitionIndex{
		parentIndex: "audit_request_idx", partition: "audit_log_2026_09",
		definition: `CREATE INDEX audit_request_idx ON ONLY public.audit_log USING btree (tenant_id, ((context ->> 'RequestID'::text)), occurred_at DESC)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE INDEX CONCURRENTLY IF NOT EXISTS "audit_log_2026_09_audit_request_idx" ON "public"."audit_log_2026_09" USING btree (tenant_id, ((context ->> 'RequestID'::text)), occurred_at DESC)`
	if build != want {
		t.Errorf("build =\n%s\nwant\n%s", build, want)
	}
}

// The request id's index reaches every partition the database already holds, so the parent's
// index is valid and support's lookup of one request reads an index, not the whole trail; a
// partition created after it takes it too.
func TestTheRequestIndexReachesEveryAuditPartition(t *testing.T) {
	if testing.Short() {
		t.Skip("this test starts a database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	dsn := startDatabase(ctx, t)
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(db.Migrations)
	if err := goose.UpToContext(ctx, pool, "migrations", 365); err != nil {
		t.Fatalf("bringing the database to 0365: %v", err)
	}
	// A month the partition job created before the index existed.
	if _, err := pool.ExecContext(ctx, `SELECT ensure_audit_partition('2026-10-01')`); err != nil {
		t.Fatal(err)
	}
	if err := up(ctx, pool, pool, migrationFiles(), time.Millisecond); err != nil {
		t.Fatalf("the run: %v", err)
	}
	if _, err := pool.ExecContext(ctx, `SELECT ensure_audit_partition('2027-03-01')`); err != nil {
		t.Fatal(err)
	}

	var valid bool
	if err := pool.QueryRowContext(ctx, `
		SELECT i.indisvalid FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		WHERE c.relname = 'audit_request_idx'`).Scan(&valid); err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Error("audit_request_idx is still invalid after the run")
	}
	rows, err := pool.QueryContext(ctx, `
		SELECT part.relname,
		       EXISTS (SELECT 1 FROM pg_inherits ci JOIN pg_index c ON c.indexrelid = ci.inhrelid
		               WHERE ci.inhparent = 'audit_request_idx'::regclass AND c.indrelid = part.oid AND c.indisvalid)
		FROM pg_inherits p JOIN pg_class part ON part.oid = p.inhrelid
		WHERE p.inhparent = 'audit_log'::regclass`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	partitions := 0
	for rows.Next() {
		var name string
		var reached bool
		if err := rows.Scan(&name, &reached); err != nil {
			t.Fatal(err)
		}
		partitions++
		if !reached {
			t.Errorf("partition %s holds no valid share of audit_request_idx", name)
		}
	}
	if partitions < 4 {
		t.Errorf("%d partitions, want the initial two and the two months", partitions)
	}
}
