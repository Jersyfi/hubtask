// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"errors"
	"net/url"
	"strings"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The message codes an email is built from. Under `email.*`, as i18n-l10n.md §3 prescribes, and
// paired: a subject and a body per category, and a second body for the message that may not carry
// the title.
//
// Two body keys rather than one with a conditional placeholder, because a placeholder with no
// parameter is left standing by the catalogue - a body written for a title it does not get would
// print `{title}` at somebody. The withheld variant is a different sentence, and a translator
// needs to see it as one.
const (
	subjectAssignment = "email.assignment.subject"
	subjectMembership = "email.membership.subject"
	subjectComment    = "email.comment.subject"
	subjectInvitation = "email.invitation.subject"
	subjectReminder   = "email.reminder.subject"
	bodyAssignment    = "email.assignment.body"
	bodyMembership    = "email.membership.body"
	bodyComment       = "email.comment.body"
	bodyInvitation    = "email.invitation.body"
	bodyReminder      = "email.reminder.body"
	// withheldSuffix names the variant of a message that has no title to put in it - because the
	// recipient asked for none, or because the entry is gone.
	withheldSuffix = ".withheld"
)

// DeliverNotification sends one notification.
//
// It owns its transactions, because it cannot live inside one: rendering is cheap and sending is
// not, and a transaction held open across an SMTP conversation is what
// observability-reliability.md §8 forbids. Three parts, in the order the media reconciliation uses
// for the same reason:
//
//  1. Read what the message is about - the record, the recipient, the entry - in one transaction.
//  2. Render and send, outside any transaction.
//  3. Write the outcome back, in a transaction of its own.
//
// Safe to run twice, which is what lets it declare itself detached (queue.Detached), and the
// second run is not free of consequence: a process that died between the send and the write-back
// leaves a message sent and a record that still says pending, and the retry sends it again. That
// is the honest trade of at-least-once for email, and it is the right side of it - a duplicate
// notification is a nuisance, and a lost one is the thing somebody needed to know.
type DeliverNotification struct {
	Notifications repository.Notifications
	Preferences   repository.Preferences
	Accounts      identityrepo.Accounts
	Items         Entries
	Mail          mail.Sender
	Renderer      i18n.Renderer
	UnitOfWork    persistence.UnitOfWork
	Clock         clock.Clock
	// BaseURL is where this installation lives, so an email can carry a link somebody can click. A
	// relative path in an email is a dead link.
	BaseURL string
	// Signals is the observability slice. Optional, like everywhere else in this package.
	Signals Signals
}

// subject is what a message needs before it can be rendered: the record, who is receiving it, and
// what it is about.
type subject struct {
	record    domain.Notification
	recipient identityAccount
	// decision is what the recipient's preference says *now*, rebuilt at delivery rather than
	// carried from the consumer. A person who switched a category off while the message waited in
	// the queue has said something more recent than the record, and the later answer is the right
	// one - which also means a queue draining after an outage does not deliver what somebody has
	// since asked not to receive.
	decision  domain.Decision
	title     string
	actorName string
}

// identityAccount is the slice of an account this package renders from: the locale that decides
// the language, and the address that decides whether there is anywhere to send.
type identityAccount struct {
	locale  string
	address string
}

// Execute sends the notification the job names, and reports whether it worked.
//
// `lastAttempt` is the queue's judgement rather than this service's: the attempt budget lives on
// the job, and a record that counted its own would be a second budget disagreeing with the first.
func (d DeliverNotification) Execute(
	ctx context.Context, tenantID, notificationID shared.ID, lastAttempt bool,
) error {
	scope := persistence.Scope{TenantID: tenantID}

	loaded, err := d.load(ctx, scope, notificationID)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		// The record went while its delivery waited - the retention sweep, or the entry it was
		// about being purged. Nothing to send and nothing to record: a job whose subject no longer
		// exists is done, not failed.
		return nil
	case err != nil:
		return err
	}
	if !loaded.record.Pending() {
		// Already decided. A duplicate job, or a retry of one that got as far as the write-back.
		return nil
	}

	if !loaded.decision.Send {
		// Switched off, or nowhere to send, since the record was written. Suppressed with the
		// reason rather than sent, and the job is done rather than failed.
		return d.write(ctx, scope, loaded.record.Suppress(loaded.decision.Reason))
	}

	started := d.Clock.Now()
	sendErr := d.send(ctx, loaded)
	if sendErr != nil {
		return d.recordFailure(ctx, scope, loaded, lastAttempt, sendErr)
	}

	sentAt := d.Clock.Now()
	if err := d.write(ctx, scope, loaded.record.Sent(sentAt)); err != nil {
		return err
	}
	if d.Signals != nil {
		d.Signals.NotificationSent(ctx,
			loaded.record.Category.String(), loaded.record.Channel.String(),
			sentAt.Sub(started).Seconds())
	}
	return nil
}

