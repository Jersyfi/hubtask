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

// The relying party's configuration use cases (H-04, SI-10).
const (
	readIdentityProviderUseCase           = "ReadIdentityProvider"
	configureFirstIdentityProviderUseCase = "ConfigureFirstIdentityProvider"
	offerIdentityProviderUseCase          = "OfferIdentityProvider"
	listIdentityProvidersUseCase          = "ListIdentityProviders"
	configureIdentityProviderUseCase      = "ConfigureIdentityProvider"
	removeIdentityProviderUseCase         = "RemoveIdentityProvider"
	listIdentityProviderPresetsUseCase    = "ListIdentityProviderPresets"

	listInstanceIdentityProvidersUseCase     = "ListInstanceIdentityProviders"
	configureInstanceIdentityProviderUseCase = "ConfigureInstanceIdentityProvider"
	removeInstanceIdentityProviderUseCase    = "RemoveInstanceIdentityProvider"
)

// ReadIdentityProvider answers GET /identity-provider — the singular surface, kept.
func (c *RestController) ReadIdentityProvider(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(
		r.Context(), readIdentityProviderUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, identityProviderResponse(out))
}

// ConfigureFirstIdentityProvider answers PUT /identity-provider.
func (c *RestController) ConfigureFirstIdentityProvider(w http.ResponseWriter, r *http.Request) {
	c.writeProvider(w, r, configureFirstIdentityProviderUseCase, "", http.StatusOK)
}

// OfferIdentityProvider answers POST /identity-providers/{providerId}:offer.
func (c *RestController) OfferIdentityProvider(
	w http.ResponseWriter, r *http.Request, providerID openapi.ProviderId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.ProviderOffer
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), offerIdentityProviderUseCase, actorOf(r), usecase.Input{
		"id":      providerID.String(),
		"offered": body.Offered,
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, identityProviderResponse(out))
}

// ListIdentityProviders answers GET /identity-providers.
func (c *RestController) ListIdentityProviders(w http.ResponseWriter, r *http.Request) {
	c.listProviders(w, r, listIdentityProvidersUseCase)
}

// ListInstanceIdentityProviders answers GET /admin/identity-providers.
func (c *RestController) ListInstanceIdentityProviders(w http.ResponseWriter, r *http.Request) {
	c.listProviders(w, r, listInstanceIdentityProvidersUseCase)
}

// listProviders serves both levels. The two answer the same shape because they project the same
// row: a second mapping would drift on the day a field is added.
func (c *RestController) listProviders(w http.ResponseWriter, r *http.Request, useCase string) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), useCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, identityProvidersResponse(out))
}

// CreateIdentityProvider answers POST /identity-providers.
func (c *RestController) CreateIdentityProvider(w http.ResponseWriter, r *http.Request) {
	c.writeProvider(w, r, configureIdentityProviderUseCase, "", http.StatusCreated)
}

// ConfigureIdentityProvider answers PUT /identity-providers/{providerId}.
func (c *RestController) ConfigureIdentityProvider(
	w http.ResponseWriter, r *http.Request, providerID openapi.ProviderId,
) {
	c.writeProvider(w, r, configureIdentityProviderUseCase, providerID.String(), http.StatusOK)
}

// CreateInstanceIdentityProvider answers POST /admin/identity-providers.
func (c *RestController) CreateInstanceIdentityProvider(w http.ResponseWriter, r *http.Request) {
	c.writeProvider(w, r, configureInstanceIdentityProviderUseCase, "", http.StatusCreated)
}

// ConfigureInstanceIdentityProvider answers PUT /admin/identity-providers/{providerId}.
func (c *RestController) ConfigureInstanceIdentityProvider(
	w http.ResponseWriter, r *http.Request, providerID openapi.ProviderId,
) {
	c.writeProvider(w, r, configureInstanceIdentityProviderUseCase, providerID.String(), http.StatusOK)
}

