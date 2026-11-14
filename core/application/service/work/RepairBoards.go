// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	RepairBoardsName = "RepairBoards"

	// BoardsRepairedAction is the audit code of a repair that wrote; it carries counts only.
	BoardsRepairedAction audit.Action = "boards.repaired"
)

// RepairBoards brings every board of the workspace to the model of domain-model.md §3.5a once:
// without a workflow exactly one done column, completed entries in it and the entries in it
// completed; under a workflow - every workflow the board's entry types run - a board mapping no
// status given the proposed mapping, then a column for each status no column shows; and the unused starting
// columns of a collection whose default view is not the board taken off, the done column last.
// An administrator's one-off (the pattern of ReindexSearch): a dry run counts and writes nothing,
// and a second run finds nothing to do.
type RepairBoards struct {
	Boards    Boards
	Workflow  WorkflowGate
	Structure StructureResolver
	// Columns writes the columns' changes and records them for offline clients.
	Columns BucketWriter
	// Types is the registry, which tells the entry types standing on a board; nil takes the types
	// the scheme names.
	Types repository.EntryTypes
}

// BoardRepair is what a run found and, unless it was a dry run, did.
type BoardRepair struct {
	DryRun         bool
	Boards         int
	DoneMarked     int
	MovedIntoDone  int
	Completed      int
	ColumnsAdded   int
	PlacedByStatus int
	ColumnsRemoved int
	StatusesMapped int
}

// errDryRun rolls a dry run's transaction back once it has counted.
var errDryRun = errors.New("board repair: dry run")

// Execute repairs the boards, or counts what it would repair.
func (h RepairBoards) Execute(ctx context.Context, actor appshared.ActorContext, dryRun bool) (BoardRepair, error) {
	w := h.Columns
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: []identity.Scope{identity.TenantScope()},
		Action: BoardsRepairedAction, TokenScope: workspaceManage,
		TargetType: workspaceTarget, TargetID: actor.TenantID,
	}); err != nil {
		return BoardRepair{}, err
	}
	var report BoardRepair
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		report = BoardRepair{DryRun: dryRun}
		now := w.Clock.Now()
		collections, err := h.Boards.Statuses.Collections(ctx)
		if err != nil {
			return err
		}
		for _, id := range collections {
			if err := h.repair(ctx, actor, id, now, &report); err != nil {
				return err
			}
		}
		if dryRun {
			return errDryRun
		}
		return h.recordAudit(ctx, actor, report, now)
	})
	if err != nil && !errors.Is(err, errDryRun) {
		return BoardRepair{}, err
	}
	return report, nil
}

// repair brings one board to the model.
func (h RepairBoards) repair(ctx context.Context, actor appshared.ActorContext, id shared.ID, now time.Time, report *BoardRepair) error {
	collection, err := findCollection(ctx, h.Columns.Containers, id)
	if err != nil {
		return err
	}
	board, err := h.Boards.Of(ctx, id)
	if err != nil || len(board.Columns) == 0 {
		return err
	}
	report.Boards++
	shape, err := h.Workflow.OnBoard(ctx, collection, h.Types)
	if err != nil {
		return err
	}
	if shape != nil {
		return h.repairMapped(ctx, actor, collection, board, *shape, now, report)
	}
	unused, err := h.unusedStartingColumns(ctx, collection, board)
	if err != nil {
		return err
	}
	if unused {
		return h.removeColumns(ctx, actor, collection, board, now, report)
	}
	return h.repairColumns(ctx, actor, collection, board, now, report)
}

// repairColumns gives a board without a workflow one done column - the rightmost marked one, else
// the rightmost column -, moves the completed entries into it and completes the open ones in it.
func (h RepairBoards) repairColumns(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, board domain.Board, now time.Time, report *BoardRepair,
) error {
	keep := board.Columns[len(board.Columns)-1]
	marks := 0
	for _, column := range board.Columns {
		if column.IsDoneBucket {
			keep = column
			marks++
		}
	}
	if marks != 1 {
		report.DoneMarked++
		marked, cleared := true, false
		for _, column := range board.Columns {
			want := &cleared
			if column.ID == keep.ID {
				want = &marked
			}
			if err := h.mark(ctx, actor, column, want, collection); err != nil {
				return err
			}
		}
	}
	moved, err := h.Boards.Statuses.PlaceByCompletion(ctx, collection.ID, true, "", keep.ID, now)
	if err != nil {
		return err
	}
	report.MovedIntoDone += len(moved)
	completed, err := h.Boards.Statuses.CompleteInColumn(ctx, collection.ID, keep.ID, actor.AccountID, now)
	if err != nil {
		return err
	}
	report.Completed += len(completed)
	if err := h.Columns.recordEntries(ctx, actor, collection.ID, moved, domain.FieldBucketID, idOrNil(keep.ID)); err != nil {
		return err
	}
	return h.Columns.recordCompleted(ctx, actor, collection.ID, completed, causeBoardRepair, now)
}

