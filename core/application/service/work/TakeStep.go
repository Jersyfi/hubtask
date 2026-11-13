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
// entry, with the roll-up a completion makes (UC-FLW-03 check 4).
type TakeStep struct {
	Completion CompletionWriter
	// Rules runs the STEP_TAKEN rules on the step inside its transaction (UC-FLW-04 check 4). Nil
	// runs none: a build without the engine.
	Rules StepRules
	// Approvals opens the approval a step waits for (UC-SVC-08). Nil refuses such a step.
	Approvals StepApprovals
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
		// A step that waits for people's yes opens its approval and leaves the entry where it
		// stands; the decision takes the step (UC-SVC-08 check 3).
		if step.Approval != nil && !TakenByApproval(ctx) {
			if h.Approvals == nil {
				return shared.ErrConflict.WithDetail("approvals.not_available")
			}
			changed = item
			return h.Approvals.Open(ctx, actor, item, collection, workflow.Key, *step)
		}
		if h.Approvals != nil {
			if err := h.Approvals.Moved(ctx, item); err != nil {
				return err
			}
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
		// Under a workflow the column follows the status on every step (domain-model.md §3.5a).
		if wanted, err = w.Boards.couple(ctx, item, wanted, actor.AccountID, now); err != nil {
			return err
		}
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
				Field: domain.FieldStatus,
				Payload: map[string]any{
					domain.FieldStatus: changed.Status, domain.FieldBucketID: idOrNil(changed.BucketID),
					"version": changed.Version,
				},
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
// from there, with what each asks and requires (UC-FLW-03 check 1); beside them the steps they may
// not take with the reason, the fewest steps into done, and the workflow's picture (UC-FLW-06).
type ListEntrySteps struct {
	Completion CompletionWriter
}

// EntrySteps is the answer: the workflow, the status, the offered and the blocked steps, the route
// to done and the picture; where no workflow runs, the default picture and nothing else.
type EntrySteps struct {
	Workflow  domain.Workflow
	InForce   bool
	Status    string
	Offered   []StepVerdict
	Blocked   []StepVerdict
	DoneRoute []domain.WorkflowStep
	Picture   domain.WorkflowShape
	// ByDefault is true where no workflow runs: the picture is the system workflow's, every status
	// reachable, and completing the entry reaches done.
	ByDefault bool
	// MayDesign reports whether the person may open the workflow in the designer.
	MayDesign bool
	// StateMode is the entry type's level in its collection (domain-model.md §3.5a); without a
	// workflow on a board, Columns are the entry's states - the done column completing - and
	// Column is the one it reads as.
	StateMode domain.StateMode
	Columns   []domain.Bucket
	Column    shared.ID
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
	out := EntrySteps{
		Picture: domain.SystemWorkflow().Published, ByDefault: true, DoneRoute: []domain.WorkflowStep{},
		MayDesign: w.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: service.PermissionStructure, Path: containerPath(collection),
			Action: ItemStepsReadAction, TargetType: itemTarget, TargetID: itemID, On: reading(subject),
		}) == nil,
	}
	err = w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		workflow, found, err := w.Workflow.InForce(ctx, collection, domain.EntryTypeKeyOf(subject))
		if err != nil {
			return err
		}
		if !found {
			return h.columns(ctx, collection, subject, &out)
		}
		out.StateMode = domain.StateModeWorkflow
		out.Workflow, out.InForce, out.ByDefault = workflow, true, false
		out.Picture = workflow.Published
		out.Status = StatusOf(workflow.Published, subject)
		out.DoneRoute = workflow.Published.RouteToDone(out.Status)
		if !mayChange {
			return nil
		}
		out.Offered, out.Blocked, err = w.Workflow.Standing(ctx, actor, collection, subject, workflow.Key, workflow.Published)
		return err
	})
	return out, err
}

// columns answers the level of an entry without a workflow, and at L1 the columns as its states.
func (h ListEntrySteps) columns(ctx context.Context, collection domain.Container, subject domain.WorkItem, out *EntrySteps) error {
	out.StateMode = domain.StateModeNone
	if !h.Completion.Boards.Wired() {
		return nil
	}
	board, err := h.Completion.Boards.Of(ctx, collection.ID)
	if err != nil {
		return err
	}
	if out.StateMode = domain.StateModeOf(subject.Type, false, len(board.Columns)); out.StateMode == domain.StateModeColumns {
		out.Columns, out.Column = board.Columns, board.ColumnOfEntry(subject)
	}
	return nil
}

