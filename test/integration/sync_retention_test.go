// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The synchronisation's records over time (N-09, offline-sync.md §7, data-retention.md §4 point
// 5 and RE-6): a device back after the window starts over and the purged entry stays gone, an
// aged month of the change log falls as a partition, and the operation log and the tombstones are
// swept per tenant and never across the boundary.

// SY-5 and RE-6, one test: a cursor past the window is `cursor_too_old`, the full synchronisation
// does not bring the purged entry back, and a push naming it is `sync.gone`.
func TestADeviceBackAfterTheWindowStartsOverWithoutThePurgedEntry(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	purged := seedTask(ctx, t, tenantB, authorB, collection)

	// The purge: the row goes and the tombstone stays, the way the retention engine leaves it.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return lifecycleRepo().Record(ctx, []lifecycle.Removal{
			{Entity: "work_item", EntityID: purged, Reason: lifecycle.DeletedByRetention},
		}, created, created.Add(90*24*time.Hour))
	}); err != nil {
		t.Fatalf("recording the removal: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM work_item WHERE id = $1`, purged.String()); err != nil {
		t.Fatalf("purging: %v", err)
	}

	pull, actor := pullFor(ctx, t), streamActor(tenantB, authorB)
	stale := pull.Encode(syncservice.Position{Seq: 1, IssuedAt: time.Now().Add(-91 * 24 * time.Hour)})
	_, err := pull.Pull(ctx, actor, syncservice.PullRequest{DeviceID: freshID(t), Cursor: stale, Limit: 100})
	if !errors.Is(err, shared.ErrGone) || shared.AsError(err).DetailCode != "sync.cursor_too_old" {
		t.Fatalf("a cursor past the window was answered %v, want cursor_too_old", err)
	}

	// Starting over: the walk names everything current and not the entry that was purged.
	request := syncservice.PullRequest{DeviceID: freshID(t), Limit: 100}
	for pages := 0; ; pages++ {
		page, err := pull.Pull(ctx, actor, request)
		if err != nil {
			t.Fatalf("walking: %v", err)
		}
		for _, record := range page.Records {
			if record.EntityID == purged {
				t.Fatalf("the full synchronisation brought the purged entry back: %+v", record)
			}
		}
		if !page.More {
			break
		}
		request.Cursor = pull.Encode(page.Cursor)
		if pages > 1000 {
			t.Fatalf("the walk does not end")
		}
	}

	// The mutation the device queued against it is gone, and the client discards it.
	push := pushFor(ctx, t, 5*time.Minute)
	reading, err := shared.NewHLC(time.Now(), 1, "device-1")
	if err != nil {
		t.Fatalf("building the reading: %v", err)
	}
	response, err := push.Push(ctx, pushActor(tenantB, authorB), syncservice.PushRequest{
		DeviceID: freshID(t),
		Mutations: []syncservice.Mutation{{
			OpID: freshID(t), Kind: syncdomain.ItemPatch, ItemID: purged, HLC: reading.String(),
			Fields: map[string]syncservice.FieldChange{"title": {Value: "Edited on the train", HLC: reading.String()}},
		}},
	})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if result := response.Results[0]; result.Result != syncdomain.Rejected || result.Error == nil || result.Error.MessageCode != "sync.gone" {
		t.Errorf("the push against the purged entry was answered %+v, want sync.gone", result)
	}
}

