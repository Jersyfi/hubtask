// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	SwitchBoardName = "SwitchBoard"

	// BoardSwitchedAction is the audit code of a list becoming a board or a board a list again.
	BoardSwitchedAction audit.Action = "board.switched"
)

// errDryRun rolls a dry run's transaction back once it has counted.
var errDryRun = errors.New("board switch: dry run")

// SwitchBoard moves a collection's tasks between a list and a board without a workflow (L0 ↔ L1,
// domain-model.md §3.5a): a board starts with an open and a done column, the completed entries in
// the done one; back to a list keeps completion and takes the columns off, the done column last.
// A dry run answers the counts and writes nothing.
type SwitchBoard struct {
	Columns  BucketWriter
	Workflow WorkflowGate
	// Workspaces and Renderer name the two columns in the switching account's language, else the
	// workspace's, as a new collection's starting columns are named.
	Workspaces WorkspaceReader
	Renderer   i18n.Renderer
}

// BoardSwitch is what a switch did, or would do.
type BoardSwitch struct {
	DryRun                  bool
	Before, After           domain.StateMode
	Created, Removed, Moved int
}

// Execute switches the collection's tasks to the level to.
func (h SwitchBoard) Execute(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, to domain.StateMode, dryRun bool,
) (BoardSwitch, error) {
	w := h.Columns
	if to != domain.StateModeNone && to != domain.StateModeColumns {
		return BoardSwitch{}, shared.ErrValidation.WithDetail("board.level_unknown").
			WithFields(shared.FieldError{Path: "/to", Code: "board.level_unknown"})
	}
	var collection domain.Container
	if err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		collection, err = w.Containers.Find(ctx, collectionID)
		return err
	}); err != nil {
		return BoardSwitch{}, err
	}
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: containerPath(collection),
		Action: BoardSwitchedAction, TokenScope: bucketsWrite, TargetType: containerTarget, TargetID: collectionID,
	}); err != nil {
		return BoardSwitch{}, err
	}
	var report BoardSwitch
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		collection, err := w.Containers.Find(ctx, collectionID)
		if err != nil {
			return err
		}
		if err := collection.EnsureEditable(); err != nil {
			return err
		}
		if collection.Type != domain.ContainerCollection {
			return shared.ErrValidation.WithDetail("board.no_columns")
		}
		_, runs, err := h.Workflow.InForce(ctx, collection, domain.EntryTypeKeyOf(domain.WorkItem{Type: domain.ItemTask}))
		if err != nil {
			return err
		}
		if runs {
			return shared.ErrConflict.WithDetail("board.workflow_runs")
		}
		board, err := w.Boards.Of(ctx, collectionID)
		if err != nil {
			return err
		}
		report = BoardSwitch{DryRun: dryRun, Before: domain.StateModeOf(domain.ItemTask, false, len(board.Columns)), After: to}
		if report.Before == to {
			return shared.ErrConflict.WithDetail("board.level_unchanged")
		}
		now := w.Clock.Now()
		if to == domain.StateModeColumns {
			err = h.toBoard(ctx, actor, collection, now, &report)
		} else {
			err = h.toList(ctx, actor, collection, board, now, &report)
		}
		if err != nil {
			return err
		}
		if dryRun {
			return errDryRun
		}
		return h.recordAudit(ctx, actor, collectionID, report, now)
	})
	if err != nil && !errors.Is(err, errDryRun) {
		return BoardSwitch{}, err
	}
	return report, nil
}

// toBoard gives the list an open and a done column, the completed entries in the done one and
// the open ones in the other.
func (h SwitchBoard) toBoard(ctx context.Context, actor appshared.ActorContext, collection domain.Container, now time.Time, report *BoardSwitch) error {
	w := h.Columns
	locale := ""
	if h.Workspaces != nil {
		workspace, err := h.Workspaces.Find(ctx)
		if err != nil {
			return err
		}
		locale = workspace.DefaultLocale
	}
	locale = actor.NamingLocale(locale)
	name := func(code string) string {
		if h.Renderer == nil {
			return code
		}
		return h.Renderer.Render(locale, code, nil)
	}
	orderKey := ""
	var columns []domain.Bucket
	for _, start := range []domain.StartingColumn{
		{NameCode: "seed.bucket.open", ColorToken: "slate"},
		{NameCode: "seed.bucket.done", ColorToken: "green", IsDoneBucket: true},
	} {
		var err error
		if orderKey, err = service.OrderKeyAfter(orderKey); err != nil {
			return err
		}
		column, err := w.addColumn(ctx, actor, collection, domain.NewBucketInput{
			Name: name(start.NameCode), OrderKey: orderKey, IsDoneBucket: start.IsDoneBucket, ColorToken: start.ColorToken,
			Text: w.Text,
		}, now)
		if err != nil {
			return err
		}
		columns = append(columns, column)
		report.Created++
	}
	open, done := columns[0].ID, columns[1].ID
	completed, err := w.Boards.Statuses.PlaceUnplaced(ctx, collection.ID, true, done, now)
	if err != nil {
		return err
	}
	opened, err := w.Boards.Statuses.PlaceUnplaced(ctx, collection.ID, false, open, now)
	if err != nil {
		return err
	}
	report.Moved = len(completed) + len(opened)
	if err := w.recordEntries(ctx, actor, collection.ID, completed, domain.FieldBucketID, done.String()); err != nil {
		return err
	}
	return w.recordEntries(ctx, actor, collection.ID, opened, domain.FieldBucketID, open.String())
}

