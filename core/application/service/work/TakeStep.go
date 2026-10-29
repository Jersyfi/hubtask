// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	TakeStepName                     = "TakeStep"
	ListEntryStepsName               = "ListEntrySteps"
	ItemStepTakenAction audit.Action = "item.step_taken"
	ItemStepsReadAction audit.Action = "item.steps_read"
)

// TakeStep moves an entry on by one step of the workflow its collection runs for its type
// (UC-FLW-03): from where it stands, if the person may take it, with what the step requires
// filled in and its checks holding. Into or out of a done status it completes or reopens the
// entry, with the roll-up a completion makes (D-02).
type TakeStep struct {
	Completion CompletionWriter
	// Rules runs the STEP_TAKEN rules on the step inside its transaction (UC-FLW-04 check 4). Nil
	// runs none: a build without the engine.
	Rules StepRules
}

// StepRules runs the rules a step starts, in the step's own change: what their actions change on
// the entry commits with the step or not at all.
type StepRules interface {
	StepTaken(
		ctx context.Context, actor appshared.ActorContext, item domain.WorkItem,
		workflow, step string, announced event.Envelope,
	) error
}

// TakeStepCommand is the input, typed: the step by key, or the status the step leads to - what a
// board's drop and a device's queued move know.
type TakeStepCommand struct {
	ItemID          shared.ID
	Step            string
	ToStatus        string
	ExpectedVersion int
}

// Execute takes the step and returns the entry as it now stands.
func (h TakeStep) Execute(ctx context.Context, actor appshared.ActorContext, cmd TakeStepCommand) (domain.WorkItem, error) {
	w := h.Completion
	if cmd.ItemID.IsZero() {
		return domain.WorkItem{}, shared.ErrValidation.WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}
	if cmd.Step == "" && cmd.ToStatus == "" {
		return domain.WorkItem{}, shared.ErrValidation.WithDetail("workflows.step_required").
			WithFields(shared.FieldError{Path: "/step", Code: "workflows.step_required"})
	}
	subject, collection, err := w.collectionOf(ctx, actor, cmd.ItemID)
	if err != nil {
		return domain.WorkItem{}, err
	}
	// Before the transaction, for the reason CompletionWriter.change gives (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems, Path: containerPath(collection),
		Action: ItemStepTakenAction, TokenScope: itemsWrite, TargetType: itemTarget,
		TargetID: cmd.ItemID, On: changing(subject),
	}); err != nil {
		return domain.WorkItem{}, err
	}

	var changed domain.WorkItem
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		item, err := w.findItem(ctx, cmd.ItemID)
		if err != nil {
			return err
		}
		collection, err := w.findCollection(ctx, item.CollectionID)
		if err != nil {
			return err
		}
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}
		if err := w.Gate.requireShown(ctx, collection, item); err != nil {
			return err
		}
		if err := ensureExpectedVersion(item, cmd.ExpectedVersion); err != nil {
			return err
		}
		workflow, found, err := w.Workflow.InForce(ctx, collection, domain.EntryTypeKeyOf(item))
		if err != nil {
			return err
		}
		if !found {
			return shared.ErrConflict.WithDetail("workflows.not_in_force")
		}
		shape := workflow.Published
		status := StatusOf(shape, item)
		step, err := h.choose(ctx, actor, collection, item, workflow.Key, shape, status, cmd)
		if err != nil {
			return err
		}

		if err := w.Gate.Forms.checkStep(ctx, actor, collection, item, step.Key); err != nil {
			return err
		}

		wanted := item
		completes := shape.Completes(step.To)
		switch {
		case completes && !item.Completion.IsCompleted:
			wanted = item.Completed(actor.AccountID, now)
		case !completes && item.Completion.IsCompleted:
			wanted = item.Reopened(now)
		default:
			wanted.UpdatedAt = now
		}
		wanted.Status = step.To
		before := item
		before.Status = status

		var announcement event.Envelope
		if wanted.Completion.IsCompleted != item.Completion.IsCompleted {
			if err := w.completable(ctx, item); err != nil {
				return err
			}
			want := directionOf(wanted)
			written, completion, err := w.write(ctx, actor, before, wanted, item.Version, want, now, event.Cause{})
			if err != nil {
				return err
			}
			changed, announcement = written, completion
			policy, err := w.completionOf(ctx, collection)
			if err != nil {
				return err
			}
			if err := w.rollUp(ctx, actor, written, policy, now, completion); err != nil {
				return err
			}
			if want == completing {
				if err := w.recordMilestones(ctx, collection, now); err != nil {
					return err
				}
			}
		} else {
			if err := w.Items.SetCompletion(ctx, wanted, item.Version); err != nil {
				return err
			}
			wanted.Version = item.Version + 1
			changed = wanted
			if err := w.Changes.Record(ctx, changelog.Change{
				TenantID: changed.TenantID, Entity: itemTarget, EntityID: changed.ID, Op: changelog.Upsert,
				ContainerID: changed.CollectionID, ActorID: actor.AccountID, HLC: w.HLC.Next(),
				Field:   domain.FieldStatus,
				Payload: map[string]any{domain.FieldStatus: changed.Status, "version": changed.Version},
			}); err != nil {
				return err
			}
		}
		announced, err := h.recordStep(ctx, actor, before, changed, *step, announcement)
		if err != nil {
			return err
		}
		if h.Rules == nil {
			return nil
		}
		if err := h.Rules.StepTaken(ctx, actor, changed, workflow.Key, step.Key, announced); err != nil {
			return err
		}
		// An action may have changed the entry: answered as it now stands.
		current, err := w.findItem(ctx, changed.ID)
		if err != nil {
			return err
		}
		changed = current
		return nil
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return changed, nil
}

