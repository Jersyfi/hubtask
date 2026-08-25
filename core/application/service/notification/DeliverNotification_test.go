// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"errors"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const baseURL = "https://hub.example.org"

type deliveryFixture struct {
	delivery      DeliverNotification
	notifications *notificationStore
	preferences   *preferenceStore
	mailbox       *mailbox
	work          *unitOfWork
	signals       *signalLog
	record        domain.Notification
}

// delivery builds the service with one pending record in it. Anna acts and speaks German; Bert
// receives and speaks English - the two accounts and two locales the acceptance criterion asks for.
func delivery(t *testing.T, category domain.Category, itemPresent bool) deliveryFixture {
	t.Helper()

	record, err := domain.New(domain.NewInput{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e1"), TenantID: tenant,
		RecipientID: bert, Category: category, Channel: domain.ChannelEmail,
		EventID: eventID, ItemID: itemID, ActorID: anna, At: now,
	})
	if err != nil {
		t.Fatalf("building the record: %v", err)
	}

	notifications, preferences := newNotifications(), newPreferences()
	if _, err := notifications.Insert(t.Context(), record); err != nil {
		t.Fatalf("seeding the record: %v", err)
	}

	items := newItems(task(bert))
	items.found = itemPresent

	box, work, signals := &mailbox{}, &unitOfWork{}, &signalLog{}
	return deliveryFixture{
		delivery: DeliverNotification{
			Notifications: notifications,
			Preferences:   preferences,
			Accounts: newAccounts(
				person(anna, "Anna", "anna@example.org", "de"),
				person(bert, "Bert", "bert@example.org", "en"),
			),
			Items: items, Mail: box, Renderer: catalogue{}, UnitOfWork: work,
			Clock: clock.Fixed(now), BaseURL: baseURL, Signals: signals,
		},
		notifications: notifications, preferences: preferences, mailbox: box,
		work: work, signals: signals, record: record,
	}
}

func (f deliveryFixture) stored(t *testing.T) domain.Notification {
	t.Helper()
	stored, err := f.notifications.Find(t.Context(), f.record.ID)
	if err != nil {
		t.Fatalf("reading the record back: %v", err)
	}
	return stored
}

func TestAMessageIsSentAndTheRecordSaysSo(t *testing.T) {
	fixture := delivery(t, domain.CategoryComment, true)

	if err := fixture.delivery.
		Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	if len(fixture.mailbox.sent) != 1 {
		t.Fatalf("%d messages sent, want one", len(fixture.mailbox.sent))
	}
	message := fixture.mailbox.sent[0]
	if message.To != "bert@example.org" {
		t.Errorf("sent to %q", message.To)
	}
	if !strings.Contains(message.Body, baseURL+"/items/"+itemID.String()) {
		t.Errorf("the body carries no link somebody can click: %q", message.Body)
	}
	if !strings.Contains(message.Subject, "title=Review the quote") {
		t.Errorf("the subject does not name the entry: %q", message.Subject)
	}
	if !strings.Contains(message.Subject, "actor=Anna") {
		t.Errorf("the subject does not name who did it: %q", message.Subject)
	}

	stored := fixture.stored(t)
	if stored.State != domain.StateSent || stored.SentAt == nil {
		t.Errorf("record %+v", stored)
	}
	if len(fixture.signals.sent) != 1 || fixture.signals.sent[0] != "COMMENT/EMAIL" {
		t.Errorf("signals %v", fixture.signals.sent)
	}

	// Every read and write bound to the tenant the job named (ADR-0010).
	for _, scope := range fixture.work.scopes {
		if scope.TenantID != tenant {
			t.Errorf("a transaction was opened for %q", scope.TenantID)
		}
	}
	if len(fixture.work.scopes) < 2 {
		t.Errorf("%d transactions - the read and the write-back are separate, and the send is "+
			"outside both", len(fixture.work.scopes))
	}
}

