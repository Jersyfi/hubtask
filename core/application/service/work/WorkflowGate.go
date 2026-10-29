// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"strings"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

// WorkflowGate answers the workflow in force for an entry and holds a step to it (F-4, P-18):
// asked inside each writing use case's transaction, so a step taken through the status control,
// a board, the bulk action, completion on any door or a device's push meets the same answer
// (UC-FLW-03 check 2). A build without it runs no workflow anywhere.
type WorkflowGate struct {
	Workflows repository.Workflows
	Compiler  expression.Compiler
	// Roles answers the actor's role along the entry's path, for a step limited by role.
	Roles FormRoles
}

// Wired reports whether the gate has what it needs.
func (g WorkflowGate) Wired() bool { return g.Workflows != nil }

// InForce answers the workflow in force for an entry type in a collection, and false where none
// runs.
func (g WorkflowGate) InForce(ctx context.Context, collection domain.Container, entryType string) (domain.Workflow, bool, error) {
	if !g.Wired() {
		return domain.Workflow{}, false, nil
	}
	schemes, err := g.Workflows.Schemes(ctx)
	if err != nil || len(schemes) == 0 {
		return domain.Workflow{}, false, err
	}
	key, found := domain.ResolveWorkflow(schemes, collection.ID, collection.ParentID, entryType)
	if !found {
		return domain.Workflow{}, false, nil
	}
	rows, err := g.Workflows.List(ctx)
	if err != nil {
		return domain.Workflow{}, false, err
	}
	workflow, found := domain.NewWorkflows(rows).Find(key)
	return workflow, found, nil
}

// StatusOf answers where an entry stands in a shape: its status, or where it enters the shape
// when it carries none the shape knows - an entry created before its collection ran the workflow.
func StatusOf(shape domain.WorkflowShape, item domain.WorkItem) string {
	return shape.Entering(item.Status, item.Completion.IsCompleted)
}

// StepVerdict is how a step stands for one person and one entry: whether they may take it, what
// is missing and which checks fail (UC-FLW-04 check 5).
type StepVerdict struct {
	Step    domain.WorkflowStep
	Allowed bool
	Missing []string
	Checks  map[string]bool
}

// Holds reports whether the step may be taken now.
func (v StepVerdict) Holds() bool {
	if !v.Allowed || len(v.Missing) > 0 {
		return false
	}
	for _, held := range v.Checks {
		if !held {
			return false
		}
	}
	return true
}

// Refusal is the error a step that does not hold answers.
func (v StepVerdict) Refusal() error {
	if !v.Allowed {
		return shared.ErrForbidden.WithDetail("workflows.step_not_allowed").
			WithParams(map[string]string{"step": v.Step.Key})
	}
	var failed []domain.StepCheck
	for _, check := range v.Step.Checks {
		if !v.Checks[check.Key] {
			failed = append(failed, check)
		}
	}
	return domain.StepRefused(v.Step, v.Missing, failed)
}

// Judge answers how a step stands for the actor and the entry, changing nothing.
func (g WorkflowGate) Judge(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, item domain.WorkItem,
	status string, step domain.WorkflowStep,
) (StepVerdict, error) {
	verdict := StepVerdict{Step: step, Checks: map[string]bool{}}
	activation, err := g.activation(ctx, actor, collection, item, status, step)
	if err != nil {
		return StepVerdict{}, err
	}
	verdict.Allowed, err = g.allowed(ctx, actor, item, step, activation)
	if err != nil {
		return StepVerdict{}, err
	}
	verdict.Missing = domain.StepRequirementsMissing(step, item)
	for _, check := range step.Checks {
		held, err := g.holds(ctx, check.Holds, activation)
		if err != nil {
			return StepVerdict{}, err
		}
		verdict.Checks[check.Key] = held
	}
	return verdict, nil
}

