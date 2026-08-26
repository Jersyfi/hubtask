// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	createRetentionPolicyUseCase  = "CreateRetentionPolicy"
	listRetentionPoliciesUseCase  = "ListRetentionPolicies"
	previewRetentionPolicyUseCase = "PreviewRetentionPolicy"
	retainItemUseCase             = "RetainItem"
)

// ListRetentionPolicies answers GET /retention-policies.
//
// An array rather than a page: a workspace has a rule per kind per level, and the catalogue has
// fifteen kinds. A cursor over that would be ceremony.
func (c *RestController) ListRetentionPolicies(
	w http.ResponseWriter, r *http.Request, params openapi.ListRetentionPoliciesParams,
) {
	in := usecase.Input{}
	if params.ContainerId != nil {
		in["container_id"] = params.ContainerId.String()
	}
	if params.Effective != nil {
		in["effective"] = *params.Effective
	}

	out, ok := c.read(w, r, listRetentionPoliciesUseCase, in)
	if !ok {
		return
	}

	policies := []openapi.RetentionPolicy{}
	for _, row := range rowsOf(out) {
		policies = append(policies, retentionPolicyResponse(row))
	}
	writeJSON(w, r, http.StatusOK, policies)
}

// CreateRetentionPolicy answers POST /retention-policies.
func (c *RestController) CreateRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	var body openapi.RetentionPolicy
	if err := decodeFrom(r.Body, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{
		"data_kind":   body.DataKind,
		"retain_days": body.RetainDays,
		"action":      string(body.Action),
	}
	if body.Scope.Kind != nil {
		in["scope"] = string(*body.Scope.Kind)
	}
	if body.Scope.Id != nil {
		in["scope_id"] = body.Scope.Id.String()
	}
	if body.ThenAfterDays != nil {
		in["then_after_days"] = *body.ThenAfterDays
	}
	if body.ThenAction != nil {
		in["then_action"] = string(*body.ThenAction)
	}
	if body.GraceDays != nil {
		in["grace_days"] = *body.GraceDays
	}
	if body.Notify != nil {
		if body.Notify.BeforeDays != nil {
			in["notify_before_days"] = *body.Notify.BeforeDays
		}
		if body.Notify.Recipients != nil {
			recipients := make([]any, 0, len(*body.Notify.Recipients))
			for _, recipient := range *body.Notify.Recipients {
				recipients = append(recipients, string(recipient))
			}
			in["notify_recipients"] = recipients
		}
	}
	for name, value := range map[string]*string{
		"justification": body.Justification, "condition": body.Condition,
	} {
		if value != nil && *value != "" {
			in[name] = *value
		}
	}
	if body.Enabled != nil {
		in["enabled"] = *body.Enabled
	}
	if body.ExportTargetId != nil {
		in["export_target_id"] = body.ExportTargetId.String()
	}

	out, ok := c.read(w, r, createRetentionPolicyUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusCreated, retentionPolicyResponse(out))
}

// PreviewRetentionPolicy answers POST /retention-policies/{policyId}:preview.
func (c *RestController) PreviewRetentionPolicy(
	w http.ResponseWriter, r *http.Request, policyID openapi_types.UUID,
) {
	out, ok := c.read(w, r, previewRetentionPolicyUseCase,
		usecase.Input{"policy_id": policyID.String()})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, retentionPreviewResponse(out))
}

// retentionPolicyResponse maps one rule.
func retentionPolicyResponse(out usecase.Output) openapi.RetentionPolicy {
	policy := openapi.RetentionPolicy{}

	id := uuidValue(out.String("id"))
	policy.Id = &id
	policy.DataKind = out.String("data_kind")
	policy.Action = openapi.RetentionPolicyAction(out.String("action"))
	if days, present := out["retain_days"].(int); present {
		policy.RetainDays = days
	}
	for name, into := range map[string]**int{
		"grace_days": &policy.GraceDays, "then_after_days": &policy.ThenAfterDays,
	} {
		if value, present := out[name].(int); present {
			count := value
			*into = &count
		}
	}
	if enabled, present := out["enabled"].(bool); present {
		policy.Enabled = &enabled
	}
	if then := out.String("then_action"); then != "" {
		named := openapi.RetentionPolicyThenAction(then)
		policy.ThenAction = &named
	}
	for name, into := range map[string]**string{
		"justification": &policy.Justification, "condition": &policy.Condition,
	} {
		if value := out.String(name); value != "" {
			text := value
			*into = &text
		}
	}
	if target := out.String("export_target_id"); target != "" {
		id := uuidValue(target)
		policy.ExportTargetId = &id
	}

	if scope, present := out["scope"].(map[string]any); present {
		kind, _ := scope["kind"].(string)
		of := openapi.RetentionPolicyScopeKind(kind)
		policy.Scope.Kind = &of
		if value, named := scope["id"].(string); named && value != "" {
			scopeID := uuidValue(value)
			policy.Scope.Id = &scopeID
		}
	}
	if notify, present := out["notify"].(map[string]any); present {
		policy.Notify = retentionNotifyResponse(notify)
	}
	return policy
}

// retentionNotifyResponse maps the advance warning.
func retentionNotifyResponse(notify map[string]any) *struct {
	BeforeDays *int                                       `json:"before_days,omitempty"`
	Recipients *[]openapi.RetentionPolicyNotifyRecipients `json:"recipients,omitempty"`
} {
	out := &struct {
		BeforeDays *int                                       `json:"before_days,omitempty"`
		Recipients *[]openapi.RetentionPolicyNotifyRecipients `json:"recipients,omitempty"`
	}{}
	if before, present := notify["before_days"].(int); present {
		days := before
		out.BeforeDays = &days
	}
	if named, present := notify["recipients"].([]string); present {
		recipients := make([]openapi.RetentionPolicyNotifyRecipients, 0, len(named))
		for _, recipient := range named {
			recipients = append(recipients, openapi.RetentionPolicyNotifyRecipients(recipient))
		}
		out.Recipients = &recipients
	}
	return out
}

// retentionPreviewResponse maps what a rule would do.
func retentionPreviewResponse(out usecase.Output) map[string]any {
	preview, present := out["preview"].(map[string]any)
	if !present {
		preview = map[string]any(out)
	}
	return preview
}

// RetainItem answers POST /items/{itemId}:retain.
func (c *RestController) RetainItem(
	w http.ResponseWriter, r *http.Request, itemID openapi_types.UUID,
) {
	out, ok := c.read(w, r, retainItemUseCase, usecase.Input{"item_id": itemID.String()})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, workItemResponse(out))
}
