// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const appRedirect = "http://127.0.0.1:51734/oauth"

func appConsent(challenge string) AuthorizeOauthClientCommand {
	return AuthorizeOauthClientCommand{
		ClientID: domain.AppClientID, RedirectURI: appRedirect,
		Challenge: challenge, Method: "S256", State: "s1",
		Device: "LENA-LAPTOP", Platform: "windows",
	}
}

// The built-in client of the installed apps (identity.md §15.3, ADR-0096; UC-SYN-07 check 3): no
// registration, a consent naming the device, a session's scopes, the code exchanged with PKCE, the
// grant listed with its platform and revocable like any other.
func TestTheAppClientSignsInWithoutARegistration(t *testing.T) {
	fixture := newOauthFixture(now)
	verifier, challenge := pkcePair()

	minted, err := AuthorizeOauthClient{Writer: fixture.writer}.Execute(t.Context(), adminActor(), appConsent(challenge))
	if err != nil {
		t.Fatalf("consenting: %v", err)
	}

	var app domain.OauthClient
	for _, client := range fixture.clients.byID {
		app = client
	}
	if len(fixture.clients.byID) != 1 || !app.IsApp() || app.Name != "LENA-LAPTOP" ||
		app.AppPlatform != domain.AppWindows || app.ID == domain.AppClientID {
		t.Fatalf("the consent wrote %+v, want one row of its own named by the device", fixture.clients.byID)
	}

	pair, err := ExchangeOauthCode{Writer: fixture.writer}.Execute(t.Context(), ExchangeOauthCodeCommand{
		Code: minted.Code, RedirectURI: appRedirect, ClientID: domain.AppClientID, Verifier: verifier,
	})
	if err != nil || pair.RefreshToken.IsEmpty() {
		t.Fatalf("exchanging: %v", err)
	}

	// A session's scopes, whatever is consentable of them - never the control plane's.
	leashed := fixture.session.sessions.inserted[len(fixture.session.sessions.inserted)-1]
	want := []string{"items:read", "items:write", "containers:read"}
	if leashed.GrantID.IsZero() || !slices.Equal(leashed.Scopes, want) {
		t.Fatalf("the session carries %v, want %v", leashed.Scopes, want)
	}

	out, err := ListOauthGrants{Writer: fixture.writer}.Descriptor().Handler.Invoke(
		t.Context(), adminActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0]["app_platform"] != "windows" || rows[0]["client_name"] != "LENA-LAPTOP" {
		t.Fatalf("the grant lists as %v", rows)
	}

	grantID, _ := shared.ParseID(rows[0].String("id"))
	if err := (RevokeOauthGrant{Writer: fixture.writer}).Execute(t.Context(), adminActor(), grantID); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if fixture.grants.ended != 1 || !slices.Contains(fixture.grants.revoked, grantID) {
		t.Error("the app's grant was not withdrawn with its session")
	}
}

// Each consent is a grant of its own, so the same person's apps on two devices sign out one by one.
func TestTwoDevicesAreTwoGrants(t *testing.T) {
	fixture := newOauthFixture(now)
	_, challenge := pkcePair()
	for _, device := range []string{"LENA-LAPTOP", "Lena's phone"} {
		consent := appConsent(challenge)
		consent.Device = device
		if _, err := (AuthorizeOauthClient{Writer: fixture.writer}).Execute(t.Context(), adminActor(), consent); err != nil {
			t.Fatalf("consenting on %s: %v", device, err)
		}
	}
	if len(fixture.grants.byID) != 2 {
		t.Fatalf("%d grants, want one per device", len(fixture.grants.byID))
	}
}

// What the built-in client refuses at the consent: a redirect that is not its own, a method that is
// not S256, scopes it may not ask for, a device it does not name.
func TestTheAppClientRefusesAtTheConsent(t *testing.T) {
	_, challenge := pkcePair()
	cases := map[string]struct {
		change func(*AuthorizeOauthClientCommand)
		code   string
	}{
		"localhost":         {func(c *AuthorizeOauthClientCommand) { c.RedirectURI = "http://localhost:51734/oauth" }, "oauth.redirect_uri_mismatch"},
		"a web page":        {func(c *AuthorizeOauthClientCommand) { c.RedirectURI = "https://evil.test/oauth" }, "oauth.redirect_uri_mismatch"},
		"android loopback":  {func(c *AuthorizeOauthClientCommand) { c.Platform = "android" }, "oauth.redirect_uri_mismatch"},
		"plain":             {func(c *AuthorizeOauthClientCommand) { c.Method = "plain" }, "oauth.pkce_challenge_invalid"},
		"no method":         {func(c *AuthorizeOauthClientCommand) { c.Method = "" }, "oauth.pkce_challenge_invalid"},
		"asked scopes":      {func(c *AuthorizeOauthClientCommand) { c.Scopes = []string{"items:read"} }, "oauth.app_scopes_fixed"},
		"no device":         {func(c *AuthorizeOauthClientCommand) { c.Device = "" }, "oauth.app_device_required"},
		"an unknown system": {func(c *AuthorizeOauthClientCommand) { c.Platform = "macos" }, "oauth.app_platform_invalid"},
	}
	for name, each := range cases {
		fixture := newOauthFixture(now)
		consent := appConsent(challenge)
		each.change(&consent)
		_, err := AuthorizeOauthClient{Writer: fixture.writer}.Execute(t.Context(), adminActor(), consent)
		if err == nil || !strings.Contains(err.Error(), each.code) {
			t.Errorf("%s: %v, want %s", name, err, each.code)
		}
		if len(fixture.clients.byID) != 0 || len(fixture.grants.byID) != 0 {
			t.Errorf("%s: a refused consent wrote a row", name)
		}
	}
}

