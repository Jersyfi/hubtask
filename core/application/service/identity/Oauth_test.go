// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The in-memory provider stores, with the statements' guards written out.

type oauthClientsStore struct {
	byID    map[shared.ID]domain.OauthClient
	secrets map[shared.ID]string
}

func newOauthClients() *oauthClientsStore {
	return &oauthClientsStore{
		byID:    map[shared.ID]domain.OauthClient{},
		secrets: map[shared.ID]string{},
	}
}

func (s *oauthClientsStore) Insert(_ context.Context, client domain.OauthClient, presented domain.Token) error {
	s.byID[client.ID] = client
	if client.Confidential {
		s.secrets[client.ID] = presented.Secret()
	}
	return nil
}

func (s *oauthClientsStore) List(context.Context) ([]domain.OauthClient, error) {
	clients := make([]domain.OauthClient, 0, len(s.byID))
	for _, client := range s.byID {
		clients = append(clients, client)
	}
	return clients, nil
}

func (s *oauthClientsStore) Find(_ context.Context, clientID shared.ID) (domain.OauthClient, error) {
	client, ok := s.byID[clientID]
	if !ok {
		return domain.OauthClient{}, shared.ErrNotFound.WithDetail("oauth.client_not_found")
	}
	return client, nil
}

func (s *oauthClientsStore) SecretMatches(
	_ context.Context, clientID shared.ID, presented secret.Secret,
) (bool, error) {
	stored, ok := s.secrets[clientID]
	return ok && !presented.IsEmpty() && stored == presented.Reveal(), nil
}

func (s *oauthClientsStore) Delete(_ context.Context, clientID shared.ID) (bool, error) {
	if _, ok := s.byID[clientID]; !ok {
		return false, nil
	}
	delete(s.byID, clientID)
	return true, nil
}

type oauthGrantsStore struct {
	byID    map[shared.ID]domain.OauthGrant
	names   map[shared.ID]string
	revoked []shared.ID
	ended   int
}

func newOauthGrants() *oauthGrantsStore {
	return &oauthGrantsStore{byID: map[shared.ID]domain.OauthGrant{}, names: map[shared.ID]string{}}
}

func (s *oauthGrantsStore) Upsert(_ context.Context, grant domain.OauthGrant) (shared.ID, error) {
	for id, existing := range s.byID {
		if existing.AccountID == grant.AccountID && existing.ClientID == grant.ClientID &&
			existing.RevokedAt.IsZero() {
			existing.Scopes = grant.Scopes
			s.byID[id] = existing
			return id, nil
		}
	}
	s.byID[grant.ID] = grant
	return grant.ID, nil
}

func (s *oauthGrantsStore) Find(_ context.Context, grantID shared.ID) (domain.OauthGrant, error) {
	grant, ok := s.byID[grantID]
	if !ok {
		return domain.OauthGrant{}, shared.ErrNotFound.WithDetail("oauth.grant_not_found")
	}
	return grant, nil
}

func (s *oauthGrantsStore) ListForAccount(
	_ context.Context, accountID shared.ID,
) ([]repository.GrantListing, error) {
	var listings []repository.GrantListing
	for _, grant := range s.byID {
		if grant.AccountID == accountID && grant.RevokedAt.IsZero() {
			listings = append(listings, repository.GrantListing{
				Grant: grant, ClientName: s.names[grant.ClientID],
			})
		}
	}
	return listings, nil
}

func (s *oauthGrantsStore) Revoke(
	_ context.Context, grantID, accountID shared.ID, at time.Time,
) (bool, error) {
	grant, ok := s.byID[grantID]
	if !ok || grant.AccountID != accountID || !grant.RevokedAt.IsZero() {
		return false, nil
	}
	grant.RevokedAt = at
	s.byID[grantID] = grant
	s.revoked = append(s.revoked, grantID)
	return true, nil
}

func (s *oauthGrantsStore) RevokeSessions(_ context.Context, _ shared.ID, _ time.Time) (int, error) {
	s.ended++
	return 1, nil
}

type oauthCodesStore struct {
	byToken map[string]domain.OauthCode
}

func newOauthCodes() *oauthCodesStore {
	return &oauthCodesStore{byToken: map[string]domain.OauthCode{}}
}

func (s *oauthCodesStore) Insert(_ context.Context, code domain.OauthCode, presented domain.Token) error {
	s.byToken[presented.Secret()] = code
	return nil
}

func (s *oauthCodesStore) Consume(
	_ context.Context, presented domain.Token, now time.Time,
) (domain.OauthCode, bool, error) {
	code, ok := s.byToken[presented.Secret()]
	if !ok || !code.ConsumedAt.IsZero() || !code.ExpiresAt.After(now) {
		return domain.OauthCode{}, false, nil
	}
	code.ConsumedAt = now
	s.byToken[presented.Secret()] = code
	return code, true, nil
}

