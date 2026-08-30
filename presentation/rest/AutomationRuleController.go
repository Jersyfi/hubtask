// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The automation rules (G-05). The controller holds no rules of its own: who may write one, what it
// may be made to do and whose account it may act as are all decided inwards of here (ADR-0005).
// What this layer does is map a body to an input and an answer to a document - and it maps the
// nested shapes whole, because a trigger's fields belong to its kind and flattening them would let
// a channel express a combination the aggregate refuses.

const (
	createRuleUseCase  = "CreateRule"
	getRuleUseCase     = "GetRule"
	listRulesUseCase   = "ListRules"
	updateRuleUseCase  = "UpdateRule"
	enableRuleUseCase  = "EnableRule"
	disableRuleUseCase = "DisableRule"
	deleteRuleUseCase  = "DeleteRule"
	triggerRuleUseCase = "TriggerRuleManually"
)

// ListAutomationRules answers GET /automation/rules.
func (c *RestController) ListRules(
	w http.ResponseWriter, r *http.Request, params openapi.ListRulesParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		input := usecase.Input{}
		if params.Enabled != nil {
			input["enabled"] = *params.Enabled
		}
		if params.Cursor != nil {
			input["cursor"] = *params.Cursor
		}
		if params.Size != nil {
			input["size"] = *params.Size
		}
		return c.UseCases.Invoke(r.Context(), listRulesUseCase, actor, input)
	}, func(out usecase.Output) {
		rows, _ := out["data"].([]usecase.Output)
		rules := make([]openapi.AutomationRule, 0, len(rows))
		for _, row := range rows {
			rules = append(rules, ruleResponse(row))
		}
		writeJSON(w, r, http.StatusOK, openapi.AutomationRulePage{
			Data: rules, Page: pageResponse(out),
		})
	})
}

// CreateAutomationRule answers POST /automation/rules.
func (c *RestController) CreateRule(
	w http.ResponseWriter, r *http.Request, _ openapi.CreateRuleParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.AutomationRuleCreate
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}

		input := usecase.Input{
			"name":    body.Name,
			"scope":   scopeInput(body.Scope),
			"run_as":  body.RunAs.String(),
			"trigger": triggerInput(body.Trigger),
			"actions": actionsInput(body.Actions),
		}
		if body.Conditions != nil {
			// Sent only when the client sent one, so that the refusal is about a condition the
			// caller actually wrote rather than about an absent field.
			input["conditions"] = conditionsInput(*body.Conditions)
		}
		if body.Throttle != nil {
			input["throttle"] = throttleInput(*body.Throttle)
		}
		if body.OnError != nil {
			input["on_error"] = string(*body.OnError)
		}
		return c.UseCases.Invoke(r.Context(), createRuleUseCase, actor, input)
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusCreated, ruleResponse(out))
	})
}

// GetAutomationRule answers GET /automation/rules/{ruleId}.
func (c *RestController) GetRule(
	w http.ResponseWriter, r *http.Request, ruleID openapi.RuleId,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), getRuleUseCase, actor,
			usecase.Input{"rule_id": ruleID.String()})
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, ruleResponse(out))
	})
}

// UpdateAutomationRule answers PATCH /automation/rules/{ruleId}.
func (c *RestController) UpdateRule(
	w http.ResponseWriter, r *http.Request, ruleID openapi.RuleId,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.AutomationRuleUpdate
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}

		input := usecase.Input{"rule_id": ruleID.String()}
		if body.Name != nil {
			input["name"] = *body.Name
		}
		if body.Scope != nil {
			input["scope"] = scopeInput(*body.Scope)
		}
		if body.RunAs != nil {
			input["run_as"] = body.RunAs.String()
		}
		if body.Trigger != nil {
			input["trigger"] = triggerInput(*body.Trigger)
		}
		if body.Conditions != nil {
			input["conditions"] = conditionsInput(*body.Conditions)
		}
		if body.Actions != nil {
			input["actions"] = actionsInput(*body.Actions)
		}
		if body.Throttle != nil {
			input["throttle"] = throttleInput(*body.Throttle)
		}
		if body.OnError != nil {
			input["on_error"] = string(*body.OnError)
		}
		if body.ExpectedVersion != nil {
			input["expected_version"] = *body.ExpectedVersion
		}
		return c.UseCases.Invoke(r.Context(), updateRuleUseCase, actor, input)
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, ruleResponse(out))
	})
}

// EnableAutomationRule answers POST /automation/rules/{ruleId}:enable.
func (c *RestController) EnableRule(
	w http.ResponseWriter, r *http.Request, ruleID openapi.RuleId,
	_ openapi.EnableRuleParams,
) {
	c.switchRule(w, r, ruleID, enableRuleUseCase)
}

// DisableAutomationRule answers POST /automation/rules/{ruleId}:disable.
func (c *RestController) DisableRule(
	w http.ResponseWriter, r *http.Request, ruleID openapi.RuleId,
	_ openapi.DisableRuleParams,
) {
	c.switchRule(w, r, ruleID, disableRuleUseCase)
}

