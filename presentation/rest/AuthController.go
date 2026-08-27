// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The credentials (G-01). The controller holds no rules: whose tokens somebody may mint, list or
// revoke is decided inwards of here, like every other authorisation (ADR-0005). What this layer
// does is map a request to an input and an answer to a document - and, once, carry a plaintext
// from the mint to the response without letting it touch anything else on the way.

const (
	listAccessTokensUseCase  = "ListAccessTokens"
	createAccessTokenUseCase = "CreateAccessToken"
	revokeAccessTokenUseCase = "RevokeAccessToken"

	listServiceAccountsUseCase  = "ListServiceAccounts"
	createServiceAccountUseCase = "CreateServiceAccount"
)

// ListAccessTokens answers GET /auth/tokens.
func (c *RestController) ListAccessTokens(
	w http.ResponseWriter, r *http.Request, params openapi.ListAccessTokensParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), listAccessTokensUseCase, actor, usecase.Input{
			"account_id": optionalUUIDField(params.AccountId),
		})
	}, func(out usecase.Output) {
		rows, _ := out["data"].([]usecase.Output)
		tokens := make([]openapi.AccessToken, 0, len(rows))
		for _, row := range rows {
			tokens = append(tokens, accessTokenResponse(row))
		}
		writeJSON(w, r, http.StatusOK, tokens)
	})
}

// CreateAccessToken answers POST /auth/tokens.
func (c *RestController) CreateAccessToken(
	w http.ResponseWriter, r *http.Request, _ openapi.CreateAccessTokenParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.AccessTokenCreate
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}

		scopes := make([]any, 0, len(body.Scopes))
		for _, scope := range body.Scopes {
			scopes = append(scopes, scope)
		}
		return c.UseCases.Invoke(r.Context(), createAccessTokenUseCase, actor, usecase.Input{
			"name":   body.Name,
			"scopes": scopes,
			// RFC 3339 is the one spelling the contract declares, and the generated type has
			// already parsed it; formatting it back is what keeps the catalogue's input the same
			// shape whichever channel filled it.
			"expires_at": body.ExpiresAt.Format(time.RFC3339),
			"account_id": optionalUUIDField(body.AccountId),
		})
	}, func(out usecase.Output) {
		minted := openapi.AccessTokenSecret{
			Id:         uuidValue(out.String("id")),
			AccountId:  uuidValue(out.String("account_id")),
			Name:       out.String("name"),
			Scopes:     scopeList(out["scopes"]),
			ExpiresAt:  timeValue(out["expires_at"]),
			CreatedAt:  timeValue(out["created_at"]),
			LastUsedAt: optionalTimeField(out["last_used_at"]),
			RevokedAt:  optionalTimeField(out["revoked_at"]),
			// The one response in the whole API that carries a credential. It is here and in no
			// projection, which is what makes "shown once" a property of the code rather than a
			// promise in the documentation.
			Token: out.String("token"),
		}
		w.Header().Set("Location", APIBasePath+"/auth/tokens/"+minted.Id.String())
		writeJSON(w, r, http.StatusCreated, minted)
	})
}

// RevokeAccessToken answers DELETE /auth/tokens/{tokenId}.
func (c *RestController) RevokeAccessToken(w http.ResponseWriter, r *http.Request, tokenID openapi.TokenId) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), revokeAccessTokenUseCase, actor, usecase.Input{
			"token_id": tokenID.String(),
		})
	}, func(usecase.Output) {
		w.WriteHeader(http.StatusNoContent)
	})
}

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

// ListServiceAccounts answers GET /auth/service-accounts.
//
// Written out rather than through the identity helper, for the reason ListCalendarFeeds is: the
// helper's closure takes no context, and an operation with no parameters gives the linter nothing
// to trace the request's context through.
func (c *RestController) ListServiceAccounts(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(
		r.Context(), listServiceAccountsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	rows, _ := out["data"].([]usecase.Output)
	accounts := make([]openapi.Account, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, accountResponse(row))
	}
	writeJSON(w, r, http.StatusOK, accounts)
}

// CreateServiceAccount answers POST /auth/service-accounts.
func (c *RestController) CreateServiceAccount(
	w http.ResponseWriter, r *http.Request, _ openapi.CreateServiceAccountParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.ServiceAccountCreate
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}
		return c.UseCases.Invoke(r.Context(), createServiceAccountUseCase, actor, usecase.Input{
			"display_name": body.DisplayName,
		})
	}, func(out usecase.Output) {
		account := accountResponse(out)
		w.Header().Set("Location", APIBasePath+"/auth/service-accounts/"+account.Id.String())
		writeJSON(w, r, http.StatusCreated, account)
	})
}
