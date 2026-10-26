// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestAPresetFillsTheEndpointsAndAnyOtherProviderNamesItsOwn(t *testing.T) {
	client, host, port, err := NewOAuthClient(OAuthRequest{Provider: "microsoft", Installation: true, Host: "ignored.example"})
	if err != nil {
		t.Fatal(err)
	}
	if host != "outlook.office365.com" || port != 993 || client.TokenURL != "https://login.microsoftonline.com/common/oauth2/v2.0/token" {
		t.Errorf("microsoft %+v %s %d", client, host, port)
	}
	client, host, _, err = NewOAuthClient(OAuthRequest{
		Provider: "zoho", AuthorizeURL: "https://accounts.zoho.eu/oauth/v2/auth", TokenURL: "https://accounts.zoho.eu/oauth/v2/token",
		Scopes: []string{" ZohoMail.messages.ALL ", ""}, ClientID: "id", Host: "IMAP.Zoho.eu",
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.Provider != "custom" || host != "imap.zoho.eu" || len(client.Scopes) != 1 {
		t.Errorf("custom %+v %s", client, host)
	}
	_, _, _, err = NewOAuthClient(OAuthRequest{Provider: "custom", AuthorizeURL: "http://a.example", TokenURL: "", Host: "x"})
	fields := map[string]bool{}
	for _, field := range shared.AsError(err).Fields {
		fields[field.Path] = true
	}
	for _, path := range []string{"/authorize_url", "/token_url", "/scopes", "/client_id", "/host"} {
		if !fields[path] {
			t.Errorf("%s was not named: %v", path, err)
		}
	}
}

func TestTheAuthorizationURLCarriesPKCEAndWhatTheProviderNeedsForARefreshToken(t *testing.T) {
	google, _ := OAuthPresetNamed("google")
	client := OAuthClient{Provider: "google", AuthorizeURL: google.AuthorizeURL, Scopes: google.Scopes, ClientID: "id"}
	parsed, err := url.Parse(client.AuthorizationURL("https://hub.example/administration/mail/oauth", "st", "verifier", "familie@gmail.com"))
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	sum := sha256.Sum256([]byte("verifier"))
	for key, want := range map[string]string{
		"response_type": "code", "client_id": "id", "state": "st", "code_challenge_method": "S256",
		"code_challenge": base64.RawURLEncoding.EncodeToString(sum[:]), "access_type": "offline", "prompt": "consent",
		"login_hint": "familie@gmail.com", "scope": "https://mail.google.com/ openid email",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if query.Has("code_verifier") {
		t.Error("the verifier left the server")
	}
	if !(PendingAuthorization{ExpiresAt: time.Unix(10, 0)}).Expired(time.Unix(10, 0)) {
		t.Error("an authorization at its expiry was still good")
	}
}
