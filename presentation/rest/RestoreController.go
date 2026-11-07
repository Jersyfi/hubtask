// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// backupArchiveResponse maps one archive at the target.
func backupArchiveResponse(out usecase.Output) openapi.BackupArchive {
	archive := openapi.BackupArchive{}

	for name, into := range map[string]**string{
		"archive_id": &archive.ArchiveId, "path": &archive.Path,
		"schema_version": &archive.SchemaVersion, "product_version": &archive.ProductVersion,
		"parent_archive_id": &archive.ParentArchiveId,
	} {
		if value := out.String(name); value != "" {
			text := value
			*into = &text
		}
	}
	if key := out.String("encryption_key_id"); key != "" {
		archive.EncryptionKeyId = &key
	}
	if at := timeValue(out["created_at"]); !at.IsZero() {
		moment := at
		archive.CreatedAt = &moment
	}
	if mode := out.String("mode"); mode != "" {
		named := openapi.BackupArchiveMode(mode)
		archive.Mode = &named
	}
	for name, into := range map[string]**int{
		"size_bytes": &archive.SizeBytes, "item_count": &archive.ItemCount,
		"media_count": &archive.MediaCount,
	} {
		if value, present := out[name].(int64); present {
			count := int(value)
			*into = &count
		}
	}
	if encrypted, present := out["encrypted"].(bool); present {
		archive.Encrypted = &encrypted
	}
	if complete, present := out["complete"].(bool); present {
		archive.Complete = &complete
	}
	// Nothing has been checked, and saying so is the honest answer. `:verify` is what turns this
	// into OK or MISMATCH, and it is a separate act because it reads every byte of the archive.
	unverified := openapi.BackupArchiveChecksumStatusUNVERIFIED
	archive.ChecksumStatus = &unverified

	if scope, present := out["scope"].(map[string]any); present {
		kind, _ := scope["kind"].(string)
		of := openapi.BackupArchiveScopeKind(kind)
		// The literal has to repeat the generated field's anonymous type exactly, spelling
		// included - the contract declares `scope` inline, so there is no name to refer to it by.
		archive.Scope = &struct {
			//nolint:revive // the field names are oapi-codegen's, and the type has to match
			Id    *openapi_types.UUID             `json:"id,omitempty"`
			Kind  *openapi.BackupArchiveScopeKind `json:"kind,omitempty"`
			Label *string                         `json:"label,omitempty"`
		}{Kind: &of}
		if id, named := scope["id"].(string); named && id != "" {
			scopeID := uuidValue(id)
			archive.Scope.Id = &scopeID
		}
		// No label. It would be the container's or the tenant's name, and the manifest carries no
		// user content on purpose: it is the one member of an archive that is never encrypted
		// (rule 10).
	}
	return archive
}

// acceptedRestore is the pointer a 202 hands back.
func acceptedRestore(out usecase.Output) openapi.JobRef {
	ref := openapi.JobRef{
		JobId:  uuidValue(out.String("job_id")),
		Status: openapi.JobStatusQUEUED,
	}
	if restore := out.String("restore_id"); restore != "" {
		url := "/restores/" + restore
		ref.ResultUrl = &url
	}
	return ref
}

// restoreRunResponse maps one restore.
func restoreRunResponse(out usecase.Output) openapi.RestoreRun {
	dryRun, _ := out["dry_run"].(bool)
	restore := openapi.RestoreRun{
		Id:            uuidValue(out.String("id")),
		TargetId:      uuidValue(out.String("target_id")),
		SourceArchive: out.String("source_archive"),
		Mode:          openapi.RestoreRunMode(out.String("mode")),
		Status:        openapi.RestoreRunStatus(out.String("status")),
		DryRun:        dryRun,
	}
	if rule := out.String("conflict_rule"); rule != "" {
		named := openapi.RestoreRunConflictRule(rule)
		restore.ConflictRule = &named
	}
	for name, into := range map[string]**openapi_types.UUID{
		"tenant_id": &restore.TenantId, "safety_backup_run_id": &restore.SafetyBackupRunId,
	} {
		if value := out.String(name); value != "" {
			id := uuidValue(value)
			*into = &id
		}
	}
	for name, into := range map[string]**time.Time{
		"started_at": &restore.StartedAt, "finished_at": &restore.FinishedAt,
	} {
		if at := timeValue(out[name]); !at.IsZero() {
			moment := at
			*into = &moment
		}
	}
	if code := out.String("error_code"); code != "" {
		restore.ErrorCode = &code
	}
	if report, present := out["report"].(map[string]any); present {
		restore.Report = restoreReportResponse(report)
	}
	return restore
}

// restoreReportResponse maps the report a dry run produced, or the one an execution left behind.
func restoreReportResponse(report map[string]any) *openapi.RestoreReport {
	out := &openapi.RestoreReport{}
	for name, into := range map[string]**int{
		"new": &out.New, "overwritten": &out.Overwritten, "skipped": &out.Skipped,
		"duplicated": &out.Duplicated, "conflicts": &out.Conflicts,
		"deleted": &out.Deleted, "media": &out.Media,
	} {
		if value, present := report[name].(int); present {
			count := value
			*into = &count
		}
	}
	for name, into := range map[string]**map[string]int{
		"withheld": &out.Withheld, "entities": &out.Entities,
	} {
		if counts, present := report[name].(map[string]int); present {
			values := counts
			*into = &values
		}
	}
	return out
}
