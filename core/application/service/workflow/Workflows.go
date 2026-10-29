// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strconv"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ListWorkflowsName          = "ListWorkflows"
	CreateWorkflowName         = "CreateWorkflow"
	SaveWorkflowDraftName      = "SaveWorkflowDraft"
	PreviewWorkflowPublishName = "PreviewWorkflowPublish"
	PublishWorkflowName        = "PublishWorkflow"
	DeleteWorkflowName         = "DeleteWorkflow"
	PreviewWorkflowSchemeName  = "PreviewWorkflowScheme"
	SetWorkflowSchemeName      = "SetWorkflowScheme"
	TryWorkflowStepName        = "TryWorkflowStep"

	WorkflowTarget = "workflow"

	WorkflowsReadAction     audit.Action = "workflow.read"
	WorkflowCreatedAction   audit.Action = "workflow.created"
	WorkflowDraftedAction   audit.Action = "workflow.drafted"
	WorkflowPublishedAction audit.Action = "workflow.published"
	WorkflowDeletedAction   audit.Action = "workflow.deleted"
	WorkflowSchemeSetAction audit.Action = "workflow.scheme_set"
	WorkflowStepTriedAction audit.Action = "workflow.step_tried"
)

// Desk is what every workflow use case shares.
type Desk struct {
	Workflows  repository.Workflows
	Containers repository.Containers
	Items      repository.Items
	Mover      workservice.StatusMover
	// Gate judges a step for the try (UC-FLW-04 check 5), as TakeStep would.
	Gate       workservice.WorkflowGate
	Compiler   expression.Compiler
	Authorizer Authorizer
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
	Text       text.Normalizer
}

func (d Desk) read(ctx context.Context) (domain.Workflows, error) {
	rows, err := d.Workflows.List(ctx)
	if err != nil {
		return nil, err
	}
	return domain.NewWorkflows(rows), nil
}

// authorizeShape asks for the workspace's structure permission (UC-FLW-02 check 8): whoever may not
// shape the workspace is not offered the designer and is refused here.
func (d Desk) authorizeShape(ctx context.Context, actor appshared.ActorContext, action audit.Action) error {
	return d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: []identity.Scope{identity.TenantScope()},
		Action: action, TokenScope: typesWrite, TargetType: WorkflowTarget, TargetID: actor.TenantID,
	})
}

// compile holds every condition of a shape to the language (UC-FLW-04 check 6).
func (d Desk) compile(shape domain.WorkflowShape) error {
	if d.Compiler == nil {
		return shared.ErrInternal.WithDetail("workflows.compiler_missing")
	}
	environment := workservice.WorkflowEnvironment()
	for _, condition := range shape.Expressions() {
		if _, err := d.Compiler.Compile(condition.Text, environment, expression.Boolean); err != nil {
			code := "expression.syntax"
			var coded *shared.Error
			if errors.As(err, &coded) && coded.DetailCode != "" {
				code = coded.DetailCode
			}
			return shared.ErrValidation.WithDetail(code).
				WithFields(shared.FieldError{Path: "/shape" + condition.Path, Code: code})
		}
	}
	return nil
}

// store writes a workflow: its row's first write inserts, later ones update.
func (d Desk) store(ctx context.Context, actor appshared.ActorContext, current, next domain.Workflow) (domain.Workflow, error) {
	if current.Stored() {
		if err := d.Workflows.Update(ctx, next, current.Version); err != nil {
			return domain.Workflow{}, err
		}
		next.Version = current.Version + 1
		return next, nil
	}
	next.ID, next.TenantID, next.CreatedAt, next.Version = d.IDs.NewID(), actor.TenantID, next.UpdatedAt, 1
	return next, d.Workflows.Insert(ctx, next)
}

