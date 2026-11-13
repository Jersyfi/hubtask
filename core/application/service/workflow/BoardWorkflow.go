// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow

import (
	"context"
	"errors"
	"strconv"
	"strings"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// MakeBoardWorkflowName is the catalogue name of *Workflow aus diesen Spalten*.
const MakeBoardWorkflowName = "MakeBoardWorkflow"

// errBoardDryRun rolls a dry run back once it has counted.
var errBoardDryRun = errors.New("board workflow: dry run")

// MakeBoardWorkflow makes a workflow of the collection's hub from a board's columns (L1 → L2,
// domain-model.md §3.5a): a status per column, run for the collection's tasks, each column
// showing its own status, every entry taking the status of the column it stands in - one
// transaction, a dry run answering the same and writing nothing.
type MakeBoardWorkflow struct{ Desk Desk }

// BoardWorkflowStatus is one status made from a column, with the entries that take it.
type BoardWorkflowStatus struct {
	Status   domain.WorkflowStatus
	BucketID shared.ID
	Count    int
}

// BoardWorkflowResult is what the workflow is, or would be.
type BoardWorkflowResult struct {
	DryRun   bool
	Key      string
	Statuses []BoardWorkflowStatus
	Entries  int
}

// ShapeFromColumns answers the shape a board's columns make: a status per column - the first
// column's the initial one, the done column's done, the rest in progress -, every step free, as
// moving between columns was.
func ShapeFromColumns(board domain.Board) (domain.WorkflowShape, []shared.ID) {
	first, _ := board.FirstColumn()
	shape := domain.WorkflowShape{}
	columns := make([]shared.ID, 0, len(board.Columns))
	for i, column := range board.Columns {
		key := "s" + strconv.Itoa(i+1)
		category := domain.CategoryInProgress
		switch {
		case column.IsDoneBucket:
			category = domain.CategoryDone
		case column.ID == first.ID:
			category, shape.Initial = domain.CategoryTodo, key
		}
		shape.Statuses = append(shape.Statuses, domain.WorkflowStatus{
			Key: key, Name: column.Name, Category: category, ColorToken: column.ColorToken,
		})
		shape.Steps = append(shape.Steps, domain.WorkflowStep{Key: "to_" + key, To: key})
		columns = append(columns, column.ID)
	}
	return shape, columns
}

// Execute makes the workflow, or answers what it would be.
func (h MakeBoardWorkflow) Execute(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, name string, dryRun bool,
) (BoardWorkflowResult, error) {
	d := h.Desk
	if !d.Boards.Wired() {
		return BoardWorkflowResult{}, shared.ErrConflict.WithDetail("board.no_columns")
	}
	if err := d.authorizeScheme(ctx, actor, collectionID); err != nil {
		return BoardWorkflowResult{}, err
	}
	var result BoardWorkflowResult
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		collection, err := d.Containers.Find(ctx, collectionID)
		if err != nil {
			return err
		}
		if err := collection.EnsureEditable(); err != nil {
			return err
		}
		if err := d.authorizeAt(ctx, actor, WorkflowCreatedAction, hubOfContainer(collection)); err != nil {
			return err
		}
		tasks := domain.EntryTypeKeyOf(domain.WorkItem{Type: domain.ItemTask})
		if _, runs, err := d.Gate.InForce(ctx, collection, tasks); err != nil || runs {
			return errors.Join(err, runsAlready(runs))
		}
		boards := d.Boards.Columns.Boards
		board, err := boards.Of(ctx, collectionID)
		if err != nil {
			return err
		}
		if len(board.Columns) == 0 || collection.Type != domain.ContainerCollection {
			return shared.ErrConflict.WithDetail("board.no_columns")
		}
		shape, columns := ShapeFromColumns(board)
		cells, err := boards.Statuses.Cells(ctx, collectionID)
		if err != nil {
			return err
		}
		result = BoardWorkflowResult{DryRun: dryRun, Key: "board_" + strings.ReplaceAll(d.IDs.NewID().String(), "-", "")[24:]}
		for i, status := range shape.Statuses {
			counted := 0
			for _, cell := range cells {
				if cell.Status == "" && board.ColumnOfEntry(domain.WorkItem{BucketID: cell.BucketID,
					Completion: domain.Completion{IsCompleted: cell.Completed}}) == columns[i] {
					counted += cell.Count
				}
			}
			result.Statuses = append(result.Statuses, BoardWorkflowStatus{Status: status, BucketID: columns[i], Count: counted})
			result.Entries += counted
		}
		if dryRun {
			return errBoardDryRun
		}
		if name == "" {
			name = collection.Name
		}
		return h.write(ctx, actor, collection, board, shape, columns, name, &result)
	})
	if err != nil && !errors.Is(err, errBoardDryRun) {
		return BoardWorkflowResult{}, err
	}
	return result, nil
}

func runsAlready(runs bool) error {
	if !runs {
		return nil
	}
	return shared.ErrConflict.WithDetail("board.workflow_runs")
}

