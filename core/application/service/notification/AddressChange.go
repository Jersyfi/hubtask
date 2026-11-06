// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"errors"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The members are told when the workspace's address changes (multi-tenancy.md §3.1): the old and the
// new address, that passkeys made at the old one ended and that everybody signs in again. A member
// with an email address by mail; one without at the next sign-in. Not a notification record, for
// SendPasswordOpening's reason: a message about the way in that no preference may silence.

const (
	subjectAddressChanged = "email.address_changed.subject"
	bodyAddressChanged    = "email.address_changed.body"
)

// RecordAddressChange tells them. It implements the control plane's AddressNotices, inside the
// transaction of the change.
type RecordAddressChange struct {
	Jobs    Queue
	Notices identityrepo.AddressChangeNotices
}

// AddressChanged queues one mail per member with an address and writes one notice per member without.
func (r RecordAddressChange) AddressChanged(
	ctx context.Context, tenantID shared.ID, from, to string, at time.Time,
	mailed, unmailed []shared.ID,
) error {
	moment := at.UTC().Format(time.RFC3339Nano)
	for _, accountID := range mailed {
		if _, err := r.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindAddressChangedEmail, TenantID: tenantID,
			// One message per member per change: a retried write queues nothing twice.
			DedupeKey: "address:" + to + ":" + moment + ":" + accountID.String(),
			Payload: map[string]any{
				"account_id": accountID.String(), "from": from, "to": to,
			},
		}); err != nil {
			return err
		}
	}
	for _, accountID := range unmailed {
		if err := r.Notices.Record(ctx, accountID, from, to, at); err != nil {
			return err
		}
	}
	return nil
}

// AddressNotice is one queued mail, as the job carries it.
type AddressNotice struct {
	AccountID shared.ID
	From, To  string
}

// SendAddressChange is the job handler's application half: it reads who and in which language, and
// sends.
type SendAddressChange struct {
	Accounts       identityrepo.Accounts
	Workspaces     WorkspaceReader
	Mail           mail.Sender
	Renderer       i18n.Renderer
	UnitOfWork     persistence.UnitOfWork
	FallbackLocale string
	// Addresses answers the workspace's address at delivery - the new one - for the mail's link.
	Addresses WorkspaceAddresses
}

// Execute sends one mail. An account that is gone, or has no address, is finished business.
func (s SendAddressChange) Execute(ctx context.Context, tenantID shared.ID, notice AddressNotice) error {
	var address, locale, workspace, link string
	err := s.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			account, err := s.Accounts.Find(ctx, notice.AccountID)
			if errors.Is(err, shared.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			address, locale = account.Email, account.Locale
			if link, err = addressOf(ctx, s.Addresses); err != nil {
				return err
			}
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
	if err != nil || address == "" {
		return err
	}
	if locale == "" {
		locale = s.FallbackLocale
	}
	params := map[string]string{
		"workspace": workspace, "from": notice.From, "to": notice.To, "link": link,
	}
	return s.Mail.Send(ctx, mail.Message{
		To:      address,
		Subject: s.Renderer.Render(locale, subjectAddressChanged, params),
		Body:    s.Renderer.Render(locale, bodyAddressChanged, params),
	})
}
