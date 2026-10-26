// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	mailboxport "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	AuthorizeMailboxName = "AuthorizeMailbox"
	CompleteMailboxName  = "CompleteMailboxAuthorization"

	mailboxAuthorizationStarted = audit.Action("mail.mailbox_authorization_started")

	// stateBytes and verifierBytes are 256 bits each: the state finds the connection, the
	// verifier proves to the provider that whoever trades the code is who asked for it.
	stateBytes    = 32
	verifierBytes = 32
)

// App is an OAuth client the installation registered at a provider, offered to every workspace so
// that connecting is a button and no administrator has to register one.
type App struct {
	ClientID     string
	ClientSecret secret.Secret
}

// OAuth is what connecting a mailbox through its provider's sign-in needs beside the mailboxes.
type OAuth struct {
	Authorizations Authorizations
	Authorizer     mailboxport.Authorizer
	Entropy        Entropy
	// Apps are the installation's own apps by provider key; a provider without one is connected
	// through an app the administrator registers.
	Apps map[string]App
	// RedirectURL is where the provider sends the browser back: the web UI's page that hands the
	// code and the state to CompleteMailboxAuthorization.
	RedirectURL string
}

// Authorizations is the repository slice the service uses, named here so a test fakes it alone.
type Authorizations interface {
	Insert(ctx context.Context, pending domain.PendingAuthorization, stateHash []byte, verifier crypto.Sealed, clientSecret *crypto.Sealed) error
	Take(ctx context.Context, stateHash []byte) (domain.PendingAuthorization, crypto.Sealed, *crypto.Sealed, error)
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}

// Entropy draws unguessable bytes.
type Entropy interface {
	Bytes(n int) ([]byte, error)
}

// AuthorizeMailbox starts connecting a mailbox through its provider: it remembers who asked and
// for what, and answers the provider's page to send the browser to.
type AuthorizeMailbox struct{ Boxes Boxes }

// CompleteMailboxAuthorization finishes it with what the provider sent back: trades the code,
// signs in once with the token, and stores the mailbox.
type CompleteMailboxAuthorization struct{ Boxes Boxes }

// oauthSecret is what an OAuth mailbox keeps sealed: the refresh token, and the client secret of an
// app the administrator entered (the installation's stays in the configuration).
type oauthSecret struct {
	RefreshToken string `json:"refresh_token"`
	ClientSecret string `json:"client_secret,omitempty"`
}

func verifierPurpose(id shared.ID) crypto.Purpose {
	return crypto.Purpose("mailbox.oauth_verifier:" + id.String())
}

func clientSecretPurpose(id shared.ID) crypto.Purpose {
	return crypto.Purpose("mailbox.oauth_client:" + id.String())
}

func stateHash(state string) []byte {
	sum := sha256.Sum256([]byte(state))
	return sum[:]
}

func (b Boxes) draw(n int) (string, error) {
	drawn, err := b.OAuth.Entropy.Bytes(n)
	if err != nil {
		return "", shared.ErrInternal.WithDetail("mail.oauth_unavailable").WithCause(err)
	}
	return base64.RawURLEncoding.EncodeToString(drawn), nil
}

// oauthReady reports whether this installation can connect through a provider at all.
func (b Boxes) oauthReady() error {
	if b.OAuth.Authorizer == nil || b.OAuth.Authorizations == nil || b.OAuth.Entropy == nil || b.OAuth.RedirectURL == "" {
		return shared.ErrUnavailable.WithDetail("mail.oauth_unavailable")
	}
	return nil
}

