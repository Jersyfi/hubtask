// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The catalogue entries. The rule's own shapes travel as nested documents rather than as flattened
// fields, because that is what they are: a trigger is one object whose fields belong to its kind,
// and a channel that had to send `trigger_kind` and `trigger_event_type` would be a channel that
// can express a combination the aggregate refuses.

// Descriptor is the catalogue entry.
func (h CreateRule) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateRuleName,
		Summary: "Writes an automation rule, switched off. Enabling it is a separate call, so " +
			"that what a rule would do can be read back before it acts. Writing one needs the " +
			"automation permission at its scope and the rights its own actions need: a rule may " +
			"not do through a service account what its writer may not do directly. A non-empty " +
			"condition is refused until the expression language arrives, and an action naming a " +
			"kind no release serves yet is refused by name.",
		SideEffects: "Writes the rule and an audit entry. The rule does nothing until it is enabled.",
		TokenScope:  automationScope,
		Input: []usecase.Field{
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "What the rule is called."},
			{
				Name: "scope", Kind: usecase.KindObject, Required: true,
				Description: "Where it applies: {type: TENANT|HUB|COLLECTION, id}. Descendants are included.",
			},
			{
				Name: "run_as", Kind: usecase.KindID, Required: true,
				Description: "The service account the rule acts as. It can never do more than that account may.",
			},
			{
				Name: "trigger", Kind: usecase.KindObject, Required: true,
				Description: "What starts a run: {kind, and the fields that kind needs}.",
			},
			{
				Name: "conditions", Kind: usecase.KindList,
				Description: "Refused while non-empty: the expression language arrives with the engine.",
			},
			{
				Name: "actions", Kind: usecase.KindList, Required: true,
				Description: "What the rule does: [{kind, params}], at least one.",
			},
			{
				Name: "throttle", Kind: usecase.KindObject,
				Description: "{max_runs_per_hour}. A dedupe expression is refused with the conditions.",
			},
			{
				Name: "on_error", Kind: usecase.KindString, Enum: []string{"STOP", "CONTINUE", "RETRY"},
				Description: "What a failing action does to the rest of the run. STOP by default.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleCreatedAction, TargetType: ruleTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A rule is workspace configuration, not something that happened to an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h GetRule) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        GetRuleName,
		Summary:     "One automation rule, as it stands.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "rule_id", Kind: usecase.KindID, Required: true, Description: "Which rule."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleReadAction, TargetType: ruleTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h ListRules) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListRulesName,
		Summary: "The workspace's automation rules, newest first - and only the ones the caller " +
			"may see at their own scope. Deleted rules are not among them.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "enabled", Kind: usecase.KindBool, Description: "Narrow to the rules that are on, or off."},
			{Name: "cursor", Kind: usecase.KindString, Description: "Where the last page stopped."},
			{Name: "size", Kind: usecase.KindInt, Description: "How many at most. Fifty by default, two hundred at most."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleReadAction, TargetType: ruleTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h UpdateRule) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateRuleName,
		Summary: "Changes what a rule does. An omitted field is left alone. `enabled` is not " +
			"among the fields: switching a rule on or off is its own call, so the trail says " +
			"which of the two somebody did. The same two-sided permission check as writing one, " +
			"and against the rule as it would be afterwards.",
		SideEffects: "Writes the rule and an audit entry.",
		TokenScope:  automationScope,
		Input: []usecase.Field{
			{Name: "rule_id", Kind: usecase.KindID, Required: true, Description: "Which rule."},
			{Name: "name", Kind: usecase.KindString, Description: "The new name. Omitted leaves it."},
			{Name: "scope", Kind: usecase.KindObject, Description: "The new scope. Omitted leaves it."},
			{Name: "run_as", Kind: usecase.KindID, Description: "The new account. Omitted leaves it."},
			{Name: "trigger", Kind: usecase.KindObject, Description: "The new trigger, whole. Omitted leaves it."},
			{Name: "conditions", Kind: usecase.KindList, Description: "Refused while non-empty."},
			{Name: "actions", Kind: usecase.KindList, Description: "The new steps. Omitted leaves them."},
			{Name: "throttle", Kind: usecase.KindObject, Description: "The new bounds. Omitted leaves them."},
			{
				Name: "on_error", Kind: usecase.KindString, Enum: []string{"STOP", "CONTINUE", "RETRY"},
				Description: "The new behaviour on failure. Omitted leaves it.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read. A mismatch is a conflict rather than an overwrite.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleUpdatedAction, TargetType: ruleTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The same reason writing one is exempt: a rule is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h EnableRule) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: EnableRuleName,
		Summary: "Switches a rule on, and clears its failure count. Its own call and its own " +
			"audit entry, because letting a rule act on the workspace is the decision worth " +
			"recording - and the permission is asked again here rather than trusted from " +
			"whenever the rule was written.",
		SideEffects: "Switches the rule on and writes an audit entry. The rule begins to act.",
		TokenScope:  automationScope,
		Input: []usecase.Field{
			{Name: "rule_id", Kind: usecase.KindID, Required: true, Description: "Which rule."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleEnabledAction, TargetType: ruleTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The same reason writing one is exempt: a rule is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h DisableRule) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DisableRuleName,
		Summary: "Switches a rule off. It stays, and stops acting. What a run of failures does " +
			"automatically, a person can do deliberately - and the trail says which of the two " +
			"it was. The failure count is left alone: switching a rule off by hand has not fixed " +
			"whatever stopped it.",
		SideEffects: "Switches the rule off and writes an audit entry.",
		TokenScope:  automationScope,
		Input: []usecase.Field{
			{Name: "rule_id", Kind: usecase.KindID, Required: true, Description: "Which rule."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleDisabledAction, TargetType: ruleTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The same reason writing one is exempt: a rule is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h DeleteRule) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteRuleName,
		Summary: "Removes a rule. A soft delete: it stops matching anything at once and leaves " +
			"the listing, and the runs it produced stay readable - a run log whose rule vanished " +
			"would be a record of actions nobody can account for.",
		SideEffects: "Stamps the rule as deleted and writes an audit entry. Its runs stay.",
		TokenScope:  automationScope,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "rule_id", Kind: usecase.KindID, Required: true, Description: "Which rule."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleDeletedAction, TargetType: ruleTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The same reason writing one is exempt: a rule is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateRule) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	scope, err := scopeFrom(in["scope"], true)
	if err != nil {
		return nil, err
	}
	runAs, err := in.ID("run_as")
	if err != nil {
		return nil, err
	}
	trigger, err := triggerFrom(in["trigger"])
	if err != nil {
		return nil, err
	}
	conditions, err := conditionsFrom(in["conditions"])
	if err != nil {
		return nil, err
	}
	actions, err := actionsFrom(in["actions"])
	if err != nil {
		return nil, err
	}

	rule, err := h.Execute(ctx, actor, CreateRuleCommand{
		Name: in.String("name"), Scope: scope, RunAs: runAs, Trigger: trigger,
		Conditions: conditions, Actions: actions,
		Throttle: throttleFrom(in["throttle"]), OnError: domain.OnError(in.String("on_error")),
	})
	if err != nil {
		return nil, err
	}
	return ruleOutput(rule), nil
}

func (h GetRule) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("rule_id")
	if err != nil {
		return nil, err
	}
	rule, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return ruleOutput(rule), nil
}

func (h ListRules) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	query := repository.Query{Cursor: in.String("cursor"), Size: in.Int("size")}
	if in.Present("enabled") {
		enabled := in.Bool("enabled")
		query.Enabled = &enabled
	}

	page, err := h.Execute(ctx, actor, query)
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(page.Rules))
	for _, rule := range page.Rules {
		rows = append(rows, ruleOutput(rule))
	}
	state := map[string]any{"next_cursor": nil, "has_more": page.HasMore}
	if page.NextCursor != "" {
		state["next_cursor"] = page.NextCursor
	}
	return usecase.Output{"data": rows, "page": state}, nil
}

