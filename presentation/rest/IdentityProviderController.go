// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The relying party's configuration use cases.
const (
	offerIdentityProviderUseCase        = "OfferIdentityProvider"
	countAccountsWithoutProviderUseCase = "CountAccountsWithoutProvider"
	listIdentityProviderPresetsUseCase  = "ListIdentityProviderPresets"

	withdrawInstanceIdentityProviderUseCase         = "WithdrawInstanceIdentityProvider"
	cancelInstanceIdentityProviderWithdrawalUseCase = "CancelInstanceIdentityProviderWithdrawal"
)

// OfferIdentityProvider answers POST /identity-providers/{providerId}:offer.
func (c *RestController) OfferIdentityProvider(
	w http.ResponseWriter, r *http.Request, providerID openapi.ProviderId,
	params openapi.OfferIdentityProviderParams,
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
		"id":            providerID.String(),
		"offered":       body.Offered,
		"step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, identityProviderResponse(out))
}

// WithdrawInstanceIdentityProvider answers POST /admin/identity-providers/{providerId}:withdraw.
//
// The body is optional: none is the default notice. Read whole and decoded only when something
// arrived, so an empty request is the fourteen days rather than a malformed one.
func (c *RestController) WithdrawInstanceIdentityProvider(
	w http.ResponseWriter, r *http.Request, providerID openapi.ProviderId,
	params openapi.WithdrawInstanceIdentityProviderParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		WriteProblem(w, shared.ErrMalformedRequest.WithDetail("request.body_unreadable").WithCause(err), requestID)
		return
	}
	var body openapi.ProviderWithdrawal
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := decodeFrom(bytes.NewReader(raw), &body); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
	}

	in := usecase.Input{
		"id":            providerID.String(),
		"step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
	}
	if body.WithdrawAt != nil {
		in["withdraw_at"] = body.WithdrawAt.UTC().Format(time.RFC3339)
	}
	if body.ConfirmCount != nil {
		in["confirm_count"] = *body.ConfirmCount
	}
	out, err := c.UseCases.Invoke(r.Context(), withdrawInstanceIdentityProviderUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, identityProviderResponse(out))
}

// CancelInstanceIdentityProviderWithdrawal answers
// POST /admin/identity-providers/{providerId}:cancel-withdrawal.
func (c *RestController) CancelInstanceIdentityProviderWithdrawal(
	w http.ResponseWriter, r *http.Request, providerID openapi.ProviderId,
	params openapi.CancelInstanceIdentityProviderWithdrawalParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), cancelInstanceIdentityProviderWithdrawalUseCase,
		actorOf(r), usecase.Input{
			"id":            providerID.String(),
			"step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
		})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, identityProviderResponse(out))
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
	// A nil list stays the empty one: the contract's arrays are never null.
	if domains, held := out["allowed_email_domains"].([]string); held && domains != nil {
		answer.AllowedEmailDomains = domains
	}
	if directories, held := out["allowed_directories"].([]string); held && directories != nil {
		answer.AllowedDirectories = directories
	}
	if enabled, held := out["enabled"].(bool); held {
		answer.Enabled = enabled
	}
	if role, held := out["newcomer_role"].(string); held && role != "" {
		newcomer := openapi.MembershipRole(role)
		answer.NewcomerRole = &newcomer
	}
	if group, held := out["newcomer_group_id"].(string); held && group != "" {
		newcomer := uuidValue(group)
		answer.NewcomerGroupId = &newcomer
	}
	if offered, held := out["offered_here"].(bool); held {
		answer.OfferedHere = &offered
	}
	if lock, held := out["installation_lock"].(string); held {
		value := openapi.IdentityProviderInstallationLock(lock)
		answer.InstallationLock = &value
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
	if withdrawAt, held := out["withdraw_at"].(time.Time); held && !withdrawAt.IsZero() {
		answer.WithdrawAt = &withdrawAt
	}
	// The operator's projection carries it and a workspace's does not; absent stays absent.
	if offered, held := out["offered_workspaces"].(int); held {
		answer.OfferedWorkspaces = &offered
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
		// Required by the contract, so never absent: the installation's list is what its own
		// screen offers (ADR-0071's addendum).
		InstallationProvisioning: []openapi.IdentityProviderProvisioning{},
	}
	answer.Scopes = append(answer.Scopes, outputStrings(out["scopes"])...)
	for _, mode := range outputStrings(out["provisioning"]) {
		answer.Provisioning = append(answer.Provisioning,
			openapi.IdentityProviderProvisioning(mode))
	}
	for _, mode := range outputStrings(out["installation_provisioning"]) {
		answer.InstallationProvisioning = append(answer.InstallationProvisioning,
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
	if issuer := out.String("issuer"); issuer != "" {
		answer.Issuer = &issuer
	}
	if claim := out.String("directory_claim"); claim != "" {
		answer.DirectoryClaim = &claim
	}
	if templated, held := out["supports_templated_issuer"].(bool); held {
		answer.SupportsTemplatedIssuer = templated
	}
	// Required in the contract: an empty list, never null, where no issuer is shared.
	answer.SharedIssuerPaths = []string{}
	if paths, held := out["shared_issuer_paths"].([]any); held {
		for _, path := range paths {
			if text, ok := path.(string); ok {
				answer.SharedIssuerPaths = append(answer.SharedIssuerPaths, text)
			}
		}
	}
	answer.PersonalIssuerPaths = []string{}
	if paths, held := out["personal_issuer_paths"].([]any); held {
		for _, path := range paths {
			if text, ok := path.(string); ok {
				answer.PersonalIssuerPaths = append(answer.PersonalIssuerPaths, text)
			}
		}
	}
	answer.PersonalDirectories = []string{}
	if directories, held := out["personal_directories"].([]any); held {
		for _, directory := range directories {
			if text, ok := directory.(string); ok {
				answer.PersonalDirectories = append(answer.PersonalDirectories, text)
			}
		}
	}
	return answer
}

// The relying-party flow's use cases.
const (
	startOidcSignInUseCase    = "StartOidcSignIn"
	completeOidcSignInUseCase = "CompleteOidcSignIn"
	locateOidcFlowUseCase     = "LocateOidcFlow"
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
		"host":          r.Host,
		"tenant_header": r.Header.Get(TenantHeader),
	}
	if body.ProviderId != nil {
		in["provider_id"] = body.ProviderId.String()
	}
	if body.LoginHint != nil {
		in["login_hint"] = *body.LoginHint
	}
	if body.InvitationToken != nil && *body.InvitationToken != "" {
		in["invitation_token"] = *body.InvitationToken
	}
	if body.ConnectToken != nil && *body.ConnectToken != "" {
		in["connect_token"] = *body.ConnectToken
	}
	if body.TermsUrl != nil && *body.TermsUrl != "" {
		in["terms_url"] = *body.TermsUrl
	}
	if body.TermsVersion != nil && *body.TermsVersion != "" {
		in["terms_version"] = *body.TermsVersion
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

// LocateOidcFlow answers POST /auth/oidc:locate: where a provider's return belongs. Anonymous, as
// the return is, and in the auth bucket (RateLimit.go).
func (c *RestController) LocateOidcFlow(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.OidcLocate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), locateOidcFlowUseCase, actorOf(r), usecase.Input{"state": body.State})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, openapi.OidcLocated{BaseUrl: out.String("base_url")})
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
		"host":          r.Host,
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if required, _ := out["mfa_required"].(bool); required {
		// The address matched an account with a password, which is asked for before the provider
		// is connected to it (ADR-0071's addendum): the LINK step.
		writeJSON(w, r, http.StatusAccepted, mfaChallengeResponse(out))
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
