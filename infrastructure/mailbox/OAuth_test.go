// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mailbox

import (
	"context"
	"encoding/base64"
	"net/url"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// tokenEndpoint answers as a provider's token endpoint does.
type tokenEndpoint struct {
	status int
	body   string
	sent   url.Values
	to     string
}

func (e *tokenEndpoint) Do(_ context.Context, req httpport.Request) (httpport.Response, error) {
	e.sent, _ = url.ParseQuery(string(req.Body))
	e.to = req.URL
	return httpport.Response{Status: e.status, Body: []byte(e.body)}, nil
}

var googleClient = domain.OAuthClient{
	Provider: "google", TokenURL: "https://oauth2.googleapis.com/token", ClientID: "client-1",
}

func idToken(claims string) string {
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".sig"
}

func TestACodeIsTradedWithItsVerifierAndTheAddressReadFromTheIDToken(t *testing.T) {
	endpoint := &tokenEndpoint{status: 200, body: `{"access_token":"a1","refresh_token":"r1","id_token":"` +
		idToken(`{"email":"Familie@Gmail.com"}`) + `"}`}
	token, err := OAuth{Client: endpoint}.Exchange(context.Background(), googleClient, secret.New("s3"), "code-1", "verifier-1", "https://hub.example/administration/mail/oauth")
	if err != nil {
		t.Fatal(err)
	}
	if token.Access.Reveal() != "a1" || token.Refresh.Reveal() != "r1" || token.Email != "familie@gmail.com" {
		t.Errorf("token %+v email %q", token, token.Email)
	}
	for key, want := range map[string]string{
		"grant_type": "authorization_code", "code": "code-1", "code_verifier": "verifier-1",
		"client_id": "client-1", "client_secret": "s3", "redirect_uri": "https://hub.example/administration/mail/oauth",
	} {
		if got := endpoint.sent.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if endpoint.to != googleClient.TokenURL {
		t.Errorf("posted to %s", endpoint.to)
	}
}

func TestMicrosoftsAddressComesFromPreferredUsernameAndAPublicClientSendsNoSecret(t *testing.T) {
	endpoint := &tokenEndpoint{status: 200, body: `{"access_token":"a1","id_token":"` + idToken(`{"preferred_username":"team@contoso.com"}`) + `"}`}
	token, err := OAuth{Client: endpoint}.Refresh(context.Background(), googleClient, secret.Secret{}, secret.New("r0"))
	if err != nil {
		t.Fatal(err)
	}
	if token.Email != "team@contoso.com" || !token.Refresh.IsEmpty() {
		t.Errorf("token %+v", token)
	}
	if endpoint.sent.Has("client_secret") || endpoint.sent.Get("refresh_token") != "r0" {
		t.Errorf("sent %v", endpoint.sent)
	}
}

func TestARevokedGrantIsSaidApartFromARefusal(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
		run    func(OAuth) error
	}{
		{400, `{"error":"invalid_grant"}`, "mail.oauth_revoked", func(o OAuth) error {
			_, err := o.Refresh(context.Background(), googleClient, secret.Secret{}, secret.New("r0"))
			return err
		}},
		{400, `{"error":"invalid_grant"}`, "mail.oauth_refused", func(o OAuth) error {
			_, err := o.Exchange(context.Background(), googleClient, secret.Secret{}, "c", "v", "https://x")
			return err
		}},
		{503, ``, "mail.mailbox_unreachable", func(o OAuth) error {
			_, err := o.Refresh(context.Background(), googleClient, secret.Secret{}, secret.New("r0"))
			return err
		}},
	}
	for _, c := range cases {
		err := c.run(OAuth{Client: &tokenEndpoint{status: c.status, body: c.body}})
		if shared.AsError(err).DetailCode != c.want {
			t.Errorf("%d %s answered %v, want %s", c.status, c.body, err, c.want)
		}
	}
	if emailOf("not-a-token") != "" || emailOf(idToken(`{"email":"no-at"}`)) != "" {
		t.Error("an address was read out of nothing")
	}
}