// trail writes the evidence of a change: keys and counts, never a name (rule 10).
func (d Desk) trail(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, targetID shared.ID,
	now time.Time, changes ...audit.Change,
) error {
	return d.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: WorkflowTarget, TargetID: targetID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// announce records the workflow for offline clients, which draw a board by its statuses.
func (d Desk) announce(ctx context.Context, actor appshared.ActorContext, workflow domain.Workflow, op changelog.Operation) error {
	var payload map[string]any
	if op != changelog.Delete {
		payload = map[string]any(WorkflowOutput(workflow))
	}
	return d.Changes.Record(ctx, changelog.Change{
		TenantID: actor.TenantID, Entity: WorkflowTarget, EntityID: workflow.ID, Op: op,
		ActorID: actor.AccountID, HLC: d.HLC.Next(), Payload: payload,
	})
}

// collectionsUnder answers the collections a container reaches: itself, or a hub's collections.
func (d Desk) collectionsUnder(ctx context.Context, container domain.Container) ([]domain.Container, error) {
	if container.Type == domain.ContainerCollection {
		return []domain.Container{container}, nil
	}
	var out []domain.Container
	page := repository.Page{Size: 200}
	for {
		found, err := d.Containers.List(ctx, repository.ContainerQuery{
			ParentID: container.ID, Type: domain.ContainerCollection, IncludeArchived: true, Page: page,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, found.Containers...)
		if !found.Info.HasMore {
			return out, nil
		}
		page.Cursor = found.Info.NextCursor
	}
}

// plan answers the moves a change of the scheme or of a workflow's shape makes, from the schemes
// and workflows before to those after: per group of the reached collections, the shape in force
// before and after (work.PlanStatusMoves).
func (d Desk) plan(
	ctx context.Context, collections []domain.Container, schemesBefore, schemesAfter []domain.WorkflowScheme,
	before, after domain.Workflows, mapping map[string]string,
) ([]domain.StatusMove, []domain.StatusGroup, error) {
	ids := make([]shared.ID, 0, len(collections))
	hubOf := map[shared.ID]shared.ID{}
	for _, collection := range collections {
		ids = append(ids, collection.ID)
		hubOf[collection.ID] = collection.ParentID
	}
	groups, err := d.Workflows.StatusGroups(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	shapeIn := func(schemes []domain.WorkflowScheme, set domain.Workflows, group domain.StatusGroup) *domain.WorkflowShape {
		key, found := domain.ResolveWorkflow(schemes, group.CollectionID, hubOf[group.CollectionID], group.EntryType)
		if !found {
			return nil
		}
		workflow, found := set.Find(key)
		if !found {
			return nil
		}
		return &workflow.Published
	}
	var moves []domain.StatusMove
	var reached []domain.StatusGroup
	for _, group := range groups {
		was, will := shapeIn(schemesBefore, before, group), shapeIn(schemesAfter, after, group)
		if was == nil && will == nil {
			continue
		}
		reached = append(reached, group)
		planned, err := domain.PlanStatusMoves([]domain.StatusGroup{group}, was, will, mapping)
		if err != nil {
			return nil, nil, err
		}
		moves = append(moves, planned...)
	}
	return moves, reached, nil
}

// PreviewOutput is what a publish or a change of scheme would do, as the contract spells it
// (UC-FLW-02 check 4, UC-FLW-05 check 2).
func PreviewOutput(moves []domain.StatusMove, removed map[string]int, columnsBefore, columnsAfter []string) usecase.Output {
	data := make([]usecase.Output, 0, len(moves))
	total := 0
	for _, move := range moves {
		total += move.Count
		data = append(data, usecase.Output{
			"collection_id": move.CollectionID.String(), "entry_type": move.EntryType,
			"from": stringOrNil(move.From), "to": stringOrNil(move.To), "completed": move.Completed,
			"count": move.Count,
		})
	}
	removedOut := make([]usecase.Output, 0, len(removed))
	keys := make([]string, 0, len(removed))
	for key := range removed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		removedOut = append(removedOut, usecase.Output{"status": key, "count": removed[key]})
	}
	return usecase.Output{
		"moves": data, "entries": total, "removed": removedOut,
		"columns_before": nonNil(columnsBefore), "columns_after": nonNil(columnsAfter),
	}
}

func stringOrNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// ShapeOutput is a shape as the contract spells it (schema WorkflowShape).
func ShapeOutput(shape domain.WorkflowShape) usecase.Output {
	statuses := make([]any, 0, len(shape.Statuses))
	for _, status := range shape.Statuses {
		statuses = append(statuses, map[string]any{
			"key": status.Key, "name": stringOrNil(status.Name), "category": string(status.Category),
			"color_token": status.ColorToken,
		})
	}
	steps := make([]any, 0, len(shape.Steps))
	for _, step := range shape.Steps {
		checks := make([]any, 0, len(step.Checks))
		for _, check := range step.Checks {
			checks = append(checks, map[string]any{"key": check.Key, "holds": check.Holds, "message": check.Message})
		}
		steps = append(steps, map[string]any{
			"key": step.Key, "name": stringOrNil(step.Name), "from": nonNil(step.From), "to": step.To,
			"who": map[string]any{
				"roles": nonNil(step.Who.Roles), "person_fields": nonNil(step.Who.PersonFields),
				"condition": step.Who.Condition,
			},
			"requires": nonNil(step.Requires), "asks": nonNil(step.Asks), "checks": checks,
		})
	}
	return usecase.Output{"statuses": statuses, "initial": shape.Initial, "steps": steps}
}

// WorkflowOutput is a workflow as every door returns it (schema Workflow).
func WorkflowOutput(w domain.Workflow) usecase.Output {
	out := usecase.Output{
		"key": w.Key, "system": w.System, "name": stringOrNil(w.Name),
		"published": ShapeOutput(w.Published), "draft": nil, "published_at": nil,
		"version": w.Version,
	}
	if w.Draft != nil {
		out["draft"] = ShapeOutput(*w.Draft)
	}
	if w.PublishedAt != nil {
		out["published_at"] = *w.PublishedAt
	}
	return out
}

// ShapeFromInput reads a shape from a door's input (schema WorkflowShape).
func ShapeFromInput(raw any) (domain.WorkflowShape, error) {
	document, ok := raw.(map[string]any)
	if !ok {
		return domain.WorkflowShape{}, malformedShape("/shape")
	}
	shape := domain.WorkflowShape{Initial: textOf(document["initial"])}
	for i, entry := range listOf(document["statuses"]) {
		member, ok := entry.(map[string]any)
		if !ok {
			return domain.WorkflowShape{}, malformedShape("/shape/statuses/" + strconv.Itoa(i))
		}
		shape.Statuses = append(shape.Statuses, domain.WorkflowStatus{
			Key: textOf(member["key"]), Name: textOf(member["name"]),
			Category: domain.StatusCategory(textOf(member["category"])), ColorToken: textOf(member["color_token"]),
		})
	}
	for i, entry := range listOf(document["steps"]) {
		member, ok := entry.(map[string]any)
		if !ok {
			return domain.WorkflowShape{}, malformedShape("/shape/steps/" + strconv.Itoa(i))
		}
		step := domain.WorkflowStep{
			Key: textOf(member["key"]), Name: textOf(member["name"]), To: textOf(member["to"]),
			From: textsOf(member["from"]), Requires: textsOf(member["requires"]), Asks: textsOf(member["asks"]),
		}
		if who, ok := member["who"].(map[string]any); ok {
			step.Who = domain.StepWho{
				Roles: textsOf(who["roles"]), PersonFields: textsOf(who["person_fields"]),
				Condition: textOf(who["condition"]),
			}
		}
		for j, raw := range listOf(member["checks"]) {
			check, ok := raw.(map[string]any)
			if !ok {
				return domain.WorkflowShape{}, malformedShape("/shape/steps/" + strconv.Itoa(i) + "/checks/" + strconv.Itoa(j))
			}
			step.Checks = append(step.Checks, domain.StepCheck{
				Key: textOf(check["key"]), Holds: textOf(check["holds"]), Message: textOf(check["message"]),
			})
		}
		shape.Steps = append(shape.Steps, step)
	}
	return shape, nil
}

func textsOf(raw any) []string {
	var out []string
	for _, value := range listOf(raw) {
		out = append(out, textOf(value))
	}
	return out
}

func malformedShape(path string) error {
	return shared.ErrValidation.WithDetail("workflows.malformed").
		WithFields(shared.FieldError{Path: path, Code: "workflows.malformed"})
}

func mappingFromInput(raw any) map[string]string {
	out := map[string]string{}
	if document, ok := raw.(map[string]any); ok {
		for key, value := range document {
			out[key] = textOf(value)
		}
	}
	return out
}

func workflowDescriptor(name, summary, sideEffects, scope string, readOnly bool, action audit.Action,
	input []usecase.Field, handler usecase.HandlerFunc,
) usecase.Descriptor {
	return usecase.Descriptor{
		Name: name, Summary: summary, SideEffects: sideEffects, TokenScope: scope, ReadOnly: readOnly,
		Module: module.Workflows, Input: input,
		Audit: usecase.AuditDeclaration{
			Action: action, TargetType: WorkflowTarget, Severity: audit.SeverityInfo, Required: !readOnly,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a workflow is the workspace's configuration; the entries a publish moves record their own history."},
		Handler:  handler,
	}
}

// ListWorkflows reads the workspace's workflows and its scheme.
type ListWorkflows struct{ Desk Desk }

// Execute answers the workflows and the scheme. Any member may read them: a board draws its
// columns from them.
func (h ListWorkflows) Execute(ctx context.Context, actor appshared.ActorContext) (domain.Workflows, []domain.WorkflowScheme, error) {
	if err := actor.RequireScope(typesRead); err != nil {
		return nil, nil, err
	}
	var (
		set     domain.Workflows
		schemes []domain.WorkflowScheme
	)
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if set, err = h.Desk.read(ctx); err != nil {
			return err
		}
		schemes, err = h.Desk.Workflows.Schemes(ctx)
		return err
	})
	return set, schemes, err
}

// Descriptor is the catalogue entry.
func (h ListWorkflows) Descriptor() usecase.Descriptor {
	return workflowDescriptor(ListWorkflowsName,
		"Lists the workspace's workflows - the shipped one and its own - each with the shape in force "+
			"and any draft, and the scheme: which hub or collection runs which workflow for which type.",
		"None. Reads only.", typesRead, true, WorkflowsReadAction, nil,
		func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			set, schemes, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(set))
			for _, w := range set {
				data = append(data, WorkflowOutput(w))
			}
			rows := make([]usecase.Output, 0, len(schemes))
			for _, scheme := range schemes {
				rows = append(rows, usecase.Output{
					"container_id": scheme.ContainerID.String(), "entry_type": stringOrNil(scheme.EntryType),
					"workflow": scheme.WorkflowKey,
				})
			}
			return usecase.Output{"data": data, "scheme": rows}, nil
		})
}