// choose answers the step asked for, held to where the entry stands, who may take it, what it
// requires and its checks.
func (h TakeStep) choose(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, item domain.WorkItem,
	workflowKey string, shape domain.WorkflowShape, status string, cmd TakeStepCommand,
) (*domain.WorkflowStep, error) {
	gate := h.Completion.Workflow
	var candidates []domain.WorkflowStep
	if cmd.Step != "" {
		step, found := shape.Step(cmd.Step)
		if !found || !step.LeavesFrom(status) {
			return nil, shared.ErrConflict.WithDetail("workflows.step_not_offered").
				WithParams(map[string]string{"step": cmd.Step, "status": status})
		}
		candidates = []domain.WorkflowStep{step}
	} else {
		for _, step := range shape.StepsFrom(status) {
			if step.To == cmd.ToStatus {
				candidates = append(candidates, step)
			}
		}
		if len(candidates) == 0 {
			return nil, shared.ErrConflict.WithDetail("workflows.step_not_offered").
				WithParams(map[string]string{"step": cmd.ToStatus, "status": status})
		}
	}
	var refusal error
	for _, step := range candidates {
		verdict, err := gate.Judge(ctx, actor, collection, item, workflowKey, status, step)
		if err != nil {
			return nil, err
		}
		if verdict.Holds() {
			return &verdict.Step, nil
		}
		if refusal == nil || verdict.Allowed {
			refusal = verdict.Refusal()
		}
	}
	return nil, refusal
}

// recordStep writes what a step owes beyond the row: the history with who took it and from which
// status (UC-FLW-03 check 6), the event a rule reacts to - `item.updated` with the status and the
// step in its change set, so a step's *then* is a rule like every other (model.md §2) - and the
// evidence, which names the step and no content (rule 10).
func (h TakeStep) recordStep(
	ctx context.Context, actor appshared.ActorContext, before, after domain.WorkItem, step domain.WorkflowStep,
	cause event.Envelope,
) (event.Envelope, error) {
	w := h.Completion
	now := after.UpdatedAt
	if err := w.Activity.record(ctx, actor, after, activity.ItemStepTaken, activity.ChangeSet(activity.Full,
		activity.Field{Name: "step", Detail: activity.WithValues, To: step.Key},
		activity.Field{Name: domain.FieldStatus, Detail: activity.WithValues, From: before.Status, To: after.Status},
	), now); err != nil {
		return event.Envelope{}, err
	}
	causedBy := event.Cause{}
	if !cause.ID.IsZero() {
		causedBy = cause.CausedBy()
	}
	announcement, err := event.NewItemUpdated(w.IDs.NewID(), after, []domain.FieldChange{
		{Field: domain.FieldStatus, From: before.Status, To: after.Status},
		{Field: "step", To: step.Key},
	}, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, causedBy)
	if err != nil {
		return event.Envelope{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return event.Envelope{}, err
	}
	return announcement, w.Audit.Append(ctx, audit.Entry{
		TenantID: after.TenantID, OccurredAt: now, Action: ItemStepTakenAction,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: itemTarget, TargetID: after.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "step", Classification: audit.Open, To: step.Key},
			audit.Change{Field: domain.FieldStatus, Classification: audit.Open, From: before.Status, To: after.Status},
			audit.Change{Field: "collection_id", Classification: audit.Open, To: after.CollectionID.String()},
		),
	})
}

