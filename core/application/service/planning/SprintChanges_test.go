// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// last answers the newest change recorded for the entity and identifier.
func (c *changes) last(entity string, id shared.ID) (changelog.Change, bool) {
	for i := len(c.recorded) - 1; i >= 0; i-- {
		if c.recorded[i].Entity == entity && c.recorded[i].EntityID == id {
			return c.recorded[i], true
		}
	}
	return changelog.Change{}, false
}

// UC-PLN-06 and UC-PLN-07 offline: a sprint, the open sprint an entry is in and its estimate reach
// the change log under the collection, as each write leaves them - a closed sprint's carried entry
// in the next one, a finished one in none, a cleared estimate removed.
func TestSprintsAndEstimatesAreToldToOfflineDevices(t *testing.T) {
	f, _ := sprintFixture()
	log := &changes{}
	f.planner.Changes, f.planner.HLC = log, hlc{}
	ctx := context.Background()

	if _, err := (SetEntryEstimate{Planner: f.planner}).Execute(ctx, actor(), SetEntryEstimateCommand{ItemID: build, Unit: domain.Size, Size: "M"}); err != nil {
		t.Fatal(err)
	}
	got, ok := log.last(EstimateEntity, build)
	if !ok || got.Op != changelog.Upsert || got.ContainerID != collection || got.Payload["size"] != "M" || got.Payload["value"] != 3.0 {
		t.Fatalf("estimate change = %+v", got)
	}

	sprint := planSprint(t, f, "Sprint 1", "2026-11-02")
	next := planSprint(t, f, "Sprint 2", "2026-11-16")
	if got, ok := log.last(SprintEntity, sprint.ID); !ok || got.ContainerID != collection || got.Payload["state"] != "PLANNED" {
		t.Fatalf("sprint change = %+v", got)
	}
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{design, build}); err != nil {
		t.Fatal(err)
	}
	if got, _ := log.last(SprintEntryEntity, design); got.Op != changelog.Upsert || got.Payload["sprint_id"] != sprint.ID.String() || got.Payload["committed"] != false {
		t.Errorf("membership on adding = %+v", got)
	}
	if _, err := (StartSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := log.last(SprintEntryEntity, build); got.Payload["committed"] != true {
		t.Errorf("membership on starting = %+v, want committed", got)
	}
	if got, _ := log.last(SprintEntity, sprint.ID); got.Payload["state"] != "ACTIVE" || got.Payload["committed_count"] != 2 {
		t.Errorf("sprint on starting = %+v", got.Payload)
	}

	if _, err := (RemoveSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{design}); err != nil {
		t.Fatal(err)
	}
	if got, _ := log.last(SprintEntryEntity, design); got.Op != changelog.Delete {
		t.Errorf("membership on removing = %+v, want a removal", got)
	}
	if _, err := (CloseSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, next.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := log.last(SprintEntryEntity, build); got.Op != changelog.Upsert || got.Payload["sprint_id"] != next.ID.String() {
		t.Errorf("the carried entry = %+v, want it in the next sprint", got)
	}
	if got, _ := log.last(SprintEntity, sprint.ID); got.Payload["state"] != "CLOSED" {
		t.Errorf("sprint on closing = %+v", got.Payload)
	}

	if err := (DeleteSprint{Planner: f.planner}).Execute(ctx, actor(), next.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := log.last(SprintEntity, next.ID); got.Op != changelog.Delete {
		t.Errorf("a deleted sprint = %+v, want a removal", got)
	}
	if got, _ := log.last(SprintEntryEntity, build); got.Op != changelog.Delete {
		t.Errorf("an entry of a deleted sprint = %+v, want a removal", got)
	}
	if err := (ClearEntryEstimate{Planner: f.planner}).Execute(ctx, actor(), build); err != nil {
		t.Fatal(err)
	}
	if got, _ := log.last(EstimateEntity, build); got.Op != changelog.Delete {
		t.Errorf("a cleared estimate = %+v, want a removal", got)
	}
}
