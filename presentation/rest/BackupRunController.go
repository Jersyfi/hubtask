// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	startBackupUseCase          = "StartBackup"
	getBackupRunUseCase         = "GetBackupRun"
	verifyBackupUseCase         = "VerifyBackup"
	createBackupScheduleUseCase = "CreateBackupSchedule"
)

// StartBackup answers POST /backups.
//
// 202 with a JobRef, because the run takes minutes: the archive is written by a worker, and what
// the caller gets now is something to poll. `result_url` names the run, whose identifier is also
// the archive's identifier in the manifest at the target.
func (c *RestController) StartBackup(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	var body openapi.BackupStart
	if err := decodeFrom(r.Body, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"target_id": body.TargetId.String()}
	if body.Mode != nil {
		in["mode"] = string(*body.Mode)
	}
	if body.IncludeMedia != nil {
		in["include_media"] = *body.IncludeMedia
	}
	if body.IncludeAudit != nil {
		in["include_audit"] = *body.IncludeAudit
	}

	out, ok := c.read(w, r, startBackupUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusAccepted, acceptedJob(out))
}

// VerifyBackup answers POST /backups/{backupId}:verify.
func (c *RestController) VerifyBackup(
	w http.ResponseWriter, r *http.Request, backupID openapi_types.UUID,
) {
	out, ok := c.read(w, r, verifyBackupUseCase, usecase.Input{"run_id": backupID.String()})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusAccepted, acceptedJob(out))
}

// GetBackupRun answers GET /backups/{backupId}.
func (c *RestController) GetBackupRun(
	w http.ResponseWriter, r *http.Request, backupID openapi_types.UUID,
) {
	out, ok := c.read(w, r, getBackupRunUseCase, usecase.Input{"run_id": backupID.String()})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, backupRunResponse(out))
}

