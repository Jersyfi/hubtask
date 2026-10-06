// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"net/url"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The reset link on its way to a mailbox (ADR-0068 §6).
//
// **Deliberately not a notification record.** Every other message in this package is something a
// person may switch off, has a read state, and belongs in a list; a reset link is a credential with
// none of those. Giving it a category would add one that no preference may touch - a category that
// exists only to be an exception - and a row a person would see listed as a notification about
// their own password.
//
// What it does share is everything that matters: the queue's retry and dedupe, the recipient's own
// language, the renderer, and the one port that sends. The token is minted **here**, at delivery,
// for the redemption token's reason: the queue's payload carries identifiers only, so the plaintext
// exists exactly once, in the message on its way out.

const (
	subjectPasswordReset = "email.password_reset.subject"
	bodyPasswordReset    = "email.password_reset.body"
	// The variant for an account that signs in through a provider. It carries no link: a reset
	// screen for somebody with no password would be a second way in that nobody asked for, and
	// "you do not sign in with a password here" is the sentence they actually need.
	subjectPasswordResetProvider = "email.password_reset_provider.subject"
	bodyPasswordResetProvider    = "email.password_reset_provider.body"
	// The variant for a provider-only account whose workspace lost its last provider (ADR-0077 §3):
	// there is no provider left to point to, so the link sets a first password.
	subjectPasswordSet = "email.password_set.subject"
	bodyPasswordSet    = "email.password_set.body"
	// The variant for a workspace that switched the password off, to an account no provider
	// switched on there lets in (ADR-0078 §1): the link connects the workspace's provider, and sets
	// no password.
	subjectPasswordConnect = "email.password_connect.subject"
	bodyPasswordConnect    = "email.password_connect.body"
)

// ResetMinter is the identity service's seam. It answers the plaintext token, the address, and
// whether this account signs in with a password at all.
type ResetMinter interface {
	MintResetToken(ctx context.Context, tenantID, accountID shared.ID) (ResetLink, error)
}

// ResetLink is what the minter answers. Declared here rather than imported so that this package
// points at nothing the identity service owns - the seam is one method and one value.
type ResetLink struct {
	Token       secret.Secret
	HasPassword bool
	// First is a link to set a first password, under ADR-0077 §3's fallback.
	First bool
	// Connect is a link to connect the workspace's provider, where the password is off (ADR-0078 §1).
	Connect bool
	Address string
	Locale  string
}

// SendPasswordReset is the job handler.
type SendPasswordReset struct {
	Resets   ResetMinter
	Mail     mail.Sender
	Renderer i18n.Renderer
	// Workspaces and FallbackLocale are §2's chain, DeliverNotification's verbatim: the account's
	// own language, otherwise the workspace's default, otherwise the installation's.
	Workspaces     WorkspaceReader
	UnitOfWork     persistence.UnitOfWork
	FallbackLocale string
	// BaseURL is where this installation lives. A relative path in an email is a dead link.
	BaseURL string
}

// Execute mints the link and sends it.
//
// An account that is no longer there, or that has no address, is finished business rather than a
// failure: the job completes, because there is nothing a retry would find.
func (s SendPasswordReset) Execute(ctx context.Context, tenantID, accountID shared.ID) error {
	link, err := s.Resets.MintResetToken(ctx, tenantID, accountID)
	if err != nil {
		return err
	}
	if link.Address == "" {
		return nil
	}

	locale, err := s.localeFor(ctx, tenantID, link.Locale)
	if err != nil {
		return err
	}

	params := map[string]string{}
	var subjectCode, bodyCode string
	// In the fragment rather than the query, the invitation's reasoning: a proxy or a server log
	// between the mail client and the interface never sees what follows the hash.
	base := strings.TrimSuffix(s.BaseURL, "/")
	switch {
	case link.Connect:
		subjectCode, bodyCode = subjectPasswordConnect, bodyPasswordConnect
		params["link"] = base + "/reset#connect=" + url.PathEscape(link.Token.Reveal())
	case link.First:
		subjectCode, bodyCode = subjectPasswordSet, bodyPasswordSet
		params["link"] = base + "/reset#token=" + url.PathEscape(link.Token.Reveal())
	case link.HasPassword:
		subjectCode, bodyCode = subjectPasswordReset, bodyPasswordReset
		params["link"] = base + "/reset#token=" + url.PathEscape(link.Token.Reveal())
	default:
		subjectCode, bodyCode = subjectPasswordResetProvider, bodyPasswordResetProvider
	}

	return s.Mail.Send(ctx, mail.Message{
		To:      link.Address,
		Subject: s.Renderer.Render(locale, subjectCode, params),
		Body:    s.Renderer.Render(locale, bodyCode, params),
	})
}

// localeFor is the recipient's chain, read inside a transaction bound to their workspace.
func (s SendPasswordReset) localeFor(
	ctx context.Context, tenantID shared.ID, own string,
) (string, error) {
	if own != "" {
		return own, nil
	}
	if s.Workspaces == nil || s.UnitOfWork == nil {
		return s.FallbackLocale, nil
	}

	locale := s.FallbackLocale
	err := s.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			workspace, err := s.Workspaces.Find(ctx)
			if err != nil {
				// A workspace row that is gone while its accounts are not is not this delivery's
				// to explain; the message still goes, in the installation's language.
				return nil //nolint:nilerr // the fallback is the answer
			}
			if workspace.DefaultLocale != "" {
				locale = workspace.DefaultLocale
			}
			return nil
		})
	return locale, err
}
