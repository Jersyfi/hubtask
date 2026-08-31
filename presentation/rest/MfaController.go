// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The use case names of the second factor (H-02).
const (
	completeSignInUseCase = "CompleteSignIn"
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
func (c *RestController) DisableTotp(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.MfaDisable
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	if _, err := c.UseCases.Invoke(r.Context(), disableTotpUseCase, actorOf(r), usecase.Input{
		"password": body.Password,
	}); err != nil {
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
	return openapi.MfaChallenge{
		PendingToken: out.String("pending_token"),
		ExpiresAt:    timeValue(out["expires_at"]),
		Methods:      methods,
	}
}
