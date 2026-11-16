// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow_test

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// *Workflow aus diesen Spalten* (domain-model.md §3.5a, L1 → L2): a status per column, run for the
// collection's tasks, each column showing its own status and every entry taking its column's.

var (
	backlog = shared.MustParseID("018f0000-0000-7000-8000-0000000000c1")
	doing   = shared.MustParseID("018f0000-0000-7000-8000-0000000000c2")
	done    = shared.MustParseID("018f0000-0000-7000-8000-0000000000c3")
)

type columns struct {
	repository.Buckets
	rows []work.Bucket
}

func (c columns) List(context.Context, shared.ID) ([]work.Bucket, error) { return c.rows, nil }

// placed is one call that gave entries a status or a column.
type placed struct {
	column    shared.ID
	completed bool
	status    string
}

type columnStatuses struct {
	repository.BucketStatuses
	cells   []work.BoardCell
	mapping []work.ColumnStatus
	given   []placed
	placed  []placed
}

func (s *columnStatuses) List(context.Context, shared.ID) ([]work.ColumnStatus, error) {
	return s.mapping, nil
}

func (s *columnStatuses) Replace(_ context.Context, _ shared.ID, mapping []work.ColumnStatus) error {
	s.mapping = mapping
	return nil
}

func (s *columnStatuses) Cells(context.Context, shared.ID) ([]work.BoardCell, error) {
	return s.cells, nil
}

// SetStatusInColumn answers the open entries counted in the cell it names.
func (s *columnStatuses) SetStatusInColumn(
	_ context.Context, _, column shared.ID, completed bool, status string, _ time.Time,
) ([]repository.MovedEntry, error) {
	s.given = append(s.given, placed{column, completed, status})
	var out []repository.MovedEntry
	for _, cell := range s.cells {
		if cell.Status == "" && cell.BucketID == column && (column != "" || cell.Completed == completed) {
			for range cell.Count {
				out = append(out, repository.MovedEntry{ID: entry, Version: 3})
			}
		}
	}
	return out, nil
}

func (s *columnStatuses) PlaceByStatus(
	_ context.Context, _ shared.ID, status string, column shared.ID, _ time.Time,
) ([]repository.MovedEntry, error) {
	s.placed = append(s.placed, placed{column: column, status: status})
	return []repository.MovedEntry{{ID: entry, Version: 4}}, nil
}

type boardFixture struct {
	deskFixture
	statuses *columnStatuses
}

// newBoardDesk is a collection "Shop" with the board Backlog | Doing | Done (the done column), and
// 15 tasks: 3 in Backlog and 2 open in no column (both read as Backlog), 4 in Doing, 5 in Done and
// 1 completed in no column (both read as Done).
func newBoardDesk() boardFixture {
	f := boardFixture{deskFixture: newDesk(), statuses: &columnStatuses{cells: []work.BoardCell{
		{BucketID: backlog, Count: 3},
		{Count: 2},
		{BucketID: doing, Count: 4},
		{BucketID: done, Completed: true, Count: 5},
		{Completed: true, Count: 1},
	}}}
	f.desk.Containers = memoryContainers{rows: map[shared.ID]work.Container{
		hub:        {ID: hub, TenantID: tenant, Type: work.ContainerHub},
		collection: {ID: collection, TenantID: tenant, Type: work.ContainerCollection, ParentID: hub, Name: "Shop"},
	}}
	f.desk.Gate = workservice.WorkflowGate{Workflows: f.workflows}
	f.desk.Boards = workservice.SchemeBoards{Columns: workservice.BucketWriter{Boards: workservice.Boards{
		Buckets: columns{rows: []work.Bucket{
			{ID: backlog, CollectionID: collection, Name: "Backlog", ColorToken: "blue"},
			{ID: doing, CollectionID: collection, Name: "Doing"},
			{ID: done, CollectionID: collection, Name: "Done", IsDoneBucket: true},
		}},
		Statuses: f.statuses,
	}}}
	return f
}

