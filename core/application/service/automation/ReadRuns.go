// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListRuleRunsName = "ListRuleRuns"
	GetRuleRunName   = "GetRuleRun"

	runTarget = "automation_run"

	// RunReadAction is what reading the log performs. Info and not required, for the reason a
	// webhook listing is not: an entry per read would bury the entries a review looks for.
	RunReadAction audit.Action = "automation.run_read"
)

// Reader is what the two run use cases share.
type Reader struct {
	Runs       repository.Runs
	Rules      repository.Rules
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// ListRuleRuns answers a page of the log.
type ListRuleRuns struct{ Reader Reader }

// GetRuleRun answers one run.
type GetRuleRun struct{ Reader Reader }

// Execute reads a page.
//
// The permission is the rule's, asked per run, and a run whose rule the caller may not manage is
// left out rather than refusing the page - a 403 for the whole listing would hide the runs they may
// read. That is the same shape ListRules has, and for the same reason.
func (h ListRuleRuns) Execute(
	ctx context.Context, actor appshared.ActorContext, query repository.RunQuery,
) (repository.RunPage, error) {
	r := h.Reader

	if err := actor.RequireScope(automationScope); err != nil {
		return repository.RunPage{}, err
	}

	var page repository.RunPage
	err := r.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var listErr error
		page, listErr = r.Runs.List(ctx, query)
		if listErr != nil {
			return listErr
		}

		visible := make([]domain.Run, 0, len(page.Runs))
		// One decision per rule rather than one per run: a page is usually many runs of a handful
		// of rules, and asking the same question fifty times would be fifty resolutions for one
		// answer. The judgement is still per run - every run is checked, and what is remembered is
		// the answer to the question it asks.
		permitted := map[shared.ID]bool{}
		for _, run := range page.Runs {
			allowed, err := r.mayRead(ctx, actor, run.RuleID, permitted)
			if err != nil {
				return err
			}
			if allowed {
				visible = append(visible, run)
			}
		}
		page.Runs = visible
		return nil
	})
	if err != nil {
		return repository.RunPage{}, err
	}
	return page, nil
}

// Execute reads one run.
func (h GetRuleRun) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.Run, error) {
	r := h.Reader

	var run domain.Run
	err := r.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var findErr error
		run, findErr = r.Runs.Find(ctx, id)
		return findErr
	})
	if err != nil {
		return domain.Run{}, err
	}

	// The rule's scope decides, read after the run: which scope applies is a property of the rule,
	// and a caller naming a run has not told us where it lives.
	rule, err := r.rule(ctx, actor, run.RuleID)
	if err != nil {
		return domain.Run{}, err
	}
	if err := r.Authorizer.Authorize(ctx, actor, runRequest(rule.Scope, RunReadAction, run.ID)); err != nil {
		return domain.Run{}, err
	}
	return run, nil
}

func (r Reader) rule(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.Rule, error) {
	var rule domain.Rule
	err := r.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var findErr error
		rule, findErr = r.Rules.Find(ctx, id)
		return findErr
	})
	return rule, err
}

// mayRead answers whether the caller may see this rule's runs, remembering the answer for the rest
// of the page.
func (r Reader) mayRead(
	ctx context.Context, actor appshared.ActorContext, ruleID shared.ID, permitted map[shared.ID]bool,
) (bool, error) {
	if allowed, decided := permitted[ruleID]; decided {
		return allowed, nil
	}

	rule, err := r.Rules.Find(ctx, ruleID)
	if err != nil {
		// The rule is gone - deleted hard by the tenant going, since an ordinary deletion is soft
		// and keeps its runs readable. Its runs go with it rather than becoming unattributable.
		permitted[ruleID] = false
		return false, nil
	}

	allowed, err := r.Authorizer.Permits(ctx, actor, runRequest(rule.Scope, "", ruleID))
	if err != nil {
		// Not "may not see": nobody was refused anything, the question could not be answered.
		return false, err
	}
	permitted[ruleID] = allowed
	return allowed, nil
}

// runRequest is the permission question a run asks: the automation permission at the rule's own
// scope, which is the same question managing the rule asks. Reading what a rule did and being able
// to change it are one concern - there is no separate read permission to name.
func runRequest(scope domain.Scope, action audit.Action, target shared.ID) access.Request {
	return access.Request{
		Permission: service.PermissionAutomation,
		Path:       scope.Path(),
		Action:     action,
		TokenScope: automationScope,
		TargetType: runTarget,
		TargetID:   target,
	}
}

