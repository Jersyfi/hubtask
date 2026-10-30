// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The credentials. The controller holds no rules: whose tokens somebody may mint, list or
// revoke is decided inwards of here, like every other authorisation (ADR-0005). What this layer
// does is map a request to an input and an answer to a document - and, once, carry a plaintext
// from the mint to the response without letting it touch anything else on the way.

func accessTokenResponse(out usecase.Output) openapi.AccessToken {
	return openapi.AccessToken{
		Id:         uuidValue(out.String("id")),
		AccountId:  uuidValue(out.String("account_id")),
		Name:       out.String("name"),
		Scopes:     scopeList(out["scopes"]),
		ExpiresAt:  timeValue(out["expires_at"]),
		CreatedAt:  timeValue(out["created_at"]),
		LastUsedAt: optionalTimeField(out["last_used_at"]),
		RevokedAt:  optionalTimeField(out["revoked_at"]),
	}
}

// scopeList reads the projection's list of scope names. The catalogue carries them as []any,
// because that is what every channel's decoder produces; the contract wants strings.
func scopeList(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return []string{}
	}
	scopes := make([]string, 0, len(values))
	for _, entry := range values {
		if scope, isString := entry.(string); isString {
			scopes = append(scopes, scope)
		}
	}
	return scopes
}
