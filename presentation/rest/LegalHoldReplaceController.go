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

const replaceLegalHoldsUseCase = "ReplaceLegalHolds"

// ReplaceLegalHolds answers POST /admin/tenants/{tenantId}:replace-legal-holds. The records travel
// to the use case as the JSON objects they arrived as, with their instants written out again: the
// use case reads the same shape over every channel.
func (c *RestController) ReplaceLegalHolds(
	w http.ResponseWriter, r *http.Request, tenantID openapi.AdminTenantId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.LegalHoldReplaceRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	input := usecase.Input{"tenant_id": tenantID.String()}
	if body.Archive != nil {
		input["archive"] = map[string]any{
			"target_id": body.Archive.TargetId.String(), "path": body.Archive.Path,
		}
	}
	var records []openapi.LegalHoldRecord
	if body.Holds != nil {
		records = *body.Holds
	}
	holds := make([]any, 0, len(records))
	for _, hold := range records {
		scope := map[string]any{"kind": string(hold.Scope.Kind)}
		if hold.Scope.Id != nil {
			scope["id"] = hold.Scope.Id.String()
		}
		record := map[string]any{
			"id": hold.Id.String(), "scope": scope, "reason": hold.Reason,
			"placed_by": hold.PlacedBy.String(),
			"placed_at": hold.PlacedAt.UTC().Format(time.RFC3339Nano),
		}
		if hold.ReleasedAt != nil {
			record["released_at"] = hold.ReleasedAt.UTC().Format(time.RFC3339Nano)
		}
		holds = append(holds, record)
	}
	if body.Holds != nil {
		input["holds"] = holds
	}
	input["recovery_point"] = ""
	if !body.RecoveryPoint.IsZero() {
		input["recovery_point"] = body.RecoveryPoint.UTC().Format(time.RFC3339Nano)
	}

	out, err := c.UseCases.Invoke(r.Context(), replaceLegalHoldsUseCase, actorOf(r), input)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, out)
}
