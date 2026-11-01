// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	calendarservice "github.com/Jersyfi/hubtask/core/application/service/calendar"
	servicedeskservice "github.com/Jersyfi/hubtask/core/application/service/servicedesk"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A workspace's SLA clocks across a restore (backup-restore.md §8.4), against the real database:
// the archive carries the goals' records and no timer, and the restore arms each live goal again
// - the one past its due moment due at once, the paused one with what it had spent - on the
// identities the restore gave the copy.
func TestARestoredWorkspacesGoalsGetTheirClocksBack(t *testing.T) {
	ctx := context.Background()
	f := newForeignRestore(ctx, t)
	overdue := created.Add(-time.Hour)
	source, prefix := foreignWorkspace(ctx, t, f.store, "Printer on fire",
		func(tenant, _ shared.ID) (string, []any) {
			return `INSERT INTO sla_policy (id, tenant_id, key, name, entry_types, goals, created_at, updated_at)
			  VALUES ($1, $2, 'incidents', 'Incidents', '{TASK}',
			          '[{"kind":"FIRST_RESPONSE","minutes":60},{"kind":"RESOLUTION","minutes":240}]', $3, $3)`,
				[]any{freshID(t).String(), tenant.String(), created}
		},
		func(tenant, item shared.ID) (string, []any) {
			return `INSERT INTO entry_sla (id, tenant_id, item_id, goal, policy_key, target_seconds, standing,
			                                armed_at, due_at, updated_at)
			  VALUES ($1, $2, $3, 'RESOLUTION', 'incidents', 14400, 'RUNNING', $4, $5, $4)`,
				[]any{freshID(t).String(), tenant.String(), item.String(), created.Add(-5 * time.Hour), overdue}
		},
		func(tenant, item shared.ID) (string, []any) {
			return `INSERT INTO entry_sla (id, tenant_id, item_id, goal, policy_key, target_seconds, standing,
			                                armed_at, consumed_seconds, updated_at)
			  VALUES ($1, $2, $3, 'FIRST_RESPONSE', 'incidents', 3600, 'PAUSED', $4, 600, $4)`,
				[]any{freshID(t).String(), tenant.String(), item.String(), created.Add(-5 * time.Hour)}
		},
	)
	deleteForGood(ctx, t, source)

	// A NEW_TENANT copy's slug is derived from the first digits of its identity; another prefix
	// than freshID's keeps it apart from the other restores of this package.
	restoreID := freshID(t)
	minted := shared.MustParseID(fmt.Sprintf("%08x-7c1e-7000-8f00-000000000001", 0x0bb00000+fixtureCounter.Add(1)))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return restoreRepo().Insert(ctx, domain.Restore{
			ID: restoreID, TargetID: f.target, TenantID: minted, SourceArchive: prefix,
			Mode: domain.RestoreNewTenant, ConflictRule: domain.ConflictSkip,
			Status: domain.RestorePending, RequestedBy: authorA,
		})
	}); err != nil {
		t.Fatalf("accepting the restore: %v", err)
	}
	applier := f.applier(t)
	store := postgres.NewServiceDeskRepository()
	applier.Clocks = []backupservice.ClockRearmer{servicedeskservice.RestoredClocks{
		Records: store.Records(), Approvals: store.Approvals(), Policies: store.Policies(),
		Calendars: store.Calendars(), HubCalendars: store.HubCalendars(),
		Containers: postgres.NewContainerRepository(pageCursors()), Items: postgres.NewItemRepository(pageCursors()),
		Timers: calendarservice.Clocks{Timers: postgres.NewTimerRepository(), Clock: clock.Fixed(created), IDs: clockadapter.NewUUIDv7(clockadapter.System{})},
	}}
	if _, err := applier.Apply(ctx, backupservice.ApplyInput{RestoreID: restoreID, TenantID: tenantA}); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	admin := adminPool(ctx, t)
	var item string
	if err := admin.QueryRow(ctx, `SELECT id FROM work_item WHERE tenant_id = $1`, minted.String()).Scan(&item); err != nil {
		t.Fatalf("the entry did not arrive: %v", err)
	}
	rows, err := admin.Query(ctx, `
		SELECT name, subject_id::text, state, due_at, consumed_seconds
		FROM durable_timer WHERE tenant_id = $1 AND purpose = 'SLA' ORDER BY name`, minted.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type timer struct {
		subject, state string
		due            *time.Time
		consumed       int64
	}
	got := map[string]timer{}
	for rows.Next() {
		var name string
		var row timer
		if err := rows.Scan(&name, &row.subject, &row.state, &row.due, &row.consumed); err != nil {
			t.Fatal(err)
		}
		got[name] = row
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("the restored workspace holds the SLA timers %v, want the two goals'", got)
	}
	if r := got["resolution"]; r.subject != item || r.state != "RUNNING" || r.due == nil || !r.due.Equal(created) {
		t.Errorf("the overdue resolution was re-armed as %+v, want running on %s and due at once", r, item)
	}
	if f := got["first_response"]; f.subject != item || f.state != "PAUSED" || f.due != nil || f.consumed != 600 {
		t.Errorf("the paused first response was re-armed as %+v, want paused with 600 s spent", f)
	}
}
