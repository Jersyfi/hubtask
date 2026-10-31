// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The automation rules. The controller holds no rules of its own: who may write one, what it
// may be made to do and whose account it may act as are all decided inwards of here (ADR-0005).
// What this layer does is map a body to an input and an answer to a document - and it maps the
// nested shapes whole, because a trigger's fields belong to its kind and flattening them would let
// a channel express a combination the aggregate refuses.

// The readers. Each maps one document whole, so that what reaches the use case is the shape the
// aggregate validates rather than a flattening of it.

// ruleResponse renders one rule.
func ruleResponse(out usecase.Output) openapi.AutomationRule {
	rule := openapi.AutomationRule{
		Id:           uuidValue(out.String("id")),
		Name:         out.String("name"),
		Enabled:      boolAt(out, "enabled"),
		RunAs:        uuidValue(out.String("run_as")),
		OnError:      openapi.AutomationRuleOnError(out.String("on_error")),
		FailureCount: out.Int("failure_count"),
		CreatedBy:    uuidValue(out.String("created_by")),
		CreatedAt:    timeValue(out["created_at"]),
		UpdatedAt:    timeValue(out["updated_at"]),
		Version:      out.Int("version"),
		Scope:        scopeResponse(out["scope"]),
		Trigger:      triggerResponse(out["trigger"]),
		Conditions:   conditionsResponse(out["conditions"]),
		Actions:      actionsResponse(out["actions"]),
	}
	if throttle, present := throttleResponse(out["throttle"]); present {
		rule.Throttle = &throttle
	}
	// The two moments this installation worked out for the rule rather than read from its
	// definition. Absent where they mean nothing, which is what a null says.
	if at, present := out["next_run_at"].(time.Time); present {
		rule.NextRunAt = &at
	}
	if at, present := out["inbound_rotated_at"].(time.Time); present {
		rule.InboundRotatedAt = &at
	}
	// What the check found (ADR-0060): always an array, and the moment only where there was one.
	findings := findingsResponse(out["findings"])
	rule.Findings = &findings
	if at, present := out["checked_at"].(time.Time); present {
		rule.CheckedAt = &at
	}
	if last, present := out["last_run"].(map[string]any); present {
		at, _ := last["at"].(time.Time)
		status, _ := last["status"].(string)
		rule.LastRun = &struct {
			At     time.Time             `json:"at"`
			Status openapi.RuleRunStatus `json:"status"`
		}{At: at, Status: openapi.RuleRunStatus(status)}
	}
	return rule
}

func findingsResponse(value any) []openapi.RuleFinding {
	rows, _ := value.([]any)
	findings := make([]openapi.RuleFinding, 0, len(rows))
	for _, row := range rows {
		entry, _ := row.(map[string]any)
		finding := openapi.RuleFinding{
			Level: openapi.RuleFindingLevel(stringAt(entry, "level")),
			Path:  stringAt(entry, "path"),
			Code:  stringAt(entry, "code"),
		}
		if params, present := entry["params"].(map[string]string); present && len(params) > 0 {
			copied := make(map[string]string, len(params))
			for key, param := range params {
				copied[key] = param
			}
			finding.Params = &copied
		}
		findings = append(findings, finding)
	}
	return findings
}

func scopeResponse(value any) openapi.RuleScope {
	document, _ := value.(map[string]any)
	scope := openapi.RuleScope{Type: openapi.RuleScopeType(textField(document, "type"))}
	if id := textField(document, "id"); id != "" {
		parsed := uuidValue(id)
		scope.Id = &parsed
	}
	return scope
}

func triggerResponse(value any) openapi.RuleTrigger {
	document, _ := value.(map[string]any)
	trigger := openapi.RuleTrigger{Kind: openapi.RuleTriggerKind(textField(document, "kind"))}

	for name, into := range map[string]**string{
		"event_type": &trigger.EventType,
		"rrule":      &trigger.Rrule,
		"timezone":   &trigger.Timezone,
		"offset":     &trigger.Offset,
		"step":       &trigger.Step,
		"workflow":   &trigger.Workflow,
	} {
		if text := textField(document, name); text != "" {
			value := text
			*into = &value
		}
	}
	if anchor := textField(document, "anchor"); anchor != "" {
		value := openapi.RuleTriggerAnchor(anchor)
		trigger.Anchor = &value
	}
	if raw, present := document["changed_fields"].([]any); present {
		fields := make([]string, 0, len(raw))
		for _, field := range raw {
			if text, ok := field.(string); ok {
				fields = append(fields, text)
			}
		}
		trigger.ChangedFields = &fields
	}
	return trigger
}

func conditionsResponse(value any) []openapi.RuleCondition {
	raw, _ := value.([]any)
	conditions := make([]openapi.RuleCondition, 0, len(raw))
	for _, entry := range raw {
		document, _ := entry.(map[string]any)
		conditions = append(conditions, openapi.RuleCondition{Expr: textField(document, "expr")})
	}
	return conditions
}

func actionsResponse(value any) []openapi.RuleAction {
	raw, _ := value.([]any)
	actions := make([]openapi.RuleAction, 0, len(raw))
	for _, entry := range raw {
		document, _ := entry.(map[string]any)
		action := openapi.RuleAction{Kind: textField(document, "kind")}
		if params, ok := document["params"].(map[string]any); ok {
			carried := map[string]any(params)
			action.Params = &carried
		}
		actions = append(actions, action)
	}
	return actions
}