// CreateWorkflow adds a workflow of the workspace's own, in force nowhere until a scheme names it.
type CreateWorkflow struct{ Desk Desk }

// Execute creates the workflow.
func (h CreateWorkflow) Execute(ctx context.Context, actor appshared.ActorContext, key, name string, shape domain.WorkflowShape) (domain.Workflow, error) {
	if err := h.Desk.authorizeShape(ctx, actor, WorkflowCreatedAction); err != nil {
		return domain.Workflow{}, err
	}
	now := h.Desk.Clock.Now()
	created, err := domain.NewWorkflow(domain.NewWorkflowInput{
		ID: h.Desk.IDs.NewID(), TenantID: actor.TenantID, Key: key, Name: name, Shape: shape,
		By: actor.AccountID, Now: now, Text: h.Desk.Text,
	})
	if err != nil {
		return domain.Workflow{}, err
	}
	if err := h.Desk.compile(created.Published); err != nil {
		return domain.Workflow{}, err
	}
	err = h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Desk.Workflows.Insert(ctx, created); err != nil {
			return err
		}
		if err := h.Desk.announce(ctx, actor, created, changelog.Upsert); err != nil {
			return err
		}
		return h.Desk.trail(ctx, actor, WorkflowCreatedAction, created.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, To: created.Key},
			audit.Change{Field: "statuses", Classification: audit.Open, To: strconv.Itoa(len(created.Published.Statuses))})
	})
	return created, err
}

