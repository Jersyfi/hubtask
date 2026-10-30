// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (s *snapshotStore) GoalPeriods(_ context.Context, after shared.ID, batch int) ([]goal.Period, error) {
	return pageAfter(s.periods, func(p goal.Period) string { return p.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) Goals(_ context.Context, after shared.ID, batch int) ([]goal.Goal, error) {
	return pageAfter(s.goals, func(g goal.Goal) string { return g.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) GoalCheckIns(_ context.Context, after shared.ID, batch int) ([]repository.GoalCheckIn, error) {
	return pageAfter(s.checkIns, func(c repository.GoalCheckIn) string { return c.CheckIn.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) Dashboards(_ context.Context, after shared.ID, batch int) ([]goal.Dashboard, error) {
	return pageAfter(s.dashboards, func(d goal.Dashboard) string { return d.ID.String() }, after.String(), batch), nil
}

// Goals and dashboards are read offline (M8): the walk carries the workspace's periods, the goals
// and check-ins of the hubs the reader reads as their definitions - never a number computed for
// somebody - and the dashboards they see, a private one only to its owner.
func TestTheWalkCarriesTheGoalsAndDashboardsTheReaderSees(t *testing.T) {
	pull, f, store := walking(t)
	period, shown, hidden := idAt('9', 1), idAt('9', 2), idAt('9', 3)
	mine, theirs, open := idAt('9', 4), idAt('9', 5), idAt('9', 6)
	store.periods = []goal.Period{{ID: period, Name: "Q4", Version: 1}}
	store.goals = []goal.Goal{
		{ID: shown, PeriodID: period, HubID: hub, Title: "Ship", Confidence: -1, Version: 2, KeyResults: []goal.KeyResult{
			{ID: idAt('9', 7), Title: "Done", Kind: goal.ResultNumber, Target: 10, Current: 4, Source: goal.SourceManual},
			{ID: idAt('9', 8), Title: "Open", Kind: goal.ResultNumber, Target: 10, Current: 9, Source: goal.SourceView, ViewID: idAt('9', 9)},
		}},
		{ID: hidden, PeriodID: period, HubID: otherHub, Title: "Merger", Confidence: -1, Version: 1},
	}
	store.checkIns = []repository.GoalCheckIn{
		{HubID: hub, CheckIn: goal.CheckIn{ID: idAt('a', 1), GoalID: shown, Status: goal.StatusOnTrack, Confidence: 7}},
		{HubID: otherHub, CheckIn: goal.CheckIn{ID: idAt('a', 2), GoalID: hidden, Status: goal.StatusAtRisk}},
	}
	store.dashboards = []goal.Dashboard{
		{ID: mine, HubID: hub, OwnerID: account, Name: "Mine", Sharing: goal.SharingPrivate, Version: 1},
		{ID: theirs, HubID: hub, OwnerID: idAt('b', 9), Name: "Theirs", Sharing: goal.SharingPrivate, Version: 1},
		{ID: open, HubID: hub, OwnerID: idAt('b', 9), Name: "Team", Sharing: goal.SharingScope, Version: 1},
	}
	f.auth.allowed[otherHub] = false
	f.auth.workspace = true

	records, _ := wholeWalk(t, pull, 100)

	got := map[string][]shared.ID{}
	for _, record := range records {
		switch record.Entity {
		case "goal_period", "goal", "goal_check_in", "dashboard":
			got[record.Entity] = append(got[record.Entity], record.EntityID)
		}
		if record.Entity == "goal" {
			if _, computed := record.Payload["progress"]; computed {
				t.Errorf("a goal record carries a reader's progress: %v", record.Payload)
			}
			results, _ := record.Payload["key_results"].([]usecase.Output)
			if len(results) != 2 {
				t.Fatalf("key results = %v", record.Payload["key_results"])
			}
			if results[0]["current"] != 4.0 || results[1]["current"] != nil {
				t.Errorf("key results %v, want the manual value only", results)
			}
		}
	}
	for entity, want := range map[string][]shared.ID{
		"goal_period": {period}, "goal": {shown}, "goal_check_in": {idAt('a', 1)}, "dashboard": {mine, open},
	} {
		if !slices.Equal(got[entity], want) {
			t.Errorf("%s delivered %v, want %v", entity, got[entity], want)
		}
	}
}
