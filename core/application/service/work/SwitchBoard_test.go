// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// List ↔ board without a workflow (domain-model.md §3.5a, L0 ↔ L1): *Als Board anzeigen* creates
// an open and a done column in the workspace's language, the completed entries in the done one;
// back to a list takes the columns off, the done column last, and keeps completion.

// unplacedTasks answers PlaceUnplaced with the tasks of one completion standing in no column.
type unplacedTasks struct {
	*mapping
	open, completed int
}

func (u unplacedTasks) PlaceUnplaced(
	ctx context.Context, collection shared.ID, completed bool, column shared.ID, at time.Time,
) ([]repository.MovedEntry, error) {
	if _, err := u.mapping.PlaceUnplaced(ctx, collection, completed, column, at); err != nil {
		return nil, err
	}
	count := u.open
	if completed {
		count = u.completed
	}
	out := make([]repository.MovedEntry, 0, count)
	for range count {
		out = append(out, repository.MovedEntry{ID: taskID, Version: 2})
	}
	return out, nil
}

type switchHarness struct {
	*bucketHarness
	board    *mapping
	switcher SwitchBoard
}

// newSwitchHarness is the collection "Shopping" as a list: no columns, 4 open and 3 completed tasks.
func newSwitchHarness() *switchHarness {
	base := newBucketHarness()
	base.withCollection()
	board := &mapping{}
	writer := BucketWriter{
		Buckets: base.buckets, Containers: base.containers, Authorizer: base.authorizer,
		Events: base.events, Changes: base.changes, Audit: base.audit, UnitOfWork: base.uow,
		Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
		Boards: Boards{Buckets: base.buckets, Statuses: unplacedTasks{mapping: board, open: 4, completed: 3}},
	}
	return &switchHarness{bucketHarness: base, board: board, switcher: SwitchBoard{
		Columns: writer, Workspaces: workspaces{locale: "de"}, Renderer: echoRenderer{},
	}}
}

func (h *switchHarness) eventsOf(kind event.Type) int {
	n := 0
	for _, announcement := range h.events.appended {
		if announcement.Type == kind {
			n++
		}
	}
	return n
}

// The two columns of a switched board follow the same rule as a new collection's (UC-WRK-05
// check 11, UC-WRK-31 check 6): the switching account's language, else the workspace's default -
// never the request's.
func TestASwitchedBoardsColumnsFollowTheAccountElseTheWorkspace(t *testing.T) {
	for _, tc := range []struct{ accountLocale, want string }{
		{"", "de:seed.bucket.open"},
		{"fr", "fr:seed.bucket.open"},
	} {
		h := newSwitchHarness()
		switching := actor()
		switching.Locale, switching.AccountLocale = "en", tc.accountLocale
		if _, err := h.switcher.Execute(t.Context(), switching, collectionID, domain.StateModeColumns, false); err != nil {
			t.Fatal(err)
		}
		board, err := h.buckets.List(t.Context(), collectionID)
		if err != nil || len(board) != 2 || board[0].Name != tc.want {
			t.Errorf("account %q: board %+v (%v), want the open column named %q", tc.accountLocale, board, err, tc.want)
		}
	}
}

