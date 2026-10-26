// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"
	"time"

	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	mailboxport "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

type waiting struct {
	pending      domain.PendingAuthorization
	verifier     crypto.Sealed
	clientSecret *crypto.Sealed
}

type authorizations struct{ byHash map[string]waiting }

func (a *authorizations) Insert(_ context.Context, pending domain.PendingAuthorization, hash []byte, verifier crypto.Sealed, clientSecret *crypto.Sealed) error {
	a.byHash[string(hash)] = waiting{pending, verifier, clientSecret}
	return nil
}
func (a *authorizations) Take(_ context.Context, hash []byte) (domain.PendingAuthorization, crypto.Sealed, *crypto.Sealed, error) {
	found, ok := a.byHash[string(hash)]
	if !ok {
		return domain.PendingAuthorization{}, crypto.Sealed{}, nil, shared.ErrNotFound.WithDetail("mail.oauth_state_unknown")
	}
	delete(a.byHash, string(hash))
	return found.pending, found.verifier, found.clientSecret, nil
}
func (a *authorizations) DeleteExpired(_ context.Context, now time.Time) (int, error) {
	gone := 0
	for hash, each := range a.byHash {
		if each.pending.Expired(now) {
			delete(a.byHash, hash)
			gone++
		}
	}
	return gone, nil
}

// provider is a token endpoint: it takes one code with its verifier, and refreshes.
type provider struct {
	code, verifier string
	email          string
	rotate         string
	revoked        bool
	gotSecret      []string
}

func (p *provider) Exchange(_ context.Context, _ domain.OAuthClient, clientSecret secret.Secret, code, verifier, _ string) (mailboxport.Token, error) {
	p.gotSecret = append(p.gotSecret, clientSecret.Reveal())
	if code != p.code || verifier != p.verifier {
		return mailboxport.Token{}, shared.ErrForbidden.WithDetail("mail.oauth_refused")
	}
	return mailboxport.Token{Access: secret.New("app-pw"), Refresh: secret.New("refresh-1"), Email: p.email}, nil
}
func (p *provider) Refresh(_ context.Context, _ domain.OAuthClient, clientSecret, refresh secret.Secret) (mailboxport.Token, error) {
	p.gotSecret = append(p.gotSecret, clientSecret.Reveal())
	if p.revoked {
		return mailboxport.Token{}, shared.ErrForbidden.WithDetail("mail.oauth_revoked")
	}
	return mailboxport.Token{Access: secret.New("app-pw"), Refresh: secret.New(p.rotate)}, nil
}

func oauthBoxes(apps map[string]App) (Boxes, *mailboxStore, *authorizations, *provider) {
	b, store, _, _ := boxes(&authorizer{})
	pending := &authorizations{byHash: map[string]waiting{}}
	tokens := &provider{code: "code-1", email: "familie@gmail.com"}
	b.OAuth = OAuth{
		Authorizations: pending, Authorizer: tokens, Entropy: clock.FixedEntropy{Seed: 7}, Apps: apps,
		RedirectURL: "https://hub.example/administration/mail/oauth",
	}
	return b, store, pending, tokens
}

