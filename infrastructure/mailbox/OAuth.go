// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mailbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	port "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// OAuthTargetClass labels the token endpoint calls for the outbound duration histogram.
const OAuthTargetClass = "mail_oauth"

// OAuth is the token endpoint of any provider, spoken through the guarded client: the same two
// form posts for Google, Microsoft and whichever provider an administrator names.
type OAuth struct {
	Client httpport.Port
}

var _ port.Authorizer = OAuth{}

// Exchange trades the code the browser brought back, with the PKCE verifier that proves this
// installation asked for it.
func (o OAuth) Exchange(ctx context.Context, client domain.OAuthClient, clientSecret secret.Secret, code, verifier, redirectURL string) (port.Token, error) {
	return o.token(ctx, client, clientSecret, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURL},
		"code_verifier": {verifier},
	})
}

// Refresh draws an access token for one read.
func (o OAuth) Refresh(ctx context.Context, client domain.OAuthClient, clientSecret, refresh secret.Secret) (port.Token, error) {
	return o.token(ctx, client, clientSecret, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh.Reveal()},
	})
}

func (o OAuth) token(ctx context.Context, client domain.OAuthClient, clientSecret secret.Secret, form url.Values) (port.Token, error) {
	if o.Client == nil {
		return port.Token{}, shared.ErrUnavailable.WithDetail("mail.mailbox_unreachable").
			WithCause(errors.New("oauth: no outbound client is wired"))
	}
	// The client's credentials go in the body (RFC 6749 §2.3.1), which every provider takes;
	// a public client - one registered without a secret - sends its id alone.
	form.Set("client_id", client.ClientID)
	if !clientSecret.IsEmpty() {
		form.Set("client_secret", clientSecret.Reveal())
	}
	response, err := o.Client.Do(ctx, httpport.Request{
		Method: http.MethodPost, URL: client.TokenURL, TargetClass: OAuthTargetClass,
		Header: map[string][]string{
			"Content-Type": {"application/x-www-form-urlencoded"},
			"Accept":       {"application/json"},
		},
		Body: []byte(form.Encode()),
	})
	if err != nil {
		return port.Token{}, unreachable(err)
	}
	var answer struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		Error        string `json:"error"`
	}
	_ = json.Unmarshal(response.Body, &answer)
	if response.Status != http.StatusOK || answer.AccessToken == "" {
		// The provider's own words stay out: a revoked grant is a code an administrator can act
		// on, everything else is a refusal.
		detail := "mail.oauth_refused"
		if answer.Error == "invalid_grant" && form.Get("grant_type") == "refresh_token" {
			detail = "mail.oauth_revoked"
		}
		if response.Status >= 500 {
			detail = "mail.mailbox_unreachable"
		}
		return port.Token{}, shared.ErrForbidden.WithDetail(detail).
			WithCause(errors.New("oauth: the token endpoint answered " + http.StatusText(response.Status) + " " + answer.Error))
	}
	return port.Token{
		Access: secret.New(answer.AccessToken), Refresh: secret.New(answer.RefreshToken),
		Email: emailOf(answer.IDToken),
	}, nil
}

// emailOf reads the address out of an ID token. The token came straight from the token endpoint
// over TLS to this server, which OpenID Connect Core §3.1.3.7 lets stand for the signature check;
// and the address is a hint only - the IMAP sign-in that follows proves the token is that
// mailbox's, or fails.
func emailOf(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	// Microsoft names a personal or work account's address in preferred_username when the
	// email claim is empty.
	for _, candidate := range []string{claims.Email, claims.PreferredUsername} {
		if candidate = strings.ToLower(strings.TrimSpace(candidate)); strings.Contains(candidate, "@") {
			return candidate
		}
	}
	return ""
}