func (h UpdateRule) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("rule_id")
	if err != nil {
		return nil, err
	}

	cmd := UpdateRuleCommand{ID: id, ExpectedVersion: in.Int("expected_version")}
	cmd.Name = in.OptionalString("name")
	if value := in.OptionalString("on_error"); value != nil {
		onError := domain.OnError(*value)
		cmd.OnError = &onError
	}
	if in.Present("scope") {
		scope, err := scopeFrom(in["scope"], true)
		if err != nil {
			return nil, err
		}
		cmd.Scope = &scope
	}
	if in.Present("run_as") {
		runAs, err := in.ID("run_as")
		if err != nil {
			return nil, err
		}
		cmd.RunAs = &runAs
	}
	if in.Present("trigger") {
		trigger, err := triggerFrom(in["trigger"])
		if err != nil {
			return nil, err
		}
		cmd.Trigger = &trigger
	}
	if in.Present("conditions") {
		conditions, err := conditionsFrom(in["conditions"])
		if err != nil {
			return nil, err
		}
		cmd.Conditions = &conditions
	}
	if in.Present("actions") {
		actions, err := actionsFrom(in["actions"])
		if err != nil {
			return nil, err
		}
		cmd.Actions = &actions
	}
	if in.Present("throttle") {
		throttle := throttleFrom(in["throttle"])
		cmd.Throttle = &throttle
	}

	rule, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return ruleOutput(rule), nil
}

