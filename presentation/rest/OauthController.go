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

// The use case names of the OAuth2 provider (H-05).
const (
	registerOauthClientUseCase  = "RegisterOauthClient"
	listOauthClientsUseCase     = "ListOauthClients"
	deleteOauthClientUseCase    = "DeleteOauthClient"
	authorizeOauthClientUseCase = "AuthorizeOauthClient"
	exchangeOauthCodeUseCase    = "ExchangeOauthCode"
	listOauthGrantsUseCase      = "ListOauthGrants"
	revokeOauthGrantUseCase     = "RevokeOauthGrant"
)

// RegisterOauthClient answers POST /oauth/clients.
func (c *RestController) RegisterOauthClient(
	w http.ResponseWriter, r *http.Request, _ openapi.RegisterOauthClientParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.OauthClientCreate
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}
		uris := make([]any, 0, len(body.RedirectUris))
		for _, uri := range body.RedirectUris {
			uris = append(uris, uri)
		}
		return c.UseCases.Invoke(r.Context(), registerOauthClientUseCase, actor, usecase.Input{
			"name":          body.Name,
			"redirect_uris": uris,
			"confidential":  body.Confidential,
		})
	}, func(out usecase.Output) {
		client := oauthClientResponse(out)
		registered := openapi.OauthClientSecret{
			Id: client.Id, Name: client.Name, RedirectUris: client.RedirectUris,
			Confidential: client.Confidential, CreatedAt: client.CreatedAt,
			// The one response the secret ever appears in (T-18's "shown once").
			ClientSecret: optionalTextField(out["client_secret"]),
		}
		w.Header().Set("Location", APIBasePath+"/oauth/clients/"+client.Id.String())
		writeJSON(w, r, http.StatusCreated, registered)
	})
}

// ListOauthClients answers GET /oauth/clients. Written out for ListServiceAccounts' reason.
func (c *RestController) ListOauthClients(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), listOauthClientsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	rows, _ := out["data"].([]usecase.Output)
	clients := make([]openapi.OauthClient, 0, len(rows))
	for _, row := range rows {
		clients = append(clients, oauthClientResponse(row))
	}
	writeJSON(w, r, http.StatusOK, clients)
}

// DeleteOauthClient answers DELETE /oauth/clients/{clientId}.
func (c *RestController) DeleteOauthClient(
	w http.ResponseWriter, r *http.Request, clientID openapi.OauthClientId,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), deleteOauthClientUseCase, actor, usecase.Input{
			"client_id": clientID.String(),
		})
	}, func(usecase.Output) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// AuthorizeOauthClient answers POST /oauth/authorize. Written out for ListServiceAccounts'
// reason: the identity helper's closure gives the linter nothing to trace the context through.
func (c *RestController) AuthorizeOauthClient(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.OauthAuthorization
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	scopes := make([]any, 0, len(body.Scopes))
	for _, scope := range body.Scopes {
		scopes = append(scopes, scope)
	}
	out, err := c.UseCases.Invoke(r.Context(), authorizeOauthClientUseCase, actorOf(r), usecase.Input{
		"client_id":             body.ClientId.String(),
		"redirect_uri":          body.RedirectUri,
		"scopes":                scopes,
		"code_challenge":        body.CodeChallenge,
		"code_challenge_method": string(body.CodeChallengeMethod),
		"state":                 optionalStringField(body.State),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, openapi.OauthCode{
		Code:      out.String("code"),
		ExpiresAt: timeValue(out["expires_at"]),
		State:     optionalTextField(out["state"]),
	})
}

// ExchangeOauthCode answers POST /oauth/token. Written out for SignIn's reason: the route is
// public, and the body is the whole of what authenticates.
func (c *RestController) ExchangeOauthCode(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.OauthTokenRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), exchangeOauthCodeUseCase, actorOf(r), usecase.Input{
		"grant_type":    string(body.GrantType),
		"code":          body.Code,
		"redirect_uri":  body.RedirectUri,
		"client_id":     body.ClientId.String(),
		"code_verifier": optionalStringField(body.CodeVerifier),
		"client_secret": optionalStringField(body.ClientSecret),
		"tenant_header": r.Header.Get(TenantHeader),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, sessionTokensResponse(out))
}

// ListOauthGrants answers GET /oauth/grants. Written out for ListServiceAccounts' reason.
func (c *RestController) ListOauthGrants(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), listOauthGrantsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	rows, _ := out["data"].([]usecase.Output)
	grants := make([]openapi.OauthGrant, 0, len(rows))
	for _, row := range rows {
		grants = append(grants, openapi.OauthGrant{
			Id:         uuidValue(row.String("id")),
			ClientId:   uuidValue(row.String("client_id")),
			ClientName: row.String("client_name"),
			Scopes:     scopeList(row["scopes"]),
			CreatedAt:  timeValue(row["created_at"]),
			LastUsedAt: optionalTimeField(row["last_used_at"]),
		})
	}
	writeJSON(w, r, http.StatusOK, grants)
}

// RevokeOauthGrant answers DELETE /oauth/grants/{grantId}.
func (c *RestController) RevokeOauthGrant(
	w http.ResponseWriter, r *http.Request, grantID openapi.OauthGrantId,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), revokeOauthGrantUseCase, actor, usecase.Input{
			"grant_id": grantID.String(),
		})
	}, func(usecase.Output) {
		w.WriteHeader(http.StatusNoContent)
	})
}

func oauthClientResponse(row usecase.Output) openapi.OauthClient {
	confidential, _ := row["confidential"].(bool)
	return openapi.OauthClient{
		Id:           uuidValue(row.String("id")),
		Name:         row.String("name"),
		RedirectUris: scopeList(row["redirect_uris"]),
		Confidential: confidential,
		CreatedAt:    timeValue(row["created_at"]),
	}
}
