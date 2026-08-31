// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/condition"
	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	TestRuleName = "TestRule"

	// RuleTestedAction is what a dry run performs. Info and not required, for the run log's
	// reason: a test is a read, and an entry per test would bury the entries a review looks for.
	RuleTestedAction audit.Action = "automation.rule_tested"
)

// TestRule is the dry run automation.md §2 promises (G-09): a sample event in, which conditions
// matched and which actions *would* run out - and no side effects.
//
// The restore's dry-run discipline is the whole design (E-06): nothing below this use case opens
// a writing transaction. Conditions are evaluated - reads, through the same lazy activation a
// real run uses - but no action dispatches, nothing is queued, no run row is written and the
// failure streak is untouched. What makes that provable rather than promised is the shape: the
// one transaction here is read-only, and the dependencies do not include a dispatcher, a queue or
// the run log.
type TestRule struct {
	Rules      repository.Rules
	Catalogue  Catalogue
	Conditions expression.Compiler
	Entries    Entries
	Containers Containers
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// TestCommand names what to test and what to test it against: exactly one of RuleID and Rule, and
// the sample.
type TestCommand struct {
	RuleID shared.ID
	// Rule is a definition that was never saved - the document the create takes, checked by the
	// same validation, so a rule the dry run accepts is a rule the create will accept.
	Rule    *CreateRuleCommand
	Event   SampleEvent
	Payload map[string]any
}

// SampleEvent is the event the CEL `event` variable will read.
type SampleEvent struct {
	Type    string
	Subject string
	Payload map[string]any
}

// TestOutcome is what the rule would have done.
type TestOutcome struct {
	Matched    bool
	Conditions []domain.ConditionResult
	Actions    []PlannedAction
}

// PlannedAction is one action of the rule, with whether it would have run. Unlike a real run's
// log, both arms of every branch appear: the run records the path it took, and the dry run
// answers "and what if it had not" as well.
type PlannedAction struct {
	Path     string
	Kind     string
	WouldRun bool
	// Matched is how a BRANCH's condition answered the sample, nil for every other kind.
	Matched *bool
	// ErrorCode is present when a branch's condition could not be evaluated for the sample.
	ErrorCode string
}

// Execute answers what the rule would do.
func (h TestRule) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd TestCommand,
) (TestOutcome, error) {
	if (cmd.RuleID.IsZero()) == (cmd.Rule == nil) {
		return TestOutcome{}, shared.ErrValidation.
			WithDetail("automation.test_source_required").
			WithFields(shared.FieldError{Path: "/rule_id", Code: "automation.test_source_required"})
	}
	if trimmed(cmd.Event.Type) == "" {
		return TestOutcome{}, shared.ErrValidation.
			WithDetail("automation.test_event_type_required").
			WithFields(shared.FieldError{
				Path: "/sample_event/type", Code: "automation.test_event_type_required",
			})
	}

	rule, err := h.resolve(ctx, actor, cmd)
	if err != nil {
		return TestOutcome{}, err
	}
	// The plain permission at the rule's own scope, TriggerRuleManually's reason inverted: a dry
	// run performs nothing, so the composition check writing a rule needs would refuse people the
	// test exists for - somebody probing what a rule they may manage would do.
	if err := h.Authorizer.Authorize(
		ctx, actor, runRequest(rule.Scope, RuleTestedAction, rule.ID)); err != nil {
		return TestOutcome{}, err
	}

	envelope := event.Envelope{
		// A minted identity rather than an empty one, so a condition reading event.id sees the
		// shape a real event has - and a distinct one per test, exactly as runs differ.
		ID:         h.IDs.NewID(),
		Type:       event.Type(trimmed(cmd.Event.Type)),
		TenantID:   actor.TenantID,
		Subject:    trimmed(cmd.Event.Subject),
		OccurredAt: h.Clock.Now().UTC(),
		Payload:    cmd.Event.Payload,
	}
	values := condition.Values{
		Envelope: envelope, Now: h.Clock.Now(), Payload: cmd.Payload,
		Entries: h.Entries, Containers: h.Containers,
	}

	outcome := TestOutcome{}
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		outcome.Conditions, outcome.Matched = h.answers(ctx, rule, values)
		planner := &planner{compiler: h.Conditions, values: values}
		planner.list(ctx, rule.Actions, "", outcome.Matched && !failed(outcome.Conditions))
		outcome.Actions = planner.out
		return nil
	})
	if err != nil {
		return TestOutcome{}, err
	}
	return outcome, nil
}

