// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Outcome is what a message became.
type Outcome string

const (
	OutcomeEntry     Outcome = "ENTRY"
	OutcomeComment   Outcome = "COMMENT"
	OutcomeJumble    Outcome = "JUMBLE"
	OutcomeDiscarded Outcome = "DISCARDED"
)

// Reason says why, where the outcome is not the obvious one: why a message waits in the jumble or
// was dropped. Empty for a message filed where it said it should go.
type Reason string

const (
	ReasonNone Reason = ""
	// ReasonNoRoute: the message named no place and the workspace has no default.
	ReasonNoRoute Reason = "NO_ROUTE"
	// ReasonUnknownPlace: it named a handle or key that does not exist (any more).
	ReasonUnknownPlace Reason = "UNKNOWN_PLACE"
	// ReasonSenderNotAllowed: the level does not admit the sender.
	ReasonSenderNotAllowed Reason = "SENDER_NOT_ALLOWED"
	// ReasonNotPermitted: the sender is a member who may not write there.
	ReasonNotPermitted Reason = "NOT_PERMITTED"
	// ReasonSpam: the provider marked the message.
	ReasonSpam Reason = "SPAM"
	// ReasonUnparseable: the message could not be read; the raw message is attached.
	ReasonUnparseable Reason = "UNPARSEABLE"
	// ReasonBlocked: the block list names the sender. Discarded.
	ReasonBlocked Reason = "SENDER_BLOCKED"
	// ReasonAutomatic: an auto-reply or a bounce, or one of Hubtask's own notifications. Discarded.
	ReasonAutomatic Reason = "AUTOMATIC"
	// ReasonTooMany: the sender passed the hourly bound. Discarded, so a flood fills nothing.
	ReasonTooMany Reason = "TOO_MANY"
)

// MailsPerSenderPerHour is how many messages one sender may send into a workspace in an hour.
const MailsPerSenderPerHour = 30

// Receipt is the intake's record of one message: it makes a redelivery a no-op and is the log
// the administrators read (ADR-0094 decision 6).
type Receipt struct {
	ID         shared.ID
	MessageID  string
	ReceivedAt time.Time
	Sender     string
	Subject    string
	Outcome    Outcome
	Reason     Reason
	ItemID     shared.ID
	EntryID    shared.ID
	// Door is how it arrived: "inbound" for the operator's service, or the mailbox's address.
	Door string
}