func (h EnableRule) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("rule_id")
	if err != nil {
		return nil, err
	}
	rule, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return ruleOutput(rule), nil
}

func (h DisableRule) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("rule_id")
	if err != nil {
		return nil, err
	}
	rule, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return ruleOutput(rule), nil
}

func (h DeleteRule) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("rule_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, id); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// ruleOutput is the projection every channel answers from. There is one, so REST, MCP and an
// automation action cannot come to describe a rule differently.
func ruleOutput(rule domain.Rule) usecase.Output {
	conditions := make([]any, 0, len(rule.Conditions))
	for _, condition := range rule.Conditions {
		conditions = append(conditions, map[string]any{"expr": condition.Expr})
	}
	actions := make([]any, 0, len(rule.Actions))
	for _, action := range rule.Actions {
		actions = append(actions, map[string]any{"kind": action.Kind, "params": action.Params})
	}

	scope := map[string]any{"type": string(rule.Scope.Type)}
	if !rule.Scope.ID.IsZero() {
		scope["id"] = rule.Scope.ID.String()
	}
	throttle := map[string]any{}
	if rule.Throttle.MaxRunsPerHour > 0 {
		throttle["max_runs_per_hour"] = rule.Throttle.MaxRunsPerHour
	}
	if rule.Throttle.DedupeKeyExpr != "" {
		throttle["dedupe_key_expr"] = rule.Throttle.DedupeKeyExpr
	}

	out := usecase.Output{
		"id":            rule.ID.String(),
		"name":          rule.Name,
		"scope":         scope,
		"enabled":       rule.Enabled,
		"run_as":        rule.RunAs.String(),
		"trigger":       triggerOutput(rule.Trigger),
		"conditions":    conditions,
		"actions":       actions,
		"throttle":      throttle,
		"on_error":      string(rule.OnError),
		"failure_count": rule.FailureCount,
		"created_by":    rule.CreatedBy.String(),
		"created_at":    rule.CreatedAt,
		"updated_at":    rule.UpdatedAt,
		"version":       rule.Version,
		// The two moments this installation worked out for the rule rather than read from its
		// definition (G-08). Absent where they mean nothing, which is what an absent key says.
		"next_run_at":        nil,
		"inbound_rotated_at": nil,
	}
	if !rule.NextRunAt.IsZero() {
		out["next_run_at"] = rule.NextRunAt
	}
	if !rule.InboundRotatedAt.IsZero() {
		out["inbound_rotated_at"] = rule.InboundRotatedAt
	}
	return out
}

// triggerOutput answers the fields the trigger's own kind carries and no others, which is what the
// aggregate stored.
func triggerOutput(trigger domain.Trigger) map[string]any {
	out := map[string]any{"kind": string(trigger.Kind)}
	for name, value := range map[string]string{
		"event_type": trigger.EventType.String(),
		"rrule":      trigger.RRule,
		"timezone":   trigger.Timezone,
		"anchor":     string(trigger.Anchor),
		"offset":     trigger.Offset,
	} {
		if value != "" {
			out[name] = value
		}
	}
	if len(trigger.ChangedFields) > 0 {
		fields := make([]any, 0, len(trigger.ChangedFields))
		for _, field := range trigger.ChangedFields {
			fields = append(fields, field)
		}
		out["changed_fields"] = fields
	}
	return out
}

// The readers below turn the channel-neutral input into the aggregate's own types.
//
// A document that is not a document is a validation refusal naming the field, never a zero value: a
// rule saved from `{"trigger": "EVENT"}` would be a rule with no trigger at all, and the caller
// would be told nothing.

func scopeFrom(value any, required bool) (domain.Scope, error) {
	document, err := objectOf(value, "/scope", required)
	if err != nil {
		return domain.Scope{}, err
	}
	if document == nil {
		return domain.Scope{}, nil
	}

	scope := domain.Scope{Type: domain.ScopeType(textOf(document["type"]))}
	if id := textOf(document["id"]); id != "" {
		parsed, err := shared.ParseID(id)
		if err != nil {
			return domain.Scope{}, fieldRefusal("/scope/id", "automation.scope_id_invalid")
		}
		scope.ID = parsed
	}
	return scope, nil
}

func triggerFrom(value any) (domain.Trigger, error) {
	document, err := objectOf(value, "/trigger", true)
	if err != nil {
		return domain.Trigger{}, err
	}

	trigger := domain.Trigger{
		Kind: domain.TriggerKind(textOf(document["kind"])),
		// The event type is not resolved against the catalogue here: the aggregate does that, and a
		// reader that judged it would be the second place an unknown type is refused.
		EventType: event.Type(textOf(document["event_type"])),
		RRule:     textOf(document["rrule"]), Timezone: textOf(document["timezone"]),
		Anchor: domain.DateAnchor(textOf(document["anchor"])), Offset: textOf(document["offset"]),
	}
	for _, field := range listOf(document["changed_fields"]) {
		if name := textOf(field); name != "" {
			trigger.ChangedFields = append(trigger.ChangedFields, name)
		}
	}
	return trigger, nil
}

func conditionsFrom(value any) ([]domain.Condition, error) {
	if value == nil {
		return nil, nil
	}
	entries := listOf(value)
	conditions := make([]domain.Condition, 0, len(entries))
	for i, entry := range entries {
		document, err := objectOf(entry, "/conditions/"+itoa(i), true)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, domain.Condition{Expr: textOf(document["expr"])})
	}
	return conditions, nil
}

