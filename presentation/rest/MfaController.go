// SPDX-License-Identifier: BUSL-1.1
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

// The use case names of the second factor (H-02).
const (
	completeSignInUseCase = "CompleteSignIn"
	completeLinkUseCase   = "CompleteLink"
	enrollTotpUseCase     = "EnrollTotp"
	confirmTotpUseCase    = "ConfirmTotp"
	disableTotpUseCase    = "DisableTotp"
)

// CompleteSignIn answers POST /auth/sessions:verify. Written out for SignIn's reason: the route
// is public, and the pending credential in the body is what authenticates the call.
func (c *RestController) CompleteSignIn(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.SignInCompletion
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), completeSignInUseCase, actorOf(r), usecase.Input{
		"pending_token": body.PendingToken,
		"code":          optionalStringField(body.Code),
		"recovery_code": optionalStringField(body.RecoveryCode),
		"tenant_header": r.Header.Get(TenantHeader),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, sessionTokensResponse(out))
}

// CompleteLink answers POST /auth/sessions:link: the account's password before a provider is
// connected to it (ADR-0071's addendum). A pair, or the second factor's step with the link carried.
func (c *RestController) CompleteLink(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.LinkCompletion
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), completeLinkUseCase, actorOf(r), usecase.Input{
		"pending_token": body.PendingToken,
		"password":      body.Password,
		"tenant_header": r.Header.Get(TenantHeader),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if required, _ := out["mfa_required"].(bool); required {
		writeJSON(w, r, http.StatusAccepted, mfaChallengeResponse(out))
		return
	}
	writeJSON(w, r, http.StatusCreated, sessionTokensResponse(out))
}