// CreateBackupSchedule answers POST /backup-schedules.
func (c *RestController) CreateBackupSchedule(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	var body openapi.BackupSchedule
	if err := decodeFrom(r.Body, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"target_id": body.TargetId.String(), "rrule": body.Rrule}
	if body.Timezone != nil {
		in["timezone"] = *body.Timezone
	}
	if body.Scope.Kind != nil {
		in["scope"] = string(*body.Scope.Kind)
	}
	if body.Scope.Id != nil {
		in["scope_id"] = body.Scope.Id.String()
	}
	if body.Mode != nil {
		in["mode"] = string(*body.Mode)
	}
	if body.FullRrule != nil {
		in["full_rrule"] = *body.FullRrule
	}
	if body.IncludeMedia != nil {
		in["include_media"] = *body.IncludeMedia
	}
	if body.IncludeAudit != nil {
		in["include_audit"] = *body.IncludeAudit
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

	out, ok := c.read(w, r, createBackupScheduleUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusCreated, backupScheduleResponse(out))
}

// retentionInput carries only the numbers a caller actually named. The use case keeps its defaults
// for the rest, so "min_keep: 5" means "the usual plan with a higher floor" rather than "keep
// nothing but five".
func retentionInput(plan openapi.BackupRetention) map[string]any {
	out := map[string]any{}
	for name, value := range map[string]*int{
		"keep_last": plan.KeepLast, "keep_daily": plan.KeepDaily,
		"keep_weekly": plan.KeepWeekly, "keep_monthly": plan.KeepMonthly,
		"keep_yearly": plan.KeepYearly, "min_keep": plan.MinKeep,
	} {
		if value != nil {
			out[name] = float64(*value)
		}
	}
	return out
}

// acceptedJob is the pointer a 202 hands back.
func acceptedJob(out usecase.Output) openapi.JobRef {
	ref := openapi.JobRef{
		JobId:  uuidValue(out.String("job_id")),
		Status: openapi.JobStatusQUEUED,
	}
	if run := out.String("run_id"); run != "" {
		url := "/backups/" + run
		ref.ResultUrl = &url
	}
	return ref
}

// backupRunResponse maps one run.
func backupRunResponse(out usecase.Output) openapi.BackupRun {
	run := openapi.BackupRun{
		Id:        uuidValue(out.String("id")),
		TargetId:  uuidValue(out.String("target_id")),
		Trigger:   openapi.BackupRunTrigger(out.String("trigger")),
		Mode:      openapi.BackupRunMode(out.String("mode")),
		Status:    openapi.BackupRunStatus(out.String("status")),
		StartedAt: timeValue(out["started_at"]),
	}
	for name, into := range map[string]**openapi_types.UUID{
		"schedule_id": &run.ScheduleId, "parent_run_id": &run.ParentRunId,
	} {
		if value := out.String(name); value != "" {
			id := uuidValue(value)
			*into = &id
		}
	}
	if path := out.String("archive_path"); path != "" {
		run.ArchivePath = &path
	}
	if code := out.String("error_code"); code != "" {
		run.ErrorCode = &code
	}
	for name, into := range map[string]**int{
		"size_bytes": &run.SizeBytes, "item_count": &run.ItemCount, "media_count": &run.MediaCount,
	} {
		if value, present := out[name].(int64); present {
			count := int(value)
			*into = &count
		}
	}
	for name, into := range map[string]**time.Time{
		"snapshot_at": &run.SnapshotAt, "finished_at": &run.FinishedAt,
		"expires_at": &run.ExpiresAt, "verified_at": &run.VerifiedAt,
	} {
		if at := timeValue(out[name]); !at.IsZero() {
			moment := at
			*into = &moment
		}
	}
	if ok, present := out["verify_ok"].(bool); present {
		run.VerifyOk = &ok
	}
	return run
}

// backupScheduleResponse maps one schedule.
func backupScheduleResponse(out usecase.Output) openapi.BackupSchedule {
	schedule := openapi.BackupSchedule{
		TargetId: uuidValue(out.String("target_id")),
		Rrule:    out.String("rrule"),
	}
	id := uuidValue(out.String("id"))
	schedule.Id = &id
	zone := out.String("timezone")
	schedule.Timezone = &zone
	mode := openapi.BackupScheduleMode(out.String("mode"))
	schedule.Mode = &mode
	includeMedia, _ := out["include_media"].(bool)
	schedule.IncludeMedia = &includeMedia
	includeAudit, _ := out["include_audit"].(bool)
	schedule.IncludeAudit = &includeAudit
	enabled, _ := out["enabled"].(bool)
	schedule.Enabled = &enabled

	if scope, present := out["scope"].(map[string]any); present {
		kind, _ := scope["kind"].(string)
		of := openapi.BackupScheduleScopeKind(kind)
		schedule.Scope.Kind = &of
		if value, named := scope["id"].(string); named && value != "" {
			scopeID := uuidValue(value)
			schedule.Scope.Id = &scopeID
		}
	}
	if rule := out.String("full_rrule"); rule != "" {
		schedule.FullRrule = &rule
	}
	if plan, present := out["retention"].(map[string]any); present {
		schedule.Retention = retentionResponse(plan)
	}
	if occasions, present := out["notify_on"].([]string); present {
		named := make([]openapi.BackupScheduleNotifyOn, 0, len(occasions))
		for _, occasion := range occasions {
			named = append(named, openapi.BackupScheduleNotifyOn(occasion))
		}
		schedule.NotifyOn = &named
	}
	if at := timeValue(out["next_run_at"]); !at.IsZero() {
		schedule.NextRunAt = &at
	}
	return schedule
}

func retentionResponse(plan map[string]any) *openapi.BackupRetention {
	out := &openapi.BackupRetention{}
	for name, into := range map[string]**int{
		"keep_last": &out.KeepLast, "keep_daily": &out.KeepDaily,
		"keep_weekly": &out.KeepWeekly, "keep_monthly": &out.KeepMonthly,
		"keep_yearly": &out.KeepYearly, "min_keep": &out.MinKeep,
	} {
		if value, present := plan[name].(int); present {
			count := value
			*into = &count
		}
	}
	return out
}