// The acceptance criterion of i18n-l10n.md §1: the recipient's locale, not the actor's. Anna
// speaks German and Bert does not, and the message Bert gets is in English.
func TestAnEmailIsRenderedInTheRecipientsLocale(t *testing.T) {
	fixture := delivery(t, domain.CategoryComment, true)

	if err := fixture.delivery.
		Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	message := fixture.mailbox.sent[0]
	if strings.HasPrefix(message.Subject, "[de]") || strings.HasPrefix(message.Body, "[de]") {
		t.Errorf("Bert was written to in Anna's language: %q / %q", message.Subject, message.Body)
	}

	// And the other way round, so that the test is about the recipient rather than about English
	// happening to be the default.
	german := delivery(t, domain.CategoryComment, true)
	german.delivery.Accounts = newAccounts(
		person(anna, "Anna", "anna@example.org", "en"),
		person(bert, "Bert", "bert@example.org", "de-AT"),
	)
	if err := german.delivery.Execute(t.Context(), tenant, german.record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if !strings.HasPrefix(german.mailbox.sent[0].Subject, "[de]") {
		t.Errorf("a German-speaking recipient was written to in English: %q",
			german.mailbox.sent[0].Subject)
	}
}

// A reminder rides the same delivery as everything else, and gets the one pair of message codes
// with no actor in it: nobody caused it, and a sentence naming somebody would name the clock
// (D-03).
func TestAReminderIsRenderedInItsOwnWordsAndInTheRecipientsLocale(t *testing.T) {
	fixture := delivery(t, domain.CategoryReminder, true)
	fixture.delivery.Accounts = newAccounts(
		person(anna, "Anna", "anna@example.org", "en"),
		person(bert, "Bert", "bert@example.org", "de-AT"),
	)

	if err := fixture.delivery.
		Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	message := fixture.mailbox.sent[0]
	if !strings.Contains(message.Subject, "email.reminder.subject") {
		t.Errorf("the subject is %q rather than the reminder's", message.Subject)
	}
	if !strings.Contains(message.Body, "email.reminder.body") {
		t.Errorf("the body is %q rather than the reminder's", message.Body)
	}
	if !strings.HasPrefix(message.Subject, "[de]") {
		t.Errorf("a German-speaking recipient was reminded in English: %q", message.Subject)
	}
	// The title travels and the link travels; nothing names an actor, because the catalogue entry
	// for a reminder has no placeholder for one.
	if !strings.Contains(message.Subject, "title=") || !strings.Contains(message.Body, "link=") {
		t.Errorf("the message carries %q / %q", message.Subject, message.Body)
	}
}

// The acceptance criterion of data-protection.md §9: no rendered email carries item content beyond
// the title. Not a habit but a test over the output.
func TestNoRenderedEmailCarriesContentBeyondTheTitle(t *testing.T) {
	fixture := delivery(t, domain.CategoryComment, true)
	// The entry's notes are content this message must never carry.
	notes := "The client's bank details are IBAN DE02 1203 0000 0000 2020 51."
	withNotes := task(bert)
	withNotes.Notes = notes
	fixture.delivery.Items = newItems(withNotes)

	if err := fixture.delivery.
		Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	message := fixture.mailbox.sent[0]
	for _, forbidden := range []string{notes, "IBAN", "bank details"} {
		if strings.Contains(message.Subject, forbidden) ||
			strings.Contains(message.Body, forbidden) {
			t.Errorf("the message carries %q:\n%s\n%s", forbidden, message.Subject, message.Body)
		}
	}
}

// The title is switchable, and switching it off leaves a message that says something concerns you
// and where to look.
func TestAWithheldTitleUsesTheOtherSentence(t *testing.T) {
	fixture := delivery(t, domain.CategoryComment, true)
	quiet := domain.DefaultPreference(tenant, bert, domain.CategoryComment, domain.ChannelEmail)
	quiet.IncludeTitle = false
	if err := fixture.preferences.Save(t.Context(), quiet); err != nil {
		t.Fatal(err)
	}

	if err := fixture.delivery.
		Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	message := fixture.mailbox.sent[0]
	if strings.Contains(message.Subject, "Review the quote") ||
		strings.Contains(message.Body, "Review the quote") {
		t.Errorf("the title travelled after being switched off: %q / %q",
			message.Subject, message.Body)
	}
	if !strings.Contains(message.Subject, ".withheld") {
		t.Errorf("the ordinary sentence was used with no title to put in it: %q", message.Subject)
	}
	if !strings.Contains(message.Body, baseURL) {
		t.Errorf("the withheld message carries no link either: %q", message.Body)
	}
}

// An entry purged while the message waited leaves the same message as a withheld title.
func TestAnEntryThatIsGoneLeavesTheMessageWithoutATitle(t *testing.T) {
	fixture := delivery(t, domain.CategoryComment, false)

	if err := fixture.delivery.
		Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if !strings.Contains(fixture.mailbox.sent[0].Subject, ".withheld") {
		t.Errorf("subject %q", fixture.mailbox.sent[0].Subject)
	}
}

// A queue draining after an outage must not deliver what somebody has since asked not to receive.
func TestSwitchingOffWhileTheMessageWaitsSuppressesIt(t *testing.T) {
	fixture := delivery(t, domain.CategoryComment, true)
	fixture.preferences.switchOff(bert, domain.CategoryComment)

	if err := fixture.delivery.
		Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	if len(fixture.mailbox.sent) != 0 {
		t.Errorf("a message was sent after the category was switched off: %v", fixture.mailbox.sent)
	}
	stored := fixture.stored(t)
	if stored.State != domain.StateSuppressed || stored.Reason != domain.ReasonCategoryOff {
		t.Errorf("record %+v - it has to say why", stored)
	}
}

// SMTP down means the notification waits: the record stays pending, the error comes back, and the
// queue's retry is what catches up (observability-reliability.md §7).
func TestAnUnreachableServerLeavesTheNotificationWaiting(t *testing.T) {
	fixture := delivery(t, domain.CategoryComment, true)
	fixture.mailbox.err = shared.ErrUnavailable.WithDetail("dependency.unavailable")

	err := fixture.delivery.Execute(t.Context(), tenant, fixture.record.ID, false)
	if err == nil {
		t.Fatal("a failed send was reported as a success - the queue would not retry")
	}

	stored := fixture.stored(t)
	if stored.State != domain.StatePending {
		t.Errorf("state %q, want it still pending: the queue is going to try again", stored.State)
	}
	if stored.Attempts != 1 {
		t.Errorf("attempts %d, want 1", stored.Attempts)
	}
	if len(fixture.signals.failed) != 1 ||
		fixture.signals.failed[0] != "COMMENT/EMAIL/dependency.unavailable" {
		t.Errorf("signals %v", fixture.signals.failed)
	}
}

// The last attempt is the queue's judgement, and the record follows it.
func TestTheLastAttemptMarksTheRecordFailed(t *testing.T) {
	fixture := delivery(t, domain.CategoryComment, true)
	fixture.mailbox.err = errors.New("refused for good")

	if err := fixture.delivery.
		Execute(t.Context(), tenant, fixture.record.ID, true); err == nil {
		t.Fatal("the last attempt was reported as a success")
	}

	stored := fixture.stored(t)
	if stored.State != domain.StateFailed || stored.Reason != domain.ReasonDeliveryFailed {
		t.Errorf("record %+v", stored)
	}
}

// A record the retention sweep took while the job waited is a job that is done, not one that
// failed: retrying it forever would fill a dead letter with ninety-day-old notifications.
func TestARecordThatIsGoneIsNotAFailure(t *testing.T) {
	fixture := delivery(t, domain.CategoryComment, true)

	err := fixture.delivery.
		Execute(t.Context(), tenant, shared.ID("01936f2a-7c1e-7000-8000-0000000000ff"), false)
	if err != nil {
		t.Errorf("a missing record reported %v, want the job to be done", err)
	}
	if len(fixture.mailbox.sent) != 0 {
		t.Error("a message was sent for a record that does not exist")
	}
}

// A duplicate job - the queue delivering at-least-once - must not send a second copy.
func TestARecordThatIsAlreadyDecidedIsNotSentAgain(t *testing.T) {
	fixture := delivery(t, domain.CategoryComment, true)

	for range 2 {
		if err := fixture.delivery.
			Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
			t.Fatalf("delivering: %v", err)
		}
	}
	if len(fixture.mailbox.sent) != 1 {
		t.Errorf("%d messages sent, want one", len(fixture.mailbox.sent))
	}
}

func TestTheReadIsReadOnlyAndTheWriteBackIsNot(t *testing.T) {
	fixture := delivery(t, domain.CategoryAssignment, true)

	if err := fixture.delivery.
		Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	// The scopes are recorded in order; what matters is that there is more than one, so the send
	// sits between two transactions rather than inside one (observability-reliability.md §8).
	var scopes []persistence.Scope
	scopes = append(scopes, fixture.work.scopes...)
	if len(scopes) != 2 {
		t.Errorf("%d transactions, want the read and the write-back", len(scopes))
	}
}
