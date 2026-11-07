// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

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
	if failure, present := out["last_failure"].(map[string]any); present {
		code, _ := failure["code"].(string)
		raw, _ := failure["at"].(string)
		at, _ := time.Parse(time.RFC3339Nano, raw)
		policy.LastFailure = &struct {
			At   time.Time `json:"at"`
			Code string    `json:"code"`
		}{At: at, Code: code}
	}
	// Present only when the listing was asked about a container: with nothing to be in force in,
	// the question has no answer, and false would be one.
	if inForce, asked := out["in_force"].(bool); asked {
		policy.InForce = &inForce
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

// legalHoldResponse maps one hold.
func legalHoldResponse(out usecase.Output) openapi.LegalHold {
	id := uuidValue(out.String("id"))
	placedBy := uuidValue(out.String("placed_by"))
	placedAt := timeValue(out["placed_at"])
	hold := openapi.LegalHold{
		Id: &id, Reason: out.String("reason"), PlacedBy: &placedBy, PlacedAt: &placedAt,
	}
	if scope, present := out["scope"].(map[string]any); present {
		kind, _ := scope["kind"].(string)
		hold.Scope.Kind = openapi.LegalHoldScopeKind(kind)
		if value, named := scope["id"].(string); named && value != "" {
			scopeID := uuidValue(value)
			hold.Scope.Id = &scopeID
		}
	}
	// The three lifting fields travel together or not at all: a hold with a date and no reason
	// would be exactly the entry migration 0040 exists to stop.
	if released := out.String("released_by"); released != "" {
		by := uuidValue(released)
		hold.ReleasedBy = &by
		at := timeValue(out["released_at"])
		hold.ReleasedAt = &at
		reason := out.String("released_reason")
		hold.ReleasedReason = &reason
	}
	return hold
}
