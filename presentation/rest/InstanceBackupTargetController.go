// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	createInstanceBackupTargetUseCase = "CreateInstanceBackupTarget"
)

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

var errPassphraseNotAvailable = shared.ErrValidation.
	WithDetail("backup.encryption_passphrase_not_available").
	WithFields(shared.FieldError{
		Path: "/encryption_passphrase", Code: "backup.encryption_passphrase_not_available",
	})