// The dry run answers the statuses and the entries each would take, and writes nothing.
func TestTheWorkflowFromColumnsPreviewsWithoutWriting(t *testing.T) {
	f := newBoardDesk()
	result, err := workflow.MakeBoardWorkflow{Desk: f.desk}.Execute(t.Context(), actor(), collection, "", true)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		key      string
		category work.StatusCategory
		column   shared.ID
		count    int
	}{
		{"s1", work.CategoryTodo, backlog, 5},
		{"s2", work.CategoryInProgress, doing, 4},
		{"s3", work.CategoryDone, done, 6},
	}
	if !result.DryRun || result.Entries != 15 || len(result.Statuses) != len(want) {
		t.Fatalf("result %+v, want three statuses over 15 entries", result)
	}
	for i, w := range want {
		got := result.Statuses[i]
		if got.Status.Key != w.key || got.Status.Category != w.category || got.BucketID != w.column || got.Count != w.count {
			t.Errorf("status %d: %+v, want %+v", i, got, w)
		}
	}
	if result.Statuses[0].Status.Name != "Backlog" || result.Statuses[0].Status.ColorToken != "blue" {
		t.Errorf("a status takes its column's name and colour: %+v", result.Statuses[0].Status)
	}
	if len(f.workflows.rows) != 0 || len(f.workflows.schemes) != 0 || f.statuses.mapping != nil ||
		len(f.statuses.given) != 0 || len(f.trail.entries) != 0 || len(f.history.entries) != 0 {
		t.Fatal("a dry run wrote something")
	}
}

