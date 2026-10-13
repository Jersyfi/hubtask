// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/presentation/rest"
	"github.com/Jersyfi/hubtask/test/dbtest"
)

// Migration 0141 empties the replay bodies an earlier release kept for answers that carried a
// credential shown once, and leaves every row, its status and every other body as it was: run
// against a database populated at 0140, as an installation's would be.
func TestTheMigrationEmptiesTheStoredCredentials(t *testing.T) {
	ctx := context.Background()
	pool, dsn := databaseMigratedTo(ctx, t, "shown_once_migration", 140)

	rows := []struct {
		endpoint, body string
		emptied        bool
	}{
		{"POST /api/v1/auth/tokens", `{"id":"a","token":"hbt_pat_secret"}`, true},
		// Provisioning keeps its body: a repeat answers the same workspace (rest.ShownOnceKept).
		{"POST /api/v1/admin/tenants", `{"id":"t","owner_redemption_token":"secret"}`, false},
		{"POST /api/v1/auth/mfa/recovery:regenerate", `{"recovery_codes":["a"]}`, true},
		{"POST /api/v1/accounts:invite", `{"id":"p","invitation_link":"https://x/secret"}`, true},
		{"POST /api/v1/accounts:invite", `{"id":"q"}`, false},
		{"POST /api/v1/items", `{"id":"i","title":"kept"}`, false},
	}
	// Every route the code marks today is one the migration empties - the table may grow after it,
	// but on the day it was written the two were the same.
	for route := range rest.ShownOnceRoutes {
		rows = append(rows, struct {
			endpoint, body string
			emptied        bool
		}{route, `{"secret":"x"}`, true})
	}
	for i, row := range rows {
		if _, err := pool.Exec(ctx, `
			INSERT INTO idempotency_key (tenant_id, key, endpoint, request_hash, response_code, response_body)
			VALUES (gen_random_uuid(), $1, $2, '\x00', 201, $3::jsonb)`,
			freshID(t).String(), row.endpoint, row.body); err != nil {
			t.Fatalf("seeding row %d: %v", i, err)
		}
	}

	if err := dbtest.MigrateTo(ctx, dsn, 141); err != nil {
		t.Fatalf("migrating to 141: %v", err)
	}

	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_key WHERE response_code = 201`).Scan(&total); err != nil ||
		total != len(rows) {
		t.Errorf("%d rows keep their status, want %d (%v)", total, len(rows), err)
	}
	for _, row := range rows {
		var kept int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM idempotency_key
			WHERE endpoint = $1 AND response_body = $2::jsonb`, row.endpoint, row.body).Scan(&kept); err != nil {
			t.Fatalf("reading %s: %v", row.endpoint, err)
		}
		if row.emptied && kept != 0 {
			t.Errorf("%s still keeps %s", row.endpoint, row.body)
		}
		if !row.emptied && kept != 1 {
			t.Errorf("%s lost %s", row.endpoint, row.body)
		}
	}
}
