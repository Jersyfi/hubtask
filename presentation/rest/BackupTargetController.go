// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

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
