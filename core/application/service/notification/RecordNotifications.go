// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package notification is the application half of the Notification context: the consumer that
// turns events into records, and the delivery that turns a record into a message (C-09).
//
// Neither is a use case, and both are deliberately absent from the catalogue in domain-model.md §5.
// The catalogue is the list of things a person, an agent or a rule can ask for (arc42 §4), and
// "notify everybody about this event, now" is not one of them - the way to influence what is sent
// is the preference, not a call. Registering them would put a "send email to everybody" action on
// three channels, which is a button nobody should be given.
package notification

import (
	"context"
	"errors"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// ConsumerName is how this subscriber is known in the deduplication and in the logs.
//
// A constant, and stable across versions: renaming it makes every event it has already seen look
// new, which for this subscriber means sending every notification of the retention window again
// (core/port/eventbus).
const ConsumerName = "notification"

// Queue is the slice of the job queue this package needs. Narrow rather than the whole port, so
// that what a consumer can do to the queue is visible in one line: it can ask for work, and it
// cannot claim, complete or fail anything.
type Queue interface {
	Enqueue(ctx context.Context, request queue.Request) error
}

// Entries and Members are the slices of the work repositories this package reads, and they are
// slices for the reason Queue is: a consumer that held the whole item repository could complete an
// entry, move it, or write its children, and none of that is anything a notification does. What is
// declared here is the entire reach of the notification path into the work management context -
// one read of an entry, one read of its member list.
type Entries interface {
	Find(ctx context.Context, id shared.ID) (work.WorkItem, error)
}

// Members is one entry's member list.
type Members interface {
	List(ctx context.Context, itemID shared.ID) ([]shared.ID, error)
}

// RecordNotifications is the outbox consumer: it decides who is to be told about an event and
// writes one record per person.
//
// It runs inside the dispatcher's transaction, which is the whole reliability argument (ADR-0007):
// the records it writes, the delivery jobs it queues and the consumption record that says this
// event was handled all commit together. A process that dies halfway leaves none of them, and the
// event is handed over again.
//
// It reaches nothing outside the database, and that is the rule rather than an accident. The email
// is a job, because an SMTP server inside a transaction holds a database connection for as long as
// somebody else's machine feels like taking (observability-reliability.md §8) - and because the
// comment that caused the notification must commit whether or not anybody can be told about it.
type RecordNotifications struct {
	Notifications repository.Notifications
	Preferences   repository.Preferences
	Accounts      identityrepo.Accounts
	Items         Entries
	ItemMembers   Members
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
	// Signals is the observability slice. Optional: the consumer runs without it, which is what
	// keeps a metrics adapter from being a dependency of the notification path.
	Signals Signals
}

// Signals is the slice of the metrics adapter this package reports through
// (observability-reliability.md §3.2).
//
// The labels are the closed sets the domain defines - the category, the channel, the state and the
// suppression reasons - so there is no way for an unbounded label to reach a metric from here.
type Signals interface {
	NotificationRecorded(ctx context.Context, category, channel, state string)
	NotificationSent(ctx context.Context, category, channel string, seconds float64)
	NotificationFailed(ctx context.Context, category, channel, reason string)
}

var _ eventbus.Subscriber = RecordNotifications{}

// Name identifies the subscriber in the deduplication and in the logs.
func (r RecordNotifications) Name() string { return ConsumerName }

// Wants reports whether this event concerns anybody.
//
// The three C-01 and C-03 publish. `membership.granted` is the fourth thing this milestone tells
// people about and it is not here, because it is not an event: B-02 queues the invitation in the
// transaction that creates the account, and this task makes that job's adapter real rather than
// inventing an event for a message that already has a delivery path.
func (r RecordNotifications) Wants(eventType event.Type) bool {
	_, wanted := categories[eventType]
	return wanted
}

// categories maps an event to the switch a person flicks. The map is the closed set: an event type
// that is not in it is not wanted, so there is no path from a new event to an unclassified record.
var categories = map[event.Type]domain.Category{
	event.ItemAssigned:    domain.CategoryAssignment,
	event.ItemMemberAdded: domain.CategoryMembership,
	event.CommentCreated:  domain.CategoryComment,
}

// Deliver writes one record per person the event concerns.
//
// An error stops the dispatcher's whole batch, which is why the only errors it produces are real
// ones: a recipient whose account has gone is not an error but a record nobody can be sent, and a
// preference that says no is not an error but a record that says why.
func (r RecordNotifications) Deliver(ctx context.Context, envelope event.Envelope) error {
	category, wanted := categories[envelope.Type]
	if !wanted {
		return nil
	}

	itemID, err := itemOf(envelope)
	if err != nil {
		return err
	}
	recipients, err := r.recipients(ctx, envelope, itemID)
	if err != nil {
		return err
	}

	for _, recipient := range recipients {
		if err := r.record(ctx, envelope, category, itemID, recipient); err != nil {
			return err
		}
	}
	return nil
}

// recipients is who this event concerns, without duplicates and in a stable order.
//
// Authorisation is not asked here, and it does not have to be: every recipient is derived from a
// membership of the entry itself - its assignee, its member list - which is the narrowing C-04
// built (ADR-0005 puts the decision in the application layer, and this is it). Somebody who was
// never put on the entry is never in this list.
func (r RecordNotifications) recipients(
	ctx context.Context, envelope event.Envelope, itemID shared.ID,
) ([]shared.ID, error) {
	switch envelope.Type {
	case event.ItemAssigned:
		return oneOf(envelope, "assignee_id")
	case event.ItemMemberAdded:
		return oneOf(envelope, "account_id")
	case event.CommentCreated:
		return r.everybodyOn(ctx, itemID)
	default:
		return nil, nil
	}
}

// everybodyOn is who a comment concerns: the entry's assignee and everybody on its member list.
//
// Not its watchers - the schema reserves the set name and nothing writes it yet, and a recipient
// list drawn from an empty set would be a promise nothing keeps. Whoever wrote the comment is in
// this list and is filtered out by the domain's self-caused rule rather than here, so that the
// record says why they heard nothing instead of there being no record at all.
func (r RecordNotifications) everybodyOn(
	ctx context.Context, itemID shared.ID,
) ([]shared.ID, error) {
	if itemID.IsZero() {
		return nil, nil
	}

	item, err := r.Items.Find(ctx, itemID)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		// The entry went between the comment and the dispatch. Nobody to tell, and not an error:
		// stopping the batch over a deleted entry would stop every other tenant's notifications
		// with it.
		return nil, nil
	case err != nil:
		return nil, err
	}

	members, err := r.ItemMembers.List(ctx, itemID)
	if err != nil {
		return nil, err
	}

	seen := make(map[shared.ID]bool, len(members)+1)
	recipients := make([]shared.ID, 0, len(members)+1)
	for _, candidate := range append([]shared.ID{item.AssigneeID}, members...) {
		if candidate.IsZero() || seen[candidate] {
			continue
		}
		seen[candidate] = true
		recipients = append(recipients, candidate)
	}
	return recipients, nil
}

