// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"strings"
	"unicode/utf8"

	repository "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// AttachmentIngest is the half of the media pipeline an intake needs (G-11): bytes this server
// already holds become sealed objects, with no actor to authorise.
//
// Declared here and satisfied by the media service, the way the conversion declares the slice of
// the registry it needs: what an intake may do to the media store is exactly this and nothing
// else - it cannot read an object, attach one, or remove one.
type AttachmentIngest interface {
	Execute(
		ctx context.Context, tenantID shared.ID, files []mediaservice.IngestedFile,
	) ([]shared.ID, error)
}

// IntakeMail turns one inbound message into a jumble entry (G-11).
//
// The transport-independent half of the mail intake. What reaches this is a message somebody has
// already parsed - a bridge's delivery today, a JMAP fetch if one is ever built (ADR-0040) - and
// what it does with one is the same either way: authenticate the tenant by its token, store what
// the message carried, and announce the arrival.
//
// Not a catalogue entry, for IntakeJumbleEntry's reason: there is no actor to authorise, because
// the token authenticates a tenant and never a person.
//
// The order of the three steps is the security posture. The token is checked *before* a byte is
// stored, so an unknown address costs a lookup rather than a file in somebody's bucket; the files
// are stored before the entry, so an entry never names an object that does not exist; and a file
// that fails leaves the entry with the rest, because an inbox is for catching things.
type IntakeMail struct {
	Intake  repository.Intake
	Entries repository.Entries
	// Media is optional. An installation wired without it takes mail without its files rather
	// than refusing mail - the message is what somebody sent, and the attachments are what came
	// with it.
	Media      AttachmentIngest
	Events     outboxEvents
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// MailDelivery is one message on the wire, parsed into what an entry is made of.
//
// The raw payload travels beside the parsed halves rather than instead of them, because a message
// that defeated the parser still has to land: Unparseable says which of the two this is, and the
// bytes are what the entry then carries.
type MailDelivery struct {
	Token   integration.InboundToken
	Sender  string
	Subject string
	Body    string
	// Attachments are the files the message carried, already decoded and bounded by whoever
	// parsed it. They are bytes here and nothing more - what they turn out to be is the media
	// guard's answer, not the sender's claim (T-11).
	Attachments []mediaservice.IngestedFile
	// Raw is the payload as it arrived. Carried for the case below, and empty otherwise.
	Raw []byte
	// Unparseable says the parser could not read the payload as a message. The entry is written
	// anyway - a jumble exists to catch, and "unparseable" is a thing to catch.
	Unparseable bool
}

// RawMailFileName is what the payload of an unreadable message is called.
const RawMailFileName = "message.eml"

// Execute stores one delivery.
func (h IntakeMail) Execute(
	ctx context.Context, delivery MailDelivery,
) (domain.Entry, error) {
	if delivery.Token.IsZero() {
		return domain.Entry{}, intakeGone()
	}

	tenantID := delivery.Token.TenantID()
	scope := persistence.Scope{TenantID: tenantID}

	// Before anything is stored. An unknown address costs one indexed lookup, and every reason not
	// to serve answers the same not-found (T-21, G-10).
	if err := h.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		opens, err := h.Intake.VerifyToken(ctx, delivery.Token)
		if err != nil {
			return err
		}
		if !opens {
			return intakeGone()
		}
		return nil
	}); err != nil {
		return domain.Entry{}, err
	}

	files := delivery.Attachments
	if delivery.Unparseable {
		// The payload itself, with no claim about what it is: the sniff decides, which is the
		// honest reading of bytes nobody could parse.
		files = append(files, mediaservice.IngestedFile{
			FileName: RawMailFileName, Content: delivery.Raw,
		})
	}

	stored, err := h.ingest(ctx, tenantID, files)
	if err != nil {
		return domain.Entry{}, err
	}

	entry, err := domain.NewEntry(domain.NewEntryInput{
		ID:       h.IDs.NewID(),
		TenantID: tenantID,
		Channel:  domain.ChannelEmail,
		Sender:   senderWithin(delivery.Sender),
		// Truncated rather than refused, both of them. The bounds exist so that a crafted message
		// costs a refusal instead of storage; a *legitimate* message with a long subject is not
		// what they are aimed at, and losing it entirely to a header nobody reads twice would be
		// the inbox failing at its one job.
		RawSubject:  cut(delivery.Subject, domain.MaxSubjectLength),
		RawBody:     bodyFor(delivery),
		Attachments: stored,
		Now:         h.Clock.Now(),
	})
	if err != nil {
		return domain.Entry{}, err
	}

	if err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := h.Entries.Insert(ctx, entry); err != nil {
			return err
		}
		if h.Events == nil {
			return nil
		}
		// The system as the actor: the token authenticates the tenant, and naming an account would
		// invent an author for something nobody in this workspace did (G-10).
		envelope, err := event.NewJumbleEntryReceived(
			h.IDs.NewID(), entry, event.Actor{Kind: shared.ActorSystem}, entry.ReceivedAt,
			event.Cause{})
		if err != nil {
			return err
		}
		return h.Events.Append(ctx, envelope)
	}); err != nil {
		return domain.Entry{}, err
	}
	return entry, nil
}

// ingest stores the files, and answers none where there is nothing to store them with.
func (h IntakeMail) ingest(
	ctx context.Context, tenantID shared.ID, files []mediaservice.IngestedFile,
) ([]shared.ID, error) {
	if h.Media == nil || len(files) == 0 {
		return nil, nil
	}
	return h.Media.Execute(ctx, tenantID, files)
}

// bodyFor is the text the entry carries.
//
// For a message the parser read, its text. For one it could not, the payload rendered as text and
// bounded - beside the attachment holding the bytes themselves, because the two answer different
// questions: the attachment is what a person forwards to somebody who can read it, and the body is
// what they see without downloading anything.
func bodyFor(delivery MailDelivery) string {
	if !delivery.Unparseable {
		return cutBytes(delivery.Body, domain.MaxBodyBytes)
	}
	return cutBytes(printable(string(delivery.Raw)), domain.MaxBodyBytes)
}

// senderWithin keeps a sender that could be an address and drops one that could not.
//
// Dropped rather than truncated, because half an address is not a shorter address - it is a
// different one, and the whole point of the field is that a person can judge where something came
// from.
func senderWithin(sender string) string {
	sender = strings.TrimSpace(sender)
	if utf8.RuneCountInString(sender) > domain.MaxSenderLength {
		return ""
	}
	return sender
}

// cut shortens text to a rune count, on a rune boundary.
func cut(value string, runes int) string {
	if utf8.RuneCountInString(value) <= runes {
		return value
	}
	counted := 0
	for index := range value {
		if counted == runes {
			return value[:index]
		}
		counted++
	}
	return value
}

// cutBytes shortens text to a byte bound, on a rune boundary: a string cut mid-rune carries an
// invalid byte, and stored text is text.
func cutBytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	cutAt := limit
	for cutAt > 0 && !utf8.RuneStart(value[cutAt]) {
		cutAt--
	}
	return value[:cutAt]
}

// printable renders arbitrary bytes as text: what is text stays, and what is not is dropped.
//
// Not an escape and not a hex dump. What this produces is read by a person deciding whether the
// message matters, and the readable half of a damaged payload is what tells them.
func printable(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for _, r := range value {
		switch {
		case r == utf8.RuneError:
			continue
		case r == '\t' || r == '\n' || r == '\r':
			out.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			continue
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