func shapeFields() []usecase.Field {
	return []usecase.Field{
		{Name: "shape", Kind: usecase.KindObject, Required: true, Description: "The statuses, the initial status and the steps (schema WorkflowShape)."},
	}
}

// Descriptor is the catalogue entry.
func (h CreateWorkflow) Descriptor() usecase.Descriptor {
	return workflowDescriptor(CreateWorkflowName,
		"Adds a workflow of the workspace's own: statuses each in BACKLOG, TODO, IN_PROGRESS or DONE "+
			"with a colour token, the status a new entry starts in, and steps from some - or any - "+
			"status to one, with who may take them, what they require, ask for and check. In force "+
			"nowhere until a hub or collection uses it.",
		"Writes the workflow, records a change for offline clients, and writes an audit entry.",
		typesWrite, false, WorkflowCreatedAction,
		append([]usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "Small letters, digits and underscores, starting with a letter; not default."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
		}, shapeFields()...),
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			shape, err := ShapeFromInput(in["shape"])
			if err != nil {
				return nil, err
			}
			created, err := h.Execute(ctx, actor, in.String("key"), in.String("name"), shape)
			if err != nil {
				return nil, err
			}
			return WorkflowOutput(created), nil
		})
}

// SaveWorkflowDraft saves a change of a workflow beside the shape in force, renames it, or
// discards the draft (UC-FLW-02 check 3, UC-FLW-05 check 1).
type SaveWorkflowDraft struct{ Desk Desk }

// SaveWorkflowDraftCommand is the input, typed.
type SaveWorkflowDraftCommand struct {
	Key             string
	Name            *string
	Shape           *domain.WorkflowShape
	Discard         bool
	ExpectedVersion int
}

// Execute saves the draft.
func (h SaveWorkflowDraft) Execute(ctx context.Context, actor appshared.ActorContext, cmd SaveWorkflowDraftCommand) (domain.Workflow, error) {
	if err := h.Desk.authorizeShape(ctx, actor, WorkflowDraftedAction); err != nil {
		return domain.Workflow{}, err
	}
	if cmd.Shape != nil {
		if err := h.Desk.compile(*cmd.Shape); err != nil {
			return domain.Workflow{}, err
		}
	}
	var saved domain.Workflow
	err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		set, err := h.Desk.read(ctx)
		if err != nil {
			return err
		}
		current, found := set.Find(cmd.Key)
		if !found {
			return shared.ErrNotFound.WithDetail("workflows.not_found")
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("workflows.version_conflict")
		}
		now := h.Desk.Clock.Now()
		next := current
		next.UpdatedAt = now
		if cmd.Name != nil {
			if next, err = next.Renamed(*cmd.Name, now, h.Desk.Text); err != nil {
				return err
			}
		}
		switch {
		case cmd.Discard:
			next = next.Discarded(now)
		case cmd.Shape != nil:
			if next, err = next.WithDraft(*cmd.Shape, now, h.Desk.Text); err != nil {
				return err
			}
		}
		if saved, err = h.Desk.store(ctx, actor, current, next); err != nil {
			return err
		}
		if err := h.Desk.announce(ctx, actor, saved, changelog.Upsert); err != nil {
			return err
		}
		return h.Desk.trail(ctx, actor, WorkflowDraftedAction, saved.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, To: saved.Key},
			audit.Change{Field: "draft", Classification: audit.Open, To: strconv.FormatBool(saved.Draft != nil)})
	})
	return saved, err
}