// writeProvider decodes the body once for four routes. Which level and whether it adds or replaces
// are the use case's name and the presence of an identifier - nothing here decides either.
func (c *RestController) writeProvider(
	w http.ResponseWriter, r *http.Request, useCase, providerID string, created int,
) {
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
		"issuer":    body.Issuer,
		"client_id": body.ClientId,
	}
	if providerID != "" {
		in["id"] = providerID
	}
	// An absent secret is the promise of the contract: it keeps the one that is sealed. Written
	// only when it is there, so that "absent" and "empty" do not become the same request.
	if body.ClientSecret != nil {
		in["client_secret"] = *body.ClientSecret
	}
	if body.DisplayName != nil {
		in["display_name"] = *body.DisplayName
	}
	if body.Kind != nil {
		in["kind"] = string(*body.Kind)
	}
	if body.Provisioning != nil {
		in["provisioning"] = string(*body.Provisioning)
	}
	if body.Position != nil {
		in["position"] = *body.Position
	}
	if body.AllowedEmailDomains != nil {
		in["allowed_email_domains"] = *body.AllowedEmailDomains
	}
	if body.AllowedDirectories != nil {
		in["allowed_directories"] = *body.AllowedDirectories
	}
	if body.Enabled != nil {
		in["enabled"] = *body.Enabled
	}

	out, err := c.UseCases.Invoke(r.Context(), useCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, created, identityProviderResponse(out))
}

// RemoveIdentityProvider answers DELETE /identity-providers/{providerId}.
func (c *RestController) RemoveIdentityProvider(
	w http.ResponseWriter, r *http.Request, providerID openapi.ProviderId,
) {
	c.removeProvider(w, r, removeIdentityProviderUseCase, providerID)
}

// RemoveInstanceIdentityProvider answers DELETE /admin/identity-providers/{providerId}.
func (c *RestController) RemoveInstanceIdentityProvider(
	w http.ResponseWriter, r *http.Request, providerID openapi.ProviderId,
) {
	c.removeProvider(w, r, removeInstanceIdentityProviderUseCase, providerID)
}

