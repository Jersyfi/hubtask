// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	DropOnColumnName = "DropItemOnColumn"

	// ItemDroppedAction is the audit code of a refused drop; a drop that writes is audited by the
	// column move or the step it runs.
	ItemDroppedAction audit.Action = "item.dropped"
)

// DropItemOnColumn resolves an entry dropped onto a board's column (domain-model.md §3.5a, UC-WRK-05,
// UC-FLW-06): without a workflow it moves the entry, whose completion follows; under one it takes
// the one step that reaches the column, offers the steps where several do, and refuses with a
// reason where none does. The writes are UpdateWorkItem's and TakeStep's, each with its own audit
// and history; a choice writes nothing.
type DropItemOnColumn struct {
	Update UpdateWorkItem
	Step   TakeStep
}

// DropCommand is the input: the column - zero for *Ohne Spalte* - and, after a choice, the step.
type DropCommand struct {
	ItemID          shared.ID
	BucketID        shared.ID
	Step            string
	ExpectedVersion int
}

// DropAnswer is what the drop did: the outcome, the steps taken or to choose among, and the entry
// as it now stands.
type DropAnswer struct {
	Outcome domain.DropOutcome
	Steps   []domain.WorkflowStep
	Item    domain.WorkItem
}

// Execute plans the drop and runs what it resolves to.
func (h DropItemOnColumn) Execute(ctx context.Context, actor appshared.ActorContext, cmd DropCommand) (DropAnswer, error) {
	w := h.Step.Completion
	if cmd.ItemID.IsZero() {
		return DropAnswer{}, itemIDRequired()
	}
	subject, collection, err := w.collectionOf(ctx, actor, cmd.ItemID)
	if err != nil {
		return DropAnswer{}, err
	}
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems, Path: containerPath(collection),
		Action: ItemDroppedAction, TokenScope: itemsWrite, TargetType: itemTarget,
		TargetID: cmd.ItemID, On: changing(subject),
	}); err != nil {
		return DropAnswer{}, err
	}

	var plan domain.DropPlan
	err = w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		plan, err = h.plan(ctx, actor, collection, subject, cmd)
		return err
	})
	if err != nil {
		return DropAnswer{}, err
	}

	answer := DropAnswer{Outcome: plan.Outcome, Steps: plan.Steps, Item: subject}
	switch plan.Outcome {
	case domain.DropRefused:
		return DropAnswer{}, shared.ErrConflict.WithDetail(plan.Reason).
			WithFields(shared.FieldError{Path: "/bucket_id", Code: plan.Reason})
	case domain.DropChoose:
		return answer, nil
	case domain.DropStep:
		answer.Item, err = h.Step.Execute(ctx, actor, TakeStepCommand{
			ItemID: cmd.ItemID, Step: plan.Steps[0].Key, ExpectedVersion: cmd.ExpectedVersion,
		})
		return answer, err
	}
	if subject.Status != "" || subject.BucketID == cmd.BucketID {
		return answer, nil
	}
	target := cmd.BucketID
	answer.Item, err = h.Update.Execute(ctx, actor, UpdateCommand{
		ItemID: cmd.ItemID, Attributes: domain.ItemAttributes{BucketID: &target}, ExpectedVersion: cmd.ExpectedVersion,
	})
	return answer, err
}

// plan reads the board, the workflow in force and the steps the person may take, and resolves the
// drop (Board.PlanDrop); a step named after a choice narrows the steps to it.
func (h DropItemOnColumn) plan(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, item domain.WorkItem, cmd DropCommand,
) (domain.DropPlan, error) {
	w := h.Step.Completion
	board, err := w.Boards.Of(ctx, collection.ID)
	if err != nil {
		return domain.DropPlan{}, err
	}
	if _, live := board.Column(cmd.BucketID); !cmd.BucketID.IsZero() && !live {
		return domain.DropPlan{}, shared.ErrNotFound.WithDetail("buckets.not_found").
			WithFields(shared.FieldError{Path: "/bucket_id", Code: "buckets.not_found"})
	}
	workflow, inForce, err := w.Workflow.InForce(ctx, collection, domain.EntryTypeKeyOf(item))
	if err != nil {
		return domain.DropPlan{}, err
	}
	mode := domain.StateModeOf(item.Type, inForce, len(board.Columns))
	var offered []domain.WorkflowStep
	if inForce {
		item.Status = StatusOf(workflow.Published, item)
		verdicts, err := w.Workflow.Offered(ctx, actor, collection, item, workflow.Key, workflow.Published)
		if err != nil {
			return domain.DropPlan{}, err
		}
		for _, verdict := range verdicts {
			if cmd.Step == "" || verdict.Step.Key == cmd.Step {
				offered = append(offered, verdict.Step)
			}
		}
	}
	return board.PlanDrop(mode, item, cmd.BucketID, workflow.Published, offered), nil
}

// Descriptor is the catalogue entry.
func (h DropItemOnColumn) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       DropOnColumnName,
		Permission: service.PermissionWriteItems,
		Summary: "Drops an entry onto a column of its collection's board, or onto the column of " +
			"unmapped statuses. Without a workflow the entry moves, and entering or leaving the " +
			"done column completes or reopens it. Under a workflow the one step reaching the column " +
			"is taken; where several do, they are answered to choose from and nothing is written; " +
			"where none does, the drop is refused with the reason.",
		SideEffects: "What the column move or the step writes: the entry, its event, history, " +
			"audit and the change for offline clients. A choice writes nothing.",
		TokenScope: itemsWrite,
		Module:     module.Workflows,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry dropped."},
			{Name: "bucket_id", Kind: usecase.KindID, Description: "The column; omitted for the column of unmapped statuses."},
			{Name: "step", Kind: usecase.KindString, Description: "The step chosen after a CHOOSE answer."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for optimistic locking."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemDroppedAction, TargetType: itemTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the column move or the step the drop runs writes the entry's history itself.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DropItemOnColumn) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	var bucketID shared.ID
	if in.String("bucket_id") != "" {
		if bucketID, err = in.ID("bucket_id"); err != nil {
			return nil, err
		}
	}
	answer, err := h.Execute(ctx, actor, DropCommand{
		ItemID: itemID, BucketID: bucketID, Step: in.String("step"), ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	steps := make([]usecase.Output, 0, len(answer.Steps))
	for _, step := range answer.Steps {
		steps = append(steps, usecase.Output{"key": step.Key, "name": stringOrNil(step.Name), "to": step.To})
	}
	item, err := h.Step.Completion.Gate.Answer(ctx, h.Step.Completion.UnitOfWork, actor, answer.Item)
	if err != nil {
		return nil, err
	}
	return usecase.Output{"outcome": string(answer.Outcome), "steps": steps, "item": item}, nil
}
