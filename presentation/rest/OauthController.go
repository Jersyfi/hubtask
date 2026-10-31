// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The use case names of the OAuth2 provider.
const (
	exchangeOauthCodeUseCase = "ExchangeOauthCode"
)

// oauthClientSecretResponse maps a registered client with the one answer its secret ever appears in (T-18's shown once).
func oauthClientSecretResponse(out usecase.Output) openapi.OauthClientSecret {
	client := oauthClientResponse(out)
	return openapi.OauthClientSecret{
		Id: client.Id, Name: client.Name, RedirectUris: client.RedirectUris,
		Confidential: client.Confidential, CreatedAt: client.CreatedAt,
		ClientSecret: optionalTextField(out["client_secret"]),
	}
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

// formEncoded says whether a request's body is a form rather than this API's JSON.
func formEncoded(r *http.Request) bool {
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return mediaType == "application/x-www-form-urlencoded"
}

// exchangeOauthForm is the token endpoint in RFC 6749's own form: a form-encoded body, the
// client's credentials in Basic or in the body, `authorization_code` and `refresh_token` grants,
// and the standard answer - `access_token`, `token_type`, `expires_in`, `refresh_token` - or the
// standard error. The same use cases answer as for the JSON form; only the shape differs.
func (c *RestController) exchangeOauthForm(w http.ResponseWriter, r *http.Request) {
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, correlation.RequestIDFrom(r.Context()))
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeOauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	form, err := url.ParseQuery(string(raw))
	if err != nil {
		writeOauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	clientID, clientSecret, basic := r.BasicAuth()
	if !basic {
		clientID, clientSecret = form.Get("client_id"), form.Get("client_secret")
	}

	var out usecase.Output
	switch form.Get("grant_type") {
	case "authorization_code":
		out, err = c.UseCases.Invoke(r.Context(), exchangeOauthCodeUseCase, actorOf(r), usecase.Input{
			"grant_type":    "authorization_code",
			"code":          form.Get("code"),
			"redirect_uri":  form.Get("redirect_uri"),
			"client_id":     clientID,
			"code_verifier": form.Get("code_verifier"),
			"client_secret": clientSecret,
			"tenant_header": r.Header.Get(TenantHeader),
		})
	case "refresh_token":
		out, err = c.UseCases.Invoke(r.Context(), refreshSessionUseCase, actorOf(r), usecase.Input{
			"refresh_token": form.Get("refresh_token"),
			"tenant_header": r.Header.Get(TenantHeader),
		})
	default:
		writeOauthError(w, http.StatusBadRequest, "unsupported_grant_type")
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, shared.ErrUnauthenticated), errors.Is(err, shared.ErrForbidden):
			writeOauthError(w, http.StatusUnauthorized, "invalid_client")
		case errors.Is(err, shared.ErrUnavailable), errors.Is(err, shared.ErrInternal):
			WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		default:
			writeOauthError(w, http.StatusBadRequest, "invalid_grant")
		}
		return
	}
	expiresIn := int64(0)
	if at, ok := out["access_token_expires_at"].(time.Time); ok {
		expiresIn = max(int64(time.Until(at).Seconds()), 0)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  out.String("access_token"),
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
		"refresh_token": out.String("refresh_token"),
	})
}

// writeOauthError is RFC 6749 §5.2's error document.
func writeOauthError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

// oauthClientSummaryResponse is what a consent screen shows of a client: its name.
func oauthClientSummaryResponse(out usecase.Output) openapi.OauthClientSummary {
	return openapi.OauthClientSummary{Id: uuidValue(out.String("id")), Name: out.String("name")}
}

// oauthGrantResponse draws one consent a person gave a client.
func oauthGrantResponse(row usecase.Output) openapi.OauthGrant {
	return openapi.OauthGrant{
		Id:         uuidValue(row.String("id")),
		ClientId:   uuidValue(row.String("client_id")),
		ClientName: row.String("client_name"),
		Scopes:     scopeList(row["scopes"]),
		CreatedAt:  timeValue(row["created_at"]),
		LastUsedAt: optionalTimeField(row["last_used_at"]),
	}
}
