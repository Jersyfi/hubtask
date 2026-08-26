// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	listBackupsAtTargetUseCase = "ListBackupsAtTarget"
)

// ListBackupsAtTarget answers GET /backup-targets/{targetId}/backups.
//
// An array rather than a page, for the reason the target list is one: a generation plan tops out at
// a few dozen archives, and a cursor over those would be ceremony. What the answer is made of comes
// from the manifests at the target - so this route is the one that still works on the day the
// database is a fresh empty one.
func (c *RestController) ListBackupsAtTarget(
	w http.ResponseWriter, r *http.Request,
	targetID openapi_types.UUID, params openapi.ListBackupsAtTargetParams,
) {
	in := usecase.Input{"target_id": targetID.String()}
	if params.TenantId != nil {
		in["tenant_id"] = params.TenantId.String()
	}
	if params.Refresh != nil {
		in["refresh"] = *params.Refresh
	}

	out, ok := c.read(w, r, listBackupsAtTargetUseCase, in)
	if !ok {
		return
	}

	archives := []openapi.BackupArchive{}
	for _, row := range rowsOf(out) {
		archives = append(archives, backupArchiveResponse(row))
	}
	writeJSON(w, r, http.StatusOK, archives)
}

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
	unverified := openapi.UNVERIFIED
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

const (
	startRestoreUseCase  = "StartRestore"
	getRestoreRunUseCase = "GetRestoreRun"
)

// StartRestore answers POST /restores.
//
// 202 with a JobRef, because a restore takes minutes: the rows are written by a worker, and what
// the caller gets now is something to poll. `result_url` names the restore run, which carries the
// report the dry run produced.
func (c *RestController) StartRestore(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	var body openapi.RestoreRequest
	if err := decodeFrom(r.Body, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"target_id":  body.TargetId.String(),
		"archive_id": body.ArchiveId,
		"mode":       string(body.Mode),
	}
	if body.TargetTenantId != nil {
		in["target_tenant_id"] = body.TargetTenantId.String()
	}
	if body.ConflictRule != nil {
		in["conflict_rule"] = string(*body.ConflictRule)
	}
	if body.DryRun != nil {
		in["dry_run"] = *body.DryRun
	}
	if body.CreateSafetyBackup != nil {
		in["create_safety_backup"] = *body.CreateSafetyBackup
	}
	for name, value := range map[string]*string{
		"confirmation": body.Confirmation, "step_up_token": body.StepUpToken,
	} {
		if value != nil {
			in[name] = *value
		}
	}
	if body.Selection != nil {
		if body.Selection.ContainerIds != nil {
			in["selection_container_ids"] = identifierList(*body.Selection.ContainerIds)
		}
		if body.Selection.ItemIds != nil {
			in["selection_item_ids"] = identifierList(*body.Selection.ItemIds)
		}
	}

	// The one field of the contract this installation does not serve, and it is refused rather
	// than ignored. A passphrase that had no effect would leave somebody believing the archive
	// they are restoring was protected by one - and the key an archive is written under is derived
	// from the installation's master key rather than from anything a caller sends (E-02).
	if body.DecryptionPassphrase != nil && *body.DecryptionPassphrase != "" {
		WriteProblem(w, shared.ErrValidation.
			WithDetail("backup.encryption_passphrase_not_available").
			WithFields(shared.FieldError{
				Path: "/decryption_passphrase",
				Code: "backup.encryption_passphrase_not_available",
			}), requestID)
		return
	}

	out, ok := c.read(w, r, startRestoreUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusAccepted, acceptedRestore(out))
}

// GetRestoreRun answers GET /restores/{restoreId}.
func (c *RestController) GetRestoreRun(
	w http.ResponseWriter, r *http.Request, restoreID openapi_types.UUID,
) {
	out, ok := c.read(w, r, getRestoreRunUseCase, usecase.Input{"restore_id": restoreID.String()})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, restoreRunResponse(out))
}

// identifierList turns a body's identifiers into what the catalogue takes.
func identifierList(ids []openapi_types.UUID) []any {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
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
