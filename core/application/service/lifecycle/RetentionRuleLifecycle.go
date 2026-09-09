// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// A rule that deletes data has to be correctable and withdrawable (F4-02). Until this, one could
// be written and then only read.
const (
	UpdateRetentionPolicyName = "UpdateRetentionPolicy"
	DeleteRetentionPolicyName = "DeleteRetentionPolicy"

	// RuleWithdrawnAction is its own code beside `rule_changed`, for the reason a removal always
	// is: "somebody corrected the rule" and "somebody withdrew it" are two different findings, and
	// the second is the one a reader of the trail looks for after data stopped disappearing.
	RuleWithdrawnAction audit.Action = "lifecycle.rule_withdrawn"
)

// UpdateRetentionPolicy corrects a rule, or takes it out of enforcement.
type UpdateRetentionPolicy struct{ Rules Rules }

// UpdateRetentionPolicyCommand is the merge-patch, typed. A nil pointer is a key the caller did
// not send. The kind and the scope are absent by construction: neither may move.
type UpdateRetentionPolicyCommand struct {
	ID              shared.ID
	Condition       *string
	RetainDays      *int
	Action          *domain.Action
	ThenAfterDays   *int
	ThenAction      *domain.Action
	GraceDays       *int
	Notify          *domain.Notify
	Justification   *string
	Enabled         *bool
	ExportTargetID  *shared.ID
	ExpectedVersion int
}

// Execute applies the patch and writes the corrected rule.
//
// §5's five per cent is **not** re-applied here, and the omission is deliberate. That switch is
// about a rule nobody has looked at yet - "a new rule always starts in NOTIFY_ONLY" - and a
// correction is the opposite situation: somebody is looking at this rule now, has the preview, and
// is deciding. Re-applying it would silently undo an operator's considered `action` every time
// they touched anything else about the rule.
func (h UpdateRetentionPolicy) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateRetentionPolicyCommand,
) (domain.Rule, error) {
	r := h.Rules
	// The owner's line, the same one writing a rule needs: correcting a standing instruction to
	// destroy work is the same class of act as writing one.
	if err := r.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     RuleChangedAction,
		TokenScope: retentionManage,
		TargetType: policyTarget,
		TargetID:   cmd.ID,
	}); err != nil {
		return domain.Rule{}, err
	}

	now := r.Clock.Now()
	var answer domain.Rule
	err := r.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, err := r.Rules.Find(ctx, cmd.ID)
		if err != nil {
			return err
		}

		ceiling, err := r.ceilingWithin(ctx, before.DataKind)
		if err != nil {
			return err
		}
		after, err := cmd.applyTo(before, ceiling, now)
		if err != nil {
			return err
		}
		if err := r.checkCondition(after.Condition); err != nil {
			return err
		}

		expected := before.Version
		if cmd.ExpectedVersion > 0 {
			expected = cmd.ExpectedVersion
		}
		written, err := r.Rules.Update(ctx, after, expected, now)
		if err != nil {
			return err
		}
		if !written {
			return shared.ErrConflict.
				WithDetail(domain.CodeRuleVersionConflict).
				WithParams(map[string]string{"policy_id": cmd.ID.String()})
		}

		after.Version = expected + 1
		after.UpdatedAt = now
		answer = after
		return r.recordChange(ctx, actor, before, after, now)
	})
	if err != nil {
		return domain.Rule{}, err
	}
	return answer, nil
}

// applyTo folds the patch into the stored rule and revalidates the whole of it.
//
// Revalidated rather than patched in place: `NewRule` is where a period, an action, a chain and
// the operator's ceiling are judged together, and a change that skipped it could store a rule
// creating one would have refused - a period beyond the bound with the justification quietly kept
// from the version before, for instance.
func (cmd UpdateRetentionPolicyCommand) applyTo(
	before domain.Rule, ceiling int, now time.Time,
) (domain.Rule, error) {
	in := domain.NewRuleInput{
		ID: before.ID, TenantID: before.TenantID, Scope: before.Scope,
		DataKind: before.DataKind, Condition: before.Condition,
		RetainDays: before.RetainDays, Action: before.Action,
		ThenAfterDays: before.ThenAfterDays, ThenAction: before.ThenAction,
		GraceDays: &before.GraceDays, Notify: &before.Notify,
		Justification: before.Justification, Enabled: &before.Enabled,
		ExportTargetID: before.ExportTargetID, CreatedBy: before.CreatedBy,
		Now: now, Ceiling: ceiling,
	}
	if cmd.Condition != nil {
		in.Condition = *cmd.Condition
	}
	if cmd.RetainDays != nil {
		in.RetainDays = *cmd.RetainDays
	}
	if cmd.Action != nil {
		in.Action = *cmd.Action
	}
	if cmd.ThenAfterDays != nil {
		in.ThenAfterDays = *cmd.ThenAfterDays
	}
	if cmd.ThenAction != nil {
		in.ThenAction = *cmd.ThenAction
	}
	if cmd.GraceDays != nil {
		in.GraceDays = cmd.GraceDays
	}
	if cmd.Notify != nil {
		in.Notify = cmd.Notify
	}
	if cmd.Justification != nil {
		in.Justification = *cmd.Justification
	}
	if cmd.Enabled != nil {
		in.Enabled = cmd.Enabled
	}
	if cmd.ExportTargetID != nil {
		in.ExportTargetID = *cmd.ExportTargetID
	}

	after, err := domain.NewRule(in)
	if err != nil {
		return domain.Rule{}, err
	}
	after.CreatedAt = before.CreatedAt
	after.Version = before.Version
	return after, nil
}

