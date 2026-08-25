// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package notification is the Notification context arc42 §5.2 names: what somebody is to be told,
// what they have said about being told it, and how far that got.
//
// The record is the point of the context, not the email. An email that was never sent because the
// mail server was down, one that was not sent because the recipient asked not to hear about
// comments, and one that was sent an hour ago are three different answers to "why did I hear
// nothing", and only a record can give them. The channel is what carries it; the record is what
// happened.
//
// Nothing here holds content. A notification names the entry, the actor and the event, and what an
// email eventually says is read from those rows when it is rendered - which is what keeps the
// deletion path of a title a single path (data-protection.md §5), and what makes an email right
// about an entry that was renamed while the notification waited in the queue.
package notification

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Category is what kind of thing happened, at the granularity a person switches off.
//
// Coarser than the event type on purpose. Somebody who does not want to hear about comments means
// all of them, and a preference per event type would be a settings screen nobody finishes - so
// `comment.created`, `comment.updated` and whatever follows them share one switch.
type Category string

const (
	// CategoryAssignment is work landing on somebody: `item.assigned`.
	CategoryAssignment Category = "ASSIGNMENT"
	// CategoryMembership is being brought into something - an entry's member list
	// (`item.member_added`) or a workspace scope (`membership.granted`). One category, because
	// both are the same sentence to the person reading it: you are now part of this.
	CategoryMembership Category = "MEMBERSHIP"
	// CategoryComment is somebody writing on an entry: `comment.created`.
	CategoryComment Category = "COMMENT"
	// CategoryReminder is a moment somebody asked to be told about: a reminder firing (D-03).
	//
	// Its own category rather than a shade of assignment or membership, because the switch is a
	// different one: somebody who does not want to hear that a comment arrived may still want to
	// hear that their own deadline is an hour away - they asked for that one.
	CategoryReminder Category = "REMINDER"
	// CategoryInvitation is the way in. Its own category and deliberately not part of
	// MEMBERSHIP: an invitation is the message that decides whether somebody can use the system
	// at all, and a preference switching it off would be a preference locking somebody out.
	CategoryInvitation Category = "INVITATION"
)

// Categories is the closed set, in the order the schema's check constraint lists them.
func Categories() []Category {
	return []Category{
		CategoryAssignment, CategoryMembership, CategoryComment, CategoryInvitation,
		CategoryReminder,
	}
}

// Valid reports whether a category is one this system knows.
func (c Category) Valid() bool {
	for _, known := range Categories() {
		if c == known {
			return true
		}
	}
	return false
}

func (c Category) String() string { return string(c) }

// Suppressible reports whether a preference may switch this category off.
//
// Everything but the invitation. The one category whose message is the way into the system cannot
// be refused by a setting inside it - and the setting cannot be reached before the account is
// usable anyway, which makes refusing it a lock nobody can open (data-protection.md §9 asks for
// switchable notifications, not for an unreachable workspace).
func (c Category) Suppressible() bool { return c != CategoryInvitation }

// Channel is what carries a notification. Email is the only one that sends in this milestone;
// webhook and push are named in arc42 §5.2 and arrive with the tasks that own them.
//
// The channel is on the record rather than implied by it because the preference is per channel: a
// person who wants comments in their inbox but not on their phone is expressing two decisions, and
// a record that could not say which channel it was written for could not honour either.
type Channel string

// ChannelEmail is the only channel this milestone sends on.
const ChannelEmail Channel = "EMAIL"

// Channels is the closed set.
func Channels() []Channel { return []Channel{ChannelEmail} }

// Valid reports whether a channel is one this system knows.
func (c Channel) Valid() bool {
	for _, known := range Channels() {
		if c == known {
			return true
		}
	}
	return false
}

func (c Channel) String() string { return string(c) }

// State is how far a notification got.
type State string

const (
	// StatePending is written and waiting for the delivery job.
	StatePending State = "PENDING"
	// StateSent is gone, and SentAt says when.
	StateSent State = "SENT"
	// StateSuppressed is a decision not to send, with the reason that decided it. A state rather
	// than an absent row, because "the record says why" is the whole argument for having records:
	// somebody asking why they heard nothing deserves better than silence.
	StateSuppressed State = "SUPPRESSED"
	// StateFailed is a delivery that used up its attempts. Distinct from suppressed: one is a
	// choice and the other is a fault, and an operator needs to be able to tell them apart.
	StateFailed State = "FAILED"
)

func (s State) String() string { return string(s) }

