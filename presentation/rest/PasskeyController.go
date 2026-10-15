// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The passkeys of the caller's own account (identity.md §23).

const (
	listOwnPasskeysUseCase          = "ListOwnPasskeys"
	startPasskeyRegistrationUseCase = "StartPasskeyRegistration"
)

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

// StartPasskeyRegistration answers POST /accounts/me/passkeys:start.
func (c *RestController) StartPasskeyRegistration(
	w http.ResponseWriter, r *http.Request, params openapi.StartPasskeyRegistrationParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), startPasskeyRegistrationUseCase, actorOf(r), usecase.Input{
		"step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	// The options are WebAuthn's own JSON shape, nested deeply; going through the generated type holds
	// the use case's answer to the contract field for field rather than trusting a map.
	var answer openapi.PasskeyRegistrationOptions
	encoded, err := json.Marshal(out)
	if err == nil {
		err = json.Unmarshal(encoded, &answer)
	}
	if err != nil {
		WriteProblem(w, shared.ErrInternal.WithDetail("rest.answer_unshapeable").WithCause(err), requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, answer)
}