// toList takes the board's columns off, the done column last; the entries keep their completion
// and stand in no column.
func (h SwitchBoard) toList(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, board domain.Board, now time.Time, report *BoardSwitch,
) error {
	w := h.Columns
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
		moved, err := w.Buckets.MoveItems(ctx, column.ID, "", now)
		if err != nil {
			return err
		}
		if err := w.Buckets.SetDeleted(ctx, deleted, column.Version); err != nil {
			return err
		}
		deleted.Version = column.Version + 1
		announcement, err := event.NewBucketDeleted(w.IDs.NewID(), deleted, "", moved,
			event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
		if err != nil {
			return err
		}
		if err := w.Events.Append(ctx, announcement); err != nil {
			return err
		}
		if err := (DeleteBucket{Writer: w}).recordChange(ctx, deleted, collection, actor); err != nil {
			return err
		}
		report.Removed++
		report.Moved += moved
	}
	return nil
}

// addColumn puts a new column on a board with what creating one owes: the event and the change for
// offline clients.
func (w BucketWriter) addColumn(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, input domain.NewBucketInput, now time.Time,
) (domain.Bucket, error) {
	input.ID, input.TenantID, input.CollectionID, input.Text = w.IDs.NewID(), actor.TenantID, collection.ID, w.Text
	column, err := domain.NewBucket(input)
	if err != nil {
		return domain.Bucket{}, err
	}
	if err := w.Buckets.Insert(ctx, column); err != nil {
		return domain.Bucket{}, err
	}
	announcement, err := event.NewBucketCreated(w.IDs.NewID(), column, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return domain.Bucket{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return domain.Bucket{}, err
	}
	return column, (CreateBucket{Changes: w.Changes, HLC: w.HLC, Text: w.Text}).recordChange(ctx, column, collection, actor, announcement.Payload)
}

// recordAudit writes the switch's evidence: the levels and counts, nothing a person typed.
func (h SwitchBoard) recordAudit(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, report BoardSwitch, now time.Time) error {
	return h.Columns.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now,
		Action: BoardSwitchedAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: containerTarget, TargetID: collectionID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "state_mode", Classification: audit.Open, From: string(report.Before), To: string(report.After)},
			audit.Change{Field: "columns_created", Classification: audit.Open, To: strconv.Itoa(report.Created)},
			audit.Change{Field: "columns_removed", Classification: audit.Open, To: strconv.Itoa(report.Removed)},
			audit.Change{Field: "entries_placed", Classification: audit.Open, To: strconv.Itoa(report.Moved)},
		),
	})
}

// Descriptor is the catalogue entry.
func (h SwitchBoard) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       SwitchBoardName,
		Permission: service.PermissionStructure,
		Summary: "Shows a collection's tasks as a board without a workflow, or as a list again. To " +
			"COLUMNS: an open and a done column, the completed entries in the done one and the open " +
			"ones in the other. To NONE: the columns are taken off, the done column last, and every " +
			"entry keeps its completion. Refused while a workflow runs for tasks there " +
			"(board.workflow_runs) and where the tasks already stand that way (board.level_unchanged). " +
			"A dry run counts and writes nothing.",
		SideEffects: "Creates or deletes the columns, places the entries, announces each column's " +
			"creation or deletion, records them for offline clients and writes an audit entry with the counts.",
		TokenScope: bucketsWrite,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection."},
			{Name: "to", Kind: usecase.KindString, Required: true, Description: "NONE for a list, COLUMNS for a board."},
			{Name: "dry_run", Kind: usecase.KindBool, Description: "Count only; write nothing."},
		},
		Audit: usecase.AuditDeclaration{
			Action: BoardSwitchedAction, TargetType: containerTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a collection's board is its configuration; an entry placed in a column or out of one " +
				"keeps its completion, and its change is recorded for offline clients.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collectionID, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			report, err := h.Execute(ctx, actor, collectionID, domain.StateMode(in.String("to")), in.Bool("dry_run"))
			if err != nil {
				return nil, err
			}
			return usecase.Output{
				"dry_run": report.DryRun, "level_before": string(report.Before), "level_after": string(report.After),
				"columns_created": report.Created, "columns_removed": report.Removed, "entries_placed": report.Moved,
			}, nil
		}),
	}
}
