// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package jumble is the inbox before the work exists (domain-model.md §2, G-10): entries arrive
// over four channels - a mail, a webhook, a quick capture, a plain API call - and become work
// items by conversion, or age out by dismissal.
//
// Jumble content is the least trusted text in the system, and this model treats it that way. The
// raw subject and body are data that arrived from outside: they are stored to be read by a
// person and matched by a rule as *data*, never rendered as instructions to anything
// (ai-first.md), never logged (rule 10), and bounded before anything else looks at them. The
// sender is data too, not an identity - a From header authenticates nothing.
package jumble

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Channel is how an entry arrived.
type Channel string

const (
	// ChannelEmail is a mail, parsed by the mail intake (G-11).
	ChannelEmail Channel = "EMAIL"
	// ChannelWebhook is the tenant's token-protected intake URL.
	ChannelWebhook Channel = "WEBHOOK"
	// ChannelQuickCapture is the product's own capture surface.
	ChannelQuickCapture Channel = "QUICK_CAPTURE"
	// ChannelAPI is a plain authenticated API call.
	ChannelAPI Channel = "API"
)

// Valid reports whether the channel is one the column allows.
func (c Channel) Valid() bool {
	switch c {
	case ChannelEmail, ChannelWebhook, ChannelQuickCapture, ChannelAPI:
		return true
	default:
		return false
	}
}

func (c Channel) String() string { return string(c) }

// Status is where an entry stands.
type Status string

const (
	// StatusNew is an entry nothing has decided about yet - the only state that can be converted
	// or dismissed.
	StatusNew Status = "NEW"
	// StatusProcessed is an entry that became work: a conversion produced an item and recorded it.
	StatusProcessed Status = "PROCESSED"
	// StatusDismissed is an entry somebody decided against. A state, not a deletion: the entry
	// stays readable, and the retention engine ages it out by rule rather than by hand.
	StatusDismissed Status = "DISMISSED"
)

// Valid reports whether the status is one the column allows.
func (s Status) Valid() bool {
	switch s {
	case StatusNew, StatusProcessed, StatusDismissed:
		return true
	default:
		return false
	}
}

// The bounds, checked before anything is stored. A jumble exists to catch, and the way to stay
// able to catch is to refuse what would drown it: the parser and the intake both run under these
// before allocation (G-11).
const (
	// MaxSubjectLength bounds the raw subject, in runes. Mail subjects top out far below this.
	MaxSubjectLength = 500
	// MaxBodyBytes bounds the raw body. Bytes rather than runes, because what is being bounded is
	// storage and evaluation cost - this document becomes a CEL activation - and a byte count is
	// the honest measure of both.
	MaxBodyBytes = 64 * 1024
	// MaxSenderLength bounds the sender, in runes. The longest address RFC 5321 allows is 320,
	// and a sender is an address or nothing.
	MaxSenderLength = 320
	// MaxAttachments bounds how many media objects one entry may carry.
	MaxAttachments = 20
)

// Entry is one arrival, as it came in.
type Entry struct {
	ID       shared.ID
	TenantID shared.ID
	Channel  Channel
	// Sender is who the transport says it came from. Data, never an identity: a From header
	// authenticates nothing - the intake token does (G-10) - and the value is stored so a person
	// can judge provenance, not so the system can trust it.
	Sender     string
	RawSubject string
	RawBody    string
	// Attachments are media object identifiers, already stored through C-05's pipeline with its
	// size and type discipline - never a second storage path.
	Attachments []shared.ID
	// Suggestion is the AI's proposal for what this entry should become (0.7.0). Stored as an
	// opaque document until the port that writes it exists.
	Suggestion map[string]any
	Status     Status
	// TargetItemID is the item a conversion produced, and zero before one did. The other half of
	// the provenance: the item carries origin_jumble_id, and the two point at each other.
	TargetItemID shared.ID
	ReceivedAt   time.Time
	// SettledAt is when the entry stopped being NEW - a conversion or a dismissal. Stored in the
	// `processed_at` column, which predates the dismissal being a state of its own.
	SettledAt *time.Time
}

