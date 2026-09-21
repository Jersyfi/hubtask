// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/application/condition"
	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	identityrepository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The check (ADR-0060, F8-03): every reference a rule carries resolved against what exists now,
// before it fails at three in the morning.
//
// The two things the engine already does about a rule that cannot work - refusing it at the write
// (§2.2) and switching it off after five failed runs - both speak after the moment they are about.
// A rule that named a label which was deleted a week later, or an action kind a later version no
// longer serves, is stored, enabled, and found by its own failures. The check speaks before: it is
// asked for by the workspace and seeded by the deletion of anything a rule may name, and what it
// finds is written on the rule, where every reader of the rule sees it for free.

const (
	CheckRulesName = "CheckRules"

	// RulesCheckedAction is the one entry a check writes for itself: a pass over the workspace's
	// rules, by whom. What it found is on the rules; what it did about it is the disable's own
	// entry, so that a review reading "this rule was switched off" finds the reason beside it.
	RulesCheckedAction audit.Action = "automation.rules_checked"

	// DisabledByCheck is the reason the check gives when it switches a rule off - beside
	// DisabledByStreak, the one the engine gives, so that A-16's metric tells the two apart.
	DisabledByCheck = "check"

	// The finding codes, one per question the check asks (ADR-0060). Message codes, never text.
	FindingEventUnknown     = "automation.finding.event_unknown"
	FindingActionUnknown    = "automation.finding.action_unknown"
	FindingParameterUnknown = "automation.finding.parameter_unknown"
	FindingConditionInvalid = "automation.finding.condition_invalid"
	FindingAccountGone      = "automation.finding.account_gone"
	FindingReferenceGone    = "automation.finding.reference_gone"
	// FindingParameterMissing is a required parameter the rule does not carry and the run cannot
	// supply: a step that fails every time it is reached (F8-19, issue 856).
	FindingParameterMissing = "automation.finding.parameter_missing"
	// FindingRunnerWithoutRole is an account that exists and holds no membership anywhere on the
	// rule's scope path: a rule that finds no entry it may touch (F8-19, issue 817).
	FindingRunnerWithoutRole = "automation.finding.runner_without_role"
)

// referenceFields is the table from a parameter's name to the kind of thing it names (ADR-0060).
//
// A use case declares `label_id` of kind `id`; that it names a label is a convention of the name,
// and this table is where the convention is written down - once, and held to the catalogue by a
// test in one direction: every name here is a field some use case declares. The reverse is
// deliberately not asserted, because most `id` fields the catalogue declares are the run's to
// supply (the entry an event is about) and a rule almost never carries them; a field this table
// does not know is simply not resolved.
var referenceFields = map[string]repository.ReferenceKind{
	"label_id":        repository.ReferenceLabel,
	"bucket_id":       repository.ReferenceBucket,
	"container_id":    repository.ReferenceContainer,
	"parent_id":       repository.ReferenceContainer,
	"collection_id":   repository.ReferenceContainer,
	"template_id":     repository.ReferenceTemplate,
	"subscription_id": repository.ReferenceSubscription,
	"group_id":        repository.ReferenceGroup,
	"account_id":      repository.ReferenceAccount,
}

// ReferenceFields is the table, for the test that holds it to the catalogue.
func ReferenceFields() map[string]repository.ReferenceKind {
	copied := make(map[string]repository.ReferenceKind, len(referenceFields))
	for name, kind := range referenceFields {
		copied[name] = kind
	}
	return copied
}