// throttleResponse answers whether there is a throttle at all: an empty document is a rule nobody
// bounded, and answering `{}` would read as one somebody bounded with nothing.
func throttleResponse(value any) (openapi.RuleThrottle, bool) {
	document, _ := value.(map[string]any)
	if len(document) == 0 {
		return openapi.RuleThrottle{}, false
	}

	throttle := openapi.RuleThrottle{}
	if count, ok := document["max_runs_per_hour"].(int); ok {
		throttle.MaxRunsPerHour = &count
	}
	if expr := textField(document, "dedupe_key_expr"); expr != "" {
		throttle.DedupeKeyExpr = &expr
	}
	return throttle, true
}

func textField(document map[string]any, name string) string {
	text, _ := document[name].(string)
	return text
}

// The run log. What a rule did, why it did not, and what each action answered.

// ruleTestResponse draws what a dry run of a rule found.
func ruleTestResponse(out usecase.Output) openapi.RuleTestResult {
	matched, _ := out["matched"].(bool)
	return openapi.RuleTestResult{
		Matched:          matched,
		ConditionResults: conditionResultsResponse(out["condition_results"]),
		Actions:          testActionsResponse(out["actions"]),
	}
}

func testActionsResponse(value any) []openapi.RuleTestAction {
	raw, _ := value.([]any)
	results := make([]openapi.RuleTestAction, 0, len(raw))
	for _, entry := range raw {
		row, _ := entry.(map[string]any)
		result := openapi.RuleTestAction{
			Path:     textField(row, "path"),
			Kind:     textField(row, "kind"),
			WouldRun: boolField(row, "would_run"),
		}
		if matched, present := row["matched"].(bool); present {
			result.Matched = &matched
		}
		if code := textField(row, "error_code"); code != "" {
			result.ErrorCode = &code
		}
		results = append(results, result)
	}
	return results
}

func runResponse(out usecase.Output) openapi.RuleRun {
	run := openapi.RuleRun{
		Id:               uuidValue(out.String("id")),
		RuleId:           uuidValue(out.String("rule_id")),
		Trigger:          openapi.RuleRunTrigger(out.String("trigger")),
		Status:           openapi.RuleRunStatus(out.String("status")),
		ConditionResults: conditionResultsResponse(out["condition_results"]),
		ActionResults:    actionResultsResponse(out["action_results"]),
		StartedAt:        timeValue(out["started_at"]),
		CausationDepth:   out.Int("causation_depth"),
	}
	if id := out.String("event_id"); id != "" {
		event := uuidValue(id)
		run.EventId = &event
	}
	if id := out.String("triggered_by"); id != "" {
		actor := uuidValue(id)
		run.TriggeredBy = &actor
	}
	if id := out.String("subject_id"); id != "" {
		subject := uuidValue(id)
		run.SubjectId = &subject
	}
	if finished, present := out["finished_at"].(time.Time); present {
		run.FinishedAt = &finished
	}
	if code := out.String("error_code"); code != "" {
		run.ErrorCode = &code
	}
	return run
}

func conditionResultsResponse(value any) []openapi.RuleConditionResult {
	raw, _ := value.([]any)
	results := make([]openapi.RuleConditionResult, 0, len(raw))
	for _, entry := range raw {
		row, _ := entry.(map[string]any)
		result := openapi.RuleConditionResult{
			Index:   intField(row, "index"),
			Matched: boolField(row, "matched"),
		}
		if code := textField(row, "error_code"); code != "" {
			result.ErrorCode = &code
		}
		results = append(results, result)
	}
	return results
}

func actionResultsResponse(value any) []openapi.RuleActionResult {
	raw, _ := value.([]any)
	results := make([]openapi.RuleActionResult, 0, len(raw))
	for _, entry := range raw {
		row, _ := entry.(map[string]any)
		result := openapi.RuleActionResult{
			Index:  intField(row, "index"),
			Kind:   textField(row, "kind"),
			Status: openapi.RuleActionResultStatus(textField(row, "status")),
		}
		if path := textField(row, "path"); path != "" {
			result.Path = &path
		}
		if matched, present := row["matched"].(bool); present {
			result.Matched = &matched
		}
		if code := textField(row, "error_code"); code != "" {
			result.ErrorCode = &code
		}
		if key := textField(row, "idempotency_key"); key != "" {
			result.IdempotencyKey = &key
		}
		results = append(results, result)
	}
	return results
}

func intField(document map[string]any, name string) int {
	switch value := document[name].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func boolField(document map[string]any, name string) bool {
	value, _ := document[name].(bool)
	return value
}

// ruleRunAcceptedResponse names the run a trigger or a replay queued.
func ruleRunAcceptedResponse(out usecase.Output) openapi.RuleRunAccepted {
	return openapi.RuleRunAccepted{RunId: uuidValue(out.String("run_id")), RuleId: uuidValue(out.String("rule_id"))}
}

// inboundTriggerTokenResponse is the one answer that carries an inbound trigger's token.
func inboundTriggerTokenResponse(out usecase.Output) openapi.InboundTriggerToken {
	return openapi.InboundTriggerToken{
		RuleId: uuidValue(out.String("rule_id")), Token: out.String("token"), RotatedAt: timeValue(out["rotated_at"]),
	}
}

// checkedRulesResponse is every rule with what the check found, unpaged.
func checkedRulesResponse(out usecase.Output) map[string]any {
	rows := rowsOf(out)
	rules := make([]openapi.AutomationRule, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, ruleResponse(row))
	}
	return map[string]any{"data": rules}
}