// Descriptor is the catalogue entry: REST, MCP, hubctl, a rule and a device's push reach the one
// handler (UC-FLW-03 check 2).
func (h TakeStep) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       TakeStepName,
		Permission: service.PermissionWriteItems,
		Summary: "Moves an entry on by one step of the workflow its collection runs for its type: " +
			"the step by key, or the status it leads to. Refused when the step does not leave the " +
			"entry's status, the person may not take it, a field it requires is empty or a check " +
			"fails - each named. Into a done status it completes the entry, out of one it reopens it.",
		SideEffects: "Writes the status (and the completion it decides), announces " + string(event.ItemUpdated) +
			" with the status and the step, runs the STEP_TAKEN rules on the step in the same change, " +
			"records a change for offline clients, the entry's history and an audit entry.",
		TokenScope: itemsWrite,
		Module:     module.Workflows,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry to move on."},
			{Name: "step", Kind: usecase.KindString, Description: "The step's key."},
			{Name: "to_status", Kind: usecase.KindString, Description: "The status to move to, where the step's key is not known: the first step there the person may take."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for optimistic locking."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemStepTakenAction, TargetType: itemTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemStepTaken},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			item, err := h.Execute(ctx, actor, TakeStepCommand{
				ItemID: itemID, Step: in.String("step"), ToStatus: in.String("to_status"),
				ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return h.Completion.Gate.Answer(ctx, h.Completion.UnitOfWork, actor, item)
		}),
	}
}

// ListEntrySteps answers where an entry stands in its workflow and the steps the person may take
// from there, with what each asks and requires (UC-FLW-03 check 1).
type ListEntrySteps struct {
	Completion CompletionWriter
}

// EntrySteps is the answer: the workflow, the status and the offered steps; empty where no
// workflow runs.
type EntrySteps struct {
	Workflow domain.Workflow
	InForce  bool
	Status   string
	Offered  []StepVerdict
}

// Execute reads the steps.
func (h ListEntrySteps) Execute(ctx context.Context, actor appshared.ActorContext, itemID shared.ID) (EntrySteps, error) {
	w := h.Completion
	subject, collection, err := w.collectionOf(ctx, actor, itemID)
	if err != nil {
		return EntrySteps{}, err
	}
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: containerPath(collection),
		Action: ItemStepsReadAction, TokenScope: itemsRead, TargetType: itemTarget,
		TargetID: itemID, On: reading(subject),
	}); err != nil {
		return EntrySteps{}, err
	}
	// Offered only to who may change the entry: a reader is shown the status, not a step. Asked
	// before the transaction, for the reason CompletionWriter.change gives.
	mayChange := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems, Path: containerPath(collection),
		Action: ItemStepsReadAction, TokenScope: itemsWrite, TargetType: itemTarget,
		TargetID: itemID, On: changing(subject),
	}) == nil
	var out EntrySteps
	err = w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		workflow, found, err := w.Workflow.InForce(ctx, collection, domain.EntryTypeKeyOf(subject))
		if err != nil || !found {
			return err
		}
		out.Workflow, out.InForce = workflow, true
		out.Status = StatusOf(workflow.Published, subject)
		if !mayChange {
			return nil
		}
		out.Offered, err = w.Workflow.Offered(ctx, actor, collection, subject, workflow.Key, workflow.Published)
		return err
	})
	return out, err
}

// StepOutput is a step as the contract spells it, with how it stands where it is judged.
func StepOutput(verdict StepVerdict) usecase.Output {
	step := verdict.Step
	checks := make([]usecase.Output, 0, len(step.Checks))
	for _, check := range step.Checks {
		checks = append(checks, usecase.Output{"key": check.Key, "holds": verdict.Checks[check.Key], "message": stringOrNil(check.Message)})
	}
	return usecase.Output{
		"key": step.Key, "name": stringOrNil(step.Name), "to": step.To,
		"requires": nonNilStrings(step.Requires), "asks": nonNilStrings(step.Asks),
		"allowed": verdict.Allowed, "missing": nonNilStrings(verdict.Missing), "checks": checks,
		"holds": verdict.Holds(),
	}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// Descriptor is the catalogue entry.
func (h ListEntrySteps) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListEntryStepsName,
		Summary: "Answers the workflow an entry's collection runs for its type, the entry's status, " +
			"and the steps the person may take from it - each with what it asks for, what it still " +
			"needs and whether its checks hold. Empty where no workflow runs.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Module:      module.Workflows,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemStepsReadAction, TargetType: itemTarget, Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			steps, err := h.Execute(ctx, actor, itemID)
			if err != nil {
				return nil, err
			}
			offered := make([]usecase.Output, 0, len(steps.Offered))
			for _, verdict := range steps.Offered {
				offered = append(offered, StepOutput(verdict))
			}
			out := usecase.Output{"workflow": nil, "status": nil, "steps": offered}
			if steps.InForce {
				out["workflow"] = steps.Workflow.Key
				out["status"] = steps.Status
			}
			return out, nil
		}),
	}
}