// DeleteRetentionPolicy withdraws a rule.
type DeleteRetentionPolicy struct{ Rules Rules }

// Execute withdraws it, and takes what it had marked out of its period.
//
// What the rule already did stands: retention deletes, and a deletion is not undone by withdrawing
// the instruction that caused it. What is undone is only the countdown - an entry waiting for a
// rule nobody holds any more would otherwise be deleted by a rule that does not exist.
func (h DeleteRetentionPolicy) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) error {
	r := h.Rules
	if err := r.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     RuleWithdrawnAction,
		TokenScope: retentionManage,
		TargetType: policyTarget,
		TargetID:   id,
	}); err != nil {
		return err
	}

	now := r.Clock.Now()
	return r.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// The markings go first. If the delete succeeded and the clearing then failed, the
		// transaction would roll both back - but the order still says what matters: the entries
		// are released because the rule is going, not the other way round.
		cleared, err := r.Rules.ClearMarks(ctx, id, now)
		if err != nil {
			return err
		}

		withdrawn, err := r.Rules.Delete(ctx, id)
		if err != nil {
			return err
		}
		if !withdrawn {
			// Nothing to withdraw is not a failure: the caller asked for it to be gone and it is.
			return nil
		}

		return r.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now,
			Action: RuleWithdrawnAction, Outcome: audit.OutcomeSuccess,
			Severity:  audit.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: policyTarget, TargetID: id,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(audit.Change{
				// How many entries stopped counting down. A number rather than the entries: a
				// trail entry is evidence about the act, and the entries are the workspace's.
				Field: "markings_cleared", Classification: audit.Open, To: itoa(cleared),
			}),
		})
	})
}

// ceilingWithin is `ceilingFor` for a caller that is already inside a transaction. The other one
// opens its own read, which nested inside a write would be a second transaction against a row the
// first one is about to change.
func (r Rules) ceilingWithin(ctx context.Context, kind domain.DataKind) (int, error) {
	if r.Policies == nil {
		return 0, nil
	}
	policy, err := r.Policies.Find(ctx, kind)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	if policy.MaxDays != nil {
		return *policy.MaxDays, nil
	}
	return 0, nil
}

