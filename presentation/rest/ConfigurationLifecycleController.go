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

// The lifecycle F4-02 completed: three pieces of operational configuration that could be created
// and never revised, and one that could not even be listed.
const (
	listBackupSchedulesUseCase   = "ListBackupSchedules"
	updateBackupScheduleUseCase  = "UpdateBackupSchedule"
	deleteBackupScheduleUseCase  = "DeleteBackupSchedule"
	deleteBackupTargetUseCase    = "DeleteBackupTarget"
	updateRetentionPolicyUseCase = "UpdateRetentionPolicy"
	deleteRetentionPolicyUseCase = "DeleteRetentionPolicy"
)

// ListBackupSchedules answers GET /backup-schedules.
//
// An array rather than a page, for `ListBackupTargets`' reason: a workspace has a handful of
// schedules, and a cursor over three rows would be ceremony.
func (c *RestController) ListBackupSchedules(w http.ResponseWriter, r *http.Request) {
	out, ok := c.read(w, r, listBackupSchedulesUseCase, usecase.Input{})
	if !ok {
		return
	}

	schedules := []openapi.BackupSchedule{}
	for _, row := range rowsOf(out) {
		schedules = append(schedules, backupScheduleResponse(row))
	}
	writeJSON(w, r, http.StatusOK, schedules)
}

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
		"enabled": body.Enabled,
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

// DeleteBackupSchedule answers DELETE /backup-schedules/{scheduleId}.
func (c *RestController) DeleteBackupSchedule(
	w http.ResponseWriter, r *http.Request, scheduleID openapi_types.UUID,
) {
	if _, ok := c.read(w, r, deleteBackupScheduleUseCase, usecase.Input{
		"schedule_id": scheduleID.String(),
	}); !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteBackupTarget answers DELETE /backup-targets/{targetId}.
func (c *RestController) DeleteBackupTarget(
	w http.ResponseWriter, r *http.Request, targetID openapi_types.UUID,
) {
	if _, ok := c.read(w, r, deleteBackupTargetUseCase, usecase.Input{
		"target_id": targetID.String(),
	}); !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// DeleteRetentionPolicy answers DELETE /retention-policies/{policyId}.
func (c *RestController) DeleteRetentionPolicy(
	w http.ResponseWriter, r *http.Request, policyID openapi_types.UUID,
) {
	if _, ok := c.read(w, r, deleteRetentionPolicyUseCase, usecase.Input{
		"policy_id": policyID.String(),
	}); !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
