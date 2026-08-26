// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	createBackupTargetUseCase = "CreateBackupTarget"
	listBackupTargetsUseCase  = "ListBackupTargets"
	testBackupTargetUseCase   = "TestBackupTarget"
)

// ListBackupTargets answers GET /backup-targets.
//
// An array rather than a page: a tenant has a handful of targets, and the 3-2-1 rule the document
// recommends tops out at three. A cursor over three rows would be ceremony.
func (c *RestController) ListBackupTargets(w http.ResponseWriter, r *http.Request) {
	out, ok := c.read(w, r, listBackupTargetsUseCase, usecase.Input{})
	if !ok {
		return
	}

	targets := []openapi.BackupTarget{}
	for _, row := range rowsOf(out) {
		targets = append(targets, backupTargetResponse(row))
	}
	writeJSON(w, r, http.StatusOK, targets)
}

// CreateBackupTarget answers POST /backup-targets.
func (c *RestController) CreateBackupTarget(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	var body openapi.BackupTargetCreate
	if err := decodeFrom(r.Body, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	// The one field of the contract this installation does not serve yet, and it is refused
	// rather than ignored. A passphrase that had no effect would leave somebody believing their
	// archives are protected by it; the key it derives is the archive writer's business, and the
	// archive writer arrives with the backup run.
	if body.EncryptionPassphrase != nil && *body.EncryptionPassphrase != "" {
		WriteProblem(w, shared.ErrValidation.
			WithDetail("backup.encryption_passphrase_not_available").
			WithFields(shared.FieldError{
				Path: "/encryption_passphrase", Code: "backup.encryption_passphrase_not_available",
			}), requestID)
		return
	}

	in := usecase.Input{
		"name":   body.Name,
		"kind":   string(body.Kind),
		"config": body.Config,
	}
	if body.Credentials != nil {
		in["credentials"] = *body.Credentials
	}
	if body.EncryptionMode != nil {
		in["encryption_mode"] = string(*body.EncryptionMode)
	}
	if body.InsecureAcknowledged != nil {
		in["insecure_acknowledged"] = *body.InsecureAcknowledged
	}

	out, ok := c.read(w, r, createBackupTargetUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusCreated, backupTargetResponse(out))
}

// TestBackupTarget answers POST /backup-targets/{targetId}:test.
//
// 200 with a result even when the target could not be reached: "it does not work and here is the
// code" is what the caller asked to find out, and a 502 would say this server failed instead.
func (c *RestController) TestBackupTarget(
	w http.ResponseWriter, r *http.Request, targetID openapi_types.UUID,
) {
	out, ok := c.read(w, r, testBackupTargetUseCase, usecase.Input{
		"target_id": targetID.String(),
	})
	if !ok {
		return
	}

	writable, _ := out["writable"].(bool)
	succeeded, _ := out["ok"].(bool)
	probe := openapi.BackupTargetProbe{
		Ok:        succeeded,
		LatencyMs: float32(out.Int("latency_ms")),
		Writable:  writable,
	}
	if free, present := out["free_bytes"].(int64); present {
		bytes := int(free)
		probe.FreeBytes = &bytes
	}
	if code := out.String("error_code"); code != "" {
		probe.ErrorCode = &code
	}
	writeJSON(w, r, http.StatusOK, probe)
}

// backupTargetResponse maps one target. There is no credential in the catalogue's answer and
// nowhere here to put one - which is the requirement rather than an omission, and is asserted in
// this package's test as well as in the repository's.
func backupTargetResponse(out usecase.Output) openapi.BackupTarget {
	target := openapi.BackupTarget{
		Id:             uuidValue(out.String("id")),
		Name:           out.String("name"),
		Kind:           openapi.BackupTargetKind(out.String("kind")),
		EncryptionMode: openapi.BackupTargetEncryptionMode(out.String("encryption_mode")),
	}

	scope := openapi.BackupTargetScope(out.String("scope"))
	target.Scope = &scope
	enabled, _ := out["enabled"].(bool)
	target.Enabled = &enabled

	if config, present := out["config"].(map[string]any); present {
		target.Config = &config
	}
	warnings := []string{}
	if listed, present := out["warnings"].([]string); present {
		warnings = listed
	}
	target.Warnings = &warnings

	if key := out.String("encryption_key_id"); key != "" {
		target.EncryptionKeyId = &key
	}
	if note := out.String("region_note"); note != "" {
		target.RegionNote = &note
	}
	if at := timeValue(out["last_test_at"]); !at.IsZero() {
		target.LastTestAt = &at
	}
	if ok, present := out["last_test_ok"].(bool); present {
		target.LastTestOk = &ok
	}
	return target
}
