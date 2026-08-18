// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// db/schema.sql says of itself that the migrations are the source and it is "the readable reference
// of the same state". Until now that was a promise on trust: the file is not applied anywhere, so
// nothing noticed when it drifted.
//
// This applies it to a database of its own and compares what it produced with what the migrations
// produce. Foreign keys and unique indexes rather than the whole catalogue, because those are what a
// reader consults the file for and what ADR-0024 turns on - a reference that disagrees about which
// key carries the tenant is worse than no reference.
func TestSchemaReferenceMatchesTheMigrations(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)

	reference := applySchemaReference(ctx, t)
	defer reference.Close()

	// The one legitimate difference: goose keeps its ledger in the database it migrates, and the
	// reference file describes the schema rather than the tool that applies it.
	exceptions := map[string]string{
		"goose_db_version": "the migration ledger, created by goose and not by the schema",
	}

	const foreignKeys = `
		SELECT src.relname, src.relname || ' ' || con.conname || ' ' || pg_get_constraintdef(con.oid)
		FROM pg_constraint con
		JOIN pg_class src ON src.oid = con.conrelid
		JOIN pg_namespace n ON n.oid = src.relnamespace
		WHERE n.nspname = 'public' AND con.contype = 'f'
		ORDER BY 2`
	const uniqueIndexes = `
		SELECT tablename, tablename || ' ' || indexdef
		FROM pg_indexes
		WHERE schemaname = 'public' AND indexdef LIKE '%UNIQUE%'
		ORDER BY 2`

	for _, probe := range []struct {
		what  string
		query string
	}{
		{"foreign keys", foreignKeys},
		{"unique indexes", uniqueIndexes},
	} {
		migrated := catalogueOf(ctx, t, admin, probe.query, exceptions)
		declared := catalogueOf(ctx, t, reference, probe.query, exceptions)

		for _, missing := range difference(migrated, declared) {
			t.Errorf("%s: the migrations have it and db/schema.sql does not: %s", probe.what, missing)
		}
		for _, extra := range difference(declared, migrated) {
			t.Errorf("%s: db/schema.sql has it and the migrations do not: %s", probe.what, extra)
		}
		if len(migrated) == 0 {
			t.Errorf("%s: nothing found at all - the query is not seeing the schema", probe.what)
		}
	}
}

// applySchemaReference runs db/schema.sql against a database of its own, in the same cluster - the
// roles it grants to are cluster-wide, so they exist because the migrations created them.
func applySchemaReference(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	db := testDatabase(t)

	if _, err := adminPool(ctx, t).Exec(ctx, `DROP DATABASE IF EXISTS schema_reference`); err != nil {
		t.Fatalf("dropping the reference database: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `CREATE DATABASE schema_reference`); err != nil {
		t.Fatalf("creating the reference database: %v", err)
	}

	dsn := regexp.MustCompile(`/[^/?]+(\?|$)`).ReplaceAllString(db.adminDSN, "/schema_reference$1")
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to the reference database: %v", err)
	}

	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	sql, err := os.ReadFile(filepath.Join(root, "db", "schema.sql"))
	if err != nil {
		t.Fatalf("reading db/schema.sql: %v", err)
	}

	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		// The position is what makes a syntax error in a thousand-line file findable at all.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Position > 0 {
			at := int(pgErr.Position)
			from := max(0, at-200)
			to := min(len(sql), at+80)
			pool.Close()
			t.Fatalf("db/schema.sql does not apply: %v\n--- around position %d ---\n%s<<< HERE >>>%s",
				err, at, sql[from:at-1], sql[at-1:to])
		}
		pool.Close()
		t.Fatalf("db/schema.sql does not apply: %v", err)
	}
	return pool
}

// catalogueOf reads one probe, dropping the rows of the documented exceptions and normalising the
// whitespace so that a line break in the source is not a difference.
func catalogueOf(
	ctx context.Context, t *testing.T, pool *pgxpool.Pool, query string, exceptions map[string]string,
) []string {
	t.Helper()

	rows, err := pool.Query(ctx, query)
	if err != nil {
		t.Fatalf("catalogue query: %v", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var table, value string
		if err := rows.Scan(&table, &value); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if _, excepted := exceptions[table]; excepted {
			continue
		}
		out = append(out, strings.Join(strings.Fields(value), " "))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	sort.Strings(out)
	return out
}

func difference(from, against []string) []string {
	present := make(map[string]bool, len(against))
	for _, value := range against {
		present[value] = true
	}

	var only []string
	for _, value := range from {
		if !present[value] {
			only = append(only, value)
		}
	}
	return only
}