func (c *RestController) removeProvider(
	w http.ResponseWriter, r *http.Request, useCase string, providerID openapi.ProviderId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	if _, err := c.UseCases.Invoke(r.Context(), useCase, actorOf(r), usecase.Input{
		"id": providerID.String(),
	}); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListIdentityProviderPresets answers GET /identity-provider-presets.
func (c *RestController) ListIdentityProviderPresets(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(
		r.Context(), listIdentityProviderPresetsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	rows, _ := out["data"].([]usecase.Output)
	presets := make([]openapi.IdentityProviderPreset, 0, len(rows))
	for _, row := range rows {
		presets = append(presets, identityProviderPresetResponse(row))
	}
	writeJSON(w, r, http.StatusOK, presets)
}

// identityProvidersResponse maps the collection.
func identityProvidersResponse(out usecase.Output) []openapi.IdentityProvider {
	rows, _ := out["data"].([]usecase.Output)
	answers := make([]openapi.IdentityProvider, 0, len(rows))
	for _, row := range rows {
		answers = append(answers, identityProviderResponse(row))
	}
	return answers
}

// identityProviderResponse maps the use case's answer. The client secret is not among the
// fields, because it is not among the use case's either - there is no call that answers it.
func identityProviderResponse(out usecase.Output) openapi.IdentityProvider {
	answer := openapi.IdentityProvider{
		Id:                  uuidValue(out.String("id")),
		Scope:               openapi.IdentityProviderScope(out.String("scope")),
		Issuer:              out.String("issuer"),
		ClientId:            out.String("client_id"),
		DisplayName:         out.String("display_name"),
		Kind:                openapi.IdentityProviderKind(out.String("kind")),
		Provisioning:        openapi.IdentityProviderProvisioning(out.String("provisioning")),
		AllowedEmailDomains: []string{},
		AllowedDirectories:  []string{},
	}
	if position, held := out["position"].(int); held {
		answer.Position = position
	}
	if domains, held := out["allowed_email_domains"].([]string); held {
		answer.AllowedEmailDomains = domains
	}
	if directories, held := out["allowed_directories"].([]string); held {
		answer.AllowedDirectories = directories
	}
	if enabled, held := out["enabled"].(bool); held {
		answer.Enabled = enabled
	}
	if offered, held := out["offered_here"].(bool); held {
		answer.OfferedHere = &offered
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

// identityProviderPresetResponse maps one preset. The instructions stay a message code: the
// backend holds no display text (rule 8), and which language it is rendered in is the client's.
func identityProviderPresetResponse(out usecase.Output) openapi.IdentityProviderPreset {
	answer := openapi.IdentityProviderPreset{
		Kind:         openapi.IdentityProviderKind(out.String("kind")),
		Scopes:       []string{},
		RedirectUri:  out.String("redirect_uri"),
		Instructions: out.String("instructions"),
		Provisioning: []openapi.IdentityProviderProvisioning{},
	}
	answer.Scopes = append(answer.Scopes, outputStrings(out["scopes"])...)
	for _, mode := range outputStrings(out["provisioning"]) {
		answer.Provisioning = append(answer.Provisioning,
			openapi.IdentityProviderProvisioning(mode))
	}
	if verified, held := out["addresses_verified"].(bool); held {
		answer.AddressesVerified = verified
	}
	if public, held := out["public"].(bool); held {
		answer.Public = public
	}
	if particular := out.String("particular"); particular != "" {
		answer.Particular = &particular
	}
	if claim := out.String("directory_claim"); claim != "" {
		answer.DirectoryClaim = &claim
	}
	if templated, held := out["supports_templated_issuer"].(bool); held {
		answer.SupportsTemplatedIssuer = templated
	}
	return answer
}

// The relying-party flow's use cases (H-04).
const (
	startOidcSignInUseCase    = "StartOidcSignIn"
	completeOidcSignInUseCase = "CompleteOidcSignIn"
)

// StartOidcSignIn answers POST /auth/oidc:start.
func (c *RestController) StartOidcSignIn(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	// The body is optional: a caller with nothing to add sends none, and an empty one is not an
	// error. Anything that is there is read, and a malformed document still is.
	var body openapi.OidcStart
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &body); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
	}

	in := usecase.Input{
		"tenant_slug":   c.tenantSlug(r),
		"tenant_header": r.Header.Get(TenantHeader),
	}
	if body.ProviderId != nil {
		in["provider_id"] = body.ProviderId.String()
	}
	if body.LoginHint != nil {
		in["login_hint"] = *body.LoginHint
	}

	out, err := c.UseCases.Invoke(r.Context(), startOidcSignInUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	answer := openapi.OidcAuthorization{
		AuthorizationUrl: out.String("authorization_url"),
		State:            out.String("state"),
	}
	if expires, held := out["expires_at"].(time.Time); held {
		answer.ExpiresAt = expires
	}
	writeJSON(w, r, http.StatusCreated, answer)
}

// CompleteOidcSignIn answers POST /auth/oidc:callback.
func (c *RestController) CompleteOidcSignIn(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.OidcCallback
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), completeOidcSignInUseCase, actorOf(r), usecase.Input{
		"code":  body.Code,
		"state": body.State,
		// The client hints and the tenant source are the request's, SignIn's reasoning.
		"user_agent":    r.UserAgent(),
		"remote_addr":   r.RemoteAddr,
		"tenant_header": r.Header.Get(TenantHeader),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, sessionTokensResponse(out))
}

// outputStrings reads a list a use case built for every channel: `[]any` of strings, which is what
// the registry's projection is made of.
func outputStrings(value any) []string {
	entries, held := value.([]any)
	if !held {
		return nil
	}
	strings := make([]string, 0, len(entries))
	for _, entry := range entries {
		if text, held := entry.(string); held {
			strings = append(strings, text)
		}
	}
	return strings
}