// NewEntryInput is what an arrival needs.
type NewEntryInput struct {
	ID          shared.ID
	TenantID    shared.ID
	Channel     Channel
	Sender      string
	RawSubject  string
	RawBody     string
	Attachments []shared.ID
	Now         time.Time
}

// NewEntry validates one arrival.
//
// The bounds are checked here, at the door, with the field named - a crafted payload costs a
// refusal rather than storage, and the refusal points at the field its author has to shorten.
func NewEntry(in NewEntryInput) (Entry, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return Entry{}, shared.ErrInternal.WithDetail("jumble.entry_incomplete")
	}

	var findings []shared.FieldError
	if !in.Channel.Valid() {
		findings = append(findings, shared.FieldError{
			Path: "/channel", Code: "jumble.channel_unknown",
		})
	}

	sender := strings.TrimSpace(in.Sender)
	subject := strings.TrimSpace(in.RawSubject)
	body := in.RawBody
	switch {
	case utf8.RuneCountInString(sender) > MaxSenderLength:
		findings = append(findings, shared.FieldError{
			Path: "/sender", Code: "jumble.sender_too_long",
		})
	case utf8.RuneCountInString(subject) > MaxSubjectLength:
		findings = append(findings, shared.FieldError{
			Path: "/raw_subject", Code: "jumble.subject_too_long",
		})
	case len(body) > MaxBodyBytes:
		findings = append(findings, shared.FieldError{
			Path: "/raw_body", Code: "jumble.body_too_large",
		})
	case len(in.Attachments) > MaxAttachments:
		findings = append(findings, shared.FieldError{
			Path: "/attachments", Code: "jumble.too_many_attachments",
		})
	}

	if subject == "" && strings.TrimSpace(body) == "" && len(in.Attachments) == 0 {
		// An entry with nothing in it is nothing to catch: no text to read, no file to keep.
		findings = append(findings, shared.FieldError{
			Path: "/raw_body", Code: "jumble.entry_empty",
		})
	}

	if len(findings) > 0 {
		return Entry{}, shared.ErrValidation.
			WithDetail("jumble.entry_invalid").
			WithFields(findings...)
	}

	attachments := make([]shared.ID, len(in.Attachments))
	copy(attachments, in.Attachments)
	return Entry{
		ID: in.ID, TenantID: in.TenantID, Channel: in.Channel,
		Sender: sender, RawSubject: subject, RawBody: body,
		Attachments: attachments,
		Status:      StatusNew,
		ReceivedAt:  in.Now.UTC(),
	}, nil
}

// Convert marks the entry as become work.
//
// Only a NEW entry converts, which is what makes a second conversion of the same entry a refusal
// rather than a second item: the first conversion settled the entry, and the refusal names the
// state it found.
func (e Entry) Convert(target shared.ID, at time.Time) (Entry, error) {
	if target.IsZero() {
		return Entry{}, shared.ErrInternal.WithDetail("jumble.entry_incomplete")
	}
	if err := e.mustBeNew(); err != nil {
		return Entry{}, err
	}

	settled := at.UTC()
	e.Status, e.TargetItemID, e.SettledAt = StatusProcessed, target, &settled
	return e, nil
}

// Dismiss marks the entry as decided against. A state, not a deletion: the entry stays readable,
// and the retention engine ages it out by rule (data-retention.md §3, G-10).
func (e Entry) Dismiss(at time.Time) (Entry, error) {
	if err := e.mustBeNew(); err != nil {
		return Entry{}, err
	}

	settled := at.UTC()
	e.Status, e.SettledAt = StatusDismissed, &settled
	return e, nil
}

// mustBeNew is the one state rule both settlements share: an entry is decided about exactly once,
// and the refusal says what already decided it.
func (e Entry) mustBeNew() error {
	if e.Status == StatusNew {
		return nil
	}
	return shared.ErrConflict.
		WithDetail("jumble.entry_settled").
		WithParams(map[string]string{"status": string(e.Status)})
}
