// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-14 checks 4 and 8 (multi-tenancy.md §4.3, migration 0183): a workspace's entries, stored
// bytes and people are kept per UTC day as the day's highest and closing count, where the count
// changes; a write with no workspace in scope, or in a workspace whose purge has begun, records
// nothing; the control plane reads every workspace's counts and nothing from inside one; counts
// older than 13 months are swept inside workspaces and in the installation's ledger.
func TestEachDayKeepsItsHighestAndClosingCount(t *testing.T) {
	ctx := context.Background()
	app, admin := appPool(ctx, t), adminPool(ctx, t)
	workspace := seedWorkspace(ctx, t, "metered")
	person, hub, collection := freshID(t), freshID(t), freshID(t)

	inScope := func(statements ...string) {
		t.Helper()
		inRawTenant(ctx, t, app, workspace.String(), true, func(tx pgx.Tx) {
			for _, statement := range statements {
				if _, err := tx.Exec(ctx, statement); err != nil {
					t.Fatalf("%s: %v", statement, err)
				}
			}
		})
	}
	today := func(metric string) (int64, int64) {
		t.Helper()
		var value, closing int64
		err := admin.QueryRow(ctx, `SELECT value, coalesce(closing, -1) FROM usage_record
			WHERE tenant_id = $1 AND metric = $2 AND period = (now() AT TIME ZONE 'UTC')::date`,
			workspace.String(), metric).Scan(&value, &closing)
		if errors.Is(err, pgx.ErrNoRows) {
			return -1, -1
		}
		if err != nil {
			t.Fatalf("reading %s: %v", metric, err)
		}
		return value, closing
	}
	item := func(id shared.ID) string {
		return fmt.Sprintf(`('%s', '%s', '%s', 'TASK', '/%s/', 1, 'Counted', 'a0', '%s')`,
			id, workspace, collection, id, person)
	}

	inScope(fmt.Sprintf(`INSERT INTO account (id, tenant_id, kind, display_name, status)
		VALUES ('%s', '%s', 'USER', 'Counted', 'ACTIVE')`, person, workspace),
		fmt.Sprintf(`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		VALUES ('%s', '%s', 'HUB', 'Hub', 'a0', '%s')`, hub, workspace, person),
		fmt.Sprintf(`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		VALUES ('%s', '%s', 'COLLECTION', '%s', 'Collection', 'a0', '%s')`, collection, workspace, hub, person))
	if value, closing := today("accounts"); value != 1 || closing != 1 {
		t.Errorf("people read %d / %d, want 1 / 1", value, closing)
	}

	a, b, c := freshID(t), freshID(t), freshID(t)
	inScope(`INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key, created_by)
		VALUES ` + item(a) + `, ` + item(b) + `, ` + item(c))
	inScope(fmt.Sprintf(`DELETE FROM work_item WHERE id IN ('%s', '%s')`, a, b))
	if value, closing := today("items"); value != 3 || closing != 1 {
		t.Errorf("entries read %d / %d, want the day's highest 3 and closing 1", value, closing)
	}

	m1, m2 := freshID(t), freshID(t)
	inScope(fmt.Sprintf(`INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, usage, status)
		VALUES ('%s', '%s', 'media/u/a', 'text/plain', 11, 'ATTACHMENT', 'READY'),
		       ('%s', '%s', 'media/u/b', 'text/plain', 22, 'ATTACHMENT', 'READY')`, m1, workspace, m2, workspace),
		fmt.Sprintf(`UPDATE media_object SET deleted_at = now() WHERE id = '%s'`, m2))
	if value, closing := today("media_bytes"); value != 33 || closing != 11 {
		t.Errorf("stored bytes read %d / %d, want 33 / 11", value, closing)
	}

	inScope(fmt.Sprintf(`INSERT INTO account (id, tenant_id, kind, display_name, status)
		VALUES ('%s', '%s', 'SERVICE_ACCOUNT', 'Robot', 'ACTIVE')`, freshID(t), workspace),
		fmt.Sprintf(`UPDATE account SET status = 'DISABLED' WHERE id = '%s'`, person))
	if value, closing := today("accounts"); value != 1 || closing != 0 {
		t.Errorf("people read %d / %d after a service account and a disabling, want 1 / 0", value, closing)
	}

	// Written with no workspace in scope - a restore loaded as the owner - nothing is recorded.
	if _, err := admin.Exec(ctx, `INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key, created_by)
		VALUES `+item(freshID(t))); err != nil {
		t.Fatal(err)
	}
	if _, closing := today("items"); closing != 1 {
		t.Errorf("a write with no workspace in scope moved the count to %d", closing)
	}

	// The purge has begun: the workspace's writes record nothing more.
	if _, err := admin.Exec(ctx, `UPDATE tenant SET purge_started_at = now() WHERE id = $1`, workspace.String()); err != nil {
		t.Fatal(err)
	}
	inScope(fmt.Sprintf(`DELETE FROM media_object WHERE id = '%s'`, m1))
	if _, closing := today("media_bytes"); closing != 11 {
		t.Errorf("a write during the purge moved the bytes to %d", closing)
	}
	if _, err := admin.Exec(ctx, `UPDATE tenant SET purge_started_at = NULL WHERE id = $1`, workspace.String()); err != nil {
		t.Fatal(err)
	}

	// The control plane reads every workspace's counts; a workspace reads none.
	read := func(scope string) int {
		t.Helper()
		var rows int
		inRawTenant(ctx, t, app, scope, false, func(tx pgx.Tx) {
			day := time.Now().UTC().Format(time.DateOnly)
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM admin_usage($1, $1, NULL, $2, NULL, 100)`,
				day, workspace.String()).Scan(&rows); err != nil {
				t.Fatalf("reading the use: %v", err)
			}
		})
		return rows
	}
	if got := read(""); got != 3 {
		t.Errorf("the control plane read %d of the workspace's counts, want its 3", got)
	}
	if got := read(workspace.String()); got != 0 {
		t.Errorf("a workspace read %d counts through the control plane's door", got)
	}
}

// A day whose first write only shrinks keeps how it began as its highest.
func TestADayThatOnlyShrankKeepsHowItBegan(t *testing.T) {
	ctx := context.Background()
	app, admin := appPool(ctx, t), adminPool(ctx, t)
	workspace := seedWorkspace(ctx, t, "shrinking")
	creator := freshID(t)
	for _, statement := range []string{
		fmt.Sprintf(`INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, usage, status)
			VALUES ('%s', '%s', 'media/s/a', 'text/plain', 70, 'ATTACHMENT', 'READY'),
			       ('%s', '%s', 'media/s/b', 'text/plain', 30, 'ATTACHMENT', 'READY')`,
			creator, workspace, freshID(t), workspace),
	} {
		// Seeded as the owner: no workspace in scope, nothing recorded - the day begins at 100 bytes.
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	inRawTenant(ctx, t, app, workspace.String(), true, func(tx pgx.Tx) {
		if _, err := tx.Exec(ctx, `DELETE FROM media_object WHERE id = $1`, creator.String()); err != nil {
			t.Fatal(err)
		}
	})
	var value, closing int64
	if err := admin.QueryRow(ctx, `SELECT value, closing FROM usage_record WHERE tenant_id = $1 AND metric = 'media_bytes'`,
		workspace.String()).Scan(&value, &closing); err != nil {
		t.Fatal(err)
	}
	if value != 100 || closing != 30 {
		t.Errorf("the shrinking day reads %d / %d, want 100 / 30", value, closing)
	}
}

// Counts older than 13 months go, inside workspaces and in the installation's ledger; younger ones
// stay. Only outside a workspace's scope.
func TestCountsOlderThanThirteenMonthsAreSwept(t *testing.T) {
	ctx := context.Background()
	app, admin := appPool(ctx, t), adminPool(ctx, t)
	workspace := seedWorkspace(ctx, t, "swept")
	gone := freshID(t)
	old := time.Now().UTC().AddDate(0, -14, 0).Format(time.DateOnly)
	recent := time.Now().UTC().AddDate(0, -1, 0).Format(time.DateOnly)
	for _, statement := range []string{
		`INSERT INTO usage_record (tenant_id, period, metric, value, closing) VALUES
			('` + workspace.String() + `', '` + old + `', 'items', 5, 5),
			('` + workspace.String() + `', '` + recent + `', 'items', 7, 7)`,
		`INSERT INTO usage_archive (tenant_id, period, metric, value, closing) VALUES
			('` + gone.String() + `', '` + old + `', 'accounts', 3, 3),
			('` + gone.String() + `', '` + recent + `', 'accounts', 2, 2)`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.WithoutCancel(ctx), `DELETE FROM usage_archive WHERE tenant_id = $1`, gone.String())
	})
	cutoff := time.Now().UTC().AddDate(0, -13, 0).Format(time.DateOnly)
	inRawTenant(ctx, t, app, workspace.String(), true, func(tx pgx.Tx) {
		var swept int64
		if err := tx.QueryRow(ctx, `SELECT sweep_usage($1)`, cutoff).Scan(&swept); err != nil || swept != 0 {
			t.Errorf("a workspace's scope swept %d (%v)", swept, err)
		}
	})
	inRawTenant(ctx, t, app, "", true, func(tx pgx.Tx) {
		var swept int64
		if err := tx.QueryRow(ctx, `SELECT sweep_usage($1)`, cutoff).Scan(&swept); err != nil || swept < 2 {
			t.Errorf("the sweep removed %d (%v), want at least the two old counts", swept, err)
		}
	})
	var left int
	if err := admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM usage_record WHERE tenant_id = $1)
		+ (SELECT count(*) FROM usage_archive WHERE tenant_id = $2)`, workspace.String(), gone.String()).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 2 {
		t.Errorf("%d counts are left, want the two younger ones", left)
	}
}