// EnrollTotp answers POST /auth/mfa/totp:enroll. Public for the enforcement flow; a signed-in
// caller's bearer was verified by the middleware exactly as on any public route.
func (c *RestController) EnrollTotp(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.TotpEnrollmentStart
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
	}

	out, err := c.UseCases.Invoke(r.Context(), enrollTotpUseCase, actorOf(r), usecase.Input{
		"pending_token": optionalStringField(body.PendingToken),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	codes := make([]string, 0)
	if raw, ok := out["recovery_codes"].([]any); ok {
		for _, code := range raw {
			if text, ok := code.(string); ok {
				codes = append(codes, text)
			}
		}
	}
	// The single showing (T-18): these three exist in this response and nowhere else.
	writeJSON(w, r, http.StatusCreated, openapi.TotpEnrollment{
		Secret:        out.String("secret"),
		OtpauthUri:    out.String("otpauth_uri"),
		RecoveryCodes: codes,
	})
}

// ConfirmTotp answers POST /auth/mfa/totp:confirm.
func (c *RestController) ConfirmTotp(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.TotpConfirmation
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), confirmTotpUseCase, actorOf(r), usecase.Input{
		"code":          body.Code,
		"pending_token": optionalStringField(body.PendingToken),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	confirmed := openapi.TotpConfirmed{Armed: true}
	if tokens, ok := out["tokens"].(usecase.Output); ok {
		pair := sessionTokensResponse(tokens)
		confirmed.Tokens = &pair
	}
	writeJSON(w, r, http.StatusOK, confirmed)
}

// DisableTotp answers POST /auth/mfa:disable. Written out for ListServiceAccounts' reason: the
// identity helper's closure gives the linter nothing to trace the request's context through.
func (c *RestController) DisableTotp(
	w http.ResponseWriter, r *http.Request, params openapi.DisableTotpParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	// The body is optional since the step-up proves the act (ADR-0075 §3): an empty one is a client
	// that sends only the header, and the deprecated password is read where one was sent.
	var body openapi.MfaDisable
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
	}
	in := usecase.Input{"step_up_token": stepUpHeaderField(params.XHubtaskStepUp)}
	if body.Password != nil {
		in["password"] = *body.Password
	}

	if _, err := c.UseCases.Invoke(r.Context(), disableTotpUseCase, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mfaChallengeResponse maps the 202 projection onto the contract's shape.
func mfaChallengeResponse(out usecase.Output) openapi.MfaChallenge {
	methods := make([]openapi.MfaChallengeMethods, 0)
	if raw, ok := out["methods"].([]any); ok {
		for _, method := range raw {
			if text, ok := method.(string); ok {
				methods = append(methods, openapi.MfaChallengeMethods(text))
			}
		}
	}
	challenge := openapi.MfaChallenge{
		PendingToken: out.String("pending_token"),
		ExpiresAt:    timeValue(out["expires_at"]),
		Methods:      methods,
	}
	if rules, held := out["password_rules"].(usecase.Output); held {
		// Only with PASSWORD_CHANGE (ADR-0068 §3): the screen that asks for a new password needs
		// the rule in the same answer, or the list under the field arrives a round trip after the
		// field does.
		answered := passwordRulesResponse(rules)
		challenge.PasswordRules = &answered
	}
	// Whose account, with LINK (ADR-0071's addendum) and after a reset; which provider, with LINK.
	if email := out.String("email"); email != "" {
		challenge.Email = &email
	}
	if name := out.String("provider_name"); name != "" {
		challenge.ProviderName = &name
	}
	return challenge
}

const stepUpUseCase = "StepUp"

// StepUp answers POST /auth/step-up. Written out for DisableTotp's reason.
func (c *RestController) StepUp(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.StepUpRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), stepUpUseCase, actorOf(r), usecase.Input{
		"password":           optionalStringField(body.Password),
		"code":               optionalStringField(body.Code),
		"recovery_code":      optionalStringField(body.RecoveryCode),
		"state":              optionalStringField(body.State),
		"authorization_code": optionalStringField(body.AuthorizationCode),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	// The single showing of the proof (T-18's discipline).
	writeJSON(w, r, http.StatusCreated, openapi.StepUpGrant{
		StepUpToken: out.String("step_up_token"),
		ExpiresAt:   timeValue(out["expires_at"]),
		Method:      openapi.StepUpGrantMethod(out.String("method")),
	})
}

const startProviderStepUpUseCase = "StartProviderStepUp"

// StartProviderStepUp answers POST /auth/step-up:provider (ADR-0075 §2).
func (c *RestController) StartProviderStepUp(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), startProviderStepUpUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	providerID, err := uuid.Parse(out.String("provider_id"))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, openapi.ProviderStepUpAuthorization{
		AuthorizationUrl: out.String("authorization_url"),
		ExpiresAt:        timeValue(out["expires_at"]),
		ProviderId:       providerID,
		ProviderName:     out.String("provider_name"),
	})
}

// stepUpHeaderField reads the proof off the header a privileged operation accepts it in.
func stepUpHeaderField(token *openapi.StepUpToken) any {
	if token == nil || *token == "" {
		return nil
	}
	return string(*token)
}

const regenerateRecoveryCodesUseCase = "RegenerateRecoveryCodes"

// RegenerateRecoveryCodes answers POST /auth/mfa/recovery:regenerate.
func (c *RestController) RegenerateRecoveryCodes(
	w http.ResponseWriter, r *http.Request, params openapi.RegenerateRecoveryCodesParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(
			r.Context(), regenerateRecoveryCodesUseCase, actor, usecase.Input{
				"step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
			})
	}, func(out usecase.Output) {
		// The second response in the whole API that carries credentials, and the enrolment's is the
		// first. They are here and in no projection, which is what makes "shown once" a property of
		// the code rather than a promise in the documentation.
		writeJSON(w, r, http.StatusCreated, openapi.RecoveryCodes{
			RecoveryCodes: recoveryCodeList(out["recovery_codes"]),
		})
	})
}

func recoveryCodeList(value any) []string {
	values, _ := value.([]any)
	codes := make([]string, 0, len(values))
	for _, entry := range values {
		if code, isString := entry.(string); isString {
			codes = append(codes, code)
		}
	}
	return codes
}