// load reads everything the message is built from, in one transaction.
//
// Read at delivery rather than carried in the job's payload, which is the reason the payload holds
// one identifier: an entry renamed while the notification waited is announced under its new name,
// and nothing personal sits in a queue table nothing cleans (rule 10).
func (d DeliverNotification) load(
	ctx context.Context, scope persistence.Scope, notificationID shared.ID,
) (subject, error) {
	var loaded subject

	err := d.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		record, err := d.Notifications.Find(ctx, notificationID)
		if err != nil {
			return err
		}
		loaded.record = record

		recipient, err := d.Accounts.Find(ctx, record.RecipientID)
		if err != nil {
			return err
		}
		loaded.recipient = identityAccount{locale: recipient.Locale, address: recipient.Email}

		preference, err := d.Preferences.Find(ctx, record.RecipientID, record.Category, record.Channel)
		switch {
		case errors.Is(err, shared.ErrNotFound):
			// Saying nothing is the default, and the default is on. Written down once, in the
			// domain, and read here rather than copied.
			preference = domain.DefaultPreference(
				record.TenantID, record.RecipientID, record.Category, record.Channel)
		case err != nil:
			return err
		}
		loaded.decision = domain.Decide(record, domain.Recipient{
			AccountID: record.RecipientID, HasAddress: recipient.Email != "",
		}, preference)

		if !record.ActorID.IsZero() {
			actor, err := d.Accounts.Find(ctx, record.ActorID)
			switch {
			case errors.Is(err, shared.ErrNotFound):
				// The person who caused it has left. The message still makes sense without a
				// name, and the catalogue's `{actor}` placeholder is left standing rather than
				// blanked - which is exactly what the withheld variants are for.
			case err != nil:
				return err
			default:
				loaded.actorName = actor.DisplayName
			}
		}

		if record.ItemID.IsZero() {
			return nil
		}
		item, err := d.Items.Find(ctx, record.ItemID)
		switch {
		case errors.Is(err, shared.ErrNotFound):
			// The entry went. The message becomes the one without a title rather than no message:
			// somebody was told about something, and the link will tell them it is gone.
		case err != nil:
			return err
		default:
			loaded.title = item.Title
		}
		return nil
	})
	if err != nil {
		return subject{}, err
	}
	return loaded, nil
}

// send renders the message and hands it to the channel.
func (d DeliverNotification) send(ctx context.Context, loaded subject) error {
	message := d.compose(loaded)

	locale := loaded.recipient.locale
	return d.Mail.Send(ctx, mail.Message{
		To: loaded.recipient.address,
		// The recipient's locale, never the actor's (i18n-l10n.md §1). The port takes it as an
		// argument for exactly this reason: there is no call that can forget it.
		Subject: d.Renderer.Render(locale, message.SubjectCode, message.Params()),
		Body:    d.Renderer.Render(locale, message.BodyCode, message.Params()),
	})
}

// compose builds the message from the record and what may travel in it.
//
// The title reaches the message through domain.Message.WithTitle and the decision that permits it,
// which is the one door there is. Whether it is permitted is the preference's answer; whether
// there is one at all is the entry's - an entry that was purged while the message waited leaves
// the same message as a preference that withholds the title, which is the honest outcome in both
// cases: somebody is told that something concerns them, and where to look.
func (d DeliverNotification) compose(loaded subject) domain.Message {
	subjectCode, bodyCode := codesFor(loaded.record.Category)

	message := domain.Message{
		SubjectCode: subjectCode,
		BodyCode:    bodyCode,
		ActorName:   loaded.actorName,
		Link:        d.link(loaded.record),
	}
	message = message.WithTitle(loaded.title, loaded.decision)
	if message.Title == "" {
		// A body written for a title it does not get would print `{title}` at somebody: the
		// catalogue leaves a placeholder with no parameter standing. The withheld variant is a
		// different sentence, and a translator needs to see it as one.
		message.SubjectCode += withheldSuffix
		message.BodyCode += withheldSuffix
	}
	return message
}

// codesFor is the category's pair of catalogue keys. A switch rather than a map, so that a
// category added without its messages does not compile.
func codesFor(category domain.Category) (subjectCode, bodyCode string) {
	switch category {
	case domain.CategoryAssignment:
		return subjectAssignment, bodyAssignment
	case domain.CategoryMembership:
		return subjectMembership, bodyMembership
	case domain.CategoryComment:
		return subjectComment, bodyComment
	case domain.CategoryInvitation:
		return subjectInvitation, bodyInvitation
	case domain.CategoryReminder:
		// The one pair with no {actor} in it: nobody caused a reminder, the clock did, and a
		// message naming an actor that is not there would print the placeholder at somebody.
		return subjectReminder, bodyReminder
	}
	// Not reachable through New, which refuses an unknown category. Answered rather than panicked:
	// a message with the wrong words is better than a worker that dies on a row.
	return subjectAssignment, bodyAssignment
}

// link is where to look, absolute so that a mail client has something to click.
func (d DeliverNotification) link(record domain.Notification) string {
	base := strings.TrimSuffix(d.BaseURL, "/")
	if base == "" {
		return ""
	}
	if record.ItemID.IsZero() {
		return base
	}
	// The identifier is a parsed UUID by the time it is here, so there is nothing to escape - and
	// it is escaped anyway, because a link built by concatenation is the shape of an injection
	// even when this particular value cannot be one.
	return base + "/items/" + url.PathEscape(record.ItemID.String())
}

// recordFailure writes what went wrong and decides whether the job comes back.
func (d DeliverNotification) recordFailure(
	ctx context.Context, scope persistence.Scope, loaded subject, lastAttempt bool, cause error,
) error {
	if err := d.write(ctx, scope, loaded.record.Failed(lastAttempt)); err != nil {
		return err
	}
	if d.Signals != nil {
		d.Signals.NotificationFailed(ctx,
			loaded.record.Category.String(), loaded.record.Channel.String(),
			shared.AsError(cause).DetailCode)
	}
	// Returned either way, so the queue retries or dead-letters. The record's state is this
	// package's answer to "what happened"; the job's fate is the queue's.
	return cause
}

// write records the outcome in a transaction of its own.
func (d DeliverNotification) write(
	ctx context.Context, scope persistence.Scope, record domain.Notification,
) error {
	err := d.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		return d.Notifications.Save(ctx, record)
	})
	if errors.Is(err, shared.ErrNotFound) {
		// The retention sweep reached the record while the message was in flight. The message went
		// out, and there is nothing left to say so - which is a fact about a ninety-day-old
		// notification and not a failure of this job.
		return nil
	}
	return err
}