// resolve answers the rule under test: a stored one read back, or an inline definition built
// through the same aggregate and the same checks the create runs - minus the delegation check,
// because a dry run delegates nothing to anybody.
func (h TestRule) resolve(
	ctx context.Context, actor appshared.ActorContext, cmd TestCommand,
) (domain.Rule, error) {
	if !cmd.RuleID.IsZero() {
		var rule domain.Rule
		err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
			func(ctx context.Context) error {
				var findErr error
				rule, findErr = h.Rules.Find(ctx, cmd.RuleID)
				return findErr
			})
		return rule, err
	}

	definition := *cmd.Rule
	rule, err := domain.NewRule(domain.NewRuleInput{
		ID: h.IDs.NewID(), TenantID: actor.TenantID, Name: definition.Name,
		Scope: definition.Scope, RunAs: definition.RunAs, Trigger: definition.Trigger,
		Conditions: definition.Conditions, Actions: definition.Actions,
		Throttle: definition.Throttle, OnError: definition.OnError,
		CreatedBy: actor.AccountID, Now: h.Clock.Now(),
	})
	if err != nil {
		return domain.Rule{}, err
	}
	if err := checkConditions(h.Conditions, rule); err != nil {
		return domain.Rule{}, err
	}
	if _, err := checkActions(h.Catalogue, rule.Actions); err != nil {
		return domain.Rule{}, err
	}
	return rule, nil
}

// answers evaluates every condition against the sample, exactly as the engine answers a real
// event: every one, not up to the first false.
func (h TestRule) answers(
	ctx context.Context, rule domain.Rule, values condition.Values,
) ([]domain.ConditionResult, bool) {
	results := make([]domain.ConditionResult, 0, len(rule.Conditions))
	matched := true

	for i, each := range rule.Conditions {
		result := domain.ConditionResult{Index: i}
		if h.Conditions == nil {
			result.ErrorCode = "automation.expression_engine_unavailable"
			results, matched = append(results, result), false
			continue
		}

		program, err := h.Conditions.Compile(
			each.Expr, condition.RuleEnvironment(), expression.Boolean)
		if err != nil {
			result.ErrorCode = codeOf(err)
			results, matched = append(results, result), false
			continue
		}
		out, err := program.Evaluate(ctx, values)
		if err != nil {
			result.ErrorCode = codeOf(err)
			results, matched = append(results, result), false
			continue
		}
		result.Matched = out.Bool
		if !out.Bool {
			matched = false
		}
		results = append(results, result)
	}
	return results, matched
}

// failed reports whether any condition could not be evaluated at all - a run would have FAILED
// rather than SKIPPED, and either way no action runs.
func failed(results []domain.ConditionResult) bool {
	for _, result := range results {
		if result.ErrorCode != "" {
			return true
		}
	}
	return false
}

// planner walks the action tree without acting: the dry-run counterpart of the engine's walk.
type planner struct {
	compiler expression.Compiler
	values   condition.Values
	out      []PlannedAction
	// ended is a STOP that would have run: everything after it, at any level, would not.
	ended bool
}

func (p *planner) list(ctx context.Context, actions []domain.Action, parent string, reached bool) {
	for i, action := range actions {
		path := domain.ActionPath(parent, i)
		would := reached && !p.ended
		entry := PlannedAction{Path: path, Kind: action.Kind, WouldRun: would}

		switch action.Kind {
		case domain.ActionStop:
			p.out = append(p.out, entry)
			if would {
				p.ended = true
			}
		case domain.ActionBranch:
			branch, err := domain.ReadBranch(action.Params, path, 0)
			if err != nil {
				entry.WouldRun, entry.ErrorCode = false, codeOf(err)
				p.out = append(p.out, entry)
				continue
			}
			matched, code := p.answer(ctx, branch.Condition)
			if code != "" {
				// The branch would run and fail; neither arm is reached. A real run under
				// on_error STOP would fail here, and the dry run says so with the same code.
				entry.ErrorCode = code
				p.out = append(p.out, entry)
				p.list(ctx, branch.Then, path+"/then", false)
				p.list(ctx, branch.Else, path+"/else", false)
				continue
			}
			entry.Matched = &matched
			p.out = append(p.out, entry)
			p.list(ctx, branch.Then, path+"/then", would && matched)
			p.list(ctx, branch.Else, path+"/else", would && !matched)
		default:
			p.out = append(p.out, entry)
		}
	}
}

