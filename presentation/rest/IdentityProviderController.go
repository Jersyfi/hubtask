// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The relying party's configuration use cases (H-04).
const (
	readIdentityProviderUseCase      = "ReadIdentityProvider"
	configureIdentityProviderUseCase = "ConfigureIdentityProvider"
	removeIdentityProviderUseCase    = "RemoveIdentityProvider"
)

// ReadIdentityProvider answers GET /identity-provider.
func (c *RestController) ReadIdentityProvider(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), readIdentityProviderUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, identityProviderResponse(out))
}

// ConfigureIdentityProvider answers PUT /identity-provider.
func (c *RestController) ConfigureIdentityProvider(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.IdentityProviderConfiguration
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"issuer":        body.Issuer,
		"client_id":     body.ClientId,
		"client_secret": body.ClientSecret,
	}
	if body.AllowedEmailDomains != nil {
		in["allowed_email_domains"] = *body.AllowedEmailDomains
	}
	if body.Enabled != nil {
		in["enabled"] = *body.Enabled
	}

	out, err := c.UseCases.Invoke(r.Context(), configureIdentityProviderUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, identityProviderResponse(out))
}

// RemoveIdentityProvider answers DELETE /identity-provider.
func (c *RestController) RemoveIdentityProvider(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	if _, err := c.UseCases.Invoke(
		r.Context(), removeIdentityProviderUseCase, actorOf(r), usecase.Input{},
	); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// identityProviderResponse maps the use case's answer. The client secret is not among the
// fields, because it is not among the use case's either - there is no call that answers it.
func identityProviderResponse(out usecase.Output) openapi.IdentityProvider {
	answer := openapi.IdentityProvider{
		Issuer:              out.String("issuer"),
		ClientId:            out.String("client_id"),
		AllowedEmailDomains: []string{},
	}
	if domains, held := out["allowed_email_domains"].([]string); held {
		answer.AllowedEmailDomains = domains
	}
	if enabled, held := out["enabled"].(bool); held {
		answer.Enabled = enabled
	}
	if created, held := out["created_at"].(time.Time); held {
		answer.CreatedAt = created
	}
	if updated, held := out["updated_at"].(time.Time); held && !updated.IsZero() {
		answer.UpdatedAt = &updated
	}
	if version, held := out["version"].(int); held {
		answer.Version = version
	}
	return answer
}
