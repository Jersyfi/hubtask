// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-SVC-05 check 3: a request's pace is the team's last public reply - never the requester's
// own words, an internal note, a system comment or a deleted reply - and the earliest running SLA
// goal, never a paused one or a team's OLA; the workspace next door reads neither.
func TestARequestsPaceIsTheTeamsReplyAndTheRequestersDeadline(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	paces := postgres.NewServiceDeskRepository().Paces()
	request := seedWorkItemForRules(ctx, t, tenantA) // raised by authorA
	quiet := seedWorkItemForRules(ctx, t, tenantA)
	// A day of its own: the database is shared, and the SLA report of another test counts every goal
	// armed in the hour around its 2 November.
	base := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	exec := func(what, sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seeding %s: %v", what, err)
		}
	}
	comment := `INSERT INTO comment (id, tenant_id, item_id, author_id, body, created_at, kind, internal, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	var none *time.Time
	deleted := base.Add(5 * time.Hour)
	exec("the team's reply", comment, freshID(t).String(), tenantA.String(), request.String(), authorB.String(), "on it", base.Add(time.Hour), "USER", false, none)
	exec("the requester's answer", comment, freshID(t).String(), tenantA.String(), request.String(), authorA.String(), "thanks", base.Add(2*time.Hour), "USER", false, none)
	exec("an internal note", comment, freshID(t).String(), tenantA.String(), request.String(), authorB.String(), "note", base.Add(3*time.Hour), "USER", true, none)
	exec("a system comment", comment, freshID(t).String(), tenantA.String(), request.String(), authorB.String(), "sys", base.Add(4*time.Hour), "SYSTEM", false, none)
	exec("a deleted reply", comment, freshID(t).String(), tenantA.String(), request.String(), authorB.String(), "", base.Add(5*time.Hour), "USER", false, &deleted)

	sla := `INSERT INTO entry_sla (id, tenant_id, item_id, goal, policy_key, target_seconds, standing, armed_at, due_at, updated_at)
		VALUES ($1, $2, $3, $4, 'std', 14400, $5, $6, $7, $6)`
	exec("the resolution goal", sla, freshID(t).String(), tenantA.String(), request.String(), "RESOLUTION", "RUNNING", base, base.Add(30*time.Hour))
	exec("the next response goal", sla, freshID(t).String(), tenantA.String(), request.String(), "NEXT_RESPONSE", "RUNNING", base, base.Add(6*time.Hour))
	exec("a paused first response", sla, freshID(t).String(), tenantA.String(), request.String(), "FIRST_RESPONSE", "PAUSED", base, base.Add(2*time.Hour))
	exec("a team's OLA", `INSERT INTO entry_ola (id, tenant_id, item_id, policy_key, group_id, goal, target_seconds, standing, armed_at, due_at, updated_at)
		VALUES ($1, $2, $3, 'net', $4, 'RESOLUTION', 3600, 'RUNNING', $5, $6, $5)`,
		freshID(t).String(), tenantA.String(), request.String(), freshID(t).String(), base, base.Add(time.Hour))

	var got map[shared.ID]servicedesk.RequestPace
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		got, err = paces.PaceOf(ctx, []shared.ID{request, quiet})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	pace := got[request]
	if pace.LastReplyAt == nil || !pace.LastReplyAt.Equal(base.Add(time.Hour)) {
		t.Errorf("the last reply reads %v, want the team's at %v", pace.LastReplyAt, base.Add(time.Hour))
	}
	if pace.NextDueAt == nil || !pace.NextDueAt.Equal(base.Add(6*time.Hour)) {
		t.Errorf("the next deadline reads %v, want the running SLA goal's at %v", pace.NextDueAt, base.Add(6*time.Hour))
	}
	if still, found := got[quiet]; !found || still.LastReplyAt != nil || still.NextDueAt != nil {
		t.Errorf("a request nobody answered reads %+v (found %v)", still, found)
	}

	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		other, err := paces.PaceOf(ctx, []shared.ID{request})
		if err == nil && len(other) != 0 {
			t.Errorf("the other workspace reads the request's pace: %+v", other)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