// Descriptor is the catalogue entry.
func (h SaveWorkflowDraft) Descriptor() usecase.Descriptor {
	return workflowDescriptor(SaveWorkflowDraftName,
		"Saves a draft of a workflow beside the shape in force, which stays in force until the draft "+
			"is published; or renames the workflow; or discards its draft. The shipped workflow is "+
			"changed the same way.",
		"Writes the workflow, records a change for offline clients, and writes an audit entry.",
		typesWrite, false, WorkflowDraftedAction,
		[]usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The workflow."},
			{Name: "name", Kind: usecase.KindString, Description: "A new name."},
			{Name: "shape", Kind: usecase.KindObject, Description: "The draft's statuses, initial status and steps (schema WorkflowShape)."},
			{Name: "discard", Kind: usecase.KindBool, Description: "True discards the draft."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for optimistic locking."},
		},
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			cmd := SaveWorkflowDraftCommand{
				Key: in.String("key"), Name: in.OptionalString("name"), Discard: in.Bool("discard"),
				ExpectedVersion: in.Int("expected_version"),
			}
			if in.Present("shape") {
				shape, err := ShapeFromInput(in["shape"])
				if err != nil {
					return nil, err
				}
				cmd.Shape = &shape
			}
			saved, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return WorkflowOutput(saved), nil
		})
}

// publishPlan answers what publishing a workflow's draft does.
func (d Desk) publishPlan(ctx context.Context, key string, mapping map[string]string) (
	domain.Workflow, domain.Workflow, []domain.StatusMove, map[string]int, error,
) {
	set, err := d.read(ctx)
	if err != nil {
		return domain.Workflow{}, domain.Workflow{}, nil, nil, err
	}
	current, found := set.Find(key)
	if !found {
		return domain.Workflow{}, domain.Workflow{}, nil, nil, shared.ErrNotFound.WithDetail("workflows.not_found")
	}
	next, err := current.Publish(shared.ID(""), d.Clock.Now())
	if err != nil {
		return domain.Workflow{}, domain.Workflow{}, nil, nil, err
	}
	schemes, err := d.Workflows.Schemes(ctx)
	if err != nil {
		return domain.Workflow{}, domain.Workflow{}, nil, nil, err
	}
	var collections []domain.Container
	seen := map[shared.ID]bool{}
	for _, scheme := range schemes {
		if scheme.WorkflowKey != key {
			continue
		}
		container, err := d.Containers.Find(ctx, scheme.ContainerID)
		if err != nil {
			return domain.Workflow{}, domain.Workflow{}, nil, nil, err
		}
		under, err := d.collectionsUnder(ctx, container)
		if err != nil {
			return domain.Workflow{}, domain.Workflow{}, nil, nil, err
		}
		for _, collection := range under {
			if !seen[collection.ID] {
				seen[collection.ID] = true
				collections = append(collections, collection)
			}
		}
	}
	after := slices.Clone(set)
	for i := range after {
		if after[i].Key == key {
			after[i] = next
		}
	}
	moves, reached, err := d.plan(ctx, collections, schemes, schemes, set, after, mapping)
	if err != nil {
		return domain.Workflow{}, domain.Workflow{}, nil, nil, err
	}
	removed := domain.RemovedStatuses(current.Published, next.Published, reachedBy(reached, schemes, set, key, collections))
	return current, next, moves, removed, nil
}

// reachedBy narrows groups to those the workflow is in force for.
func reachedBy(groups []domain.StatusGroup, schemes []domain.WorkflowScheme, set domain.Workflows, key string, collections []domain.Container) []domain.StatusGroup {
	hubOf := map[shared.ID]shared.ID{}
	for _, collection := range collections {
		hubOf[collection.ID] = collection.ParentID
	}
	var out []domain.StatusGroup
	for _, group := range groups {
		if resolved, found := domain.ResolveWorkflow(schemes, group.CollectionID, hubOf[group.CollectionID], group.EntryType); found && resolved == key {
			out = append(out, group)
		}
	}
	return out
}

// PreviewWorkflowPublish answers what publishing a draft would do, changing nothing (UC-FLW-02
// check 4, UC-FLW-05 check 2).
type PreviewWorkflowPublish struct{ Desk Desk }

// Descriptor is the catalogue entry.
func (h PreviewWorkflowPublish) Descriptor() usecase.Descriptor {
	return workflowDescriptor(PreviewWorkflowPublishName,
		"Answers what publishing a workflow's draft would do: which statuses in use it removes with "+
			"their counts, how many entries move from which status to which in each collection, and "+
			"the board's columns before and after. Changes nothing.",
		"None. Reads only.", typesWrite, true, WorkflowPublishedAction,
		[]usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The workflow."},
			{Name: "mapping", Kind: usecase.KindObject, Description: "Per removed status, the status its entries move to."},
		},
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorizeShape(ctx, actor, WorkflowPublishedAction); err != nil {
				return nil, err
			}
			var out usecase.Output
			err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				current, next, moves, removed, err := h.Desk.publishPlan(ctx, in.String("key"), mappingFromInput(in["mapping"]))
				if err != nil {
					return err
				}
				out = PreviewOutput(moves, removed, current.Published.Columns(), next.Published.Columns())
				return nil
			})
			return out, err
		})
}

