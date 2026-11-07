// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/google/uuid"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The passkeys of the caller's own account (identity.md §23).

// passkeyListResponse maps the caller's passkeys with what they may do with them.
func passkeyListResponse(out usecase.Output) openapi.PasskeyList {
	rows, _ := out["data"].([]usecase.Output)
	answer := openapi.PasskeyList{
		Passkeys: make([]openapi.Passkey, 0, len(rows)),
		CanAdd:   out["can_add"] == true, CanRemove: out["can_remove"] == true,
	}
	answer.Limit, _ = out["limit"].(int)
	if rpID := out.String("rp_id"); rpID != "" {
		answer.RpId = &rpID
	}
	if reason := out.String("cannot_add_reason"); reason != "" {
		code := openapi.PasskeyListCannotAddReason(reason)
		answer.CannotAddReason = &code
	}
	if reason := out.String("cannot_remove_reason"); reason != "" {
		code := openapi.PasskeyListCannotRemoveReason(reason)
		answer.CannotRemoveReason = &code
	}
	for _, row := range rows {
		answer.Passkeys = append(answer.Passkeys, passkeyResponse(row))
	}
	return answer
}

func passkeyResponse(out usecase.Output) openapi.Passkey {
	id, _ := uuid.Parse(out.String("id"))
	answer := openapi.Passkey{
		Id: id, Name: out.String("name"),
		CreatedAt: timeValue(out["created_at"]), LastUsedAt: timePointer(out["last_used_at"]),
	}
	if rpID := out.String("rp_id"); rpID != "" {
		answer.RpId = &rpID
	}
	return answer
}

// renamedPasskeyResponse maps the passkey a rename answers.
func renamedPasskeyResponse(out usecase.Output) openapi.Passkey {
	passkey, _ := out["passkey"].(usecase.Output)
	return passkeyResponse(passkey)
}

// passkeyRegisteredResponse draws a registered passkey, and the recovery codes the first one
// brings, shown once.
func passkeyRegisteredResponse(out usecase.Output) openapi.PasskeyRegistered {
	passkey, _ := out["passkey"].(usecase.Output)
	answer := openapi.PasskeyRegistered{Passkey: passkeyResponse(passkey)}
	if codes, ok := out["recovery_codes"].([]any); ok && len(codes) > 0 {
		listed := make([]string, 0, len(codes))
		for _, code := range codes {
			text, _ := code.(string)
			listed = append(listed, text)
		}
		answer.RecoveryCodes = &listed
	}
	return answer
}

// The passkey sign-in (identity.md §23.4). Written out, SignIn's reasoning: the routes are public, so
// there is no actor to resolve - the call exists to produce one - and the request's host, client and
// workspace sources travel as declared inputs, because only this adapter has the request.

// passkeyEnrolledResponse maps an enrolled passkey: the first factor's recovery codes, and the session or the step still owed.
func passkeyEnrolledResponse(out usecase.Output) openapi.PasskeyEnrolled {
	passkey, _ := out["passkey"].(usecase.Output)
	answer := openapi.PasskeyEnrolled{Passkey: passkeyResponse(passkey)}
	if codes, ok := out["recovery_codes"].([]any); ok && len(codes) > 0 {
		listed := make([]string, 0, len(codes))
		for _, code := range codes {
			text, _ := code.(string)
			listed = append(listed, text)
		}
		answer.RecoveryCodes = &listed
	}
	if tokens, ok := out["tokens"].(usecase.Output); ok {
		pair := sessionTokensResponse(tokens)
		answer.Tokens = &pair
	}
	if challenge, ok := out["challenge"].(usecase.Output); ok {
		owed := mfaChallengeResponse(challenge)
		answer.Challenge = &owed
	}
	return answer
}
