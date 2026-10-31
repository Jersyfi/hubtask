// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

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