// authorize starts a connection and answers the state and the verifier the provider would see.
func authorize(t *testing.T, b Boxes, actor appshared.ActorContext, in usecase.Input) (string, string) {
	t.Helper()
	out, err := AuthorizeMailbox{Boxes: b}.Descriptor().Handler.Invoke(context.Background(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(out["authorize_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Errorf("no PKCE in %s", parsed)
	}
	// The fixed entropy draws the same bytes for the state and the verifier.
	return query.Get("state"), query.Get("state")
}

func TestGoogleConnectsWithTheInstallationsAppAndKeepsOnlyTheRefreshToken(t *testing.T) {
	b, store, pending, tokens := oauthBoxes(map[string]App{"google": {ClientID: "inst-id", ClientSecret: secret.New("inst-secret")}})
	state, verifier := authorize(t, b, person, usecase.Input{"provider": "google", "address": "familie@gmail.com"})
	tokens.verifier = verifier
	if len(pending.byHash) != 1 {
		t.Fatalf("waiting %d", len(pending.byHash))
	}
	for hash := range pending.byHash {
		if hash == state {
			t.Error("the state was stored as it is, not hashed")
		}
	}
	out, err := CompleteMailboxAuthorization{Boxes: b}.Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"state": state, "code": "code-1"})
	if err != nil {
		t.Fatal(err)
	}
	if out["auth"] != "OAUTH" || out["oauth_provider"] != "google" || out["host"] != "imap.gmail.com" || out["address"] != "familie@gmail.com" {
		t.Errorf("connected %+v", out)
	}
	var kept oauthSecret
	_ = json.Unmarshal(store.sealed[boxID].Ciphertext, &kept)
	if kept.RefreshToken != "refresh-1" || kept.ClientSecret != "" {
		t.Errorf("kept %+v: the installation's secret stays in the configuration", kept)
	}
	if tokens.gotSecret[0] != "inst-secret" {
		t.Errorf("exchanged with %q", tokens.gotSecret[0])
	}
	if len(pending.byHash) != 0 {
		t.Error("the connection stayed after it was used")
	}
	if _, err := (CompleteMailboxAuthorization{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"state": state, "code": "code-1"}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a state used twice answered %v", err)
	}
}

func TestAnyProviderConnectsThroughAnAppTheAdministratorEntered(t *testing.T) {
	b, store, _, tokens := oauthBoxes(nil)
	tokens.email = ""
	state, verifier := authorize(t, b, person, usecase.Input{
		"provider": "custom", "authorize_url": "https://id.example.org/authorize", "token_url": "https://id.example.org/token",
		"scopes": []any{"mail"}, "client_id": "own-id", "client_secret": "own-secret", "host": "imap.example.org",
		"address": "team@example.org",
	})
	tokens.verifier = verifier
	out, err := CompleteMailboxAuthorization{Boxes: b}.Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"state": state, "code": "code-1"})
	if err != nil {
		t.Fatal(err)
	}
	if out["address"] != "team@example.org" || out["host"] != "imap.example.org" || out["oauth_provider"] != "custom" {
		t.Errorf("connected %+v", out)
	}
	var kept oauthSecret
	_ = json.Unmarshal(store.sealed[boxID].Ciphertext, &kept)
	if kept.ClientSecret != "own-secret" || tokens.gotSecret[0] != "own-secret" {
		t.Errorf("kept %+v, exchanged with %v", kept, tokens.gotSecret)
	}
	if _, err := (AuthorizeMailbox{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"provider": "custom", "authorize_url": "http://id.example.org/a", "token_url": "https://id.example.org/t", "client_id": "x", "host": "imap.example.org"}); shared.AsError(err).DetailCode != "mail.oauth_invalid" {
		t.Errorf("an endpoint without TLS answered %v", err)
	}
	if _, err := (AuthorizeMailbox{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"provider": "microsoft"}); shared.AsError(err).DetailCode != "mail.oauth_invalid" {
		t.Errorf("Microsoft without the installation's app or one's own answered %v", err)
	}
}

func TestAStateIsBoundToWhoeverStartedItAndToTenMinutes(t *testing.T) {
	b, store, _, tokens := oauthBoxes(map[string]App{"google": {ClientID: "inst-id", ClientSecret: secret.New("s")}})
	state, verifier := authorize(t, b, person, usecase.Input{"provider": "google"})
	tokens.verifier = verifier
	other := person
	other.AccountID = shared.ID("01936f2a-7c1e-7000-8000-0000000000ff")
	if _, err := (CompleteMailboxAuthorization{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), other,
		usecase.Input{"state": state, "code": "code-1"}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("someone else's state answered %v", err)
	}
	state, _ = authorize(t, b, person, usecase.Input{"provider": "google"})
	late := b
	late.Writer.Clock = clock.Fixed(fixed.Add(domain.PendingTTL))
	if _, err := (CompleteMailboxAuthorization{Boxes: late}).Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"state": state, "code": "code-1"}); shared.AsError(err).DetailCode != "mail.oauth_expired" {
		t.Errorf("a late answer answered %v", err)
	}
	state, _ = authorize(t, b, person, usecase.Input{"provider": "google"})
	if _, err := (CompleteMailboxAuthorization{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"state": state, "error": "access_denied"}); shared.AsError(err).DetailCode != "mail.oauth_denied" {
		t.Errorf("a refusal at the provider answered %v", err)
	}
	if len(store.boxes) != 0 {
		t.Error("a mailbox was stored")
	}
}

func TestAReadRefreshesTheTokenAndKeepsARotatedOne(t *testing.T) {
	b, store, _, tokens := oauthBoxes(map[string]App{"google": {ClientID: "inst-id", ClientSecret: secret.New("s")}})
	state, verifier := authorize(t, b, person, usecase.Input{"provider": "google"})
	tokens.verifier = verifier
	if _, err := (CompleteMailboxAuthorization{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"state": state, "code": "code-1"}); err != nil {
		t.Fatal(err)
	}
	tokens.rotate = "refresh-2"
	poll := PollMailbox{Boxes: b, Receive: &receiver{}, Parse: func([]byte, string) (jumbleservice.ReceivedMail, error) {
		return jumbleservice.ReceivedMail{}, nil
	}}
	job := queue.Job{TenantID: tenant, Payload: map[string]any{"mailbox_id": boxID.String()}}
	if result, err := poll.Run(context.Background(), job); err != nil || !result.Repeat {
		t.Fatalf("read %+v, %v", result, err)
	}
	var kept oauthSecret
	_ = json.Unmarshal(store.sealed[boxID].Ciphertext, &kept)
	if kept.RefreshToken != "refresh-2" {
		t.Errorf("kept %q after a rotation", kept.RefreshToken)
	}
	tokens.revoked = true
	if result, err := poll.Run(context.Background(), job); err != nil || !result.Repeat {
		t.Fatalf("read %+v, %v", result, err)
	}
	if store.boxes[boxID].Failure != "mail.oauth_revoked" {
		t.Errorf("a revoked grant recorded %q", store.boxes[boxID].Failure)
	}
}
