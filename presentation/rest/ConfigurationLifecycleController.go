// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The lifecycle of operational configuration: listed, revised and removed through the same
// catalogue that creates it.
const (
	updateBackupScheduleUseCase  = "UpdateBackupSchedule"
	updateRetentionPolicyUseCase = "UpdateRetentionPolicy"
)

// UpdateBackupSchedule answers PATCH /backup-schedules/{scheduleId}.
func (c *RestController) UpdateBackupSchedule(
	w http.ResponseWriter, r *http.Request,
	scheduleID openapi_types.UUID, params openapi.UpdateBackupScheduleParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	var body openapi.BackupScheduleUpdate
	if err := decodeFrom(r.Body, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"schedule_id": scheduleID.String(),
		"rrule":       optionalStringField(body.Rrule),
		"timezone":    optionalStringField(body.Timezone),
		"full_rrule":  optionalStringField(body.FullRrule),
	}
	if body.Mode != nil {
		in["mode"] = string(*body.Mode)
	}
	for name, value := range map[string]*bool{
		"include_media": body.IncludeMedia, "include_audit": body.IncludeAudit,
		"trial_restore": body.TrialRestore, "enabled": body.Enabled,
	} {
		if value != nil {
			in[name] = *value
		}
	}
	if body.NotifyOn != nil {
		occasions := make([]any, 0, len(*body.NotifyOn))
		for _, occasion := range *body.NotifyOn {
			occasions = append(occasions, string(occasion))
		}
		in["notify_on"] = occasions
	}
	if body.Retention != nil {
		in["retention"] = retentionInput(*body.Retention)
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, ok := c.read(w, r, updateBackupScheduleUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, backupScheduleResponse(out))
}

// UpdateRetentionPolicy answers PATCH /retention-policies/{policyId}.
func (c *RestController) UpdateRetentionPolicy(
	w http.ResponseWriter, r *http.Request,
	policyID openapi_types.UUID, params openapi.UpdateRetentionPolicyParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	var body openapi.RetentionPolicyUpdate
	if err := decodeFrom(r.Body, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"policy_id":     policyID.String(),
		"condition":     optionalStringField(body.Condition),
		"justification": optionalStringField(body.Justification),
	}
	for name, value := range map[string]*int{
		"retain_days": body.RetainDays, "then_after_days": body.ThenAfterDays,
		"grace_days": body.GraceDays,
	} {
		if value != nil {
			in[name] = *value
		}
	}
	if body.Action != nil {
		in["action"] = string(*body.Action)
	}
	if body.ThenAction != nil {
		in["then_action"] = string(*body.ThenAction)
	}
	if body.Enabled != nil {
		in["enabled"] = *body.Enabled
	}
	if body.ExportTargetId != nil {
		in["export_target_id"] = body.ExportTargetId.String()
	}
	if body.Notify != nil {
		// The contract nests the warning and the catalogue takes it flat, because a use case
		// field is one value and MCP has no nested form for one.
		if body.Notify.BeforeDays != nil {
			in["notify_before_days"] = *body.Notify.BeforeDays
		}
		if body.Notify.Recipients != nil {
			named := make([]any, 0, len(*body.Notify.Recipients))
			for _, recipient := range *body.Notify.Recipients {
				named = append(named, string(recipient))
			}
			in["notify_recipients"] = named
		}
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, ok := c.read(w, r, updateRetentionPolicyUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, retentionPolicyResponse(out))
}
