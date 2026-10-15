// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The passkeys of the caller's own account (identity.md §23).

const listOwnPasskeysUseCase = "ListOwnPasskeys"

// ListOwnPasskeys answers GET /accounts/me/passkeys. Written out, GetOwnAccount's reasoning.
func (c *RestController) ListOwnPasskeys(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), listOwnPasskeysUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	rows, _ := out["data"].([]usecase.Output)
	answer := openapi.PasskeyList{
		Passkeys: make([]openapi.Passkey, 0, len(rows)),
		CanAdd:   out["can_add"] == true,
	}
	answer.Limit, _ = out["limit"].(int)
	if rpID := out.String("rp_id"); rpID != "" {
		answer.RpId = &rpID
	}
	if reason := out.String("cannot_add_reason"); reason != "" {
		code := openapi.PasskeyListCannotAddReason(reason)
		answer.CannotAddReason = &code
	}
	for _, row := range rows {
		answer.Passkeys = append(answer.Passkeys, passkeyResponse(row))
	}
	writeJSON(w, r, http.StatusOK, answer)
}

func passkeyResponse(out usecase.Output) openapi.Passkey {
	id, _ := uuid.Parse(out.String("id"))
	return openapi.Passkey{
		Id: id, Name: out.String("name"),
		CreatedAt: timeValue(out["created_at"]), LastUsedAt: timePointer(out["last_used_at"]),
	}
}
