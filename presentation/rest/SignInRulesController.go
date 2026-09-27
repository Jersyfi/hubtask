// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The rules a signed-out visitor may read (ADR-0068 §7, SI-02).
//
// The controller holds no rules, as ever: what a workspace demands of a password is resolved
// inwards of here, and this layer maps a request to an input and an answer to a document. The one
// thing it contributes is the host - only the adapter has the connection, so the workspace's
// address travels as a declared input rather than being re-derived somewhere that never saw it.

const getSignInRulesUseCase = "GetSignInRules"

// GetSignInRules answers GET /auth/sign-in-rules.
//
// Written out rather than through the identity helper: the route is public, so there is no actor
// to resolve - the whole call exists so that somebody can produce one.
func (c *RestController) GetSignInRules(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), getSignInRulesUseCase, actorOf(r), usecase.Input{
		"host":          r.Host,
		"tenant_slug":   c.tenantSlug(r),
		"tenant_header": r.Header.Get(TenantHeader),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, signInRulesResponse(out))
}

func signInRulesResponse(out usecase.Output) openapi.SignInRules {
	methods := make([]openapi.SignInRulesMethods, 0, 2)
	for _, method := range nameList(out["methods"]) {
		methods = append(methods, openapi.SignInRulesMethods(method))
	}

	rows, _ := out["providers"].([]any)
	providers := make([]openapi.ProviderSummary, 0, len(rows))
	for _, row := range rows {
		provider, isOutput := row.(usecase.Output)
		if !isOutput {
			continue
		}
		providers = append(providers, openapi.ProviderSummary{
			Id:          provider.String("id"),
			DisplayName: provider.String("display_name"),
			Kind:        provider.String("kind"),
			Scope:       openapi.ProviderSummaryScope(provider.String("scope")),
		})
	}

	legal, _ := out["legal"].(usecase.Output)
	return openapi.SignInRules{
		WorkspaceHost: out.String("workspace_host"),
		Methods:       methods,
		Providers:     providers,
		Password:      passwordRulesResponse(out["password"]),
		Legal: openapi.LegalLinks{
			ImprintUrl:       optionalTextField(legal["imprint_url"]),
			PrivacyUrl:       optionalTextField(legal["privacy_url"]),
			TermsUrl:         optionalTextField(legal["terms_url"]),
			AccessibilityUrl: optionalTextField(legal["accessibility_url"]),
		},
	}
}

// passwordRulesResponse maps the rules projection. Zero is off for every count, `max_repeat`
// included: one spelling for "this switch does nothing" rather than a nullable number beside twelve
// that are not.
func passwordRulesResponse(value any) openapi.PasswordRules {
	rules, _ := value.(usecase.Output)
	answer := openapi.PasswordRules{
		MinLength:       intValue(rules["min_length"]),
		MinLowercase:    intValue(rules["min_lowercase"]),
		MinUppercase:    intValue(rules["min_uppercase"]),
		MinDigits:       intValue(rules["min_digits"]),
		MinSymbols:      intValue(rules["min_symbols"]),
		MinClasses:      intValue(rules["min_classes"]),
		CommonPasswords: boolValue(rules["common_passwords"]),
		ContextWords:    boolValue(rules["context_words"]),
		BreachCheck:     boolValue(rules["breach_check"]),
		HistoryCount:    intValue(rules["history_count"]),
		NotCurrent:      boolValue(rules["not_current"]),
		MaxRepeat:       intValue(rules["max_repeat"]),
	}
	return answer
}

// intValue and boolValue read a projection's number and flag. The catalogue is untyped by
// construction - it is one shape for three channels - and the contract is not, so the narrowing
// happens here rather than in a type assertion repeated at every field.
func intValue(value any) int {
	number, _ := value.(int)
	return number
}

func boolValue(value any) bool {
	flag, _ := value.(bool)
	return flag
}

// nameList reads a projection's list of names. The catalogue carries them as []any, because that
// is what every channel's decoder produces; the contract wants strings.
func nameList(value any) []string {
	values, _ := value.([]any)
	names := make([]string, 0, len(values))
	for _, entry := range values {
		if name, isString := entry.(string); isString {
			names = append(names, name)
		}
	}
	return names
}

const (
	changePasswordUseCase = "ChangePassword"
	checkPasswordUseCase  = "CheckPassword"
)

// ChangePassword answers POST /auth/password.
func (c *RestController) ChangePassword(
	w http.ResponseWriter, r *http.Request, params openapi.ChangePasswordParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.PasswordChange
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}
		return c.UseCases.Invoke(r.Context(), changePasswordUseCase, actor, usecase.Input{
			"password":      body.Password,
			"step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
		})
	}, func(usecase.Output) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// CheckPassword answers POST /auth/password:check.
//
// Written out rather than through the identity helper: the route is public, because three of the
// four proofs it accepts are the tokens of a flow that has no bearer yet.
func (c *RestController) CheckPassword(
	w http.ResponseWriter, r *http.Request, params openapi.CheckPasswordParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.PasswordCheck
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), checkPasswordUseCase, actorOf(r), usecase.Input{
		"password":         body.Password,
		"step_up_token":    stepUpHeaderField(params.XHubtaskStepUp),
		"pending_token":    optionalStringField(body.PendingToken),
		"invitation_token": optionalStringField(body.InvitationToken),
		"reset_token":      optionalStringField(body.ResetToken),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, passwordCheckResponse(out))
}

func passwordCheckResponse(out usecase.Output) openapi.PasswordCheckResult {
	rows, _ := out["violations"].([]any)
	violations := make([]openapi.PasswordViolation, 0, len(rows))
	for _, row := range rows {
		entry, isOutput := row.(usecase.Output)
		if !isOutput {
			continue
		}
		violation := openapi.PasswordViolation{Rule: entry.String("rule")}
		if params, held := entry["params"].(usecase.Output); held {
			named := map[string]string{}
			for name, value := range params {
				if text, isString := value.(string); isString {
					named[name] = text
				}
			}
			violation.Params = &named
		}
		violations = append(violations, violation)
	}
	return openapi.PasswordCheckResult{Violations: violations}
}

const (
	forgetPasswordUseCase = "ForgetPassword"
	resetPasswordUseCase  = "ResetPassword"
)

// ForgetPassword answers POST /auth/password:forgot.
//
// Written out rather than through the identity helper: the route is public, because the whole point
// is that somebody who cannot sign in can reach it.
func (c *RestController) ForgetPassword(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.PasswordForgot
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	if _, err := c.UseCases.Invoke(
		r.Context(), forgetPasswordUseCase, actorOf(r), usecase.Input{
			"email":         string(body.Email),
			"tenant_slug":   c.tenantSlug(r),
			"tenant_header": r.Header.Get(TenantHeader),
		}); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	// No body. There is nothing to say that would be true for one address and not for another.
	w.WriteHeader(http.StatusAccepted)
}

// ResetPassword answers POST /auth/password:reset.
func (c *RestController) ResetPassword(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.PasswordReset
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), resetPasswordUseCase, actorOf(r), usecase.Input{
		"token":         body.Token,
		"password":      body.Password,
		"user_agent":    r.UserAgent(),
		"remote_addr":   r.RemoteAddr,
		"tenant_header": r.Header.Get(TenantHeader),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if required, _ := out["mfa_required"].(bool); required {
		// The password is set and the account's second factor is still owed (ADR-0068 §6).
		writeJSON(w, r, http.StatusAccepted, mfaChallengeResponse(out))
		return
	}
	writeJSON(w, r, http.StatusCreated, sessionTokensResponse(out))
}