// PublishWorkflow puts a draft in force and moves the entries of removed statuses in the same
// change (UC-FLW-05 checks 2 … 6).
type PublishWorkflow struct{ Desk Desk }

// Execute publishes and answers the workflow and how many entries moved.
func (h PublishWorkflow) Execute(ctx context.Context, actor appshared.ActorContext, key string, mapping map[string]string, expectedVersion int) (domain.Workflow, int, error) {
	if err := h.Desk.authorizeShape(ctx, actor, WorkflowPublishedAction); err != nil {
		return domain.Workflow{}, 0, err
	}
	var (
		published domain.Workflow
		moved     int
	)
	err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, next, moves, removed, err := h.Desk.publishPlan(ctx, key, mapping)
		if err != nil {
			return err
		}
		if expectedVersion != 0 && expectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("workflows.version_conflict")
		}
		// Every removed status in use needs its target: nobody's entry is moved without the
		// person publishing having said where (UC-FLW-05 check 2).
		for status, count := range removed {
			if mapping[status] == "" {
				return shared.ErrValidation.WithDetail("workflows.mapping_required").
					WithParams(map[string]string{"status": status, "count": strconv.Itoa(count)}).
					WithFields(shared.FieldError{Path: "/mapping/" + status, Code: "workflows.mapping_required",
						Params: map[string]string{"status": status, "count": strconv.Itoa(count)}})
			}
		}
		now := h.Desk.Clock.Now()
		next.PublishedBy, next.PublishedAt, next.UpdatedAt = actor.AccountID, &now, now
		if published, err = h.Desk.store(ctx, actor, current, next); err != nil {
			return err
		}
		if moved, err = h.Desk.Mover.Move(ctx, actor, moves, now); err != nil {
			return err
		}
		if err := h.Desk.announce(ctx, actor, published, changelog.Upsert); err != nil {
			return err
		}
		return h.Desk.trail(ctx, actor, WorkflowPublishedAction, published.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, To: published.Key},
			audit.Change{Field: "statuses", Classification: audit.Open, To: strconv.Itoa(len(published.Published.Statuses))},
			audit.Change{Field: "entries_moved", Classification: audit.Open, To: strconv.Itoa(moved)})
	})
	return published, moved, err
}

// Descriptor is the catalogue entry.
func (h PublishWorkflow) Descriptor() usecase.Descriptor {
	return workflowDescriptor(PublishWorkflowName,
		"Puts a workflow's draft in force. Every removed status that entries are in needs a target "+
			"in the mapping, on the same side of completion; the entries move in the same change, "+
			"each with a history entry, and no step's conditions or actions run.",
		"Writes the workflow and the moved entries' statuses, records their changes for offline "+
			"clients and their history, and writes an audit entry with the counts.",
		typesWrite, false, WorkflowPublishedAction,
		[]usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The workflow."},
			{Name: "mapping", Kind: usecase.KindObject, Description: "Per removed status, the status its entries move to."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for optimistic locking."},
		},
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			published, moved, err := h.Execute(ctx, actor, in.String("key"), mappingFromInput(in["mapping"]), in.Int("expected_version"))
			if err != nil {
				return nil, err
			}
			out := WorkflowOutput(published)
			out["entries_moved"] = moved
			return out, nil
		})
}

// DeleteWorkflow removes a workflow of the workspace's own that no hub or collection uses.
type DeleteWorkflow struct{ Desk Desk }

// Descriptor is the catalogue entry.
func (h DeleteWorkflow) Descriptor() usecase.Descriptor {
	return workflowDescriptor(DeleteWorkflowName,
		"Deletes a workflow of the workspace's own that no hub or collection uses. The shipped one "+
			"is never deleted.",
		"Removes the workflow, records a change for offline clients, and writes an audit entry.",
		typesWrite, false, WorkflowDeletedAction,
		[]usecase.Field{{Name: "key", Kind: usecase.KindString, Required: true, Description: "The workflow."}},
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			key := in.String("key")
			if key == domain.SystemWorkflowKey {
				return nil, shared.ErrConflict.WithDetail("workflows.system_kept")
			}
			if err := h.Desk.authorizeShape(ctx, actor, WorkflowDeletedAction); err != nil {
				return nil, err
			}
			err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				set, err := h.Desk.read(ctx)
				if err != nil {
					return err
				}
				current, found := set.Find(key)
				if !found || !current.Stored() {
					return shared.ErrNotFound.WithDetail("workflows.not_found")
				}
				if err := h.Desk.Workflows.Delete(ctx, key); err != nil {
					return err
				}
				if err := h.Desk.announce(ctx, actor, current, changelog.Delete); err != nil {
					return err
				}
				return h.Desk.trail(ctx, actor, WorkflowDeletedAction, current.ID, h.Desk.Clock.Now(),
					audit.Change{Field: "key", Classification: audit.Open, To: key})
			})
			return nil, err
		})
}

