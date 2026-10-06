// SPDX-License-Identifier: BUSL-1.1
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

// A workspace's administrators are told when an operator opens its password, and when the opening
// ends (ADR-0078 §3, SC-34).
//
// **Deliberately not a notification record**, SendPasswordReset's reasoning: this is a message about
// the way into the workspace that no preference may silence, and a category that existed only to be
// the exception would be the wrong shape. What it shares is everything else - the queue's retry and
// dedupe, the recipient's own language, the renderer, the one port that sends.
//
// **Who is told** is the role matrix's answer for the workspace as a whole - the accounts that
// administer it - read in the transaction that opened or closed it, so a later change of roles does
// not change who was told about this act. The job carries identifiers, the event and a moment; the
// address, the language and the workspace's name are read at delivery.

// The two events a notice is about.
const (
	OpeningEventOpened = "OPENED"
	OpeningEventClosed = "CLOSED"
)

const (
	subjectPasswordOpened = "email.password_opened.subject"
	bodyPasswordOpened    = "email.password_opened.body"
	subjectPasswordClosed = "email.password_closed.subject"
	bodyPasswordClosed    = "email.password_closed.body"
	// The close at its time and the close by the operator are different sentences: one is what was
	// announced, the other is news.
	bodyPasswordClosedEarly = "email.password_closed_early.body"
)

// closedEarly is the `ended` the admin service writes for an operator's close (OpeningEndedByOperator).
// Repeated rather than imported, so this package points at nothing the control plane owns.
const closedEarly = "OPERATOR"

// openingTimeLayout is how the end reads in the mail: to the minute, in UTC, said so - the recipient's
// time zone is not known to the notice, and an hour given without its zone is a wrong hour somewhere.
const openingTimeLayout = "2006-01-02 15:04 UTC"

// RecordPasswordOpening queues the notices. It implements the control plane's OpeningNotices.
type RecordPasswordOpening struct {
	Memberships identityrepo.Memberships
	Jobs        Queue
}

// PasswordOpened tells every administrator that the password is open until `until`.
func (r RecordPasswordOpening) PasswordOpened(
	ctx context.Context, tenantID shared.ID, until time.Time,
) error {
	return r.queue(ctx, tenantID, map[string]any{
		"event": OpeningEventOpened, "until": until.UTC().Format(time.RFC3339),
	}, OpeningEventOpened+":"+until.UTC().Format(time.RFC3339))
}

// PasswordClosed tells every administrator that the opening ended, and how.
func (r RecordPasswordOpening) PasswordClosed(
	ctx context.Context, tenantID shared.ID, ended string,
) error {
	return r.queue(ctx, tenantID, map[string]any{"event": OpeningEventClosed, "ended": ended},
		OpeningEventClosed+":"+ended)
}

// queue writes one job per administrator, inside the caller's transaction.
func (r RecordPasswordOpening) queue(
	ctx context.Context, tenantID shared.ID, payload map[string]any, key string,
) error {
	administrators, err := r.Memberships.Administrators(ctx, []identity.Scope{identity.TenantScope()})
	if err != nil {
		return err
	}
	for _, accountID := range administrators {
		message := map[string]any{"account_id": accountID.String()}
		for name, value := range payload {
			message[name] = value
		}
		if _, err := r.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindPasswordOpeningEmail, TenantID: tenantID,
			// One message per person per act: a retried write queues nothing twice. The key carries
			// no moment of the act itself, so two acts that say the same thing within one pending
			// window collapse into one message, which is what the person needs to read.
			DedupeKey: key + ":" + accountID.String(),
			Payload:   message,
		}); err != nil {
			return err
		}
	}
	return nil
}

// OpeningNotice is one queued notice, as the job carries it.
type OpeningNotice struct {
	AccountID shared.ID
	Event     string
	Until     time.Time
	Ended     string
}

// SendPasswordOpening is the job handler's application half: it reads who and where, and sends.
type SendPasswordOpening struct {
	Accounts identityrepo.Accounts
	// Workspaces answers the workspace's name for the message, and its default language - the
	// second link of §2's chain.
	Workspaces     WorkspaceReader
	Mail           mail.Sender
	Renderer       i18n.Renderer
	UnitOfWork     persistence.UnitOfWork
	FallbackLocale string
	// BaseURL is where this installation lives: the message links to the sign-in settings.
	BaseURL string
}

// Execute sends one notice. An account that is gone, or has no address, is finished business: there
// is nothing a retry would find.
func (s SendPasswordOpening) Execute(ctx context.Context, tenantID shared.ID, notice OpeningNotice) error {
	var (
		address, locale, workspace string
		found                      bool
	)
	err := s.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			account, err := s.Accounts.Find(ctx, notice.AccountID)
			if errors.Is(err, shared.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			found = true
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
	if err != nil || !found || address == "" {
		return err
	}
	if locale == "" {
		locale = s.FallbackLocale
	}

	params := map[string]string{"workspace": workspace, "link": ""}
	if base := strings.TrimSuffix(s.BaseURL, "/"); base != "" {
		// The administrators' sign-in settings, where the opening is shown while it stands.
		params["link"] = base + "/administration/sign-in"
	}
	subjectCode, bodyCode := subjectPasswordClosed, bodyPasswordClosed
	switch {
	case notice.Event == OpeningEventOpened:
		subjectCode, bodyCode = subjectPasswordOpened, bodyPasswordOpened
		params["until"] = notice.Until.UTC().Format(openingTimeLayout)
	case notice.Ended == closedEarly:
		bodyCode = bodyPasswordClosedEarly
	}
	return s.Mail.Send(ctx, mail.Message{
		To:      address,
		Subject: s.Renderer.Render(locale, subjectCode, params),
		Body:    s.Renderer.Render(locale, bodyCode, params),
	})
}
