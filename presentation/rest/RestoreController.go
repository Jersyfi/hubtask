// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
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