// recordChange writes the entry with the before and after of every field that moved.
func (r Rules) recordChange(
	ctx context.Context, actor appshared.ActorContext, before, after domain.Rule, at time.Time,
) error {
	changes := []audit.Change{}
	add := func(field, from, to string) {
		if from != to {
			changes = append(changes, audit.Change{
				Field: field, Classification: audit.Open, From: from, To: to,
			})
		}
	}
	add("retain_days", itoa(before.RetainDays), itoa(after.RetainDays))
	add("action", string(before.Action), string(after.Action))
	add("then_after_days", itoa(before.ThenAfterDays), itoa(after.ThenAfterDays))
	add("then_action", string(before.ThenAction), string(after.ThenAction))
	add("grace_days", itoa(before.GraceDays), itoa(after.GraceDays))
	add("enabled", boolText(before.Enabled), boolText(after.Enabled))
	add("condition", before.Condition, after.Condition)
	// The operator's own words about their own policy, which is why it travels in clear where an
	// entry's content never would.
	add("justification", before.Justification, after.Justification)

	return r.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: at,
		Action: RuleChangedAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: policyTarget, TargetID: after.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (h UpdateRetentionPolicy) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateRetentionPolicyName,
		Summary: "Corrects a retention rule, or takes it out of enforcement. A field the caller " +
			"does not send does not move. `enabled: false` stops the rule entirely and " +
			"`action: NOTIFY_ONLY` keeps it reporting what it would remove while removing " +
			"nothing. The data kind and the scope do not move - a rule that changed either " +
			"would be a different rule under an old identifier. The five per cent switch is not " +
			"re-applied: it is for a rule nobody has looked at yet, and this is somebody looking.",
		SideEffects: "Writes the rule and an audit entry naming every field that moved.",
		TokenScope:  retentionManage,
		Input: []usecase.Field{
			{Name: "policy_id", Kind: usecase.KindID, Required: true},
			{Name: "condition", Kind: usecase.KindString,
				Description: "An expression narrowing what the rule matches. Empty clears it."},
			{Name: "retain_days", Kind: usecase.KindInt,
				Description: "How long the period is. Beyond the kind's upper bound needs a justification."},
			{Name: "action", Kind: usecase.KindString,
				Enum: []string{"ARCHIVE", "TRASH", "ANONYMIZE", "HARD_DELETE",
					"EXPORT_THEN_DELETE", "NOTIFY_ONLY"},
				Description: "What the rule does when the period runs out."},
			{Name: "then_after_days", Kind: usecase.KindInt, Description: "The chain's second stage, in days."},
			{Name: "then_action", Kind: usecase.KindString,
				Enum:        []string{"ARCHIVE", "TRASH", "ANONYMIZE", "HARD_DELETE", "EXPORT_THEN_DELETE"},
				Description: "What the second stage does."},
			{Name: "grace_days", Kind: usecase.KindInt, Description: "How long a warning stands before the act."},
			{Name: "notify_before_days", Kind: usecase.KindInt,
				Description: "How many days before the act a warning goes out."},
			{Name: "notify_recipients", Kind: usecase.KindList,
				Description: "Who is told: ITEM_MEMBERS, COLLECTION_ADMINS, TENANT_ADMINS."},
			{Name: "justification", Kind: usecase.KindString,
				Description: "Why the period exceeds the kind's upper bound. Replaced, not kept: " +
					"a justification belongs to the period it justifies."},
			{Name: "enabled", Kind: usecase.KindBool},
			{Name: "export_target_id", Kind: usecase.KindID,
				Description: "Where EXPORT_THEN_DELETE writes its archive."},
			{Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read. Omitted means the caller named none."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleChangedAction, TargetType: policyTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateRetentionPolicy) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("policy_id")
	if err != nil {
		return nil, err
	}
	cmd := UpdateRetentionPolicyCommand{ID: id, ExpectedVersion: in.Int("expected_version")}
	cmd.Condition = in.OptionalString("condition")
	cmd.Justification = in.OptionalString("justification")
	if in.Present("retain_days") {
		wanted := in.Int("retain_days")
		cmd.RetainDays = &wanted
	}
	if wanted := in.OptionalString("action"); wanted != nil && *wanted != "" {
		act := domain.Action(*wanted)
		cmd.Action = &act
	}
	if in.Present("then_after_days") {
		wanted := in.Int("then_after_days")
		cmd.ThenAfterDays = &wanted
	}
	if wanted := in.OptionalString("then_action"); wanted != nil {
		act := domain.Action(*wanted)
		cmd.ThenAction = &act
	}
	if in.Present("grace_days") {
		wanted := in.Int("grace_days")
		cmd.GraceDays = &wanted
	}
	if in.Present("enabled") {
		wanted := in.Bool("enabled")
		cmd.Enabled = &wanted
	}
	if in.Present("export_target_id") {
		target, err := in.ID("export_target_id")
		if err != nil {
			return nil, err
		}
		cmd.ExportTargetID = &target
	}
	if in.Present("notify_recipients") || in.Present("notify_before_days") {
		notify := domain.Notify{BeforeDays: in.Int("notify_before_days")}
		named, err := in.StringList("notify_recipients")
		if err != nil {
			return nil, err
		}
		for _, recipient := range named {
			notify.Recipients = append(notify.Recipients, domain.Recipient(recipient))
		}
		cmd.Notify = &notify
	}

	rule, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return ruleOutput(rule), nil
}

func (h DeleteRetentionPolicy) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteRetentionPolicyName,
		Summary: "Withdraws a retention rule. What it already did stands - retention deletes, " +
			"and a deletion is not undone by withdrawing the instruction that caused it - and " +
			"what it had marked for a future pass stops counting down, because an entry waiting " +
			"for a rule nobody holds any more would be deleted by a rule that does not exist.",
		SideEffects: "Removes the rule, clears its markings, and writes an audit entry.",
		TokenScope:  retentionManage,
		Input: []usecase.Field{
			{Name: "policy_id", Kind: usecase.KindID, Required: true},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleWithdrawnAction, TargetType: policyTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteRetentionPolicy) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("policy_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, id); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