// SchemeChange is a change of the scheme: the workflow a container runs for a type, empty for
// every type; an empty workflow removes the row.
type SchemeChange struct {
	ContainerID shared.ID
	EntryType   string
	WorkflowKey string
}

// schemePlan answers what a change of the scheme does.
func (d Desk) schemePlan(ctx context.Context, change SchemeChange) ([]domain.WorkflowScheme, []domain.StatusMove, domain.Container, error) {
	container, err := d.Containers.Find(ctx, change.ContainerID)
	if err != nil {
		return nil, nil, domain.Container{}, err
	}
	set, err := d.read(ctx)
	if err != nil {
		return nil, nil, domain.Container{}, err
	}
	if change.WorkflowKey != "" {
		if _, found := set.Find(change.WorkflowKey); !found {
			return nil, nil, domain.Container{}, shared.ErrNotFound.WithDetail("workflows.not_found")
		}
	}
	before, err := d.Workflows.Schemes(ctx)
	if err != nil {
		return nil, nil, domain.Container{}, err
	}
	var after []domain.WorkflowScheme
	for _, scheme := range before {
		if scheme.ContainerID != change.ContainerID || scheme.EntryType != change.EntryType {
			after = append(after, scheme)
		}
	}
	if change.WorkflowKey != "" {
		after = append(after, domain.WorkflowScheme{ContainerID: change.ContainerID, EntryType: change.EntryType, WorkflowKey: change.WorkflowKey})
	}
	collections, err := d.collectionsUnder(ctx, container)
	if err != nil {
		return nil, nil, domain.Container{}, err
	}
	moves, _, err := d.plan(ctx, collections, before, after, set, set, nil)
	return after, moves, container, err
}

func (d Desk) authorizeScheme(ctx context.Context, actor appshared.ActorContext, containerID shared.ID) error {
	var container domain.Container
	if err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := d.Containers.Find(ctx, containerID)
		container = found
		return err
	}); err != nil {
		return err
	}
	return d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: service.ContainerScopes(container),
		Action: WorkflowSchemeSetAction, TokenScope: typesWrite, TargetType: WorkflowTarget, TargetID: containerID,
	})
}

func schemeChangeFrom(in usecase.Input) (SchemeChange, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return SchemeChange{}, err
	}
	return SchemeChange{ContainerID: containerID, EntryType: in.String("entry_type"), WorkflowKey: in.String("workflow")}, nil
}

func schemeFields() []usecase.Field {
	return []usecase.Field{
		{Name: "container_id", Kind: usecase.KindID, Required: true, Description: "The hub or collection."},
		{Name: "entry_type", Kind: usecase.KindString, Description: "The type the row is for; empty for every type."},
		{Name: "workflow", Kind: usecase.KindString, Description: "The workflow's key; empty removes the row."},
	}
}

// PreviewWorkflowScheme answers what a change of the scheme would do (UC-FLW-02 check 4).
type PreviewWorkflowScheme struct{ Desk Desk }

// Descriptor is the catalogue entry.
func (h PreviewWorkflowScheme) Descriptor() usecase.Descriptor {
	return workflowDescriptor(PreviewWorkflowSchemeName,
		"Answers what using a workflow in a hub or collection - or no longer using one - would do: "+
			"which status every existing entry gets, by collection, type and status, and the board's "+
			"columns. Changes nothing.",
		"None. Reads only.", typesWrite, true, WorkflowSchemeSetAction, schemeFields(),
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			change, err := schemeChangeFrom(in)
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeScheme(ctx, actor, change.ContainerID); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				_, moves, _, err := h.Desk.schemePlan(ctx, change)
				if err != nil {
					return err
				}
				var columns []string
				if set, err := h.Desk.read(ctx); err == nil {
					if w, found := set.Find(change.WorkflowKey); found {
						columns = w.Published.Columns()
					}
				}
				out = PreviewOutput(moves, nil, nil, columns)
				return nil
			})
			return out, err
		})
}

// SetWorkflowScheme names the workflow a hub or collection runs for a type and moves the entries
// there into it, once the person confirmed the preview (UC-FLW-02 check 4).
type SetWorkflowScheme struct{ Desk Desk }

