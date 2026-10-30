// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

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
	if trial, present := out["trial_restore"].(map[string]any); present {
		run.TrialRestore = trialResponse(trial)
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
	trialRestore, _ := out["trial_restore"].(bool)
	schedule.TrialRestore = &trialRestore
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

// trialResponse maps what the run kept of its trial restore (B-4) onto the contract's shape.
func trialResponse(trial map[string]any) *openapi.BackupRunTrial {
	out := &openapi.BackupRunTrial{InspectedAt: timeValue(trial["inspected_at"])}
	if report, held := trial["report"].(map[string]any); held {
		out.Report = restoreReportResponse(report)
	}
	if failure, held := trial["failure"].(map[string]any); held {
		code, _ := failure["code"].(string)
		out.Failure = &openapi.BackupRunTrialFailure{Code: code}
		if member, _ := failure["member"].(string); member != "" {
			out.Failure.Member = &member
		}
	}
	return out
}