// The workflow is the hub's, the first column's status initial, every step free; it runs for the
// collection's tasks, each column shows its own status, and every entry takes its column's
// status with its history, its change and an audit entry with the counts.
func TestTheWorkflowFromColumnsRunsForTheCollectionsTasks(t *testing.T) {
	f := newBoardDesk()
	result, err := workflow.MakeBoardWorkflow{Desk: f.desk}.Execute(t.Context(), actor(), collection, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.DryRun || result.Entries != 15 || len(f.workflows.rows) != 1 {
		t.Fatalf("result %+v, %d workflows", result, len(f.workflows.rows))
	}
	made := f.workflows.rows[0]
	shape := made.Published
	if made.Key != result.Key || made.HubID != hub || made.Name != "Shop" || shape.Initial != "s1" {
		t.Fatalf("workflow %+v, want the hub's, named after the collection, starting in s1", made)
	}
	for _, status := range []string{"s1", "s2", "s3"} {
		free := false
		for _, step := range shape.Steps {
			free = free || (step.To == status && len(step.From) == 0 && len(step.Who.Roles)+len(step.Who.Groups)+len(step.Who.PersonFields) == 0 && step.Who.Condition == "" && len(step.Requires) == 0)
		}
		if !free {
			t.Errorf("no free step into %s: %+v", status, shape.Steps)
		}
	}
	tasks := work.EntryTypeKeyOf(work.WorkItem{Type: work.ItemTask})
	if len(f.workflows.schemes) != 1 || f.workflows.schemes[0] != (work.WorkflowScheme{ContainerID: collection, EntryType: tasks, WorkflowKey: made.Key}) {
		t.Fatalf("schemes %+v, want the workflow for the collection's tasks", f.workflows.schemes)
	}
	wantMapping := []work.ColumnStatus{{BucketID: backlog, Status: "s1"}, {BucketID: doing, Status: "s2"}, {BucketID: done, Status: "s3"}}
	if len(f.statuses.mapping) != len(wantMapping) {
		t.Fatalf("mapping %+v", f.statuses.mapping)
	}
	for i := range wantMapping {
		if f.statuses.mapping[i] != wantMapping[i] {
			t.Fatalf("mapping %+v, want each column its own status", f.statuses.mapping)
		}
	}
	// The entries in no column take the first column's status when open, the done one's when
	// completed, and are then placed into that column.
	wantGiven := []placed{
		{backlog, false, "s1"}, {"", false, "s1"}, {doing, false, "s2"}, {done, false, "s3"}, {"", true, "s3"},
	}
	if len(f.statuses.given) != len(wantGiven) {
		t.Fatalf("given %+v, want %+v", f.statuses.given, wantGiven)
	}
	for i := range wantGiven {
		if f.statuses.given[i] != wantGiven[i] {
			t.Fatalf("given %+v, want %+v", f.statuses.given, wantGiven)
		}
	}
	if len(f.statuses.placed) != 2 || f.statuses.placed[0] != (placed{column: backlog, status: "s1"}) ||
		f.statuses.placed[1] != (placed{column: done, status: "s3"}) {
		t.Fatalf("placed %+v, want the loose entries into the first and the done column", f.statuses.placed)
	}
	if len(f.history.entries) != 15 || f.history.entries[0].Verb != activity.ItemWorkflowChanged {
		t.Fatalf("%d history entries, want one workflow change per entry", len(f.history.entries))
	}
	statusChanges, columnChanges := 0, 0
	for _, change := range f.log.changes {
		switch change.Field {
		case work.FieldStatus:
			statusChanges++
		case work.FieldBucketID:
			columnChanges++
		}
	}
	if statusChanges != 15 || columnChanges != 2 {
		t.Fatalf("%d status and %d column changes, want 15 and 2", statusChanges, columnChanges)
	}
	last := f.trail.entries[len(f.trail.entries)-1]
	moved, _ := last.Changes["entries_moved"].(map[string]any)
	statuses, _ := last.Changes["statuses"].(map[string]any)
	if last.Action != workflow.WorkflowCreatedAction || moved["to"] != "15" || statuses["to"] != "3" {
		t.Fatalf("trail %+v, want the creation with its counts", last)
	}
}

// A name given names the workflow, through the catalogue entry as through Execute.
func TestTheWorkflowFromColumnsTakesTheNameGiven(t *testing.T) {
	f := newBoardDesk()
	out, err := workflow.MakeBoardWorkflow{Desk: f.desk}.Descriptor().Handler.Invoke(t.Context(), actor(), usecase.Input{
		"collection_id": collection.String(), "name": "Shopping",
	})
	if err != nil {
		t.Fatal(err)
	}
	statuses, _ := out["statuses"].([]usecase.Output)
	if out["dry_run"] != false || out["entries"] != 15 || len(statuses) != 3 || statuses[2]["category"] != string(work.CategoryDone) {
		t.Fatalf("output %+v", out)
	}
	if len(f.workflows.rows) != 1 || f.workflows.rows[0].Name != "Shopping" || out["workflow"] != f.workflows.rows[0].Key {
		t.Fatalf("workflows %+v, want one named Shopping", f.workflows.rows)
	}
	if _, err := (workflow.MakeBoardWorkflow{Desk: f.desk}).Descriptor().Handler.Invoke(t.Context(), actor(), usecase.Input{}); err == nil {
		t.Fatal("a call without a collection was answered")
	}
}

// Refused without columns, where a workflow already runs for tasks, and for whoever may not
// shape the hub - each before anything is written.
func TestTheWorkflowFromColumnsIsRefused(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(*testing.T, *boardFixture)
		detail  string
	}{
		{"a build without boards", func(_ *testing.T, f *boardFixture) { f.desk.Boards = workservice.SchemeBoards{} }, "board.no_columns"},
		{"a collection without columns", func(_ *testing.T, f *boardFixture) {
			f.desk.Boards.Columns.Boards.Buckets = columns{}
		}, "board.no_columns"},
		{"a hub rather than a collection", func(_ *testing.T, f *boardFixture) {
			rows := f.desk.Containers.(memoryContainers).rows
			rows[collection] = work.Container{ID: collection, TenantID: tenant, Type: work.ContainerHub}
		}, "board.no_columns"},
		{"a workflow running for tasks", func(t *testing.T, f *boardFixture) {
			if _, err := (workflow.CreateWorkflow{Desk: f.desk}).Execute(t.Context(), actor(), "service", "Service", serviceShape(false)); err != nil {
				t.Fatal(err)
			}
			f.workflows.schemes = []work.WorkflowScheme{{ContainerID: hub, WorkflowKey: "service"}}
			f.trail.entries = nil
		}, "board.workflow_runs"},
		{"somebody who may not shape the hub", func(_ *testing.T, f *boardFixture) { f.auth.refuse = true }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newBoardDesk()
			tt.arrange(t, &f)
			rows := len(f.workflows.rows)
			_, err := workflow.MakeBoardWorkflow{Desk: f.desk}.Execute(t.Context(), actor(), collection, "", false)
			if err == nil || detail(err) != tt.detail {
				t.Fatalf("err %v, want the refusal %q", err, tt.detail)
			}
			if len(f.workflows.rows) != rows || f.statuses.mapping != nil || len(f.statuses.given) != 0 || len(f.trail.entries) != 0 {
				t.Fatal("a refused call wrote something")
			}
		})
	}
}