func (h AuthorizeMailbox) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	b := h.Boxes
	if err := b.Writer.authorizeIntake(ctx, actor, mailboxConnected); err != nil {
		return nil, err
	}
	if err := b.oauthReady(); err != nil {
		return nil, err
	}
	scopes, err := in.StringList("scopes")
	if err != nil {
		return nil, err
	}
	request := domain.OAuthRequest{
		Provider: in.String("provider"), AuthorizeURL: in.String("authorize_url"), TokenURL: in.String("token_url"),
		Scopes: scopes, ClientID: in.String("client_id"), Host: in.String("host"), Port: in.Int("port"),
	}
	clientSecret := secret.New(in.String("client_secret"))
	// The installation's app, where there is one and the administrator brought none of their own.
	app, installed := b.OAuth.Apps[request.Provider]
	if installed && request.ClientID == "" {
		request.ClientID, request.Installation = app.ClientID, true
		clientSecret = secret.Secret{}
	}
	client, host, port, err := domain.NewOAuthClient(request)
	if err != nil {
		return nil, err
	}
	now := b.Writer.Clock.Now()
	pending := domain.PendingAuthorization{
		ID: b.IDs.NewID(), TenantID: actor.TenantID, AccountID: actor.AccountID, Client: client,
		Host: host, Port: port, LoginHint: in.String("address"), ExpiresAt: now.Add(domain.PendingTTL),
	}
	state, err := b.draw(stateBytes)
	if err != nil {
		return nil, err
	}
	verifier, err := b.draw(verifierBytes)
	if err != nil {
		return nil, err
	}
	sealedVerifier, err := b.Encryptor.Seal(ctx, secret.New(verifier), verifierPurpose(pending.ID))
	if err != nil {
		return nil, err
	}
	var sealedClientSecret *crypto.Sealed
	if !clientSecret.IsEmpty() {
		sealed, err := b.Encryptor.Seal(ctx, clientSecret, clientSecretPurpose(pending.ID))
		if err != nil {
			return nil, err
		}
		sealedClientSecret = &sealed
	}
	err = b.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// Connections nobody came back to are forgotten here, by the next one of the same
		// workspace, rather than by a sweep across tenants.
		if _, err := b.OAuth.Authorizations.DeleteExpired(ctx, now); err != nil {
			return err
		}
		count, err := b.Mailboxes.Count(ctx)
		if err != nil {
			return err
		}
		if count >= domain.MaxMailboxes {
			return shared.ErrConflict.WithDetail("mail.mailboxes_full")
		}
		return b.OAuth.Authorizations.Insert(ctx, pending, stateHash(state), sealedVerifier, sealedClientSecret)
	})
	if err != nil {
		return nil, err
	}
	if b.Writer.Audit != nil {
		_ = b.Writer.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now, Action: mailboxAuthorizationStarted,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: mailboxTarget, TargetID: pending.ID,
			Changes: audit.Changes(
				audit.Change{Field: "provider", Classification: audit.Open, To: client.Provider},
				audit.Change{Field: "host", Classification: audit.Open, To: host},
			),
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	}
	return usecase.Output{
		"authorize_url": client.AuthorizationURL(b.OAuth.RedirectURL, state, verifier, pending.LoginHint),
		"expires_at":    pending.ExpiresAt,
	}, nil
}

func (h CompleteMailboxAuthorization) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	b := h.Boxes
	if err := b.Writer.authorizeIntake(ctx, actor, mailboxConnected); err != nil {
		return nil, err
	}
	if err := b.oauthReady(); err != nil {
		return nil, err
	}
	state := in.String("state")
	if state == "" {
		return nil, shared.ErrValidation.WithDetail("mail.oauth_state_unknown")
	}
	var (
		pending            domain.PendingAuthorization
		sealedVerifier     crypto.Sealed
		sealedClientSecret *crypto.Sealed
	)
	// Taken - and so gone - before anything else: a state is good for one try, whatever it brings.
	err := b.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		pending, sealedVerifier, sealedClientSecret, err = b.OAuth.Authorizations.Take(ctx, stateHash(state))
		return err
	})
	if err != nil {
		return nil, err
	}
	now := b.Writer.Clock.Now()
	// Bound to whoever started it: a link with someone else's state does not connect a mailbox
	// in their name.
	if pending.AccountID != actor.AccountID {
		return nil, shared.ErrNotFound.WithDetail("mail.oauth_state_unknown")
	}
	if pending.Expired(now) {
		return nil, shared.ErrValidation.WithDetail("mail.oauth_expired")
	}
	if in.String("error") != "" {
		// The administrator said no at the provider, or the provider refused the app.
		return nil, shared.ErrValidation.WithDetail("mail.oauth_denied")
	}
	code := in.String("code")
	if code == "" {
		return nil, shared.ErrValidation.WithDetail("mail.oauth_denied")
	}
	verifier, err := b.Encryptor.Open(ctx, sealedVerifier, verifierPurpose(pending.ID))
	if err != nil {
		return nil, err
	}
	clientSecret, err := b.pendingClientSecret(ctx, pending, sealedClientSecret)
	if err != nil {
		return nil, err
	}
	exchangeCtx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	token, err := b.OAuth.Authorizer.Exchange(exchangeCtx, pending.Client, clientSecret, code, verifier.Reveal(), b.OAuth.RedirectURL)
	if err != nil {
		return nil, err
	}
	if token.Refresh.IsEmpty() {
		// Without a refresh token the mailbox could be read for an hour and never again.
		return nil, shared.ErrValidation.WithDetail("mail.oauth_no_refresh")
	}
	address := token.Email
	if address == "" {
		address = pending.LoginHint
	}
	preset := ""
	if pending.Client.Provider == "google" {
		// Gmail's plus addresses route; the preset says so.
		preset = "gmail"
	}
	box, err := domain.NewMailbox(domain.NewMailboxInput{
		ID: b.IDs.NewID(), TenantID: actor.TenantID, Preset: preset, Protocol: domain.ProtocolIMAP,
		Address: address, Host: pending.Host, Port: pending.Port, Now: now,
	})
	if err != nil {
		return nil, shared.ErrValidation.WithDetail("mail.oauth_address_unknown").WithCause(err)
	}
	client := pending.Client
	box.Auth, box.OAuth = domain.AuthOAuth, &client
	if err := b.probe(ctx, box, token.Access); err != nil {
		return nil, err
	}
	box = box.Checked(now, "")
	kept := oauthSecret{RefreshToken: token.Refresh.Reveal()}
	if !client.Installation {
		kept.ClientSecret = clientSecret.Reveal()
	}
	sealed, err := b.sealOAuth(ctx, box.ID, kept)
	if err != nil {
		return nil, err
	}
	err = b.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		count, err := b.Mailboxes.Count(ctx)
		if err != nil {
			return err
		}
		if count >= domain.MaxMailboxes {
			return shared.ErrConflict.WithDetail("mail.mailboxes_full")
		}
		if err := b.Mailboxes.Insert(ctx, box, sealed, actor.AccountID); err != nil {
			return err
		}
		return b.Mailboxes.RecordCheck(ctx, box)
	})
	if err != nil {
		return nil, err
	}
	if err := b.seed(ctx, box, now); err != nil {
		return nil, err
	}
	b.record(ctx, actor, mailboxConnected, box)
	return mailboxOutput(box, now), nil
}

