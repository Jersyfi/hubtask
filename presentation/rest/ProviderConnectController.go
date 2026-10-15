// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/google/uuid"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Connecting a provider from a signed-in session (ADR-0078 §1): the two halves, as the provider
// step-up's are, on the caller's own account.

const (
	startProviderConnectUseCase    = "StartProviderConnect"
	completeProviderConnectUseCase = "CompleteProviderConnect"
)

// StartProviderConnect answers POST /accounts/me/identities:connect.
func (c *RestController) StartProviderConnect(
	w http.ResponseWriter, r *http.Request, params openapi.StartProviderConnectParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.ProviderConnectStart
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}
		return c.UseCases.Invoke(r.Context(), startProviderConnectUseCase, actor, usecase.Input{
			"provider_id":   body.ProviderId.String(),
			"step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
		})
	}, func(out usecase.Output) {
		providerID, _ := uuid.Parse(out.String("provider_id"))
		writeJSON(w, r, http.StatusCreated, openapi.ProviderConnectAuthorization{
			AuthorizationUrl: out.String("authorization_url"),
			ExpiresAt:        timeValue(out["expires_at"]),
			ProviderId:       providerID,
			ProviderName:     out.String("provider_name"),
		})
	})
}

// CompleteProviderConnect answers POST /accounts/me/identities:complete.
//
// Written out rather than through the `identity` helper, GetOwnAccount's reasoning: without a path
// parameter the helper's closure hides the request from `contextcheck`.
func (c *RestController) CompleteProviderConnect(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.ProviderConnectReturn
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), completeProviderConnectUseCase, actorOf(r), usecase.Input{
		"state":              body.State,
		"authorization_code": body.AuthorizationCode,
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, connectedIdentityResponse(out))
}

func connectedIdentityResponse(out usecase.Output) openapi.ConnectedIdentity {
	providerID, _ := uuid.Parse(out.String("provider_id"))
	return openapi.ConnectedIdentity{
		ProviderId:   providerID,
		ProviderName: out.String("provider_name"),
		LinkedAt:     timeValue(out["linked_at"]),
	}
}