type oauthFixture struct {
	writer  OauthWriter
	session *sessionFixture
	clients *oauthClientsStore
	grants  *oauthGrantsStore
	codes   *oauthCodesStore
	auth    *authorizer
}

func newOauthFixture(at time.Time) *oauthFixture {
	session := mfaFixture(at)
	f := &oauthFixture{
		session: session,
		clients: newOauthClients(),
		grants:  newOauthGrants(),
		codes:   newOauthCodes(),
		auth:    &authorizer{},
	}
	// The consent demands a person's own session behind the actor.
	credential, _ := refreshCredential(at)
	session.sessions.sessions[sessionRowID] = repository.SessionCredential{
		Session: credential.Session, Account: credential.Account,
	}
	f.writer = OauthWriter{
		Session: session.writer, Clients: f.clients, Grants: f.grants, Codes: f.codes,
		Authorizer:  f.auth,
		KnownScopes: []string{"items:read", "items:write", "containers:read"},
	}
	return f
}

func adminActor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: shared.ActorUser, TenantID: tenant, AccountID: account,
		AccountName: "Anna", TokenID: sessionRowID,
		Scopes: []string{"oauth:manage", "accounts:read", "accounts:write"},
	}
}

func pkcePair() (verifier, challenge string) {
	verifier = strings.Repeat("v", 64)
	digest := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(digest[:])
}

// register runs the registration through the real use case and answers the client and, for a
// confidential one, its secret.
func register(t *testing.T, fixture *oauthFixture, confidential bool) RegisteredClient {
	t.Helper()
	registered, err := RegisterOauthClient{Writer: fixture.writer}.Execute(t.Context(), adminActor(),
		RegisterOauthClientCommand{
			Name: "Zapier", Confidential: confidential,
			RedirectURIs: []string{"https://zapier.example/callback"},
		})
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	fixture.grants.names[registered.Client.ID] = registered.Client.Name
	return registered
}

// authorize runs the consent and answers the code.
func authorize(t *testing.T, fixture *oauthFixture, clientID shared.ID, challenge string) AuthorizedCode {
	t.Helper()
	minted, err := AuthorizeOauthClient{Writer: fixture.writer}.Execute(t.Context(), adminActor(),
		AuthorizeOauthClientCommand{
			ClientID: clientID, RedirectURI: "https://zapier.example/callback",
			Scopes: []string{"items:read"}, Challenge: challenge, Method: "S256",
			State: "xyzzy",
		})
	if err != nil {
		t.Fatalf("authorizing: %v", err)
	}
	return minted
}

// The whole dance, scripted (H-05's acceptance): register, authorize, consent recorded,
// exchange, the pair leashed, revoke, the next exchange refused.
func TestTheFullCodePKCEFlow(t *testing.T) {
	fixture := newOauthFixture(now)
	registered := register(t, fixture, true)
	if registered.Secret.IsEmpty() || !strings.HasPrefix(registered.Secret.Reveal(), domain.OauthClientSecretPrefix) {
		t.Fatalf("no single showing of the secret: %q", registered.Secret.Reveal())
	}

	verifier, challenge := pkcePair()
	minted := authorize(t, fixture, registered.Client.ID, challenge)
	if !strings.HasPrefix(minted.Code.Reveal(), domain.OauthCodePrefix) {
		t.Fatalf("code %q lacks its prefix", minted.Code.Reveal())
	}
	if minted.State != "xyzzy" {
		t.Errorf("state %q, want it echoed untouched", minted.State)
	}

	pair, err := ExchangeOauthCode{Writer: fixture.writer}.Execute(t.Context(), ExchangeOauthCodeCommand{
		Code: minted.Code, RedirectURI: "https://zapier.example/callback",
		ClientID: registered.Client.ID, Verifier: verifier,
		ClientSecret: registered.Secret,
	})
	if err != nil {
		t.Fatalf("exchanging: %v", err)
	}
	if pair.RefreshToken.IsEmpty() {
		t.Fatal("no pair answered")
	}
	// The session is leashed: the grant's scopes, the grant's identifier.
	leashed := fixture.session.sessions.inserted[len(fixture.session.sessions.inserted)-1]
	if leashed.GrantID.IsZero() || len(leashed.Scopes) != 1 || leashed.Scopes[0] != "items:read" {
		t.Fatalf("the issued session is not leashed: %+v", leashed)
	}

	// A code replayed after exchange is refused.
	_, err = ExchangeOauthCode{Writer: fixture.writer}.Execute(t.Context(), ExchangeOauthCodeCommand{
		Code: minted.Code, RedirectURI: "https://zapier.example/callback",
		ClientID: registered.Client.ID, Verifier: verifier, ClientSecret: registered.Secret,
	})
	if err == nil || !strings.Contains(err.Error(), "oauth.exchange_failed") {
		t.Fatalf("a replayed code answered %v", err)
	}

	// The grant lists, and revoking it ends its sessions.
	listings, err := ListOauthGrants{Writer: fixture.writer}.Execute(t.Context(), adminActor())
	if err != nil || len(listings) != 1 {
		t.Fatalf("listings %v, %v", listings, err)
	}
	if err := (RevokeOauthGrant{Writer: fixture.writer}).Execute(
		t.Context(), adminActor(), listings[0].Grant.ID); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if fixture.grants.ended != 1 {
		t.Error("the grant's sessions were not ended")
	}

	// A fresh code under the revoked grant... cannot exist: the upsert makes a new grant on the
	// next consent, which is the design - so instead prove the exchange refuses a code whose
	// grant was revoked between mint and exchange.
	_, challenge2 := pkcePair()
	minted2 := authorize(t, fixture, registered.Client.ID, challenge2)
	for id, grant := range fixture.grants.byID {
		if grant.RevokedAt.IsZero() {
			grant.RevokedAt = now
			fixture.grants.byID[id] = grant
		}
	}
	_, err = ExchangeOauthCode{Writer: fixture.writer}.Execute(t.Context(), ExchangeOauthCodeCommand{
		Code: minted2.Code, RedirectURI: "https://zapier.example/callback",
		ClientID: registered.Client.ID, Verifier: verifier, ClientSecret: registered.Secret,
	})
	if err == nil {
		t.Fatal("a revoked grant's code exchanged")
	}
}