func (g WorkflowGate) allowed(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem, step domain.WorkflowStep,
	activation formActivation,
) (bool, error) {
	who := step.Who
	if who.IsOpen() {
		return true, nil
	}
	if len(who.Roles) > 0 || len(who.PersonFields) > 0 {
		met := false
		role, _ := activation.values["actor"].(map[string]any)["role"].(string)
		if slices.Contains(who.Roles, role) {
			met = true
		}
		for _, field := range who.PersonFields {
			if namesPerson(item, field, actor.AccountID) {
				met = true
			}
		}
		if !met {
			return false, nil
		}
	}
	if who.Condition == "" {
		return true, nil
	}
	return g.holds(ctx, who.Condition, activation)
}

// namesPerson reports whether a person field of the entry names the account.
func namesPerson(item domain.WorkItem, field string, account shared.ID) bool {
	switch field {
	case "assignee":
		return !item.AssigneeID.IsZero() && item.AssigneeID == account
	case "created_by":
		return item.CreatedBy == account
	}
	key, _ := strings.CutPrefix(field, domain.FormCustomPrefix)
	switch value := item.CustomFields[key].(type) {
	case string:
		return value == account.String()
	case []any:
		for _, one := range value {
			if text, ok := one.(string); ok && text == account.String() {
				return true
			}
		}
	case []string:
		return slices.Contains(value, account.String())
	}
	return false
}

func (g WorkflowGate) holds(ctx context.Context, condition string, activation formActivation) (bool, error) {
	if g.Compiler == nil {
		return false, shared.ErrInternal.WithDetail("workflows.compiler_missing")
	}
	program, err := g.Compiler.Compile(condition, WorkflowEnvironment(), expression.Boolean)
	if err != nil {
		return false, shared.ErrConflict.WithDetail("workflows.condition_broken")
	}
	value, err := program.Evaluate(ctx, activation)
	if err != nil {
		return false, err
	}
	return value.Bool, nil
}

func (g WorkflowGate) activation(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, item domain.WorkItem,
	status string, step domain.WorkflowStep,
) (formActivation, error) {
	role := ""
	if g.Roles != nil {
		held, found, err := g.Roles.RoleAlong(ctx, actor, containerPath(collection))
		if err != nil {
			return formActivation{}, err
		}
		if found {
			role = string(held)
		}
	}
	custom := make(map[string]any, len(item.CustomFields))
	for key, value := range item.CustomFields {
		custom[key] = value
	}
	return formActivation{values: map[string]any{
		"entry": map[string]any{
			"title": item.Title, "notes": item.Notes, "level": string(item.Type),
			"entry_type": domain.EntryTypeKeyOf(item), "completed": item.Completion.IsCompleted,
			"due_date":      domain.FormValue(item, domain.FormKeyDueDate),
			"start_at":      domain.FormValue(item, domain.FormKeyStartAt),
			"custom_fields": custom,
			"status":        status,
		},
		"actor":   map[string]any{"role": role, "kind": string(actor.Kind)},
		"channel": string(FormChannelOf(actor)),
		"step":    step.Key,
	}}, nil
}

// WorkflowEnvironment is a step condition's CEL environment: the form's (FormEnvironment), with the
// entry's status beside its fields - the same language and the same names as a form's conditions
// (UC-FLW-04 check 6).
func WorkflowEnvironment() expression.Environment {
	env := FormEnvironment(nil)
	for i, variable := range env.Variables {
		if variable.Name == "entry" {
			fields := slices.Clone(variable.Fields)
			fields = append(fields, expression.Variable{Name: "status", Kind: expression.KindString})
			env.Variables[i].Fields = fields
		}
	}
	return env
}

// Offered answers the steps the actor may take from where the entry stands, in the workflow's
// order (UC-FLW-03 check 1): a step the person may not take is not offered at all.
func (g WorkflowGate) Offered(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, item domain.WorkItem,
	shape domain.WorkflowShape,
) ([]StepVerdict, error) {
	status := StatusOf(shape, item)
	var offered []StepVerdict
	for _, step := range shape.StepsFrom(status) {
		verdict, err := g.Judge(ctx, actor, collection, item, status, step)
		if err != nil {
			return nil, err
		}
		if verdict.Allowed {
			offered = append(offered, verdict)
		}
	}
	return offered, nil
}

