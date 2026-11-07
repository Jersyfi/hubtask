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

func releaseDoors(t *testing.T, f fixture) *usecase.Registry {
	t.Helper()
	p := f.planner
	registry, err := usecase.NewRegistry(nil,
		CreateRelease{Planner: p}.Descriptor(), UpdateRelease{Planner: p}.Descriptor(),
		ListReleases{Planner: p}.Descriptor(), ReadRelease{Planner: p}.Descriptor(),
		DeleteRelease{Planner: p}.Descriptor(), AddReleaseEntries{Planner: p}.Descriptor(),
		RemoveReleaseEntries{Planner: p}.Descriptor(), PublishRelease{Planner: p}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return registry
}

// UC-PLN-08 checks 1-4 and 9 through the doors REST and MCP call: a release is planned and changed,
// collects and gives back entries, is released with its notes and the open entries carried, and
// a released one changes no more.
func TestAReleaseRunsItsWholeLifeThroughTheDoors(t *testing.T) {
	f, store := releaseFixture()
	doors := releaseDoors(t, f)
	ctx := context.Background()
	call := func(name string, in usecase.Input) usecase.Output {
		t.Helper()
		out, err := doors.Invoke(ctx, name, actor(), in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}

	release, next := planRelease(t, f, "2.4"), planRelease(t, f, "2.5")
	id := release.ID.String()
	if _, err := doors.Invoke(ctx, UpdateReleaseName, actor(), usecase.Input{"release_id": id, "name": "2.4.0", "expected_version": release.Version + 2}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale change: %v", err)
	}
	if updated := call(UpdateReleaseName, usecase.Input{"release_id": id, "name": "2.4.0", "release_on": "2026-12-04"}); updated["name"] != "2.4.0" {
		t.Fatalf("the change came back as %v", updated)
	}
	if listed := call(ListReleasesName, usecase.Input{"hub_id": hubID.String(), "state": "PLANNED"}); len(listed["data"].([]usecase.Output)) != 2 {
		t.Fatalf("the planned releases: %v", listed["data"])
	}

	call(AddReleaseEntriesName, usecase.Input{"release_id": id, "item_ids": []any{design.String(), build.String(), launch.String()}})
	call(RemoveReleaseEntriesName, usecase.Input{"release_id": id, "item_ids": []any{launch.String()}})
	if read := call(ReadReleaseName, usecase.Input{"release_id": id}); len(read["entries"].([]usecase.Output)) != 2 {
		t.Fatalf("the release reads %v", read["entries"])
	}

	store.done[design] = true
	published := call(PublishReleaseName, usecase.Input{"release_id": id, "write_notes": true, "carry_to": next.ID.String()})
	if published["state"] != string(domain.ReleaseReleased) || published["notes_page_id"] != notesPage.String() {
		t.Fatalf("publishing: %v", published)
	}
	if held := store.members[next.ID]; len(held) != 1 || held[0].EntryID != build {
		t.Fatalf("the next release holds %+v, want the open build", held)
	}
	if _, err := doors.Invoke(ctx, UpdateReleaseName, actor(), usecase.Input{"release_id": id, "name": "again"}); detail(err) != "releases.released" {
		t.Fatalf("a released release was changed: %v", err)
	}
	if _, err := doors.Invoke(ctx, DeleteReleaseName, actor(), usecase.Input{"release_id": id}); detail(err) != "releases.released" {
		t.Fatalf("a released release was deleted: %v", err)
	}
	call(DeleteReleaseName, usecase.Input{"release_id": next.ID.String()})
	if _, kept := store.releases[next.ID]; kept {
		t.Fatal("the planned release was not deleted")
	}
}

// Rule 2: a hub out of the caller's reach is not found by any release door, and nothing is
// written.
func TestTheReleaseDoorsRefuseAHubOutOfReach(t *testing.T) {
	f, store := releaseFixture()
	release := planRelease(t, f, "2.4")
	doors := releaseDoors(t, f)
	f.gate.deny = true
	id := release.ID.String()

	for name, in := range map[string]usecase.Input{
		CreateReleaseName:        {"hub_id": hubID.String(), "name": "3.0", "release_on": "2027-01-15"},
		UpdateReleaseName:        {"release_id": id, "name": "2.4.0"},
		ListReleasesName:         {"hub_id": hubID.String()},
		ReadReleaseName:          {"release_id": id},
		DeleteReleaseName:        {"release_id": id},
		AddReleaseEntriesName:    {"release_id": id, "item_ids": []any{design.String()}},
		RemoveReleaseEntriesName: {"release_id": id, "item_ids": []any{design.String()}},
		PublishReleaseName:       {"release_id": id},
	} {
		if _, err := doors.Invoke(context.Background(), name, actor(), in); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%s out of reach: %v", name, err)
		}
	}
	if len(store.releases) != 1 || store.releases[release.ID].Name != "2.4" || len(store.members[release.ID]) != 0 {
		t.Fatalf("a refused door wrote: %d releases, %+v", len(store.releases), store.releases[release.ID])
	}
}

// UC-PLN-04 checks 1-3 through the doors: a baseline is taken, listed, compared and deleted,
// the deletion on the trail; a plan out of reach is not found and nothing is deleted.
func TestABaselineRunsItsWholeLifeThroughTheDoors(t *testing.T) {
	f := newFixture()
	p := f.planner
	doors, err := usecase.NewRegistry(nil,
		TakeBaseline{Planner: p}.Descriptor(), ListBaselines{Planner: p}.Descriptor(),
		CompareBaseline{Planner: p}.Descriptor(), DeleteBaseline{Planner: p}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	ctx := context.Background()

	taken, err := doors.Invoke(ctx, TakeBaselineName, actor(), usecase.Input{"plan_id": hubID.String(), "name": "Agreed"})
	if err != nil {
		t.Fatalf("taking: %v", err)
	}
	listed, err := doors.Invoke(ctx, ListBaselinesName, actor(), usecase.Input{"plan_id": hubID.String()})
	if err != nil || len(listed["data"].([]usecase.Output)) != 1 {
		t.Fatalf("listing: %v %v", listed, err)
	}
	id := taken["id"].(string)
	f.baselines.found = domain.Baseline{ID: shared.MustParseID(id), PlanID: hubID, Entries: []domain.BaselineEntry{
		{EntryID: design, StartAt: at("2026-11-02"), DueAt: at("2026-11-03")},
	}}
	compared, err := doors.Invoke(ctx, CompareBaselineName, actor(), usecase.Input{"baseline_id": id})
	if err != nil {
		t.Fatalf("comparing: %v", err)
	}
	// The design was agreed to end on the 3rd and now ends on the 4th: a day late.
	if first := compared["entries"].([]usecase.Output)[0]; first["item_id"] != design.String() || first["finish_variance_days"] != 1 {
		t.Fatalf("the design's variance: %v", first)
	}

	f.gate.deny = true
	before := len(f.audit.entries)
	if _, err := doors.Invoke(ctx, DeleteBaselineName, actor(), usecase.Input{"baseline_id": id}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("deleting out of reach: %v", err)
	}
	if len(f.audit.entries) != before {
		t.Fatal("a refused deletion was recorded")
	}
	f.gate.deny = false
	if _, err := doors.Invoke(ctx, DeleteBaselineName, actor(), usecase.Input{"baseline_id": id}); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if last := f.audit.entries[len(f.audit.entries)-1]; last.Action != BaselineDeletedAction {
		t.Fatalf("the deletion is not on the trail: %s", last.Action)
	}
}