// repairMapped maps a board that maps no status by the proposal (domain.DefaultMapping), gives
// each status still no column shows a column of its own, in the workflow's order with the done
// column last (domain.Board.PlaceOfStatus), and puts every entry
// into the column showing its status. The shape is every workflow the board's types run
// (domain.BoardShape). Statuses carry no work-in-progress limit, so the new columns carry none.
func (h RepairBoards) repairMapped(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, board domain.Board,
	shape domain.WorkflowShape, now time.Time, report *BoardRepair,
) error {
	w := h.Columns
	proposed := 0
	if len(board.Mapping) == 0 {
		board.Mapping, _ = domain.DefaultMapping(board.Columns, shape)
		proposed = len(board.Mapping)
		report.StatusesMapped += proposed
	}
	unmapped := board.Unmapped(shape)
	for _, key := range unmapped {
		status, _ := shape.Status(key)
		name := status.Name
		if name == "" {
			name = status.Key
		}
		orderKey, err := service.OrderKeyBetween(board.PlaceOfStatus(shape, key))
		if err != nil {
			return err
		}
		column, err := domain.NewBucket(domain.NewBucketInput{
			ID: w.IDs.NewID(), TenantID: actor.TenantID, CollectionID: collection.ID, Name: name,
			OrderKey: orderKey, ColorToken: status.ColorToken, Text: w.Text,
		})
		if err != nil {
			return err
		}
		if err := w.Buckets.Insert(ctx, column); err != nil {
			return err
		}
		announcement, err := event.NewBucketCreated(w.IDs.NewID(), column, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
		if err != nil {
			return err
		}
		if err := w.Events.Append(ctx, announcement); err != nil {
			return err
		}
		if err := (CreateBucket{Changes: w.Changes, HLC: w.HLC, Text: w.Text}).recordChange(ctx, column, collection, actor, announcement.Payload); err != nil {
			return err
		}
		board.Columns = slices.Insert(board.Columns, columnPlace(board.Columns, orderKey), column)
		board.Mapping = append(board.Mapping, domain.ColumnStatus{BucketID: column.ID, Status: key})
		report.ColumnsAdded++
	}
	if len(unmapped) > 0 || proposed > 0 {
		if err := h.Boards.Statuses.Replace(ctx, collection.ID, board.Mapping); err != nil {
			return err
		}
		var err error
		if board, err = h.Boards.Of(ctx, collection.ID); err != nil {
			return err
		}
	}
	for _, status := range shape.Statuses {
		column, _ := board.ColumnOfStatus(status.Key)
		placed, err := h.Boards.Statuses.PlaceByStatus(ctx, collection.ID, status.Key, column, now)
		if err != nil {
			return err
		}
		report.PlacedByStatus += len(placed)
		if err := h.Columns.recordEntries(ctx, actor, collection.ID, placed, domain.FieldBucketID, idOrNil(column)); err != nil {
			return err
		}
	}
	return nil
}

// columnPlace answers where a column with the order key stands among columns ordered left to right.
func columnPlace(columns []domain.Bucket, orderKey string) int {
	for index, column := range columns {
		if column.OrderKey > orderKey {
			return index
		}
	}
	return len(columns)
}

// unusedStartingColumns reports a board that is only the starting columns a collection was given
// where the board was not its default view (Projekt before §3.5a): to do, in progress and done,
// no entry in any of them, no mapping, and the board still not the default view.
func (h RepairBoards) unusedStartingColumns(ctx context.Context, collection domain.Container, board domain.Board) (bool, error) {
	if len(board.Columns) != 3 || len(board.Mapping) > 0 || !board.Columns[2].IsDoneBucket ||
		board.Columns[0].IsDoneBucket || board.Columns[1].IsDoneBucket || !h.Structure.Wired() {
		return false, nil
	}
	effective, err := h.Structure.within(repository.LayerRead).Of(ctx, collection)
	if err != nil {
		return false, err
	}
	if views := effective.Values.Views; views != nil && views.Default == domain.ViewKanban {
		return false, nil
	}
	entries, err := h.Boards.Statuses.EntriesOnBoard(ctx, collection.ID)
	return entries == 0, err
}

// removeColumns soft-deletes a board's columns, the done column last.
func (h RepairBoards) removeColumns(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, board domain.Board, now time.Time, report *BoardRepair,
) error {
	ordered := make([]domain.Bucket, 0, len(board.Columns))
	var done []domain.Bucket
	for _, column := range board.Columns {
		if column.IsDoneBucket {
			done = append(done, column)
			continue
		}
		ordered = append(ordered, column)
	}
	for _, column := range append(ordered, done...) {
		deleted, changes, err := column.Deleted(now)
		if err != nil || len(changes) == 0 {
			return err
		}
		if err := h.Columns.Buckets.SetDeleted(ctx, deleted, column.Version); err != nil {
			return err
		}
		deleted.Version = column.Version + 1
		if err := (DeleteBucket{Writer: h.Columns}).recordChange(ctx, deleted, collection, actor); err != nil {
			return err
		}
		report.ColumnsRemoved++
	}
	return nil
}

// mark writes one column's done mark and records it for offline clients.
func (h RepairBoards) mark(ctx context.Context, actor appshared.ActorContext, column domain.Bucket, done *bool, collection domain.Container) error {
	w := h.Columns
	wanted, changes, err := column.Updated(domain.BucketAttributes{IsDoneBucket: done}, w.Text)
	if err != nil || len(changes) == 0 {
		return err
	}
	if err := w.Buckets.SetAttributes(ctx, wanted, column.Version); err != nil {
		return err
	}
	wanted.Version = column.Version + 1
	return w.recordChanges(ctx, wanted, collection, actor, changes)
}

// recordAudit writes the run's evidence: counts only, nothing a person typed (rule 10).
func (h RepairBoards) recordAudit(ctx context.Context, actor appshared.ActorContext, report BoardRepair, now time.Time) error {
	count := func(field string, n int) audit.Change {
		return audit.Change{Field: field, Classification: audit.Open, To: strconv.Itoa(n)}
	}
	return h.Columns.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now,
		Action: BoardsRepairedAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: workspaceTarget, TargetID: actor.TenantID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			count("boards", report.Boards), count("done_marked", report.DoneMarked),
			count("moved_into_done", report.MovedIntoDone), count("completed", report.Completed),
			count("columns_added", report.ColumnsAdded), count("placed_by_status", report.PlacedByStatus),
			count("columns_removed", report.ColumnsRemoved),
			count("statuses_mapped", report.StatusesMapped),
		),
	})
}