// pendingClientSecret is the client secret for a waiting connection: the installation's, the one
// the administrator entered, or none for a public client.
func (b Boxes) pendingClientSecret(ctx context.Context, pending domain.PendingAuthorization, sealed *crypto.Sealed) (secret.Secret, error) {
	if pending.Client.Installation {
		app, ok := b.OAuth.Apps[pending.Client.Provider]
		if !ok {
			return secret.Secret{}, shared.ErrUnavailable.WithDetail("mail.oauth_app_missing")
		}
		return app.ClientSecret, nil
	}
	if sealed == nil {
		return secret.Secret{}, nil
	}
	return b.Encryptor.Open(ctx, *sealed, clientSecretPurpose(pending.ID))
}

func (b Boxes) sealOAuth(ctx context.Context, id shared.ID, kept oauthSecret) (crypto.Sealed, error) {
	plain, err := json.Marshal(kept) //nolint:gosec // G117: marshalled only to be sealed, never written out
	if err != nil {
		return crypto.Sealed{}, err
	}
	return b.Encryptor.Seal(ctx, secret.New(string(plain)), secretPurpose(id))
}

// credential is what a mailbox signs in with for one read: the password as it is, or for OAuth an
// access token drawn from the refresh token now. A provider that rotates the refresh token gets
// the new one sealed in place of the old before the read goes on.
func (b Boxes) credential(ctx context.Context, scope persistence.Scope, box domain.Mailbox, opened secret.Secret) (secret.Secret, error) {
	if box.Auth != domain.AuthOAuth || box.OAuth == nil {
		return opened, nil
	}
	if b.OAuth.Authorizer == nil {
		return secret.Secret{}, shared.ErrUnavailable.WithDetail("mail.oauth_unavailable")
	}
	var kept oauthSecret
	if err := json.Unmarshal([]byte(opened.Reveal()), &kept); err != nil || kept.RefreshToken == "" {
		return secret.Secret{}, shared.ErrInternal.WithDetail("mail.oauth_revoked").WithCause(errors.New("mail: an OAuth mailbox's secret is unreadable"))
	}
	clientSecret := secret.New(kept.ClientSecret)
	if box.OAuth.Installation {
		app, ok := b.OAuth.Apps[box.OAuth.Provider]
		if !ok {
			// The operator took the app out of the configuration.
			return secret.Secret{}, shared.ErrUnavailable.WithDetail("mail.oauth_app_missing")
		}
		clientSecret = app.ClientSecret
	}
	refreshCtx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	token, err := b.OAuth.Authorizer.Refresh(refreshCtx, *box.OAuth, clientSecret, secret.New(kept.RefreshToken))
	if err != nil {
		return secret.Secret{}, err
	}
	if rotated := token.Refresh.Reveal(); rotated != "" && rotated != kept.RefreshToken {
		kept.RefreshToken = rotated
		sealed, err := b.sealOAuth(ctx, box.ID, kept)
		if err != nil {
			return secret.Secret{}, err
		}
		if err := b.Writer.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
			return b.Mailboxes.Reseal(ctx, box.ID, sealed)
		}); err != nil {
			return secret.Secret{}, err
		}
	}
	return token.Access, nil
}

// oauthProviders are the providers offered, and whether the installation brings an app for each.
func (b Boxes) oauthProviders() []any {
	if b.oauthReady() != nil {
		return []any{}
	}
	out := make([]any, 0, len(domain.OAuthPresets))
	for _, preset := range domain.OAuthPresets {
		_, installed := b.OAuth.Apps[preset.Key]
		out = append(out, map[string]any{"key": preset.Key, "installation": installed, "hint": preset.Hint})
	}
	return out
}
