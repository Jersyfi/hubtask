// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func sprintDoors(t *testing.T, f fixture) *usecase.Registry {
	t.Helper()
	p := f.planner
	registry, err := usecase.NewRegistry(nil,
		CreateSprint{Planner: p}.Descriptor(), UpdateSprint{Planner: p}.Descriptor(),
		ListSprints{Planner: p}.Descriptor(), ReadSprint{Planner: p}.Descriptor(),
		DeleteSprint{Planner: p}.Descriptor(), AddSprintEntries{Planner: p}.Descriptor(),
		RemoveSprintEntries{Planner: p}.Descriptor(), StartSprint{Planner: p}.Descriptor(),
		CloseSprint{Planner: p}.Descriptor(), ReadSprintBacklog{Planner: p}.Descriptor(),
		ReadVelocity{Planner: p}.Descriptor(), ListMyRunningSprints{Planner: p}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return registry
}

// UC-PLN-06 checks 1, 2, 5 and 8: through the doors REST, MCP and hubctl call, a sprint is
// planned, changed, filled, started, closed into the next one and read, and only a planned one is
// deleted.
func TestASprintRunsItsWholeLifeThroughTheDoors(t *testing.T) {
	f, store := sprintFixture()
	doors := sprintDoors(t, f)
	ctx := context.Background()
	call := func(name string, in usecase.Input) usecase.Output {
		t.Helper()
		out, err := doors.Invoke(ctx, name, actor(), in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}

	sprint := planSprint(t, f, "Sprint 1", "2026-11-02")
	next := planSprint(t, f, "Sprint 2", "2026-11-16")
	id := sprint.ID.String()

	if _, err := doors.Invoke(ctx, UpdateSprintName, actor(), usecase.Input{"sprint_id": id, "name": "Beta", "expected_version": sprint.Version + 3}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale change: %v", err)
	}
	if updated := call(UpdateSprintName, usecase.Input{"sprint_id": id, "name": "Beta", "goal": "Ship it"}); updated["name"] != "Beta" || updated["goal"] != "Ship it" {
		t.Fatalf("the change came back as %v", updated)
	}
	if listed := call(ListSprintsName, usecase.Input{"collection_id": collection.String(), "state": "PLANNED"}); len(listed["data"].([]usecase.Output)) != 2 {
		t.Fatalf("the planned sprints: %v", listed["data"])
	}

	call(AddSprintEntriesName, usecase.Input{"sprint_id": id, "item_ids": []any{design.String(), build.String(), launch.String()}})
	call(RemoveSprintEntriesName, usecase.Input{"sprint_id": id, "item_ids": []any{launch.String()}})
	held := 0
	for _, m := range store.members[sprint.ID] {
		if m.RemovedAt == nil {
			held++
		}
	}
	if held != 2 {
		t.Fatalf("the sprint holds %d entries, want 2", held)
	}
	if started := call(StartSprintName, usecase.Input{"sprint_id": id}); started["state"] != string(domain.SprintActive) {
		t.Fatalf("starting: %v", started["state"])
	}
	// The removed entry stays in the sprint's read, marked removed: the scope change is history.
	for _, e := range call(ReadSprintName, usecase.Input{"sprint_id": id})["entries"].([]usecase.Output) {
		if removed := e["removed_at"] != nil; removed != (e["item_id"] == launch.String()) {
			t.Fatalf("entry %v removed_at %v", e["item_id"], e["removed_at"])
		}
	}
	call(CloseSprintName, usecase.Input{"sprint_id": id, "carry_to_sprint_id": next.ID.String()})
	if store.sprints[sprint.ID].State != domain.SprintClosed || len(store.members[next.ID]) != 2 {
		t.Fatalf("closing: state %s, carried %d", store.sprints[sprint.ID].State, len(store.members[next.ID]))
	}
	if velocity := call(ReadVelocityName, usecase.Input{"collection_id": collection.String()}); velocity == nil {
		t.Fatal("no velocity")
	}
	call(ReadSprintBacklogName, usecase.Input{"collection_id": collection.String()})
	call(ListMyRunningSprintsName, usecase.Input{})

	if _, err := doors.Invoke(ctx, DeleteSprintName, actor(), usecase.Input{"sprint_id": id}); detail(err) != "sprints.not_planned" {
		t.Fatalf("a closed sprint was deleted: %v", err)
	}
	other := planSprint(t, f, "Sprint 3", "2026-12-01")
	call(DeleteSprintName, usecase.Input{"sprint_id": other.ID.String()})
	if _, kept := store.sprints[other.ID]; kept {
		t.Fatal("the planned sprint was not deleted")
	}
}

// Rule 2: a collection out of the caller's reach is not found by any sprint door, and nothing is
// written.
func TestTheSprintDoorsRefuseACollectionOutOfReach(t *testing.T) {
	f, store := sprintFixture()
	sprint := planSprint(t, f, "Sprint 1", "2026-11-02")
	doors := sprintDoors(t, f)
	f.gate.deny = true
	id := sprint.ID.String()

	for name, in := range map[string]usecase.Input{
		CreateSprintName:        {"collection_id": collection.String(), "name": "S", "start_on": "2026-12-01", "end_on": "2026-12-12"},
		UpdateSprintName:        {"sprint_id": id, "name": "Beta"},
		ListSprintsName:         {"collection_id": collection.String()},
		ReadSprintName:          {"sprint_id": id},
		DeleteSprintName:        {"sprint_id": id},
		AddSprintEntriesName:    {"sprint_id": id, "item_ids": []any{design.String()}},
		RemoveSprintEntriesName: {"sprint_id": id, "item_ids": []any{design.String()}},
		StartSprintName:         {"sprint_id": id},
		CloseSprintName:         {"sprint_id": id},
		ReadSprintBacklogName:   {"collection_id": collection.String()},
		ReadVelocityName:        {"collection_id": collection.String()},
	} {
		if _, err := doors.Invoke(context.Background(), name, actor(), in); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%s out of reach: %v", name, err)
		}
	}
	if len(store.sprints) != 1 || store.sprints[sprint.ID].Name != "Sprint 1" || len(store.members[sprint.ID]) != 0 {
		t.Fatalf("a refused door wrote: %d sprints, %+v", len(store.sprints), store.sprints[sprint.ID])
	}
}
