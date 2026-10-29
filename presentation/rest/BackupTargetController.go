// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	createBackupTargetUseCase = "CreateBackupTarget"
)

// CreateBackupTarget answers POST /backup-targets.
func (c *RestController) CreateBackupTarget(w http.ResponseWriter, r *http.Request) {
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

	out, ok := c.read(w, r, createBackupTargetUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusCreated, backupTargetResponse(out))
}

// backupTargetInput is a target's body as the catalogue reads it - for a workspace's target and
// the installation's alike.
func backupTargetInput(body openapi.BackupTargetCreate) (usecase.Input, error) {
	// The one field of the contract this installation does not serve yet, and it is refused
	// rather than ignored. A passphrase that had no effect would leave somebody believing their
	// archives are protected by it; the key it derives is the archive writer's business, and the
	// archive writer arrives with the backup run.
	if body.EncryptionPassphrase != nil && *body.EncryptionPassphrase != "" {
		return nil, errPassphraseNotAvailable
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
	return in, nil
}

// backupProbeResponse maps a probe.
func backupProbeResponse(out usecase.Output) openapi.BackupTargetProbe {
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
	return probe
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