// record writes one person's record and, where it is to be sent, the job that sends it.
func (r RecordNotifications) record(
	ctx context.Context, envelope event.Envelope, category domain.Category,
	itemID, recipientID shared.ID,
) error {
	written, err := domain.New(domain.NewInput{
		ID:          r.IDs.NewID(),
		TenantID:    envelope.TenantID,
		RecipientID: recipientID,
		Category:    category,
		Channel:     domain.ChannelEmail,
		EventID:     envelope.ID,
		ItemID:      itemID,
		ActorID:     actorOf(envelope),
		At:          r.Clock.Now(),
	})
	if err != nil {
		return err
	}

	decision, err := r.decide(ctx, written, recipientID, category)
	if err != nil {
		return err
	}
	if !decision.Send {
		written = written.Suppress(decision.Reason)
	}

	first, err := r.Notifications.Insert(ctx, written)
	if err != nil {
		return err
	}
	if !first {
		// Somebody already wrote this record: the same event delivered twice, which the outbox's
		// at-least-once guarantee makes normal (ADR-0007). Queueing the delivery again would be
		// the second email the deduplication exists to prevent.
		return nil
	}

	r.report(ctx, written)
	if !decision.Send {
		return nil
	}
	return r.Jobs.Enqueue(ctx, queue.Request{
		Kind:     queue.KindNotificationDeliver,
		TenantID: envelope.TenantID,
		// One pending delivery per record. The record's own identifier, so a retried consumption
		// cannot queue a second send of the same message.
		DedupeKey: written.ID.String(),
		Payload: map[string]any{
			// The identifier and nothing else. Everything the delivery needs is read when it runs,
			// which is what makes the email right about an entry that was renamed while it waited -
			// and what keeps personal data out of a table nothing cleans (rule 10).
			"notification_id": written.ID.String(),
		},
	})
}