// CheckRules checks every rule of the workspace the caller may read.
type CheckRules struct {
	Rules      repository.Rules
	References repository.References
	// Memberships answers whether the account the rule runs as holds a role anywhere on the
	// rule's scope path (F8-19). Nil in a build that does not ask - the question is then not
	// asked, as a nil Conditions asks nothing about the conditions.
	Memberships identityrepository.Memberships
	Catalogue   Catalogue
	// Conditions compiles every condition and every branch's, as the write does (ADR-0009).
	Conditions expression.Compiler
	Authorizer Authorizer
	Audit      audit.Sink
	// Owners tells a rule's author when the check switched it off - the path the failure streak
	// uses, because a rule that stopped working is the same news whoever noticed it first.
	Owners Owners
	// Signals counts each switch-off by reason (A-16); nil in a build that measures nothing.
	Signals    RunSignals
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute runs the check over the workspace and answers the rules with their findings.
//
// One transaction: the findings, the disables and their audit entries commit together, so a
// process that dies halfway leaves the rules as they were rather than half assessed. The rules
// are read in pages because the port pages them; the walk is inside the transaction because a
// rule written between two pages would otherwise be checked against a vocabulary the first page
// was not - which is the check contradicting itself within one answer.
func (h CheckRules) Execute(ctx context.Context, actor appshared.ActorContext) ([]domain.Rule, error) {
	if err := actor.RequireScope(automationScope); err != nil {
		return nil, err
	}
	return h.run(ctx, actor, func(ctx context.Context, rule domain.Rule) (bool, error) {
		return h.permits(ctx, actor, rule.Scope)
	})
}

// Sweep is the check as the installation runs it for one workspace - the job a deletion seeds
// (ADR-0060). Every rule of the tenant, because the system is not a caller with a scope to be
// held to; the audit entry names the installation, as the privacy performer's does.
func (h CheckRules) Sweep(ctx context.Context, tenantID shared.ID) error {
	if tenantID.IsZero() {
		return shared.ErrInternal.WithDetail("automation.check_without_tenant")
	}
	actor := appshared.ActorContext{
		Kind: appshared.ActorSystem, TenantID: tenantID, AccountName: "the installation",
	}
	_, err := h.run(ctx, actor, func(context.Context, domain.Rule) (bool, error) { return true, nil })
	return err
}

// run walks the workspace's rules and checks the ones the filter admits.
func (h CheckRules) run(
	ctx context.Context, actor appshared.ActorContext,
	admits func(context.Context, domain.Rule) (bool, error),
) ([]domain.Rule, error) {
	var checked []domain.Rule
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := h.Clock.Now()
		cursor := ""
		for {
			page, err := h.Rules.List(ctx, repository.Query{Cursor: cursor, Size: maxRulePage})
			if err != nil {
				return err
			}
			for _, rule := range page.Rules {
				allowed, err := admits(ctx, rule)
				if err != nil {
					return err
				}
				if !allowed {
					continue
				}
				rule, err = h.checkOne(ctx, actor, rule, now)
				if err != nil {
					return err
				}
				checked = append(checked, rule)
			}
			if !page.HasMore {
				break
			}
			cursor = page.NextCursor
		}
		if h.Audit != nil && len(checked) > 0 {
			_ = h.Audit.Append(ctx, audit.Entry{
				TenantID: checked[0].TenantID, OccurredAt: now, Action: RulesCheckedAction,
				Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
				ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
				TargetType: ruleTarget, TargetID: actor.TenantID,
				Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if checked == nil {
		checked = []domain.Rule{}
	}
	return checked, nil
}

// maxRulePage is the port's largest page; the walk asks for it so that a workspace's rules are as
// few round trips as they can be.
const maxRulePage = 200

// permits is the listing's quiet question: may this caller see rules at this scope.
func (h CheckRules) permits(
	ctx context.Context, actor appshared.ActorContext, scope domain.Scope,
) (bool, error) {
	return permitsRead(ctx, h.Authorizer, actor, scope)
}

// checkOne inspects a rule, records what it found, and acts on a BROKEN finding.
func (h CheckRules) checkOne(
	ctx context.Context, actor appshared.ActorContext, rule domain.Rule, now time.Time,
) (domain.Rule, error) {
	findings, err := h.inspect(ctx, rule)
	if err != nil {
		return domain.Rule{}, err
	}
	rule = rule.Checked(findings, now)
	if err := h.Rules.RecordCheck(ctx, rule.ID, rule.Findings, now); err != nil {
		return domain.Rule{}, err
	}
	if !rule.Broken() || !rule.Enabled {
		return rule, nil
	}

	// The rule cannot run and is on: off it goes, once, through the streak's path - the audit
	// entry with the reason beside it, the author told, the metric counted.
	changed, err := h.Rules.DisableBroken(ctx, rule.ID, now)
	if err != nil {
		return domain.Rule{}, err
	}
	if !changed {
		return rule, nil
	}
	rule = rule.Disable(now)
	if h.Audit != nil {
		_ = h.Audit.Append(ctx, audit.Entry{
			TenantID: rule.TenantID, OccurredAt: now, Action: RuleDisabledAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			OnBehalfOf: rule.RunAs, TargetType: ruleTarget, TargetID: rule.ID,
			Changes: map[string]any{"reason": DisabledByCheck},
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx), RuleID: rule.ID},
		})
	}
	if h.Signals != nil {
		h.Signals.RuleDisabled(ctx, DisabledByCheck)
	}
	if h.Owners != nil {
		if err := h.Owners.RuleDisabled(ctx, rule, now); err != nil {
			return domain.Rule{}, err
		}
	}
	return rule, nil
}

// inspect asks the six questions of ADR-0060 and answers with findings, cheapest first.
func (h CheckRules) inspect(ctx context.Context, rule domain.Rule) ([]domain.Finding, error) {
	var findings []domain.Finding

	// The trigger's event type, against what this build emits.
	if rule.Trigger.Kind == domain.TriggerEvent && !slices.Contains(event.Types(), rule.Trigger.EventType) {
		findings = append(findings, domain.Finding{
			Level: domain.FindingBroken, Path: "/trigger/event_type", Code: FindingEventUnknown,
			Params: map[string]string{"event_type": string(rule.Trigger.EventType)},
		})
	}

	// The account it runs as: gone, or unable to act, is a rule that can do nothing.
	exists, err := h.References.Exists(ctx, repository.ReferenceAccount, rule.RunAs)
	if err != nil {
		return nil, err
	}
	if !exists {
		findings = append(findings, domain.Finding{
			Level: domain.FindingBroken, Path: "/run_as", Code: FindingAccountGone,
			Params: map[string]string{"account_id": rule.RunAs.String()},
		})
	} else if h.Memberships != nil {
		// An account that exists and holds nothing on the path can run and will find nothing:
		// every entry action answers items.not_found, which is the right refusal for the API and
		// useless to the rule's writer (issue 817). ATTENTION rather than BROKEN, because the run
		// answers the question per action and a rule of outbound steps needs no role at all.
		held, err := h.Memberships.Along(ctx, rule.RunAs, rule.Scope.Path())
		if err != nil {
			return nil, err
		}
		if len(held) == 0 {
			findings = append(findings, domain.Finding{
				Level: domain.FindingAttention, Path: "/run_as", Code: FindingRunnerWithoutRole,
				Params: map[string]string{"account_id": rule.RunAs.String(), "scope": string(rule.Scope.Type)},
			})
		}
	}

	// Every condition, compiled as the write compiles it.
	if h.Conditions != nil {
		environment := condition.RuleEnvironment()
		for i, each := range rule.Conditions {
			if _, err := h.Conditions.Compile(each.Expr, environment, expression.Boolean); err != nil {
				findings = append(findings, conditionFinding("/conditions/"+itoa(i)+"/expr", err))
			}
		}
	}

	// Every action, and every action inside a branch, at the path a refusal would name.
	more, err := h.inspectActions(ctx, rule.Actions, "/actions", 0)
	if err != nil {
		return nil, err
	}
	return append(findings, more...), nil
}

// inspectActions walks an action list, branch arms included.
func (h CheckRules) inspectActions(
	ctx context.Context, actions []domain.Action, path string, depth int,
) ([]domain.Finding, error) {
	var findings []domain.Finding
	for i, action := range actions {
		at := path + "/" + itoa(i)

		if domain.IsFlowAction(action.Kind) {
			if action.Kind != domain.ActionBranch {
				continue
			}
			branch, err := domain.ReadBranch(action.Params, at, depth)
			if err != nil {
				// Unreadable through the aggregate that wrote it; nothing to resolve.
				continue
			}
			if h.Conditions != nil {
				if _, err := h.Conditions.Compile(
					branch.Condition, condition.RuleEnvironment(), expression.Boolean); err != nil {
					findings = append(findings, conditionFinding(at+"/params/condition", err))
				}
			}
			for arm, list := range map[string][]domain.Action{"then": branch.Then, "else": branch.Else} {
				more, err := h.inspectActions(ctx, list, at+"/params/"+arm, depth+1)
				if err != nil {
					return nil, err
				}
				findings = append(findings, more...)
			}
			continue
		}

		descriptor, found := h.Catalogue.ByAutomationAction(action.Kind)
		if !found {
			findings = append(findings, domain.Finding{
				Level: domain.FindingBroken, Path: at + "/kind", Code: FindingActionUnknown,
				Params: map[string]string{"kind": action.Kind},
			})
			continue
		}

		declared := map[string]usecase.Kind{}
		for _, field := range descriptor.Input {
			declared[field.Name] = field.Kind
			// A required parameter the rule does not carry and the run cannot supply is a step
			// that fails every time it is reached (issue 856). The write accepts the absence on
			// purpose - it may be the entry - so the check is where the two are told apart.
			if field.Required && !SuppliedByRun(field.Name) {
				if _, carried := action.Params[field.Name]; !carried {
					findings = append(findings, domain.Finding{
						Level: domain.FindingAttention, Path: at + "/params/" + field.Name, Code: FindingParameterMissing,
						Params: map[string]string{"kind": action.Kind, "parameter": field.Name},
					})
				}
			}
		}
		for _, name := range sortedKeys(action.Params) {
			kind, isDeclared := declared[name]
			if !isDeclared {
				findings = append(findings, domain.Finding{
					Level: domain.FindingAttention, Path: at + "/params/" + name, Code: FindingParameterUnknown,
					Params: map[string]string{"kind": action.Kind, "parameter": name},
				})
				continue
			}
			referenceKind, resolvable := referenceFields[name]
			if kind != usecase.KindID || !resolvable {
				continue
			}
			raw, _ := action.Params[name].(string)
			id, err := shared.ParseID(raw)
			if err != nil {
				// Not an identifier at all: the run's registry refuses it, and a finding about it
				// would be the check restating a validation it does not own.
				continue
			}
			exists, err := h.References.Exists(ctx, referenceKind, id)
			if err != nil {
				return nil, err
			}
			if !exists {
				findings = append(findings, domain.Finding{
					Level: domain.FindingAttention, Path: at + "/params/" + name, Code: FindingReferenceGone,
					Params: map[string]string{"kind": string(referenceKind), "id": raw},
				})
			}
		}
	}
	return findings, nil
}

// conditionFinding carries the compiler's own code and parameters - a line and a column - under
// the check's level, so that the finding says where exactly as the refusal would have.
func conditionFinding(path string, err error) domain.Finding {
	finding := domain.Finding{Level: domain.FindingBroken, Path: path, Code: FindingConditionInvalid}
	var coded *shared.Error
	if errors.As(err, &coded) {
		params := map[string]string{"reason": coded.DetailCode}
		for key, value := range coded.Params {
			params[key] = value
		}
		finding.Params = params
	}
	return finding
}

// Descriptor is the catalogue entry.
func (h CheckRules) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CheckRulesName,
		Summary: "Checks every rule of the workspace the caller may read against what exists " +
			"now - the trigger's event type, each action's kind and parameters, each condition, " +
			"the account it runs as, and every identifier a parameter names - and writes what it " +
			"finds on the rules. A rule that cannot run is switched off and its author told, " +
			"exactly as five failed runs would. Answers the rules with their findings.",
		SideEffects: "Writes the findings on every rule it checked; switches off a rule that " +
			"cannot run, with an audit entry and a notification to its author.",
		TokenScope: automationScope,
		Input:      []usecase.Field{},
		Audit: usecase.AuditDeclaration{
			Action: RulesCheckedAction, TargetType: ruleTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CheckRules) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	rules, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(rules))
	for _, rule := range rules {
		rows = append(rows, ruleOutput(rule))
	}
	return usecase.Output{"data": rows}, nil
}