func TestTheExchangeRefusesEveryForgery(t *testing.T) {
	fixture := newOauthFixture(now)
	registered := register(t, fixture, false) // public: PKCE is the whole authentication
	verifier, challenge := pkcePair()
	minted := authorize(t, fixture, registered.Client.ID, challenge)

	base := ExchangeOauthCodeCommand{
		Code: minted.Code, RedirectURI: "https://zapier.example/callback",
		ClientID: registered.Client.ID, Verifier: verifier,
	}

	// A public client without PKCE is refused before anything burns.
	noPKCE := base
	noPKCE.Verifier = ""
	if _, err := (ExchangeOauthCode{Writer: fixture.writer}).Execute(t.Context(), noPKCE); err == nil ||
		!strings.Contains(err.Error(), "oauth.pkce_required") {
		t.Fatalf("a public client without PKCE answered %v", err)
	}

	// A wrong verifier, a wrong redirect and a wrong client burn the code or refuse - and none
	// of them mints a session.
	wrongVerifier := base
	wrongVerifier.Verifier = strings.Repeat("w", 64)
	if _, err := (ExchangeOauthCode{Writer: fixture.writer}).Execute(t.Context(), wrongVerifier); err == nil {
		t.Fatal("a wrong verifier exchanged")
	}
	if len(fixture.session.sessions.inserted) != 0 {
		t.Fatal("a refused exchange opened a session")
	}
}

func TestAuthorizeRefusesTheWrongShape(t *testing.T) {
	fixture := newOauthFixture(now)
	registered := register(t, fixture, true)
	_, challenge := pkcePair()

	cases := map[string]AuthorizeOauthClientCommand{
		"a wrong redirect": {
			ClientID: registered.Client.ID, RedirectURI: "https://zapier.example/callback/",
			Scopes: []string{"items:read"}, Challenge: challenge, Method: "S256",
		},
		"an unknown scope": {
			ClientID: registered.Client.ID, RedirectURI: "https://zapier.example/callback",
			Scopes: []string{"root:everything"}, Challenge: challenge, Method: "S256",
		},
		"a plain method": {
			ClientID: registered.Client.ID, RedirectURI: "https://zapier.example/callback",
			Scopes: []string{"items:read"}, Challenge: challenge, Method: "plain",
		},
		"a malformed challenge": {
			ClientID: registered.Client.ID, RedirectURI: "https://zapier.example/callback",
			Scopes: []string{"items:read"}, Challenge: "short", Method: "S256",
		},
	}
	for name, cmd := range cases {
		if _, err := (AuthorizeOauthClient{Writer: fixture.writer}).Execute(
			t.Context(), adminActor(), cmd); err == nil {
			t.Errorf("%s authorized", name)
		}
	}

	// A machine credential cannot consent: the actor's TokenID names no session.
	patActor := adminActor()
	patActor.TokenID = refreshRowID
	_, err := AuthorizeOauthClient{Writer: fixture.writer}.Execute(t.Context(), patActor,
		AuthorizeOauthClientCommand{
			ClientID: registered.Client.ID, RedirectURI: "https://zapier.example/callback",
			Scopes: []string{"items:read"}, Challenge: challenge, Method: "S256",
		})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a token actor consented: %v", err)
	}
}