// decide asks the domain whether this person is told, having read what they said.
func (r RecordNotifications) decide(
	ctx context.Context, written domain.Notification, recipientID shared.ID,
	category domain.Category,
) (domain.Decision, error) {
	account, err := r.Accounts.Find(ctx, recipientID)
	if errors.Is(err, shared.ErrNotFound) {
		// The account went between the event and the dispatch. There is nowhere to send, which is
		// what the domain calls it - and no record either, because the foreign key would refuse
		// one. Reported as a decision so the caller's insert fails cleanly rather than here.
		return domain.Decision{Reason: domain.ReasonNoAddress}, nil
	}
	if err != nil {
		return domain.Decision{}, err
	}

	preference, err := r.Preferences.Find(ctx, recipientID, category, domain.ChannelEmail)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		// Saying nothing is the default, and the default is on. Written down once, in the domain.
		preference = domain.DefaultPreference(
			written.TenantID, recipientID, category, domain.ChannelEmail)
	case err != nil:
		return domain.Decision{}, err
	}

	return domain.Decide(written, domain.Recipient{
		AccountID:  recipientID,
		HasAddress: account.Email != "",
	}, preference), nil
}

func (r RecordNotifications) report(ctx context.Context, written domain.Notification) {
	if r.Signals == nil {
		return
	}
	r.Signals.NotificationRecorded(ctx,
		written.Category.String(), written.Channel.String(), written.State.String())
}

// itemOf reads the entry an event is about. Empty where the payload names none, which today is
// nothing - every event this consumer wants is about an entry.
func itemOf(envelope event.Envelope) (shared.ID, error) {
	ids, err := oneOf(envelope, "item_id")
	if err != nil || len(ids) == 0 {
		return "", err
	}
	return ids[0], nil
}

// oneOf reads one identifier out of a payload, and refuses what is not one.
//
// A payload is data that outlived the process that wrote it (core/port/queue says the same about a
// job), so a key that is missing or is not a string is a defect in whoever wrote the event rather
// than something to work around. Parsed rather than trusted: an identifier that reaches a query
// unparsed is the shape of T-06 even when the query is parameterised.
func oneOf(envelope event.Envelope, key string) ([]shared.ID, error) {
	raw, present := envelope.Payload[key]
	if !present {
		return nil, nil
	}
	text, ok := raw.(string)
	if !ok || text == "" {
		return nil, shared.ErrInternal.
			WithDetail("notifications.payload_malformed").
			WithParams(map[string]string{"field": key})
	}
	id, err := shared.ParseID(text)
	if err != nil {
		return nil, shared.ErrInternal.
			WithDetail("notifications.payload_malformed").
			WithParams(map[string]string{"field": key})
	}
	return []shared.ID{id}, nil
}

// actorOf is who caused the event, where a person did. Zero for the system and for automation: the
// automatic assignment acts for nobody (C-02), and a record that named a person who did not act
// would suppress their own notification for a decision they never made.
func actorOf(envelope event.Envelope) shared.ID {
	if envelope.Actor.Kind != shared.ActorUser {
		return ""
	}
	return envelope.Actor.ID
}
