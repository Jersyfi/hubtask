// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The rules a signed-out visitor may read (ADR-0068 §7).
//
// The controller holds no rules, as ever: what a workspace demands of a password is resolved
// inwards of here, and this layer maps a request to an input and an answer to a document. The one
// thing it contributes is the host - only the adapter has the connection, so the workspace's
// address travels as a declared input rather than being re-derived somewhere that never saw it.

const getSignInRulesUseCase = "GetSignInRules"

func signInRulesResponse(out usecase.Output) openapi.SignInRules {
	methods := make([]openapi.SignInMethod, 0, 2)
	for _, method := range nameList(out["methods"]) {
		methods = append(methods, openapi.SignInMethod(method))
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
	fallback := boolValue(out["password_fallback"])
	names := boolValue(out["sign_in_names"])
	return openapi.SignInRules{
		PasswordFallback: &fallback,
		SignInNames:      &names,
		WorkspaceHost:    out.String("workspace_host"),
		Methods:          methods,
		Providers:        providers,
		Password:         passwordRulesResponse(out["password"]),
		Legal: openapi.LegalLinks{
			ImprintUrl:       optionalTextField(legal["imprint_url"]),
			PrivacyUrl:       optionalTextField(legal["privacy_url"]),
			TermsUrl:         optionalTextField(legal["terms_url"]),
			AccessibilityUrl: optionalTextField(legal["accessibility_url"]),
			TermsVersion:     optionalTextField(legal["terms_version"]),
			TermsSetBy:       termsSetByField(legal["terms_set_by"]),
		},
	}
}

// termsSetByField answers who set the terms, absent where no terms are in force.
func termsSetByField(value any) *openapi.LegalLinksTermsSetBy {
	text, _ := value.(string)
	if text == "" {
		return nil
	}
	setBy := openapi.LegalLinksTermsSetBy(text)
	return &setBy
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

const ()

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
)

const setPasswordAndSignInUseCase = "SetPasswordAndSignIn"