// The reasons a notification is in the state it is in. Detail codes, never sentences (rule 8), and
// they are the values the record carries - so the set is closed and written by hand.
const (
	// ReasonCategoryOff is the recipient's own decision: a preference row with enabled false.
	ReasonCategoryOff = "notifications.category_off"
	// ReasonNoAddress is a recipient with no email address to send to. A suppression rather than a
	// failure: nothing is going to change by trying again.
	ReasonNoAddress = "notifications.no_address"
	// ReasonSelfCaused is somebody being told about their own action. Suppressed for everybody's
	// sanity: an assignment you made to yourself is not news.
	ReasonSelfCaused = "notifications.self_caused"
	// ReasonDeliveryFailed is what a delivery that used up its attempts leaves behind. The detail
	// of which failure it was belongs in the log and the metric, not in a column a person reads.
	ReasonDeliveryFailed = "notifications.delivery_failed"
)

// Notification is one thing somebody is to be told.
//
// Every reference is an identifier, and that is the invariant of the whole package: there is no
// field here that a title, a note or a comment body could be put in. A renderer reads the entry
// when it renders, and what it may take from it is decided in Content.
type Notification struct {
	ID       shared.ID
	TenantID shared.ID
	// RecipientID is whose inbox this is for. Their locale decides the language the message is
	// rendered in - the recipient's, never the actor's (i18n-l10n.md §1).
	RecipientID shared.ID
	Category    Category
	Channel     Channel
	State       State
	// Reason is why, for the states that need explaining. Empty for PENDING and SENT.
	Reason string
	// EventID is the outbox event that caused this, and what the deduplication is over. Zero for
	// the invitation, which is queued by the use case that created the account rather than by an
	// event (ADR-0007 delivers at-least-once, so a consumer may see the same event twice).
	EventID shared.ID
	// ItemID is the entry this is about, zero where there is none - an invitation is about the
	// workspace.
	ItemID shared.ID
	// ActorID is who caused it, zero where nobody did: the automatic assignment acts for the
	// system rather than for a person (C-02).
	ActorID   shared.ID
	CreatedAt time.Time
	// SentAt is when it left, and is set only in SENT.
	SentAt *time.Time
	// Attempts is what the delivery has tried. The queue counts its own; this is the record's
	// copy, so an operator reading the table sees a stuck notification without joining the job.
	Attempts int
}

// NewInput is what writing a notification needs decided.
type NewInput struct {
	ID          shared.ID
	TenantID    shared.ID
	RecipientID shared.ID
	Category    Category
	Channel     Channel
	EventID     shared.ID
	ItemID      shared.ID
	ActorID     shared.ID
	At          time.Time
}

// New writes a pending notification.
//
// The validation is of the shape rather than of the decision: whether this person should be told
// at all is Decide's business, and it runs before this. What is refused here is a record that
// could not be honoured by anything downstream - no recipient, an unknown category, an unknown
// channel.
func New(in NewInput) (Notification, error) {
	switch {
	case in.TenantID.IsZero():
		return Notification{}, shared.ErrInternal.WithDetail("notifications.tenant_missing")
	case in.RecipientID.IsZero():
		return Notification{}, shared.ErrInternal.WithDetail("notifications.recipient_missing")
	case !in.Category.Valid():
		return Notification{}, shared.ErrInternal.
			WithDetail("notifications.category_unknown").
			WithParams(map[string]string{"value": string(in.Category)})
	case !in.Channel.Valid():
		return Notification{}, shared.ErrInternal.
			WithDetail("notifications.channel_unknown").
			WithParams(map[string]string{"value": string(in.Channel)})
	}

	return Notification{
		ID:          in.ID,
		TenantID:    in.TenantID,
		RecipientID: in.RecipientID,
		Category:    in.Category,
		Channel:     in.Channel,
		State:       StatePending,
		EventID:     in.EventID,
		ItemID:      in.ItemID,
		ActorID:     in.ActorID,
		CreatedAt:   in.At,
	}, nil
}

// Suppress records a decision not to send, and why.
//
// It is applied to a record rather than replacing one: a suppressed notification is written like
// any other, so the table answers "why did I hear nothing" instead of having nothing to say.
func (n Notification) Suppress(reason string) Notification {
	n.State = StateSuppressed
	n.Reason = reason
	return n
}

// Sent records that the message left.
func (n Notification) Sent(at time.Time) Notification {
	n.State = StateSent
	n.Reason = ""
	n.SentAt = &at
	return n
}

// Failed records an attempt that did not work.
//
// `final` is the queue's judgement rather than this package's: the attempt budget lives on the job
// (queue.Job.LastAttempt), and a record that counted its own would be a second budget disagreeing
// with the first. Until it is final the record stays PENDING - it is still going to be sent, and a
// row that read FAILED between two attempts would have an operator chasing a working system.
func (n Notification) Failed(final bool) Notification {
	n.Attempts++
	if final {
		n.State = StateFailed
		n.Reason = ReasonDeliveryFailed
	}
	return n
}

// Pending reports whether the delivery still has to do something about this.
func (n Notification) Pending() bool { return n.State == StatePending }