// UC-FLW-02 check 4: the preview of using a workflow names the boards it reaches - the level
// before and after, each column with the statuses it would show - and a board one of whose
// columns is named takes exactly the statuses chosen.
func TestThePreviewOfASchemeNamesTheBoardsItReaches(t *testing.T) {
	f := newBoardDesk()
	if _, err := (workflow.CreateWorkflow{Desk: f.desk}).Execute(t.Context(), actor(), "service", "Service", serviceShape(false)); err != nil {
		t.Fatal(err)
	}
	preview := workflow.PreviewWorkflowScheme{Desk: f.desk}.Descriptor().Handler
	ask := func(columns any) (usecase.Output, error) {
		in := usecase.Input{"container_id": hub.String(), "workflow": "service"}
		if columns != nil {
			in["columns"] = columns
		}
		return preview.Invoke(t.Context(), actor(), in)
	}

	out, err := ask([]any{
		map[string]any{"bucket_id": backlog.String(), "statuses": []any{"todo"}},
		map[string]any{"bucket_id": doing.String(), "statuses": []any{"doing"}},
		map[string]any{"bucket_id": done.String(), "statuses": []any{"done"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	boards, _ := out["boards"].([]usecase.Output)
	if len(boards) != 1 {
		t.Fatalf("boards %+v, want the collection's", out["boards"])
	}
	board := boards[0]
	if board["collection_id"] != collection.String() || board["level_before"] != string(work.StateModeColumns) ||
		board["level_after"] != string(work.StateModeWorkflow) {
		t.Fatalf("board %+v, want the collection going from columns to a workflow", board)
	}
	columns, _ := board["columns"].([]usecase.Output)
	unmapped, _ := board["unmapped"].([]usecase.Output)
	want := map[string]struct {
		status    string
		completes bool
	}{backlog.String(): {"todo", false}, doing.String(): {"doing", false}, done.String(): {"done", true}}
	if len(columns) != 3 || len(unmapped) != 0 {
		t.Fatalf("columns %+v, unmapped %+v", columns, unmapped)
	}
	for _, column := range columns {
		w := want[column["bucket_id"].(string)]
		statuses, _ := column["statuses"].([]string)
		if len(statuses) != 1 || statuses[0] != w.status || column["completes"] != w.completes {
			t.Errorf("column %+v, want %+v", column, w)
		}
	}
	if len(f.statuses.given) != 0 || f.statuses.mapping != nil {
		t.Fatal("a preview wrote the board")
	}

	// Without a choice every status of the workflow is shown by a column or suggested for one.
	out, err = ask(nil)
	if err != nil {
		t.Fatal(err)
	}
	board = out["boards"].([]usecase.Output)[0]
	shown := map[string]bool{}
	for _, column := range board["columns"].([]usecase.Output) {
		for _, status := range column["statuses"].([]string) {
			shown[status] = true
		}
	}
	for _, suggestion := range board["unmapped"].([]usecase.Output) {
		shown[suggestion["status"].(string)] = true
	}
	if !shown["todo"] || !shown["doing"] || !shown["done"] {
		t.Fatalf("the proposal leaves a status out: %+v", board)
	}

	// A column of no board reached, or no column at all, is refused.
	elsewhere := shared.MustParseID("018f0000-0000-7000-8000-0000000000c9")
	if _, err := ask([]any{map[string]any{"bucket_id": elsewhere.String()}}); detail(err) != "buckets.not_found" {
		t.Fatalf("a column of no board reached: %v", err)
	}
	if _, err := ask([]any{map[string]any{"bucket_id": "column"}}); detail(err) != "shared.id_malformed" {
		t.Fatalf("a malformed column: %v", err)
	}
}
