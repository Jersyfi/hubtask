// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The use case names of the second factor.
const (
	completeSignInUseCase = "CompleteSignIn"
	completeLinkUseCase   = "CompleteLink"
	agreeToTermsUseCase   = "AgreeToTerms"
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
		"pending_token":      body.PendingToken,
		"code":               optionalStringField(body.Code),
		"recovery_code":      optionalStringField(body.RecoveryCode),
		"raw_id":             optionalStringField(body.RawId),
		"client_data_json":   optionalStringField(body.ClientDataJson),
		"authenticator_data": optionalStringField(body.AuthenticatorData),
		"signature":          optionalStringField(body.Signature),
		"tenant_header":      r.Header.Get(TenantHeader),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if required, _ := out["mfa_required"].(bool); required {
		// The code was right and the password still owes the change step.
		writeJSON(w, r, http.StatusAccepted, mfaChallengeResponse(out))
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
		"host":          r.Host,
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

// AgreeToTerms answers POST /auth/sessions:agree-terms: the TERMS step (UC-ID-19). Public, for
// SignIn's reason: the pending credential in the body is what authenticates the call.
func (c *RestController) AgreeToTerms(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.TermsAgreement
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), agreeToTermsUseCase, actorOf(r), usecase.Input{
		"pending_token": body.PendingToken,
		"terms_url":     body.TermsUrl,
		"terms_version": optionalStringField(body.TermsVersion),
		"tenant_header": r.Header.Get(TenantHeader),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, sessionTokensResponse(out))
}

// totpConfirmedResponse maps an armed authenticator: the session pair, or the change step the sign-in still owes standing where the pair would.
func totpConfirmedResponse(out usecase.Output) openapi.TotpConfirmed {
	confirmed := openapi.TotpConfirmed{Armed: true}
	if tokens, ok := out["tokens"].(usecase.Output); ok {
		pair := sessionTokensResponse(tokens)
		confirmed.Tokens = &pair
	}
	if challenge, ok := out["challenge"].(usecase.Output); ok {
		owed := mfaChallengeResponse(challenge)
		confirmed.Challenge = &owed
	}
	return confirmed
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
	// The key's ceremony, with PASSKEY; whether a key can be set up instead of the app, with ENROLL.
	if options, held := out["passkey_options"].(usecase.Output); held {
		var answered openapi.PasskeyAssertionOptions
		if encoded, err := json.Marshal(options); err == nil && json.Unmarshal(encoded, &answered) == nil {
			challenge.PasskeyOptions = &answered
		}
	}
	if setup, _ := out["passkey_setup"].(bool); setup {
		challenge.PasskeySetup = &setup
	}
	// The terms to agree to, with TERMS (UC-ID-19).
	if terms, held := out["terms"].(usecase.Output); held {
		answered := openapi.TermsInForce{
			Url:   terms.String("url"),
			SetBy: openapi.TermsInForceSetBy(terms.String("set_by")),
		}
		if version := terms.String("version"); version != "" {
			answered.Version = &version
		}
		challenge.Terms = &answered
	}
	return challenge
}

const stepUpUseCase = "StepUp"

const startPasskeyStepUpUseCase = "StartPasskeyStepUp"

// StartPasskeyStepUp answers POST /auth/step-up:passkey-options.
func (c *RestController) StartPasskeyStepUp(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), startPasskeyStepUpUseCase, actorOf(r), usecase.Input{"host": r.Host})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	var answer openapi.PasskeyAssertionOptions
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

const startProviderStepUpUseCase = "StartProviderStepUp"

// stepUpHeaderField reads the proof off the header a privileged operation accepts it in.
func stepUpHeaderField(token *openapi.StepUpToken) any {
	if token == nil || *token == "" {
		return nil
	}
	return string(*token)
}

const regenerateRecoveryCodesUseCase = "RegenerateRecoveryCodes"

const ()

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
