// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package mail is the port for the one channel that sends in this milestone (C-09, arc42 §5.2).
//
// It is deliberately the smallest thing that can carry an email: an address, a subject, a body. No
// attachments, no alternative parts, no headers a caller can set - every one of those is a way for
// something to reach an inbox that the sender did not decide, and none of them is needed by a
// message whose whole content is a sentence and a link.
//
// Two rules from elsewhere shape the contract. The body arrives rendered, because rendering is the
// declared exception to rule 8 and happens in the *recipient's* locale before anything gets here
// (i18n-l10n.md §1) - a port that took codes would have to know about locales, and a mail server is
// the last place that knowledge belongs. And no call runs inside a database transaction
// (observability-reliability.md §8): an SMTP server is somebody else's machine, and a transaction
// waiting on one holds a connection for as long as they feel like taking.
package mail

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Message is one email, already rendered.
//
// The sender address is not here: it is the installation's configuration (HUBTASK_SMTP_FROM), and a
// caller that could choose it could send as anybody. What varies per message is who receives it and
// what it says.
type Message struct {
	// To is the recipient's address, as the account carries it. One address: a notification is
	// about one person, and a message with several recipients would tell each of them who else was
	// told (data-protection.md §9 keeps profile data minimal for exactly that reason).
	To string
	// Subject is the rendered subject line, on one line. An adapter refuses one that is not: a
	// newline in a subject is header injection, not a long title.
	Subject string
	// Body is the rendered message. Plain text, because what it contains is a sentence and a link
	// and an HTML alternative would only be a second place for content to appear.
	Body string
}

// ErrRecipientMissing is a message with nowhere to go. Internal rather than validation: whoever
// built this message had a decision in hand that said there was an address (notification.Decide),
// so an empty one here is a defect rather than somebody's input.
var ErrRecipientMissing = shared.ErrInternal.WithDetail("mail.recipient_missing")

// ErrHeaderInjection is a subject or an address carrying a line break.
//
// A separate error from the one above because it is a different kind of wrong: not a value that was
// forgotten, but one that would end the header and start whatever follows it. The rendered subject
// comes from a catalogue and a title, and a title is user content - so this is checked at the
// boundary rather than assumed away (T-06's reasoning, applied to SMTP instead of SQL).
var ErrHeaderInjection = shared.ErrInternal.WithDetail("mail.header_injection")

// Sender delivers a message.
//
// The error contract is the shared one: an unreachable server is ErrUnavailable with a `dependency.`
// detail code, never a raw driver message (T-18). That is what lets the delivery job tell "this
// message is wrong and will be wrong next time" from "come back later", which is the difference
// between a dead letter and a retry.
type Sender interface {
	Send(ctx context.Context, message Message) error
}
