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

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func assignTo(ctx context.Context, t *testing.T, item, account shared.ID) {
	t.Helper()
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE work_item SET assignee_id = $1 WHERE id = $2`,
		account.String(), item.String()); err != nil {
		t.Fatalf("assigning: %v", err)
	}
}

// UC-SVC-02 check 15 against the real statements: over the entries assigned to one person, the
// goals met or breached in the window count per kind - the requester's per goal, a team's per group
// and goal - and the met ones without a breach as kept; a goal still running, one that ended before
// the window and one on somebody else's entry do not count, and another workspace counts none.
func TestTheDeadlinesKeptAreOnesOwnAndStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewServiceDeskRepository()
	now := time.Date(2026, 12, 7, 8, 0, 0, 0, time.UTC)
	from, to := now.AddDate(0, 0, -30), now
	team := freshID(t)

	mine, others := seedWorkItemForRules(ctx, t, tenantA), seedWorkItemForRules(ctx, t, tenantA)
	assignTo(ctx, t, mine, authorA)
	at := func(ago time.Duration) *time.Time { moment := now.Add(-ago); return &moment }
	goal := func(item shared.ID, kind servicedesk.GoalKind, standing servicedesk.Standing, met, breached *time.Time) servicedesk.EntrySLA {
		return servicedesk.EntrySLA{
			ID: freshID(t), TenantID: tenantA, ItemID: item, Goal: kind, PolicyKey: "incidents",
			Target: time.Hour, Standing: standing, ArmedAt: now.AddDate(0, 0, -40), MetAt: met, BreachedAt: breached,
		}
	}
	kept := goal(mine, servicedesk.GoalFirstResponse, servicedesk.StandingMet, at(48*time.Hour), nil)
	missed := goal(mine, servicedesk.GoalResolution, servicedesk.StandingBreached, nil, at(24*time.Hour))
	before := goal(mine, servicedesk.GoalNextResponse, servicedesk.StandingMet, at(40*24*time.Hour), nil)
	elsewhere := goal(others, servicedesk.GoalFirstResponse, servicedesk.StandingMet, at(time.Hour), nil)
	teams := goal(mine, servicedesk.GoalResolution, servicedesk.StandingMet, at(2*time.Hour), nil)
	teams.PolicyKey, teams.Group = "network_team", team
	running := goal(mine, servicedesk.GoalFirstResponse, servicedesk.StandingRunning, nil, nil)
	running.PolicyKey, running.Group = "desk_team", freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, record := range []servicedesk.EntrySLA{kept, missed, before, elsewhere} {
			if err := repo.Records().Save(ctx, record, now); err != nil {
				return err
			}
		}
		for _, record := range []servicedesk.EntrySLA{teams, running} {
			if err := repo.Records().SaveOLA(ctx, record, now); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		rows, err := repo.Records().Kept(ctx, authorA, from, to)
		if err != nil {
			return err
		}
		want := []servicedesk.KeptRow{
			{Goal: servicedesk.GoalFirstResponse, Ended: 1, Kept: 1},
			{Goal: servicedesk.GoalResolution, Ended: 1, Kept: 0},
			{OLA: true, Group: team, Goal: servicedesk.GoalResolution, Ended: 1, Kept: 1},
		}
		if fmt.Sprint(rows) != fmt.Sprint(want) {
			return fmt.Errorf("counted %+v, want %+v", rows, want)
		}
		return nil
	}); err != nil {
		t.Error(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		rows, err := repo.Records().Kept(ctx, authorA, from, to)
		if err != nil || len(rows) != 0 {
			return errors.Join(err, fmt.Errorf("another workspace counted %+v", rows))
		}
		return nil
	}); err != nil {
		t.Error(err)
	}
}
