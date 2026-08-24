// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

type consumerFixture struct {
	consumer      RecordNotifications
	notifications *notificationStore
	preferences   *preferenceStore
	jobs          *jobQueue
	signals       *signalLog
	members       *memberStore
}

func consumer(t *testing.T, assignee shared.ID, members ...shared.ID) consumerFixture {
	t.Helper()

	notifications, preferences := newNotifications(), newPreferences()
	jobs, signals := &jobQueue{}, &signalLog{}
	memberList := &memberStore{members: members}

	return consumerFixture{
		consumer: RecordNotifications{
			Notifications: notifications,
			Preferences:   preferences,
			Accounts: newAccounts(
				person(anna, "Anna", "anna@example.org", "de"),
				person(bert, "Bert", "bert@example.org", "en"),
				person(carla, "Carla", "carla@example.org", "en"),
			),
			Items:       newItems(task(assignee)),
			ItemMembers: memberList,
			Jobs:        jobs,
			Clock:       clock.Fixed(now),
			IDs:         &idSequence{},
			Signals:     signals,
		},
		notifications: notifications, preferences: preferences,
		jobs: jobs, signals: signals, members: memberList,
	}
}

func assigned(t *testing.T, actor, assignee shared.ID) event.Envelope {
	t.Helper()
	envelope, err := event.NewEnvelope(eventID, event.ItemAssigned, tenant,
		event.ItemSubject(itemID),
		event.Actor{Kind: shared.ActorUser, ID: actor}, now, event.Cause{},
		map[string]any{
			"item_id": itemID.String(), "collection_id": collection.String(),
			"assignee_id": assignee.String(),
		})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

func commented(t *testing.T, author shared.ID) event.Envelope {
	t.Helper()
	envelope, err := event.NewEnvelope(eventID, event.CommentCreated, tenant,
		event.CommentSubject(shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")),
		event.Actor{Kind: shared.ActorUser, ID: author}, now, event.Cause{},
		map[string]any{
			"item_id": itemID.String(), "collection_id": collection.String(),
			"author_id": author.String(),
			// The body is in the payload the outbox carries, and nothing in this package may read
			// it. It is here so that a test proving so has something to fail against.
			"body": "The quote is wrong on page four, and the discount is missing entirely.",
		})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

func TestTheConsumerWantsOnlyWhatConcernsSomebody(t *testing.T) {
	subscriber := RecordNotifications{}

	for _, wanted := range []event.Type{
		event.ItemAssigned, event.ItemMemberAdded, event.CommentCreated,
	} {
		if !subscriber.Wants(wanted) {
			t.Errorf("%s is not wanted, and it is what this milestone tells people about", wanted)
		}
	}
	for _, ignored := range []event.Type{
		event.ItemCreated, event.ItemMoved, event.ItemUnassigned, event.CommentDeleted,
		event.ItemTrashed, event.BucketCreated,
	} {
		if subscriber.Wants(ignored) {
			t.Errorf("%s is wanted - somebody would get an email about it", ignored)
		}
	}
	if subscriber.Name() != ConsumerName {
		t.Errorf("the subscriber is called %q", subscriber.Name())
	}
}

func TestAnAssignmentTellsTheAssignee(t *testing.T) {
	fixture := consumer(t, bert)

	if err := fixture.consumer.Deliver(t.Context(), assigned(t, anna, bert)); err != nil {
		t.Fatalf("consuming: %v", err)
	}

	written := fixture.notifications.written()
	if len(written) != 1 {
		t.Fatalf("%d records written, want one", len(written))
	}
	record := written[0]
	if record.RecipientID != bert || record.ActorID != anna {
		t.Errorf("record %+v", record)
	}
	if record.Category != domain.CategoryAssignment || record.State != domain.StatePending {
		t.Errorf("record %+v", record)
	}
	if record.ItemID != itemID || record.EventID != eventID {
		t.Errorf("the references did not survive: %+v", record)
	}

	if len(fixture.jobs.requests) != 1 {
		t.Fatalf("%d jobs queued, want one delivery", len(fixture.jobs.requests))
	}
	job := fixture.jobs.requests[0]
	if job.Kind != queue.KindNotificationDeliver || job.TenantID != tenant {
		t.Errorf("job %+v", job)
	}
	if job.DedupeKey != record.ID.String() {
		t.Errorf("dedupe key %q, want the record's identifier", job.DedupeKey)
	}
	// Identifiers only: a payload is a place personal data would sit unencrypted in a table
	// nothing cleans (rule 10).
	if len(job.Payload) != 1 || job.Payload["notification_id"] != record.ID.String() {
		t.Errorf("payload %v, want only the record's identifier", job.Payload)
	}
}

// The whole entry is told about a comment, and each person once: somebody who is both the assignee
// and on the member list is one recipient, not two.
func TestACommentTellsTheAssigneeAndTheMembersOnce(t *testing.T) {
	fixture := consumer(t, bert, bert, carla)

	if err := fixture.consumer.Deliver(t.Context(), commented(t, anna)); err != nil {
		t.Fatalf("consuming: %v", err)
	}

	written := fixture.notifications.written()
	if len(written) != 2 {
		t.Fatalf("%d records written, want one each for Bert and Carla", len(written))
	}
	told := map[shared.ID]bool{}
	for _, record := range written {
		if told[record.RecipientID] {
			t.Errorf("%s was told twice", record.RecipientID)
		}
		told[record.RecipientID] = true
		if record.Category != domain.CategoryComment {
			t.Errorf("category %q", record.Category)
		}
	}
	if !told[bert] || !told[carla] {
		t.Errorf("told %v, want Bert and Carla", told)
	}
}

// Being told about your own comment is not news. The record is written anyway, because "why did I
// hear nothing" deserves an answer.
func TestSomebodyIsNotToldAboutTheirOwnAction(t *testing.T) {
	fixture := consumer(t, anna, anna)

	if err := fixture.consumer.Deliver(t.Context(), commented(t, anna)); err != nil {
		t.Fatalf("consuming: %v", err)
	}

	written := fixture.notifications.written()
	if len(written) != 1 {
		t.Fatalf("%d records written, want one", len(written))
	}
	if written[0].State != domain.StateSuppressed {
		t.Errorf("state %q, want suppressed", written[0].State)
	}
	if written[0].Reason != domain.ReasonSelfCaused {
		t.Errorf("reason %q", written[0].Reason)
	}
	if len(fixture.jobs.requests) != 0 {
		t.Errorf("a suppressed notification was queued for delivery: %v", fixture.jobs.requests)
	}
}

// The acceptance criterion: a recipient who has switched a category off receives nothing, and the
// record says why.
func TestSwitchingACategoryOffLeavesARecordThatSaysWhy(t *testing.T) {
	fixture := consumer(t, bert)
	fixture.preferences.switchOff(bert, domain.CategoryAssignment)

	if err := fixture.consumer.Deliver(t.Context(), assigned(t, anna, bert)); err != nil {
		t.Fatalf("consuming: %v", err)
	}

	written := fixture.notifications.written()
	if len(written) != 1 {
		t.Fatalf("%d records written, want one", len(written))
	}
	if written[0].State != domain.StateSuppressed ||
		written[0].Reason != domain.ReasonCategoryOff {
		t.Errorf("record %+v - it has to say why nothing was sent", written[0])
	}
	if len(fixture.jobs.requests) != 0 {
		t.Errorf("a message was queued after the category was switched off: %v",
			fixture.jobs.requests)
	}
	if len(fixture.signals.recorded) != 1 ||
		fixture.signals.recorded[0] != "ASSIGNMENT/EMAIL/SUPPRESSED" {
		t.Errorf("signals %v", fixture.signals.recorded)
	}
}

// The correction for at-least-once delivery: the second consumption writes nothing and queues
// nothing (ADR-0007).
func TestTheSameEventDeliveredTwiceSendsOneMessage(t *testing.T) {
	fixture := consumer(t, bert)

	for range 2 {
		if err := fixture.consumer.Deliver(t.Context(), assigned(t, anna, bert)); err != nil {
			t.Fatalf("consuming: %v", err)
		}
	}

	if written := fixture.notifications.written(); len(written) != 1 {
		t.Errorf("%d records written, want one", len(written))
	}
	if len(fixture.jobs.requests) != 1 {
		t.Errorf("%d deliveries queued, want one", len(fixture.jobs.requests))
	}
}

// A recipient whose account went between the event and the dispatch is not an error: stopping the
// batch over one deleted account would stop every other tenant's notifications with it.
func TestARecipientWhoIsGoneStopsNothing(t *testing.T) {
	fixture := consumer(t, bert)
	fixture.consumer.Accounts = newAccounts() // nobody at all

	if err := fixture.consumer.Deliver(t.Context(), assigned(t, anna, bert)); err != nil {
		t.Fatalf("consuming: %v", err)
	}
	if len(fixture.jobs.requests) != 0 {
		t.Errorf("a message was queued to nobody: %v", fixture.jobs.requests)
	}
}

// An entry that went between the comment and the dispatch has nobody left to tell.
func TestACommentOnAnEntryThatIsGoneTellsNobody(t *testing.T) {
	fixture := consumer(t, bert, carla)
	fixture.consumer.Items = &itemStore{}

	if err := fixture.consumer.Deliver(t.Context(), commented(t, anna)); err != nil {
		t.Fatalf("consuming: %v", err)
	}
	if written := fixture.notifications.written(); len(written) != 0 {
		t.Errorf("%d records written for an entry that does not exist", len(written))
	}
}

// The system acting for nobody - the automatic assignment (C-02) - must not suppress the
// recipient's own notification by matching a zero actor against them.
func TestAnAssignmentNobodyMadeStillTellsTheAssignee(t *testing.T) {
	fixture := consumer(t, bert)

	envelope := assigned(t, anna, bert)
	envelope.Actor = event.Actor{Kind: shared.ActorSystem}

	if err := fixture.consumer.Deliver(t.Context(), envelope); err != nil {
		t.Fatalf("consuming: %v", err)
	}

	written := fixture.notifications.written()
	if len(written) != 1 || written[0].State != domain.StatePending {
		t.Fatalf("records %+v", written)
	}
	if !written[0].ActorID.IsZero() {
		t.Errorf("the record names %q as the actor, and nobody acted", written[0].ActorID)
	}
}

// A payload that is not what the event catalogue says is a defect in whoever wrote the event, and
// it stops the batch rather than being worked around.
func TestAMalformedPayloadIsRefused(t *testing.T) {
	fixture := consumer(t, bert)

	envelope := assigned(t, anna, bert)
	envelope.Payload["assignee_id"] = 42

	err := fixture.consumer.Deliver(t.Context(), envelope)
	if err == nil {
		t.Fatal("a payload with a number where an identifier belongs was accepted")
	}
	if got := shared.AsError(err).DetailCode; got != "notifications.payload_malformed" {
		t.Errorf("detail %q", got)
	}
}

// A queue that refuses is a real failure: the record and the job commit together, so a job that
// was not written must take the record with it (ADR-0008).
func TestAQueueFailureStopsTheBatch(t *testing.T) {
	fixture := consumer(t, bert)
	fixture.jobs.err = errors.New("the queue is unreachable")

	if err := fixture.consumer.Deliver(t.Context(), assigned(t, anna, bert)); err == nil {
		t.Fatal("a queue failure was swallowed")
	}
}
