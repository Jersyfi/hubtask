// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// TIMER and CONNECTOR_RUN (data-retention.md §3): an ended timer past the cutoff goes and a live
// one stays; a connector's old runs go with their resolved holds, while its newest run and a run
// with an item still held stay - and only in the workspace the transaction is bound to.
func TestTheSweepRemovesEndedTimersAndOldConnectorRuns(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	hub := hubFor(ctx, t, tenantA, authorA)
	old := time.Now().UTC().AddDate(0, 0, -120)
	timer := func(tenant shared.ID, state string) shared.ID {
		id := freshID(t)
		fired := any(nil)
		if state == "FIRED" {
			fired = old
		}
		due := any(nil)
		running := any(nil)
		if state == "RUNNING" {
			due, running = old, old
		}
		if _, err := admin.Exec(ctx, `INSERT INTO durable_timer (id, tenant_id, purpose, subject_id, name, calendar,
			budget_seconds, running_since, due_at, state, fired_at, created_at, updated_at)
			VALUES ($1, $2, 'SLA', $3, 'resolution', '{}', 60, $4, $5, $6, $7, $8, $8)`,
			id.String(), tenant.String(), freshID(t).String(), running, due, state, fired, old); err != nil {
			t.Fatal(err)
		}
		return id
	}
	ended, live, foreign := timer(tenantA, "FIRED"), timer(tenantA, "RUNNING"), timer(tenantB, "STOPPED")

	run := func(source string, started time.Time) shared.ID {
		id := freshID(t)
		if _, err := admin.Exec(ctx, `INSERT INTO cmdb_run (id, tenant_id, hub_id, source_key, actor_id, started_at)
			VALUES ($1, $2, $3, $4, $5, $6)`, id.String(), tenantA.String(), hub.String(), source, authorA.String(), started); err != nil {
			t.Fatal(err)
		}
		return id
	}
	hold := func(runID shared.ID, resolved any) {
		if _, err := admin.Exec(ctx, `INSERT INTO cmdb_hold (id, tenant_id, hub_id, run_id, source_key, item, reason, created_at, resolved_at)
			VALUES ($1, $2, $3, $4, 'agent', '{}', 'ambiguous', $5, $6)`,
			freshID(t).String(), tenantA.String(), hub.String(), runID.String(), old, resolved); err != nil {
			t.Fatal(err)
		}
	}
	source := "agent" + freshID(t).String()[:8]
	gone, heldRun := run(source, old), run(source, old.Add(time.Minute))
	newest := run(source, old.Add(2*time.Minute))
	hold(gone, old)
	hold(heldRun, nil)

	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := (postgres.EndedTimerExpiry{}).DeleteExpired(ctx, cutoff, 1000); err != nil {
			return err
		}
		_, err := (postgres.ConnectorRunExpiry{}).DeleteExpired(ctx, cutoff, 1000)
		return err
	}); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	exists := func(table string, id shared.ID) bool {
		var n int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = $1`, id.String()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if exists("durable_timer", ended) || !exists("durable_timer", live) || !exists("durable_timer", foreign) {
		t.Errorf("timers: ended kept %v, live kept %v, the other workspace's kept %v; want only the ended one gone",
			exists("durable_timer", ended), exists("durable_timer", live), exists("durable_timer", foreign))
	}
	if exists("cmdb_run", gone) || !exists("cmdb_run", heldRun) || !exists("cmdb_run", newest) {
		t.Errorf("runs: old kept %v, held kept %v, newest kept %v; want only the old one gone",
			exists("cmdb_run", gone), exists("cmdb_run", heldRun), exists("cmdb_run", newest))
	}
	var holds int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM cmdb_hold WHERE run_id = $1`, gone.String()).Scan(&holds); err != nil || holds != 0 {
		t.Errorf("the removed run's holds: %d (%v)", holds, err)
	}
}
