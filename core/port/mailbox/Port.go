// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package mailbox is how the mail intake reads a connected mailbox (ADR-0094 decision 1): sign in,
// take the messages not yet taken, move each to the done folder once it is filed. IMAP and JMAP
// are two adapters behind it; the intake does not know which it talks to.
package mailbox

import (
	"context"

	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Connection is a mailbox with its secret, opened for one read and forgotten after it.
type Connection struct {
	Mailbox domain.Mailbox
	Secret  secret.Secret
}

// Message is one message as it lies in the mailbox: a reference the provider knows it by, and the
// raw RFC 5322 bytes.
type Message struct {
	Ref string
	Raw []byte
}

// Reader opens a session on a mailbox. Refusals are coded, never the server's own words: a
// password that does not work is `mail.mailbox_refused`, a server that cannot be reached
// `mail.mailbox_unreachable`, a folder that does not exist `mail.mailbox_folder_missing`.
type Reader interface {
	Open(ctx context.Context, connection Connection) (Session, error)
}

// Session is one signed-in read.
type Session interface {
	// Fetch answers up to limit messages in the folder, oldest first.
	Fetch(ctx context.Context, limit int) ([]Message, error)
	// Done moves a message to the done folder, creating the folder the first time.
	Done(ctx context.Context, ref string) error
	// Close signs out.
	Close() error
}

// Token is what an OAuth provider hands out for a mailbox: an access token for one read, a refresh
// token for the next (empty when the provider keeps the one it gave before), and whose it is.
type Token struct {
	Access  secret.Secret
	Refresh secret.Secret
	// Email is the signed-in account's address as the ID token names it; empty when the provider
	// sends none.
	Email string
}

// Authorizer trades with an OAuth provider at its token endpoint (RFC 6749 §4.1.3, §6; PKCE RFC
// 7636). Refusals are coded: a code or refresh token the provider does not take is
// `mail.oauth_refused` (`mail.oauth_revoked` when the grant is gone), a provider that cannot be
// reached `mail.mailbox_unreachable`.
type Authorizer interface {
	Exchange(ctx context.Context, client domain.OAuthClient, clientSecret secret.Secret, code, verifier, redirectURL string) (Token, error)
	Refresh(ctx context.Context, client domain.OAuthClient, clientSecret, refresh secret.Secret) (Token, error)
}