// Android's redirect is its own scheme, and nothing else.
func TestTheAndroidAppRedirectsToItsScheme(t *testing.T) {
	fixture := newOauthFixture(now)
	_, challenge := pkcePair()
	consent := appConsent(challenge)
	consent.Platform, consent.RedirectURI = "android", domain.AppRedirectAndroid
	if _, err := (AuthorizeOauthClient{Writer: fixture.writer}).Execute(t.Context(), adminActor(), consent); err != nil {
		t.Fatalf("consenting: %v", err)
	}
}

// The exchange: PKCE is the whole authentication, the redirect repeats, and a consent's own row is
// no client to present.
func TestTheAppClientsExchangeNeedsItsVerifier(t *testing.T) {
	verifier, challenge := pkcePair()
	cases := map[string]func(*ExchangeOauthCodeCommand, domain.OauthClient){
		"no verifier":      func(c *ExchangeOauthCodeCommand, _ domain.OauthClient) { c.Verifier = "" },
		"a wrong verifier": func(c *ExchangeOauthCodeCommand, _ domain.OauthClient) { c.Verifier = strings.Repeat("w", 64) },
		"another port": func(c *ExchangeOauthCodeCommand, _ domain.OauthClient) {
			c.RedirectURI = "http://127.0.0.1:51735/oauth"
		},
		"the row by its id": func(c *ExchangeOauthCodeCommand, row domain.OauthClient) { c.ClientID = row.ID },
	}
	for name, change := range cases {
		fixture := newOauthFixture(now)
		minted, err := AuthorizeOauthClient{Writer: fixture.writer}.Execute(t.Context(), adminActor(), appConsent(challenge))
		if err != nil {
			t.Fatalf("%s: consenting: %v", name, err)
		}
		var row domain.OauthClient
		for _, client := range fixture.clients.byID {
			row = client
		}
		exchange := ExchangeOauthCodeCommand{
			Code: minted.Code, RedirectURI: appRedirect, ClientID: domain.AppClientID, Verifier: verifier,
		}
		change(&exchange, row)
		if _, err := (ExchangeOauthCode{Writer: fixture.writer}).Execute(t.Context(), exchange); err == nil {
			t.Errorf("%s: the exchange answered a pair", name)
		}
	}
}

// A registered client's code is not the built-in client's to exchange.
func TestTheAppClientCannotExchangeARegisteredClientsCode(t *testing.T) {
	fixture := newOauthFixture(now)
	registered := register(t, fixture, false)
	verifier, challenge := pkcePair()
	minted := authorize(t, fixture, registered.Client.ID, challenge)
	_, err := ExchangeOauthCode{Writer: fixture.writer}.Execute(t.Context(), ExchangeOauthCodeCommand{
		Code: minted.Code, RedirectURI: "https://zapier.example/callback",
		ClientID: domain.AppClientID, Verifier: verifier,
	})
	if err == nil || !strings.Contains(err.Error(), "oauth.exchange_failed") {
		t.Fatalf("answered %v", err)
	}
}

// An app signing out withdraws its grant; a person's own session ending withdraws nothing.
func TestSigningAnAppOutWithdrawsItsGrant(t *testing.T) {
	fixture := newOauthFixture(now)
	fixture.session.sessions.revokeChanged = true
	grant := shared.ID("0192f000-0000-7000-8000-0000000000e1")
	client := shared.ID("0192f000-0000-7000-8000-0000000000e2")
	fixture.session.sessions.appGrants = map[shared.ID][2]shared.ID{sessionRowID: {grant, client}}

	if err := (RevokeSession{Writer: fixture.session.writer}).Execute(t.Context(), adminActor(), sessionRowID); err != nil {
		t.Fatalf("signing out: %v", err)
	}
	if len(fixture.session.sessions.appGrants) != 0 {
		t.Error("the app's grant was not withdrawn")
	}
	withdrawn := false
	for _, entry := range fixture.session.audit.entries {
		withdrawn = withdrawn || entry.Action == OauthGrantRevokedAction && entry.TargetID == grant
	}
	if !withdrawn {
		t.Errorf("audit %v, want the grant's withdrawal", fixture.session.audit.entries)
	}
}