func actionsFrom(value any) ([]domain.Action, error) {
	entries := listOf(value)
	actions := make([]domain.Action, 0, len(entries))
	for i, entry := range entries {
		document, err := objectOf(entry, "/actions/"+itoa(i), true)
		if err != nil {
			return nil, err
		}
		action := domain.Action{Kind: textOf(document["kind"]), Params: map[string]any{}}
		if params, ok := document["params"].(map[string]any); ok {
			action.Params = params
		}
		actions = append(actions, action)
	}
	return actions, nil
}

func throttleFrom(value any) domain.Throttle {
	document, _ := value.(map[string]any)
	if document == nil {
		return domain.Throttle{}
	}
	return domain.Throttle{
		MaxRunsPerHour: intOf(document["max_runs_per_hour"]),
		DedupeKeyExpr:  textOf(document["dedupe_key_expr"]),
	}
}

func objectOf(value any, path string, required bool) (map[string]any, error) {
	if value == nil {
		if required {
			return nil, fieldRefusal(path, "automation.object_required")
		}
		return nil, nil
	}
	document, ok := value.(map[string]any)
	if !ok {
		return nil, fieldRefusal(path, "automation.object_invalid")
	}
	return document, nil
}

func listOf(value any) []any {
	entries, _ := value.([]any)
	return entries
}

func textOf(value any) string {
	text, _ := value.(string)
	return trimmed(text)
}

// intOf reads a number that may have arrived as either of JSON's two shapes: an int from a Go
// caller, a float64 from a decoded body.
func intOf(value any) int {
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	default:
		return 0
	}
}

func fieldRefusal(path, code string) error {
	return shared.ErrValidation.
		WithDetail(code).
		WithFields(shared.FieldError{Path: path, Code: code})
}