// StepOutput is a step as the contract spells it, with how it stands where it is judged.
func StepOutput(verdict StepVerdict) usecase.Output {
	step := verdict.Step
	checks := make([]usecase.Output, 0, len(step.Checks))
	for _, check := range step.Checks {
		messages := make(map[string]any, len(check.Messages))
		for language, words := range check.Messages {
			messages[language] = words
		}
		checks = append(checks, usecase.Output{
			"key": check.Key, "holds": verdict.Checks[check.Key], "message": stringOrNil(check.Message),
			"messages": messages, "message_code": stringOrNil(check.MessageCode),
		})
	}
	return usecase.Output{
		"key": step.Key, "name": stringOrNil(step.Name), "to": step.To,
		"requires": nonNilStrings(step.Requires), "asks": nonNilStrings(step.Asks),
		"allowed": verdict.Allowed, "missing": nonNilStrings(verdict.Missing), "checks": checks,
		"holds": verdict.Holds(), "approval_levels": approvalLevels(step),
	}
}

func approvalLevels(step domain.WorkflowStep) int {
	if step.Approval == nil {
		return 0
	}
	return len(step.Approval.Levels)
}

// BlockedOutput is a step the person may not take as the contract spells it: the reason and who
// may (UC-FLW-06 check 1).
func BlockedOutput(workflowKey string, verdict StepVerdict) usecase.Output {
	step := verdict.Step
	people := make([]string, 0, len(verdict.People))
	for _, person := range verdict.People {
		people = append(people, person.String())
	}
	return usecase.Output{
		"key": step.Key, "name": stringOrNil(step.Name), "to": step.To, "reason": verdict.Reason,
		"roles": nonNilStrings(step.Who.Roles), "people": people, "groups": nonNilStrings(step.Who.Groups),
		"permission": string(service.TakeStep(workflowKey, step.Key)),
	}
}

// PictureOutput is a workflow drawn read-only: its statuses and steps without who may take them.
func PictureOutput(shape domain.WorkflowShape) usecase.Output {
	statuses := make([]usecase.Output, 0, len(shape.Statuses))
	for _, status := range shape.Statuses {
		statuses = append(statuses, usecase.Output{
			"key": status.Key, "name": stringOrNil(status.Name), "category": string(status.Category),
			"color_token": status.ColorToken,
		})
	}
	steps := make([]usecase.Output, 0, len(shape.Steps))
	for _, step := range shape.Steps {
		steps = append(steps, usecase.Output{
			"key": step.Key, "name": stringOrNil(step.Name), "from": nonNilStrings(step.From), "to": step.To,
			"limited": !step.Who.IsOpen(), "approval": step.Approval != nil,
		})
	}
	return usecase.Output{"statuses": statuses, "initial": shape.Initial, "steps": steps}
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
		Permission: service.PermissionRead,
		Name:       ListEntryStepsName,
		Summary: "Answers the workflow an entry's collection runs for its type, the entry's status, " +
			"and the steps the person may take from it - each with what it asks for, what it still " +
			"needs and whether its checks hold - beside the steps they may not take with the reason " +
			"and who may, the fewest steps into done, and the workflow's read-only picture. Where " +
			"no workflow runs, the default picture.",
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
			blocked := make([]usecase.Output, 0, len(steps.Blocked))
			for _, verdict := range steps.Blocked {
				blocked = append(blocked, BlockedOutput(steps.Workflow.Key, verdict))
			}
			route := make([]usecase.Output, 0, len(steps.DoneRoute))
			for _, step := range steps.DoneRoute {
				route = append(route, usecase.Output{"key": step.Key, "name": stringOrNil(step.Name), "to": step.To})
			}
			out := usecase.Output{
				"workflow": nil, "status": nil, "steps": offered, "blocked": blocked, "done_route": route,
				"picture": PictureOutput(steps.Picture), "by_default": steps.ByDefault, "may_design": steps.MayDesign,
				"state_mode": string(steps.StateMode), "column_id": idOrNil(steps.Column),
			}
			columns := make([]usecase.Output, 0, len(steps.Columns))
			for _, column := range steps.Columns {
				columns = append(columns, usecase.Output{
					"id": column.ID.String(), "name": column.Name, "completes": column.IsDoneBucket,
				})
			}
			out["columns"] = columns
			if steps.InForce {
				out["workflow"] = steps.Workflow.Key
				out["status"] = steps.Status
			}
			return out, nil
		}),
	}
}
