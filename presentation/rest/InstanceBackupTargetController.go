// SPDX-License-Identifier: Apache-2.0
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
	listInstanceBackupTargetsUseCase  = "ListInstanceBackupTargets"
	createInstanceBackupTargetUseCase = "CreateInstanceBackupTarget"
	testInstanceBackupTargetUseCase   = "TestInstanceBackupTarget"
	deleteInstanceBackupTargetUseCase = "DeleteInstanceBackupTarget"
)

// ListInstanceBackupTargets answers GET /admin/backup-targets.
func (c *RestController) ListInstanceBackupTargets(w http.ResponseWriter, r *http.Request) {
	out, ok := c.read(w, r, listInstanceBackupTargetsUseCase, usecase.Input{})
	if !ok {
		return
	}
	targets := []openapi.BackupTarget{}
	for _, row := range rowsOf(out) {
		targets = append(targets, backupTargetResponse(row))
	}
	writeJSON(w, r, http.StatusOK, targets)
}

// CreateInstanceBackupTarget answers POST /admin/backup-targets: the workspace's body, the
// installation's scope, and the step-up header.
func (c *RestController) CreateInstanceBackupTarget(
	w http.ResponseWriter, r *http.Request, params openapi.CreateInstanceBackupTargetParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	var body openapi.BackupTargetCreate
	if err := decodeFrom(r.Body, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in, err := backupTargetInput(body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in["step_up_token"] = stepUpHeaderField(params.XHubtaskStepUp)
	out, ok := c.read(w, r, createInstanceBackupTargetUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusCreated, backupTargetResponse(out))
}

// TestInstanceBackupTarget answers POST /admin/backup-targets/{targetId}:test.
func (c *RestController) TestInstanceBackupTarget(
	w http.ResponseWriter, r *http.Request, targetID openapi_types.UUID,
) {
	out, ok := c.read(w, r, testInstanceBackupTargetUseCase, usecase.Input{"target_id": targetID.String()})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, backupProbeResponse(out))
}

// DeleteInstanceBackupTarget answers DELETE /admin/backup-targets/{targetId}.
func (c *RestController) DeleteInstanceBackupTarget(
	w http.ResponseWriter, r *http.Request, targetID openapi_types.UUID,
	params openapi.DeleteInstanceBackupTargetParams,
) {
	if _, ok := c.read(w, r, deleteInstanceBackupTargetUseCase, usecase.Input{
		"target_id":     targetID.String(),
		"step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
	}); !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var errPassphraseNotAvailable = shared.ErrValidation.
	WithDetail("backup.encryption_passphrase_not_available").
	WithFields(shared.FieldError{
		Path: "/encryption_passphrase", Code: "backup.encryption_passphrase_not_available",
	})