// write stores the workflow, runs it for the collection's tasks, maps each column to its status
// and gives every entry the status of its column, with its history and its change.
func (h MakeBoardWorkflow) write(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, board domain.Board,
	shape domain.WorkflowShape, columns []shared.ID, name string, result *BoardWorkflowResult,
) error {
	d := h.Desk
	now := d.Clock.Now()
	hub := hubOfContainer(collection)
	created, err := domain.NewWorkflow(domain.NewWorkflowInput{
		ID: d.IDs.NewID(), TenantID: actor.TenantID, Key: result.Key, Name: name, Shape: shape,
		By: actor.AccountID, Now: now, Text: d.Text, HubID: hub,
	})
	if err != nil {
		return err
	}
	if err := d.Workflows.Insert(ctx, created); err != nil {
		return err
	}
	if err := d.announce(ctx, actor, created, changelog.Upsert); err != nil {
		return err
	}
	tasks := domain.EntryTypeKeyOf(domain.WorkItem{Type: domain.ItemTask})
	if err := d.Workflows.SetScheme(ctx, domain.WorkflowScheme{ContainerID: collection.ID, EntryType: tasks, WorkflowKey: created.Key}, now); err != nil {
		return err
	}
	if err := d.announceScheme(ctx, actor); err != nil {
		return err
	}
	mapping := make([]domain.ColumnStatus, 0, len(columns))
	for i, column := range columns {
		mapping = append(mapping, domain.ColumnStatus{BucketID: column, Status: shape.Statuses[i].Key})
	}
	statuses := d.Boards.Columns.Boards.Statuses
	if err := statuses.Replace(ctx, collection.ID, mapping); err != nil {
		return err
	}
	done, _ := board.DoneColumn()
	first, _ := board.FirstColumn()
	for _, row := range mapping {
		given, err := statuses.SetStatusInColumn(ctx, collection.ID, row.BucketID, false, row.Status, now)
		if err != nil {
			return err
		}
		if row.BucketID == done.ID || row.BucketID == first.ID {
			// Entries in no column read as the first column when open, the done one when completed.
			loose, err := statuses.SetStatusInColumn(ctx, collection.ID, "", row.BucketID == done.ID, row.Status, now)
			if err != nil {
				return err
			}
			given = append(given, loose...)
			placed, err := statuses.PlaceByStatus(ctx, collection.ID, row.Status, row.BucketID, now)
			if err != nil {
				return err
			}
			if err := h.recordChange(ctx, actor, collection.ID, placed, domain.FieldBucketID, row.BucketID.String()); err != nil {
				return err
			}
		}
		for _, entry := range given {
			if err := d.Mover.Activity.RecordStep(ctx, actor, actor.TenantID, entry.ID, collection.ID, activity.ItemWorkflowChanged,
				activity.ChangeSet(activity.Full, activity.Field{Name: domain.FieldStatus, Detail: activity.WithValues, To: row.Status}), now); err != nil {
				return err
			}
		}
		if err := h.recordChange(ctx, actor, collection.ID, given, domain.FieldStatus, row.Status); err != nil {
			return err
		}
	}
	return d.trail(ctx, actor, WorkflowCreatedAction, created.ID, now,
		audit.Change{Field: "key", Classification: audit.Open, To: created.Key},
		audit.Change{Field: "statuses", Classification: audit.Open, To: strconv.Itoa(len(shape.Statuses))},
		audit.Change{Field: "entries_moved", Classification: audit.Open, To: strconv.Itoa(result.Entries)})
}

// recordChange tells offline clients the field written on each entry.
func (h MakeBoardWorkflow) recordChange(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, entries []repository.MovedEntry, field, value string,
) error {
	for _, entry := range entries {
		if err := h.Desk.Changes.Record(ctx, changelog.Change{
			TenantID: actor.TenantID, Entity: "item", EntityID: entry.ID, Op: changelog.Upsert,
			ContainerID: collectionID, ActorID: actor.AccountID, HLC: h.Desk.HLC.Next(),
			Field: field, Payload: map[string]any{field: value, "version": entry.Version},
		}); err != nil {
			return err
		}
	}
	return nil
}

// Descriptor is the catalogue entry.
func (h MakeBoardWorkflow) Descriptor() usecase.Descriptor {
	return workflowDescriptor(MakeBoardWorkflowName,
		"Makes a workflow of the collection's hub from its board's columns - a status per column, the "+
			"first column's the initial one, the done column's done, every step free - and runs it for "+
			"the collection's tasks, each column showing its own status and every entry taking the "+
			"status of its column. Refused without columns (board.no_columns) and where a workflow "+
			"already runs for tasks (board.workflow_runs). A dry run answers the same and writes nothing.",
		"Creates the workflow, sets the scheme, maps the columns and gives the entries their status, "+
			"with each entry's history and change; writes an audit entry with the counts.",
		typesWrite, false, WorkflowCreatedAction,
		[]usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection."},
			{Name: "name", Kind: usecase.KindString, Description: "The workflow's name; the collection's by default."},
			{Name: "dry_run", Kind: usecase.KindBool, Description: "Answer only; write nothing."},
		},
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collectionID, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			result, err := h.Execute(ctx, actor, collectionID, in.String("name"), in.Bool("dry_run"))
			if err != nil {
				return nil, err
			}
			statuses := make([]usecase.Output, 0, len(result.Statuses))
			for _, made := range result.Statuses {
				statuses = append(statuses, usecase.Output{
					"key": made.Status.Key, "name": made.Status.Name, "category": string(made.Status.Category),
					"bucket_id": made.BucketID.String(), "count": made.Count,
				})
			}
			return usecase.Output{"dry_run": result.DryRun, "workflow": result.Key, "statuses": statuses, "entries": result.Entries}, nil
		})
}
