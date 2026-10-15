// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const reapplyErasuresUseCase = "ReapplyErasures"

// ReapplyErasures answers POST /admin/tenants/{tenantId}:reapply-erasures. The cases travel to the
// use case as JSON objects of strings, with their instants written out again: the use case reads
// the same shape over every channel.
func (c *RestController) ReapplyErasures(
	w http.ResponseWriter, r *http.Request, tenantID openapi.AdminTenantId, params openapi.ReapplyErasuresParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.ErasureReapplyRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	erasures := make([]any, 0, len(body.Erasures))
	for _, record := range body.Erasures {
		erasures = append(erasures, erasureRecordInput(record))
	}
	input := usecase.Input{
		"tenant_id":      tenantID.String(),
		"recovery_point": "",
		"erasures":       erasures,
		"step_up_token":  stepUpHeaderField(params.XHubtaskStepUp),
	}
	if !body.RecoveryPoint.IsZero() {
		input["recovery_point"] = body.RecoveryPoint.UTC().Format(time.RFC3339Nano)
	}

	out, err := c.UseCases.Invoke(r.Context(), reapplyErasuresUseCase, actorOf(r), input)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, out)
}

// erasureRecordInput is one case as the use case reads it; absent fields stay absent.
func erasureRecordInput(record openapi.ErasureCaseRecord) map[string]any {
	instant := func(at time.Time) string { return at.UTC().Format(time.RFC3339Nano) }
	fields := map[string]any{
		"id": record.Id.String(), "erasure_mode": string(record.ErasureMode),
		"received_at": instant(record.ReceivedAt), "due_at": instant(record.DueAt),
		"completed_at": instant(record.CompletedAt),
	}
	if record.SubjectAccountId != nil {
		fields["subject_account_id"] = record.SubjectAccountId.String()
	}
	if record.SubjectEmail != nil {
		fields["subject_email"] = *record.SubjectEmail
	}
	if record.HandledBy != nil {
		fields["handled_by"] = record.HandledBy.String()
	}
	if record.Notes != nil {
		fields["notes"] = *record.Notes
	}
	if record.OriginalDueAt != nil {
		fields["original_due_at"] = instant(*record.OriginalDueAt)
	}
	if record.ExtensionReason != nil {
		fields["extension_reason"] = string(*record.ExtensionReason)
	}
	if record.InformedOn != nil {
		fields["informed_on"] = record.InformedOn.String()
	}
	return fields
}
