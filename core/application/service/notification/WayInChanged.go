// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"errors"
	"strings"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// A person is told by mail when a way into their account changes (ADR-0078 §6, identity.md §6):
// a password changed or set through a reset, a provider connected or disconnected.
//
// **Deliberately not a notification record**, SendPasswordReset's reasoning: a notice about the
// way into an account that no preference may silence. The job is queued in the transaction of the
// change itself, so the notice exists exactly when the change committed; it carries identifiers,
// the event, the moment and a provider's name, and the address and language are read at delivery.

const (
	subjectPasswordChanged      = "email.way_in.password_changed.subject" //nolint:gosec // G101: a message code, not a credential
	bodyPasswordChanged         = "email.way_in.password_changed.body"    //nolint:gosec // G101: a message code, not a credential
	subjectWayInReset           = "email.way_in.password_reset.subject"
	bodyWayInReset              = "email.way_in.password_reset.body"
	subjectProviderConnected    = "email.way_in.provider_connected.subject"
	bodyProviderConnected       = "email.way_in.provider_connected.body"
	subjectProviderDisconnected = "email.way_in.provider_disconnected.subject"
	bodyProviderDisconnected    = "email.way_in.provider_disconnected.body"
)

// RecordWayInChanged queues the notice. It implements the identity writers' WayInNotices.
type RecordWayInChanged struct {
	Jobs Queue
}

// WayInChanged queues one notice for the account, inside the caller's transaction.
func (r RecordWayInChanged) WayInChanged(
	ctx context.Context, tenantID, accountID shared.ID, change identity.WayInChange,
) error {
	at := change.At.UTC().Format(time.RFC3339Nano)
	payload := map[string]any{
		"account_id": accountID.String(), "event": string(change.Event), "at": at,
	}
	if !change.ProviderID.IsZero() {
		payload["provider_id"] = change.ProviderID.String()
		payload["provider_name"] = change.ProviderName
	}
	_, err := r.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindWayInChangedEmail, TenantID: tenantID,
		// The moment is in the key: two changes are two notices, a retried write is one.
		DedupeKey: string(change.Event) + ":" + accountID.String() + ":" + at,
		Payload:   payload,
	})
	return err
}

// WayInNotice is one queued notice, as the job carries it.
type WayInNotice struct {
	AccountID shared.ID
	Change    identity.WayInChange
}

// ProviderFinder answers whether a provider still exists, which is all a disconnection's notice
// asks of it.
type ProviderFinder interface {
	Find(ctx context.Context, id shared.ID) (identity.IdentityProvider, error)
}

// SendWayInChanged is the job handler's application half: it reads who and where, and sends.
type SendWayInChanged struct {
	Accounts identityrepo.Accounts
	// Workspaces answers the workspace's name for the message, and its default language.
	Workspaces WorkspaceReader
	// Providers is read for a disconnection: a provider that still exists was not removed after
	// all - the removal that queued the notice failed - and nothing is sent.
	Providers      ProviderFinder
	Mail           mail.Sender
	Renderer       i18n.Renderer
	UnitOfWork     persistence.UnitOfWork
	FallbackLocale string
	// BaseURL is where this installation lives: the message links back to it.
	BaseURL string
}

// Execute sends one notice. An account that is gone, or has no address, is finished business.
func (s SendWayInChanged) Execute(ctx context.Context, tenantID shared.ID, notice WayInNotice) error {
	var (
		address, locale, workspace string
		send                       bool
	)
	err := s.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			if notice.Change.Event == identity.WayInProviderDisconnected && s.Providers != nil {
				_, err := s.Providers.Find(ctx, notice.Change.ProviderID)
				if err == nil {
					return nil
				}
				if !errors.Is(err, shared.ErrNotFound) {
					return err
				}
			}
			account, err := s.Accounts.Find(ctx, notice.AccountID)
			if errors.Is(err, shared.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			send = true
			address, locale = account.Email, account.Locale
			if s.Workspaces == nil {
				return nil
			}
			row, err := s.Workspaces.Find(ctx)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			workspace = row.DisplayName
			if locale == "" {
				locale = row.DefaultLocale
			}
			return nil
		})
	if err != nil || !send || address == "" {
		return err
	}
	if locale == "" {
		locale = s.FallbackLocale
	}

	params := map[string]string{
		"workspace": workspace,
		"at":        notice.Change.At.UTC().Format(openingTimeLayout),
		"provider":  notice.Change.ProviderName,
		"link":      "",
	}
	base := strings.TrimSuffix(s.BaseURL, "/")
	var subjectCode, bodyCode string
	switch notice.Change.Event {
	case identity.WayInPasswordChanged:
		subjectCode, bodyCode = subjectPasswordChanged, bodyPasswordChanged
	case identity.WayInPasswordReset:
		subjectCode, bodyCode = subjectWayInReset, bodyWayInReset
	case identity.WayInProviderConnected:
		subjectCode, bodyCode = subjectProviderConnected, bodyProviderConnected
	case identity.WayInProviderDisconnected:
		subjectCode, bodyCode = subjectProviderDisconnected, bodyProviderDisconnected
	default:
		return shared.ErrInternal.WithDetail("notifications.payload_malformed").
			WithParams(map[string]string{"field": "event"})
	}
	if base != "" {
		// A connected provider is undone from the person's own security settings; a password from
		// the sign-in page, where *Forgot your password?* ends every session.
		params["link"] = base + "/"
		if notice.Change.Event == identity.WayInProviderConnected {
			params["link"] = base + "/profile/security"
		}
	}
	return s.Mail.Send(ctx, mail.Message{
		To:      address,
		Subject: s.Renderer.Render(locale, subjectCode, params),
		Body:    s.Renderer.Render(locale, bodyCode, params),
	})
}