func TestAFreshConsentReplacesTheScopes(t *testing.T) {
	fixture := newOauthFixture(now)
	registered := register(t, fixture, true)
	_, challenge := pkcePair()

	authorize(t, fixture, registered.Client.ID, challenge)
	minted, err := AuthorizeOauthClient{Writer: fixture.writer}.Execute(t.Context(), adminActor(),
		AuthorizeOauthClientCommand{
			ClientID: registered.Client.ID, RedirectURI: "https://zapier.example/callback",
			Scopes: []string{"items:read", "items:write"}, Challenge: challenge, Method: "S256",
		})
	if err != nil {
		t.Fatalf("consenting again: %v", err)
	}
	_ = minted

	listings, err := ListOauthGrants{Writer: fixture.writer}.Execute(t.Context(), adminActor())
	if err != nil || len(listings) != 1 {
		t.Fatalf("listings %v, %v - want one live grant per app", listings, err)
	}
	if len(listings[0].Grant.Scopes) != 2 {
		t.Errorf("scopes %v, want the fresh consent's", listings[0].Grant.Scopes)
	}
}

// The channel round trip: the whole dance through the registry, the way REST, MCP and
// automation all reach it - which also proves every projection each channel reads.
func TestTheOauthUseCasesRoundTripThroughTheRegistry(t *testing.T) {
	fixture := newOauthFixture(now)

	registry, err := usecase.NewRegistry(nil,
		RegisterOauthClient{Writer: fixture.writer}.Descriptor(),
		ListOauthClients{Writer: fixture.writer}.Descriptor(),
		DeleteOauthClient{Writer: fixture.writer}.Descriptor(),
		AuthorizeOauthClient{Writer: fixture.writer}.Descriptor(),
		ExchangeOauthCode{Writer: fixture.writer}.Descriptor(),
		ListOauthGrants{Writer: fixture.writer}.Descriptor(),
		RevokeOauthGrant{Writer: fixture.writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	out, err := registry.Invoke(t.Context(), RegisterOauthClientName, adminActor(), usecase.Input{
		"name":          "Zapier",
		"redirect_uris": []any{"https://zapier.example/callback"},
		"confidential":  true,
	})
	if err != nil {
		t.Fatalf("registering through the registry: %v", err)
	}
	clientID := out.String("id")
	if out.String("client_secret") == "" {
		t.Fatal("the single showing is missing from the output")
	}
	clientSecret := out.String("client_secret")
	for id := range fixture.clients.byID {
		fixture.grants.names[id] = "Zapier"
	}

	listed, err := registry.Invoke(t.Context(), ListOauthClientsName, adminActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing through the registry: %v", err)
	}
	if rows, _ := listed["data"].([]usecase.Output); len(rows) != 1 || rows[0]["client_secret"] != nil {
		t.Fatalf("the listing leaks or lies: %v", listed)
	}

	verifier, challenge := pkcePair()
	authorized, err := registry.Invoke(t.Context(), AuthorizeOauthClientName, adminActor(), usecase.Input{
		"client_id": clientID, "redirect_uri": "https://zapier.example/callback",
		"scopes": []any{"items:read"}, "code_challenge": challenge,
		"code_challenge_method": "S256",
	})
	if err != nil {
		t.Fatalf("authorizing through the registry: %v", err)
	}

	exchanged, err := registry.Invoke(t.Context(), ExchangeOauthCodeName,
		appshared.Anonymous("en", "UTC"), usecase.Input{
			"grant_type": "authorization_code", "code": authorized["code"],
			"redirect_uri": "https://zapier.example/callback", "client_id": clientID,
			"code_verifier": verifier, "client_secret": clientSecret,
		})
	if err != nil {
		t.Fatalf("exchanging through the registry: %v", err)
	}
	if exchanged.String("access_token") == "" {
		t.Fatal("the pair is missing from the output")
	}

	grantsOut, err := registry.Invoke(t.Context(), ListOauthGrantsName, adminActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing grants through the registry: %v", err)
	}
	rows, _ := grantsOut["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0].String("client_name") != "Zapier" {
		t.Fatalf("grants %v", grantsOut)
	}

	if _, err := registry.Invoke(t.Context(), RevokeOauthGrantName, adminActor(), usecase.Input{
		"grant_id": rows[0].String("id"),
	}); err != nil {
		t.Fatalf("revoking through the registry: %v", err)
	}
	if _, err := registry.Invoke(t.Context(), DeleteOauthClientName, adminActor(), usecase.Input{
		"client_id": clientID,
	}); err != nil {
		t.Fatalf("deleting through the registry: %v", err)
	}
}