// Descriptor is the catalogue entry.
func (h ListRuleRuns) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListRuleRunsName,
		Summary: "What the rules have done, newest first: which rule, what started it, how its " +
			"conditions answered and what each action did. Only the runs of rules the caller may " +
			"manage. A run outlives the rule that produced it, which is why deleting a rule is " +
			"soft.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "rule_id", Kind: usecase.KindID, Description: "Narrow to one rule."},
			{
				Name: "status", Kind: usecase.KindString,
				Enum: []string{"RUNNING", "SUCCEEDED", "SKIPPED", "FAILED", "ABORTED_LOOP", "THROTTLED"},
				Description: "Narrow to one outcome. FAILED and ABORTED_LOOP are the two an " +
					"operator usually wants.",
			},
			{
				Name: "trigger", Kind: usecase.KindString,
				Enum: []string{
					"EVENT", "SCHEDULE", "RELATIVE_DATE", "INBOUND_WEBHOOK", "MANUAL", "JUMBLE_ENTRY",
				},
				Description: "Narrow to one way of starting. \"Did the schedule fire last night\" " +
					"and \"did anybody press the button\" are two questions about the same rule.",
			},
			{Name: "cursor", Kind: usecase.KindString, Description: "Where the last page stopped."},
			{Name: "size", Kind: usecase.KindInt, Description: "How many at most. Fifty by default, two hundred at most."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RunReadAction, TargetType: runTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h GetRuleRun) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        GetRuleRunName,
		Summary:     "One automation run, with every condition's answer and every action's outcome.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "run_id", Kind: usecase.KindID, Required: true, Description: "Which run."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RunReadAction, TargetType: runTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListRuleRuns) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	query := repository.RunQuery{
		Status:  domain.RunStatus(in.String("status")),
		Trigger: domain.TriggerKind(in.String("trigger")),
		Cursor:  in.String("cursor"), Size: in.Int("size"),
	}
	if in.Present("rule_id") {
		ruleID, err := in.ID("rule_id")
		if err != nil {
			return nil, err
		}
		query.RuleID = ruleID
	}
	if query.Status != "" && !query.Status.Valid() {
		return nil, shared.ErrValidation.
			WithDetail("automation.run_status_unknown").
			WithParams(map[string]string{"status": string(query.Status)}).
			WithFields(shared.FieldError{Path: "/status", Code: "automation.run_status_unknown"})
	}
	if query.Trigger != "" && !query.Trigger.Valid() {
		return nil, shared.ErrValidation.
			WithDetail("automation.trigger_kind_unknown").
			WithParams(map[string]string{"kind": query.Trigger.String()}).
			WithFields(shared.FieldError{Path: "/trigger", Code: "automation.trigger_kind_unknown"})
	}

	page, err := h.Execute(ctx, actor, query)
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(page.Runs))
	for _, run := range page.Runs {
		rows = append(rows, runOutput(run))
	}
	state := map[string]any{"next_cursor": nil, "has_more": page.HasMore}
	if page.NextCursor != "" {
		state["next_cursor"] = page.NextCursor
	}
	return usecase.Output{"data": rows, "page": state}, nil
}

func (h GetRuleRun) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("run_id")
	if err != nil {
		return nil, err
	}
	run, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return runOutput(run), nil
}

// runOutput is the projection every channel answers from.
func runOutput(run domain.Run) usecase.Output {
	conditions := make([]any, 0, len(run.ConditionResults))
	for _, result := range run.ConditionResults {
		row := map[string]any{"index": result.Index, "matched": result.Matched}
		if result.ErrorCode != "" {
			row["error_code"] = result.ErrorCode
		}
		conditions = append(conditions, row)
	}
	actions := make([]any, 0, len(run.ActionResults))
	for _, result := range run.ActionResults {
		row := map[string]any{
			"index": result.Index, "kind": result.Kind, "status": string(result.Status),
		}
		if result.ErrorCode != "" {
			row["error_code"] = result.ErrorCode
		}
		if result.IdempotencyKey != "" {
			row["idempotency_key"] = result.IdempotencyKey
		}
		actions = append(actions, row)
	}

	out := usecase.Output{
		"id":                run.ID.String(),
		"rule_id":           run.RuleID.String(),
		"trigger":           run.Trigger.String(),
		"triggered_by":      nil,
		"subject_id":        nil,
		"event_id":          nil,
		"status":            string(run.Status),
		"condition_results": conditions,
		"action_results":    actions,
		"started_at":        run.StartedAt,
		"finished_at":       nil,
		"causation_depth":   run.CausationDepth,
	}
	if !run.EventID.IsZero() {
		out["event_id"] = run.EventID.String()
	}
	if !run.TriggeredBy.IsZero() {
		out["triggered_by"] = run.TriggeredBy.String()
	}
	if !run.SubjectID.IsZero() {
		out["subject_id"] = run.SubjectID.String()
	}
	if run.FinishedAt != nil {
		out["finished_at"] = *run.FinishedAt
	}
	if run.ErrorCode != "" {
		out["error_code"] = run.ErrorCode
	}
	return out
}