// *Als Board anzeigen*: an open and a done column named in the workspace's language, the
// completed entries into the done one and the open ones into the other, each column announced and
// recorded, the switch audited with its counts.
func TestShowingAListAsABoardGivesItAnOpenAndADoneColumn(t *testing.T) {
	h := newSwitchHarness()
	report, err := h.switcher.Execute(t.Context(), actor(), collectionID, domain.StateModeColumns, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Before != domain.StateModeNone || report.After != domain.StateModeColumns || report.Created != 2 || report.Moved != 7 {
		t.Fatalf("report %+v, want two columns and seven entries placed", report)
	}
	board, err := h.buckets.List(t.Context(), collectionID)
	if err != nil || len(board) != 2 {
		t.Fatalf("board %+v, %v", board, err)
	}
	open, done := board[0], board[1]
	if open.Name != "de:seed.bucket.open" || open.IsDoneBucket || done.Name != "de:seed.bucket.done" || !done.IsDoneBucket {
		t.Fatalf("columns %q (done %v) and %q (done %v), want the open one first, then the done one",
			open.Name, open.IsDoneBucket, done.Name, done.IsDoneBucket)
	}
	want := []placedByStatus{{completed: true, from: unplaced, column: done.ID}, {completed: false, from: unplaced, column: open.ID}}
	if len(h.board.placed) != 2 || h.board.placed[0] != want[0] || h.board.placed[1] != want[1] {
		t.Fatalf("placed %+v, want the completed into the done column and the open into the other", h.board.placed)
	}
	if h.eventsOf(event.BucketCreated) != 2 {
		t.Errorf("events %+v, want each column announced", h.events.appended)
	}
	placedChanges := 0
	for _, change := range h.changes.recorded {
		if change.Entity == "item" && change.Field == domain.FieldBucketID {
			placedChanges++
		}
	}
	if placedChanges != 7 {
		t.Errorf("%d column changes recorded, want one per entry placed", placedChanges)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != BoardSwitchedAction {
		t.Fatalf("audit %+v, want the switch", h.audit.entries)
	}
	changes := h.audit.entries[0].Changes
	level, _ := changes["state_mode"].(map[string]any)
	placed, _ := changes["entries_placed"].(map[string]any)
	if level["from"] != "NONE" || level["to"] != "COLUMNS" || placed["to"] != "7" {
		t.Errorf("audited %+v, want the levels and the count", changes)
	}
}

// Back to a list: every column is taken off, the done column last, its entries standing in no
// column and keeping their completion - what *Remove columns* confirms (UC-WRK-05 check 13).
func TestABoardBackToAListTakesTheColumnsOffTheDoneOneLast(t *testing.T) {
	h := newSwitchHarness()
	threeColumnBoard(h.buckets)
	report, err := h.switcher.Execute(t.Context(), actor(), collectionID, domain.StateModeNone, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Before != domain.StateModeColumns || report.After != domain.StateModeNone || report.Removed != 3 || report.Created != 0 {
		t.Fatalf("report %+v, want three columns removed", report)
	}
	if len(h.buckets.moved) != 3 || h.buckets.moved[2].source != doneColumn {
		t.Fatalf("moved %+v, want the done column's entries last", h.buckets.moved)
	}
	for _, move := range h.buckets.moved {
		if !move.target.IsZero() {
			t.Errorf("entries of %s went into %s, want no column", move.source, move.target)
		}
	}
	if left, _ := h.buckets.List(t.Context(), collectionID); len(left) != 0 {
		t.Errorf("columns left: %+v", left)
	}
	if len(h.board.completedIn) != 0 || len(h.board.placed) != 0 {
		t.Error("going back to a list changed a completion")
	}
	if h.eventsOf(event.BucketDeleted) != 3 || len(h.audit.entries) != 1 {
		t.Errorf("%d deletions announced, %d audit entries", h.eventsOf(event.BucketDeleted), len(h.audit.entries))
	}
}

// A dry run answers the counts and leaves no audit entry; through the catalogue entry it answers
// the contract's names.
func TestADryRunOfTheSwitchCountsOnly(t *testing.T) {
	h := newSwitchHarness()
	out, err := h.switcher.Descriptor().Handler.Invoke(t.Context(), actor(), usecase.Input{
		"collection_id": collectionID.String(), "to": "COLUMNS", "dry_run": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["dry_run"] != true || out["level_before"] != "NONE" || out["level_after"] != "COLUMNS" ||
		out["columns_created"] != 2 || out["entries_placed"] != 7 || out["columns_removed"] != 0 {
		t.Fatalf("output %+v", out)
	}
	if len(h.audit.entries) != 0 || !h.uow.rolledBack {
		t.Fatal("a dry run was committed")
	}
	if _, err := h.switcher.Descriptor().Handler.Invoke(t.Context(), actor(), usecase.Input{"to": "COLUMNS"}); err == nil {
		t.Fatal("a switch without a collection was answered")
	}
}

// Refused: an unknown level, a level the tasks already stand at, a workflow running for tasks, a
// hub, and whoever may not shape the collection - nothing written in each case.
func TestTheSwitchIsRefused(t *testing.T) {
	tests := []struct {
		name    string
		to      domain.StateMode
		arrange func(*switchHarness)
		detail  string
	}{
		{"an unknown level", domain.StateModeWorkflow, func(*switchHarness) {}, "board.level_unknown"},
		{"a list to a list", domain.StateModeNone, func(*switchHarness) {}, "board.level_unchanged"},
		{"a board to a board", domain.StateModeColumns, func(h *switchHarness) { threeColumnBoard(h.buckets) }, "board.level_unchanged"},
		{"a workflow running for tasks", domain.StateModeColumns, func(h *switchHarness) {
			h.switcher.Workflow = WorkflowGate{Workflows: hubWorkflows{
				rows:    []domain.Workflow{{Key: "service"}},
				schemes: []domain.WorkflowScheme{{ContainerID: hubID, WorkflowKey: "service"}},
			}}
		}, "board.workflow_runs"},
		{"a hub", domain.StateModeColumns, func(h *switchHarness) {
			hub := h.containers.stored[collectionID]
			hub.Type, hub.ParentID = domain.ContainerHub, ""
			h.containers.stored[collectionID] = hub
		}, "board.no_columns"},
		{"somebody who may not shape it", domain.StateModeColumns, func(h *switchHarness) { h.authorizer.err = shared.ErrForbidden }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSwitchHarness()
			tt.arrange(h)
			inserted, written := len(h.buckets.inserted), len(h.buckets.written)
			_, err := h.switcher.Execute(t.Context(), actor(), collectionID, tt.to, false)
			if err == nil || detail(err) != tt.detail {
				t.Fatalf("err %v, want the refusal %q", err, tt.detail)
			}
			if len(h.buckets.inserted) != inserted || len(h.buckets.written) != written || len(h.board.placed) != 0 || len(h.audit.entries) != 0 {
				t.Fatal("a refused switch wrote something")
			}
		})
	}
}