// An aged-out month of the change log falls whole as a partition, under the window as the floor
// and held back by a tenant's longer retention - 0068's drop, over the stream 0085 added.
func TestAnAgedChangeLogMonthFallsAsAPartition(t *testing.T) {
	ctx := context.Background()
	pool, _ := databaseMigratedTo(ctx, t, "change_log_aged", 85)

	tenant := "01936f2a-7c1e-7000-8000-00000000d201"
	if _, err := pool.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'aged-log', 'Aged log')`, tenant); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	// Next month exists since the migration; an old month can be made to.
	var next int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_class WHERE relname = $1`,
		"change_log_"+time.Now().UTC().AddDate(0, 1, 0).Format("2006_01")).Scan(&next); err != nil || next != 1 {
		t.Fatalf("next month's partition of the change log is not there (%d, %v)", next, err)
	}
	old := time.Now().UTC().AddDate(0, -5, 0)
	var name string
	if err := pool.QueryRow(ctx,
		`SELECT ensure_stream_partition('change_log', date_trunc('month', $1::timestamptz)::date)`,
		old).Scan(&name); err != nil || name == "" {
		t.Fatalf("ensuring the old month (%q, %v)", name, err)
	}
	inMonth := time.Date(old.Year(), old.Month(), 15, 12, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO change_log (tenant_id, entity, entity_id, op, hlc, occurred_at)
		VALUES ($1, 'item', '01936f2a-7c1e-7000-8000-00000000d211', 'UPSERT', '1:1:server', $2),
		       ($1, 'item', '01936f2a-7c1e-7000-8000-00000000d212', 'DELETE', '1:2:server', $2)`,
		tenant, inMonth); err != nil {
		t.Fatalf("seeding the aged rows: %v", err)
	}

	// A tenant that keeps the log for a year holds the month back for everybody.
	if _, err := pool.Exec(ctx, `
		INSERT INTO retention_policy (tenant_id, data_kind, retain_days) VALUES ($1, 'SYNC_LOG', 365)`,
		tenant); err != nil {
		t.Fatalf("configuring the long retention: %v", err)
	}
	rows, err := pool.Query(ctx, `SELECT dropped, rows_removed FROM drop_stream_partition('change_log', 90)`)
	if err != nil {
		t.Fatalf("dropping under the long retention: %v", err)
	}
	held := 0
	for rows.Next() {
		held++
	}
	rows.Close()
	if held != 0 {
		t.Fatalf("a month of the change log fell although a tenant keeps it for a year")
	}

	// The configuration gone, the month falls whole - and the window is the floor: a floor of a
	// year would hold a five-month-old partition back.
	if _, err := pool.Exec(ctx,
		`DELETE FROM retention_policy WHERE tenant_id = $1 AND data_kind = 'SYNC_LOG'`, tenant); err != nil {
		t.Fatalf("clearing the retention: %v", err)
	}
	rows, err = pool.Query(ctx, `SELECT dropped, rows_removed FROM drop_stream_partition('change_log', 365)`)
	if err != nil {
		t.Fatalf("dropping under a long floor: %v", err)
	}
	for rows.Next() {
		held++
	}
	rows.Close()
	if held != 0 {
		t.Fatalf("a month inside the floor fell")
	}
	var fellName string
	var fellRows int64
	if err := pool.QueryRow(ctx,
		`SELECT dropped, rows_removed FROM drop_stream_partition('change_log', 90)`,
	).Scan(&fellName, &fellRows); err != nil {
		t.Fatalf("dropping: %v", err)
	}
	if fellName != name || fellRows != 2 {
		t.Fatalf("dropped %s with %d rows, want %s with 2", fellName, fellRows, name)
	}
	var defaults int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_class WHERE relname = 'change_log_default'`).Scan(&defaults); err != nil || defaults != 1 {
		t.Errorf("the default partition did not survive (%d, %v)", defaults, err)
	}
}

// The tenant's sweep removes operation log rows and tombstones past the window, and nothing of
// another tenant's - the cross-tenant negative for both methods (gate SG-3).
func TestTheSyncLogSweepRemovesWhatIsPastTheWindowInItsOwnTenantOnly(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	sweeper := postgres.NewSyncLogSweeper()
	old, recent := time.Now().Add(-100*24*time.Hour), time.Now().Add(-time.Hour)
	agedOp, recentOp, agedStone, recentStone := freshID(t), freshID(t), freshID(t), freshID(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, op := range []repository.OpRecord{
			{OpID: agedOp, Result: syncdomain.Applied, AppliedAt: old},
			{OpID: recentOp, Result: syncdomain.Applied, AppliedAt: recent},
		} {
			if err := opLog().Record(ctx, op); err != nil {
				return err
			}
		}
		if err := lifecycleRepo().Record(ctx, []lifecycle.Removal{
			{Entity: "work_item", EntityID: agedStone, Reason: lifecycle.DeletedByRetention},
		}, old, old.Add(90*24*time.Hour)); err != nil {
			return err
		}
		return lifecycleRepo().Record(ctx, []lifecycle.Removal{
			{Entity: "work_item", EntityID: recentStone, Reason: lifecycle.DeletedByRetention},
		}, recent, recent.Add(90*24*time.Hour))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	cutoff := time.Now().Add(-90 * 24 * time.Hour)

	// From the other tenant, nothing is due and nothing goes.
	var due, removed int
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		if due, err = sweeper.CountExpired(ctx, cutoff, 100); err != nil {
			return err
		}
		removed, err = sweeper.DeleteExpired(ctx, cutoff, 100)
		return err
	}); err != nil {
		t.Fatalf("sweeping as the other tenant: %v", err)
	}
	if due != 0 || removed != 0 {
		t.Errorf("the other tenant counted %d and removed %d of tenant A's records", due, removed)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM sync_op_log WHERE op_id = $1`, agedOp.String()); rows != 1 {
		t.Fatalf("tenant A's aged operation is gone after tenant B's sweep")
	}

	// From its own, the aged records go and the recent ones stay.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if due, err = sweeper.CountExpired(ctx, cutoff, 100); err != nil {
			return err
		}
		removed, err = sweeper.DeleteExpired(ctx, cutoff, 100)
		return err
	}); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if due < 2 || removed < 2 {
		t.Errorf("the sweep counted %d and removed %d, want the aged operation and the aged tombstone at least", due, removed)
	}
	for _, check := range []struct {
		sql  string
		id   shared.ID
		want int
	}{
		{`SELECT count(*) FROM sync_op_log WHERE op_id = $1`, agedOp, 0},
		{`SELECT count(*) FROM sync_op_log WHERE op_id = $1`, recentOp, 1},
		{`SELECT count(*) FROM tombstone WHERE entity_id = $1`, agedStone, 0},
		{`SELECT count(*) FROM tombstone WHERE entity_id = $1`, recentStone, 1},
	} {
		if rows := countIn(ctx, t, check.sql, check.id.String()); rows != check.want {
			t.Errorf("%s has %d rows for %s, want %d", check.sql, rows, check.id, check.want)
		}
	}
}