// answer evaluates one branch condition against the sample.
func (p *planner) answer(ctx context.Context, expr string) (bool, string) {
	if p.compiler == nil {
		return false, "automation.expression_engine_unavailable"
	}
	program, err := p.compiler.Compile(expr, condition.RuleEnvironment(), expression.Boolean)
	if err != nil {
		return false, codeOf(err)
	}
	out, err := program.Evaluate(ctx, p.values)
	if err != nil {
		return false, codeOf(err)
	}
	return out.Bool, ""
}

// Descriptor is the catalogue entry.
func (h TestRule) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: TestRuleName,
		Summary: "Dry-runs a rule against a sample event: which conditions matched, and which " +
			"actions would run - both arms of every branch, which is more than a real run's log " +
			"shows. Tests a stored rule by identifier or a definition that was never saved, " +
			"through the same validation the create runs. No side effects: nothing below opens " +
			"a writing transaction, no action dispatches, and nothing is queued.",
		SideEffects: "None. Reads only - the same lazy reads a real run's conditions make.",
		TokenScope:  automationScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "rule_id", Kind: usecase.KindID, Description: "A stored rule to test."},
			{
				Name: "rule", Kind: usecase.KindObject,
				Description: "A definition to test instead: the document the create takes.",
			},
			{
				Name: "sample_event", Kind: usecase.KindObject, Required: true,
				Description: "The sample: type (required), subject, payload.",
			},
			{
				Name: "payload", Kind: usecase.KindObject,
				Description: "The body an inbound delivery would have carried, for the CEL " +
					"payload variable.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleTestedAction, TargetType: ruleTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h TestRule) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd := TestCommand{}
	if in.Present("rule_id") {
		id, err := in.ID("rule_id")
		if err != nil {
			return nil, err
		}
		cmd.RuleID = id
	}
	if raw, present := in["rule"].(map[string]any); present {
		definition, err := ruleDefinitionFrom(raw)
		if err != nil {
			return nil, err
		}
		cmd.Rule = &definition
	}
	if raw, present := in["sample_event"].(map[string]any); present {
		cmd.Event.Type, _ = raw["type"].(string)
		cmd.Event.Subject, _ = raw["subject"].(string)
		cmd.Event.Payload, _ = raw["payload"].(map[string]any)
	}
	cmd.Payload, _ = in["payload"].(map[string]any)

	outcome, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}

	conditions := make([]any, 0, len(outcome.Conditions))
	for _, result := range outcome.Conditions {
		row := map[string]any{"index": result.Index, "matched": result.Matched}
		if result.ErrorCode != "" {
			row["error_code"] = result.ErrorCode
		}
		conditions = append(conditions, row)
	}
	actions := make([]any, 0, len(outcome.Actions))
	for _, planned := range outcome.Actions {
		row := map[string]any{
			"path": planned.Path, "kind": planned.Kind, "would_run": planned.WouldRun,
		}
		if planned.Matched != nil {
			row["matched"] = *planned.Matched
		}
		if planned.ErrorCode != "" {
			row["error_code"] = planned.ErrorCode
		}
		actions = append(actions, row)
	}
	return usecase.Output{
		"matched":           outcome.Matched,
		"condition_results": conditions,
		"actions":           actions,
	}, nil
}

// ruleDefinitionFrom reads an inline definition out of the input document, through the same
// readers the create's own input goes through.
func ruleDefinitionFrom(raw map[string]any) (CreateRuleCommand, error) {
	scope, err := scopeFrom(raw["scope"], true)
	if err != nil {
		return CreateRuleCommand{}, err
	}
	runAs, err := usecase.Input(raw).ID("run_as")
	if err != nil {
		return CreateRuleCommand{}, err
	}
	trigger, err := triggerFrom(raw["trigger"])
	if err != nil {
		return CreateRuleCommand{}, err
	}
	conditions, err := conditionsFrom(raw["conditions"])
	if err != nil {
		return CreateRuleCommand{}, err
	}
	actions, err := actionsFrom(raw["actions"])
	if err != nil {
		return CreateRuleCommand{}, err
	}

	name, _ := raw["name"].(string)
	onError, _ := raw["on_error"].(string)
	return CreateRuleCommand{
		Name: name, Scope: scope, RunAs: runAs, Trigger: trigger,
		Conditions: conditions, Actions: actions,
		Throttle: throttleFrom(raw["throttle"]), OnError: domain.OnError(onError),
	}, nil
}