// Descriptor is the catalogue entry.
func (h RepairBoards) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       RepairBoardsName,
		Permission: service.PermissionStructure,
		Summary: "Brings every board of the workspace to one model, once: without a workflow " +
			"exactly one done column, the completed entries in it and the open entries in it " +
			"completed; under a workflow - every workflow the board's entry types run - a board " +
			"mapping no status given the proposed mapping, a column for each status no column shows " +
			"and every entry in the column of its status; the unused starting columns of a collection whose default " +
			"view is not the board taken off, the done column last. A dry run counts and writes " +
			"nothing; a second run finds nothing to do.",
		SideEffects: "Writes the columns and entries it repairs, records them for offline clients, " +
			"and writes one audit entry with the counts. A dry run writes nothing.",
		TokenScope: workspaceManage,
		Input: []usecase.Field{
			{Name: "dry_run", Kind: usecase.KindBool, Description: "Count only; write nothing."},
		},
		Audit: usecase.AuditDeclaration{
			Action: BoardsRepairedAction, TargetType: workspaceTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a repair of the workspace's boards, not something a person did to an entry; " +
				"the audit entry carries its counts.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			report, err := h.Execute(ctx, actor, in.Bool("dry_run"))
			if err != nil {
				return nil, err
			}
			return usecase.Output{
				"dry_run": report.DryRun, "boards": report.Boards, "done_marked": report.DoneMarked,
				"moved_into_done": report.MovedIntoDone, "completed": report.Completed,
				"columns_added": report.ColumnsAdded, "placed_by_status": report.PlacedByStatus,
				"columns_removed": report.ColumnsRemoved, "statuses_mapped": report.StatusesMapped,
			}, nil
		}),
	}
}