// switchRule is what the two switches share: the same request and the same answer, and only the use
// case behind them differs - which is exactly the distinction the two routes exist to record.
func (c *RestController) switchRule(
	w http.ResponseWriter, r *http.Request, ruleID openapi.RuleId, useCase string,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), useCase, actor,
			usecase.Input{"rule_id": ruleID.String()})
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, ruleResponse(out))
	})
}

// DeleteAutomationRule answers DELETE /automation/rules/{ruleId}.
func (c *RestController) DeleteRule(
	w http.ResponseWriter, r *http.Request, ruleID openapi.RuleId,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), deleteRuleUseCase, actor,
			usecase.Input{"rule_id": ruleID.String()})
	}, func(_ usecase.Output) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// The readers. Each maps one document whole, so that what reaches the use case is the shape the
// aggregate validates rather than a flattening of it.

// TriggerRuleManually answers POST /automation/rules/{ruleId}:trigger.
//
// 202 rather than 200: the run happens on a worker, and what comes back is the identifier it will
// carry rather than the run itself. A caller that wants the outcome watches
// GET /automation/runs/{runId}, which answers 404 until a worker claims the job.
func (c *RestController) TriggerRuleManually(
	w http.ResponseWriter, r *http.Request, ruleID openapi.RuleId,
	_ openapi.TriggerRuleManuallyParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), triggerRuleUseCase, actor,
			usecase.Input{"rule_id": ruleID.String()})
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusAccepted, openapi.RuleRunAccepted{
			RunId: uuidValue(out.String("run_id")), RuleId: uuidValue(out.String("rule_id")),
		})
	})
}

func scopeInput(scope openapi.RuleScope) map[string]any {
	document := map[string]any{"type": string(scope.Type)}
	if scope.Id != nil {
		document["id"] = scope.Id.String()
	}
	return document
}

func triggerInput(trigger openapi.RuleTrigger) map[string]any {
	document := map[string]any{"kind": string(trigger.Kind)}
	for name, value := range map[string]*string{
		"event_type": trigger.EventType,
		"rrule":      trigger.Rrule,
		"timezone":   trigger.Timezone,
		"offset":     trigger.Offset,
	} {
		if value != nil {
			// Present when the client sent it, empty string and all: a field belonging to another
			// kind is refused inwards of here, and a reader that dropped an empty one would decide
			// that question in the wrong place.
			document[name] = *value
		}
	}
	if trigger.Anchor != nil {
		document["anchor"] = string(*trigger.Anchor)
	}
	if trigger.ChangedFields != nil {
		fields := make([]any, 0, len(*trigger.ChangedFields))
		for _, field := range *trigger.ChangedFields {
			fields = append(fields, field)
		}
		document["changed_fields"] = fields
	}
	return document
}

func conditionsInput(conditions []openapi.RuleCondition) []any {
	rows := make([]any, 0, len(conditions))
	for _, condition := range conditions {
		rows = append(rows, map[string]any{"expr": condition.Expr})
	}
	return rows
}

func actionsInput(actions []openapi.RuleAction) []any {
	rows := make([]any, 0, len(actions))
	for _, action := range actions {
		row := map[string]any{"kind": action.Kind}
		if action.Params != nil {
			row["params"] = map[string]any(*action.Params)
		}
		rows = append(rows, row)
	}
	return rows
}

func throttleInput(throttle openapi.RuleThrottle) map[string]any {
	document := map[string]any{}
	if throttle.MaxRunsPerHour != nil {
		document["max_runs_per_hour"] = *throttle.MaxRunsPerHour
	}
	if throttle.DedupeKeyExpr != nil {
		document["dedupe_key_expr"] = *throttle.DedupeKeyExpr
	}
	return document
}

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
	return rule
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

// The run log (G-07). What a rule did, why it did not, and what each action answered.

const (
	listRuleRunsUseCase = "ListRuleRuns"
	getRuleRunUseCase   = "GetRuleRun"
)

// ListRuleRuns answers GET /automation/runs.
func (c *RestController) ListRuleRuns(
	w http.ResponseWriter, r *http.Request, params openapi.ListRuleRunsParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		input := usecase.Input{}
		if params.RuleId != nil {
			input["rule_id"] = params.RuleId.String()
		}
		if params.Status != nil {
			input["status"] = string(*params.Status)
		}
		if params.Trigger != nil {
			input["trigger"] = string(*params.Trigger)
		}
		if params.Cursor != nil {
			input["cursor"] = *params.Cursor
		}
		if params.Size != nil {
			input["size"] = *params.Size
		}
		return c.UseCases.Invoke(r.Context(), listRuleRunsUseCase, actor, input)
	}, func(out usecase.Output) {
		rows, _ := out["data"].([]usecase.Output)
		runs := make([]openapi.RuleRun, 0, len(rows))
		for _, row := range rows {
			runs = append(runs, runResponse(row))
		}
		writeJSON(w, r, http.StatusOK, openapi.RuleRunPage{Data: runs, Page: pageResponse(out)})
	})
}

// GetRuleRun answers GET /automation/runs/{runId}.
func (c *RestController) GetRuleRun(
	w http.ResponseWriter, r *http.Request, runID openapi_types.UUID,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), getRuleRunUseCase, actor,
			usecase.Input{"run_id": runID.String()})
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, runResponse(out))
	})
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
