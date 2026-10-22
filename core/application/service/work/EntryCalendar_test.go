// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A calendar has no place for the mark: it leaves out an entry of a hidden level and a due date the
// collection hides, so the entry is no member at all. The export keeps everything.
func TestACalendarLeavesOutWhatTheCollectionHides(t *testing.T) {
	h := newExportHarness(t)
	h.withCollectionView(accountID)
	due, err := domain.NewDueDate(&berlinFriday, false, "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	task := shared.MustParseID("0192f000-0000-7000-8000-0000000c0a01")
	pack := shared.MustParseID("0192f000-0000-7000-8000-0000000c0a02")
	rows := []domain.WorkItem{
		{ID: task, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemTask,
			Path: domain.RootPath(task), Depth: 1, Title: "Shop", Due: due, OrderKey: "a0", Version: 1},
		{ID: pack, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemWorkPackage,
			ParentID: task, Path: domain.RootPath(task) + pack.String() + "/", Depth: 2, Title: "Aisle",
			Due: due, OrderKey: "a0", Version: 1},
	}
	for _, row := range rows {
		h.items.stored[row.ID] = row
	}
	h.items.result = repository.ItemQueryResult{Items: rows}
	h.export.Query.Gate = gateOver(h.items, h.containers)
	narrowTo(t, h.containers, collectionID, domain.PresetTasks)

	shown, err := h.export.Select(t.Context(), feedActor(), savedViewID)
	if err != nil {
		t.Fatalf("selecting: %v", err)
	}
	if len(shown.Items) != 1 || shown.Items[0].ID != task || shown.Items[0].Due == nil {
		t.Fatalf("Tasks shows %+v, want the task with its date and not the hidden work package", shown.Items)
	}

	narrowTo(t, h.containers, collectionID, domain.PresetList)
	shown, err = h.export.Select(t.Context(), feedActor(), savedViewID)
	if err != nil {
		t.Fatalf("selecting: %v", err)
	}
	if len(shown.Items) != 1 || shown.Items[0].Due != nil {
		t.Fatalf("List shows %+v, want the task without its hidden date", shown.Items)
	}
	if h.items.stored[task].Due == nil {
		t.Fatal("the stored date was touched")
	}

	exported, err := h.export.Execute(t.Context(), feedActor(), savedViewID)
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}
	if len(exported.Items) != 2 || exported.Items[0].Due == nil {
		t.Errorf("the export left out stored data: %+v", exported.Items)
	}
}

// The CalDAV write half diffs a client's todo against the entry as the calendar shows it: read for
// the calendar, an entry in List has no date, so a PUT that leaves DUE out clears nothing.
func TestAnEntryReadForTheCalendarHasNoHiddenDate(t *testing.T) {
	store, containerStore := readFixture()
	due, err := domain.NewDueDate(&berlinFriday, false, "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	task := store.stored[readItemID]
	task.Due = due
	store.stored[readItemID] = task
	narrowTo(t, containerStore, readCollectionID, domain.PresetList)
	read := GetWorkItem{
		Items: store, Containers: containerStore, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
		Gate: gateOver(store, containerStore),
	}

	shown, err := read.Execute(t.Context(), actorFixture(), GetWorkItemQuery{ItemID: readItemID, ForCalendar: true})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if shown.Due != nil {
		t.Errorf("the calendar's read carries the hidden date %+v", shown.Due)
	}
	plain, err := read.Execute(t.Context(), actorFixture(), GetWorkItemQuery{ItemID: readItemID})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if plain.Due == nil {
		t.Error("a plain read lost the stored date")
	}
}