// completionStep answers the step a completion or a reopening takes in a workflow collection
// (UC-FLW-03 check 4): the first step the actor may take from where the entry stands into a status
// on the asked side of completion. Nil where no workflow runs; refused where no such step is
// offered, or the one offered does not hold.
func (g WorkflowGate) completionStep(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, item domain.WorkItem,
	completing bool,
) (*domain.WorkflowStep, error) {
	workflow, found, err := g.InForce(ctx, collection, domain.EntryTypeKeyOf(item))
	if err != nil || !found {
		return nil, err
	}
	shape := workflow.Published
	status := StatusOf(shape, item)
	var refusal error
	for _, step := range shape.StepsFrom(status) {
		if shape.Completes(step.To) != completing {
			continue
		}
		verdict, err := g.Judge(ctx, actor, collection, item, status, step)
		if err != nil {
			return nil, err
		}
		if verdict.Holds() {
			return &verdict.Step, nil
		}
		if refusal == nil && verdict.Allowed {
			refusal = verdict.Refusal()
		}
	}
	if refusal != nil {
		return nil, refusal
	}
	code := "workflows.no_step_to_open"
	if completing {
		code = "workflows.no_step_to_done"
	}
	return nil, shared.ErrValidation.WithDetail(code).WithParams(map[string]string{"status": status})
}

// rolledStatus answers the status a parent takes when a roll-up completes or reopens it: a
// consequence takes no step, so it lands where entering the workflow would put it.
func (g WorkflowGate) rolledStatus(ctx context.Context, collection domain.Container, item domain.WorkItem) (string, error) {
	workflow, found, err := g.InForce(ctx, collection, domain.EntryTypeKeyOf(item))
	if err != nil || !found {
		return item.Status, err
	}
	return workflow.Published.Entering(item.Status, item.Completion.IsCompleted), nil
}

// initialStatus answers the status a new entry starts in, empty where no workflow runs.
func (g WorkflowGate) initialStatus(ctx context.Context, collection domain.Container, item domain.WorkItem) (string, error) {
	workflow, found, err := g.InForce(ctx, collection, domain.EntryTypeKeyOf(item))
	if err != nil || !found {
		return "", err
	}
	return workflow.Published.Entering("", item.Completion.IsCompleted), nil
}

// StatusMover writes the moves of a publish or a change of scheme (UC-FLW-05 checks 3, 4, 6):
// every group in the caller's transaction, each moved entry's history saying the workflow changed
// and from which status to which, its change recorded for offline clients - and no step's
// conditions or actions run.
type StatusMover struct {
	Workflows repository.Workflows
	Activity  ActivityJournal
	Changes   changelog.ChangeLog
	HLC       clock.HLCSource
}

// Move applies the moves and answers how many entries moved.
func (m StatusMover) Move(
	ctx context.Context, actor appshared.ActorContext, moves []domain.StatusMove, now time.Time,
) (int, error) {
	moved := 0
	for _, move := range moves {
		entries, err := m.Workflows.MoveStatuses(ctx, move, now)
		if err != nil {
			return 0, err
		}
		for _, entry := range entries {
			item := domain.WorkItem{
				ID: entry.ID, TenantID: actor.TenantID, CollectionID: move.CollectionID,
				Status: move.To, Version: entry.Version,
			}
			if err := m.Activity.record(ctx, actor, item, activity.ItemWorkflowChanged, activity.ChangeSet(activity.Full,
				activity.Field{Name: domain.FieldStatus, Detail: activity.WithValues, From: move.From, To: move.To},
			), now); err != nil {
				return 0, err
			}
			if err := m.Changes.Record(ctx, changelog.Change{
				TenantID: actor.TenantID, Entity: itemTarget, EntityID: entry.ID, Op: changelog.Upsert,
				ContainerID: move.CollectionID, ActorID: actor.AccountID, HLC: m.HLC.Next(),
				Field:   domain.FieldStatus,
				Payload: map[string]any{domain.FieldStatus: stringOrNil(move.To), "version": entry.Version},
			}); err != nil {
				return 0, err
			}
		}
		moved += len(entries)
	}
	return moved, nil
}