// Execute writes the change and answers how many entries moved.
func (h SetWorkflowScheme) Execute(ctx context.Context, actor appshared.ActorContext, change SchemeChange, confirmed bool) (int, error) {
	if !confirmed {
		return 0, shared.ErrValidation.WithDetail("workflows.confirmation_required").
			WithFields(shared.FieldError{Path: "/confirm", Code: "workflows.confirmation_required"})
	}
	if err := h.Desk.authorizeScheme(ctx, actor, change.ContainerID); err != nil {
		return 0, err
	}
	moved := 0
	err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		_, moves, _, err := h.Desk.schemePlan(ctx, change)
		if err != nil {
			return err
		}
		now := h.Desk.Clock.Now()
		if change.WorkflowKey == "" {
			if err := h.Desk.Workflows.RemoveScheme(ctx, change.ContainerID, change.EntryType); err != nil {
				return err
			}
		} else {
			// The shipped workflow gains its row when it is first used: the scheme's reference
			// needs one.
			set, err := h.Desk.read(ctx)
			if err != nil {
				return err
			}
			if used, _ := set.Find(change.WorkflowKey); !used.Stored() {
				used.UpdatedAt = now
				if _, err := h.Desk.store(ctx, actor, domain.Workflow{}, used); err != nil {
					return err
				}
			}
			if err := h.Desk.Workflows.SetScheme(ctx, domain.WorkflowScheme{
				ContainerID: change.ContainerID, EntryType: change.EntryType, WorkflowKey: change.WorkflowKey,
			}, now); err != nil {
				return err
			}
		}
		if moved, err = h.Desk.Mover.Move(ctx, actor, moves, now); err != nil {
			return err
		}
		return h.Desk.trail(ctx, actor, WorkflowSchemeSetAction, change.ContainerID, now,
			audit.Change{Field: "workflow", Classification: audit.Open, To: change.WorkflowKey},
			audit.Change{Field: "entry_type", Classification: audit.Open, To: change.EntryType},
			audit.Change{Field: "entries_moved", Classification: audit.Open, To: strconv.Itoa(moved)})
	})
	return moved, err
}

// Descriptor is the catalogue entry.
func (h SetWorkflowScheme) Descriptor() usecase.Descriptor {
	return workflowDescriptor(SetWorkflowSchemeName,
		"Names the workflow a hub or collection runs for a type - or for every type - or removes "+
			"the row. Existing entries take the status the preview showed: one the workflow knows is "+
			"kept, a done entry enters the first done status, an open one the initial status. "+
			"Refused unless confirm is true.",
		"Writes the scheme and the moved entries' statuses, records their changes and history, and "+
			"writes an audit entry with the count.",
		typesWrite, false, WorkflowSchemeSetAction,
		append(schemeFields(), usecase.Field{Name: "confirm", Kind: usecase.KindBool, Description: "True once the person has seen the preview."}),
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			change, err := schemeChangeFrom(in)
			if err != nil {
				return nil, err
			}
			moved, err := h.Execute(ctx, actor, change, in.Bool("confirm"))
			if err != nil {
				return nil, err
			}
			return usecase.Output{
				"container_id": change.ContainerID.String(), "entry_type": stringOrNil(change.EntryType),
				"workflow": stringOrNil(change.WorkflowKey), "entries_moved": moved,
			}, nil
		})
}

// TryWorkflowStep judges a step of a workflow - its draft, or the shape in force - against a
// chosen entry, changing nothing (UC-FLW-04 check 5).
type TryWorkflowStep struct{ Desk Desk }

// Descriptor is the catalogue entry.
func (h TryWorkflowStep) Descriptor() usecase.Descriptor {
	return workflowDescriptor(TryWorkflowStepName,
		"Tries a step of a workflow against an entry, without taking it: whether the person may take "+
			"it, which required fields are empty, and each check's outcome. The draft's step where the "+
			"workflow has a draft and draft is true, else the step in force.",
		"None. Reads only.", typesWrite, true, WorkflowStepTriedAction,
		[]usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The workflow."},
			{Name: "step", Kind: usecase.KindString, Required: true, Description: "The step."},
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry to try it on."},
			{Name: "draft", Kind: usecase.KindBool, Description: "True tries the draft's step."},
		},
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorizeShape(ctx, actor, WorkflowStepTriedAction); err != nil {
				return nil, err
			}
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			var out usecase.Output
			err = h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				set, err := h.Desk.read(ctx)
				if err != nil {
					return err
				}
				w, found := set.Find(in.String("key"))
				if !found {
					return shared.ErrNotFound.WithDetail("workflows.not_found")
				}
				shape := w.Published
				if in.Bool("draft") && w.Draft != nil {
					shape = *w.Draft
				}
				step, found := shape.Step(in.String("step"))
				if !found {
					return shared.ErrNotFound.WithDetail("workflows.step_unknown")
				}
				item, err := h.Desk.Items.Find(ctx, itemID)
				if err != nil {
					return err
				}
				collection, err := h.Desk.Containers.Find(ctx, item.CollectionID)
				if err != nil {
					return err
				}
				status := workservice.StatusOf(shape, item)
				verdict, err := h.Desk.Gate.Judge(ctx, actor, collection, item, status, step)
				if err != nil {
					return err
				}
				out = workservice.StepOutput(verdict)
				out["from"] = status
				out["leaves_from"] = step.LeavesFrom(status)
				return nil
			})
			return out, err
		})
}
