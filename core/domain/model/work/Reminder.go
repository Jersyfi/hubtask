// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Reminder is one promise to say something about an entry at a particular moment
// (domain-model.md §3.5).
//
// Its own entity rather than a field of the item, because an entry may carry several and each has
// its own moment, its own channels and its own recipients. What it does not carry is a message:
// nothing here holds content, and what an email eventually says is read from the entry when it is
// rendered - which is what keeps the deletion path of a title a single path
// (data-protection.md §5) and what makes a reminder right about an entry renamed while it waited.
//
// "Predefined plus custom" (§2) is the two forms of the offset and not a preset table: a preset is
// a client's vocabulary for a common relative offset, and what is stored is the offset itself.
type Reminder struct {
	ID       shared.ID
	TenantID shared.ID
	ItemID   shared.ID
	// Offset is when to fire, expressed against the entry's due date or against the clock.
	Offset ReminderOffset
	// Channels is what carries it. A list because the preference is per channel and somebody may
	// want both; never empty, because a reminder nothing carries is a row that reminds nobody.
	Channels []ReminderChannel
	// Recipients is who is reminded, and empty means the assignee and the entry's members.
	//
	// Empty is stored as empty and resolved when the reminder fires (D-03), never expanded here:
	// a list written down today would remind exactly the people who were on the entry today, and
	// somebody added tomorrow would never hear about it.
	Recipients []shared.ID
	State      ReminderState
	// FireAt is the computed moment, and nil when there is none: a relative reminder whose entry
	// has no due date has nothing to count from. Derived rather than given - a client sends the
	// offset, the server owns the instant - which is why it never merges (offline-sync.md §4.2).
	FireAt    *time.Time
	CreatedAt time.Time
	UpdatedAt *time.Time
	Version   int
}

// The reminder's own field names, as the contract spells them. They travel into the change log,
// where a client matches them against the members it sent (offline-sync.md §4.2).
const (
	FieldOffsetSpec = "offset_spec"
	FieldChannels   = "channels"
	FieldRecipients = "recipients"
)

// MaxRemindersPerItem bounds what one entry may carry.
//
// A bound rather than a page, and the reason the list is unpaged: reminders on one entry are a
// handful by nature, and a limit is a better answer than a cursor through something that should
// never be long. It is published in /meta/capabilities, so a client knows the number rather than
// discovering it (api-guidelines.md §1).
const MaxRemindersPerItem = 25

// MaxReminderRecipients bounds the named list. Naming everybody is what the empty list is for, so
// a long list is not the way anybody uses this - which is what makes a bound safe here (T-17).
const MaxReminderRecipients = 50

// MaxReminderOffsetSpecLength is the contract's maxLength in code points, so that a refusal comes
// from the same number the specification declares.
const MaxReminderOffsetSpecLength = 64

// DueSoonLead is how far ahead of a deadline `item.due_soon` is announced, and DueSoonThresholdSpec
// is the same value as the event carries it (D-03, domain-model.md §4).
//
// A fixed lead rather than one derived from the entry's own reminders, and the decision is worth
// keeping: a rule that reacts to "due soon" has to mean the same thing for every entry in a
// workspace, and a threshold that followed whatever reminder somebody happened to set would make
// the event fire at the same moment as that reminder - telling automation exactly what the
// reminder already said, and saying nothing at all about an entry nobody set one on.
const (
	DueSoonLead          = 24 * time.Hour
	DueSoonThresholdSpec = "PT24H"
)

// MaxReminderOffset is the longest relative offset that means anything: ten years before or after
// a due date. Not a technical limit but a plausibility one - a duration beyond it is a typo or an
// attempt to overflow the arithmetic, and neither should reach the scheduler.
const MaxReminderOffset = 3650 * 24 * time.Hour

// ReminderState is how far a reminder got.
//
// The states are the schema's, and only the first is a client's business: a reminder is written
// PENDING, the scheduler moves it to SENT when it has fired (D-03), and CANCELLED is the answer
// for one that never will. Deleting a reminder removes the row rather than cancelling it - what
// somebody deleted is gone, and a tombstone with a state would be a reminder they could still see.
type ReminderState string

const (
	ReminderPending   ReminderState = "PENDING"
	ReminderSent      ReminderState = "SENT"
	ReminderCancelled ReminderState = "CANCELLED"
	// ReminderLapsed is a reminder whose moment passed while the data was in an archive
	// (backup-restore.md §8.4, E-06). It is set by a restore and by nothing else.
	//
	// Not CANCELLED, which would be the cheap answer and the wrong one: somebody cancelled that,
	// and an auditor reading a workspace after a restore would find hundreds of cancellations
	// nobody made. Not PENDING either - a restore that left them pending would have the
	// scheduler send every one of them at once, which is exactly what §8.4 forbids.
	ReminderLapsed ReminderState = "LAPSED"
)

// ReminderStates is the closed set, in the order the schema's check constraint lists them.
func ReminderStates() []ReminderState {
	return []ReminderState{ReminderPending, ReminderSent, ReminderCancelled, ReminderLapsed}
}

// Valid reports whether a state is one this system knows.
func (s ReminderState) Valid() bool {
	for _, known := range ReminderStates() {
		if s == known {
			return true
		}
	}
	return false
}

func (s ReminderState) String() string { return string(s) }

// ReminderChannel is what carries a reminder.
//
// The same set the notification context sends on, spelled where a reminder validates it: the
// domain models are independent of one another (ADR-0001), so the two lists cannot be one constant
// - and a test in the application layer, which may see both, holds them to each other.
type ReminderChannel string

// ReminderChannelEmail is the one channel this installation sends on. Webhook and push are named
// in arc42 §5.2 and arrive with the tasks that build them.
const ReminderChannelEmail ReminderChannel = "EMAIL"

// ReminderChannels is the closed set.
func ReminderChannels() []ReminderChannel { return []ReminderChannel{ReminderChannelEmail} }

// Valid reports whether a channel is one this system knows.
func (c ReminderChannel) Valid() bool {
	for _, known := range ReminderChannels() {
		if c == known {
			return true
		}
	}
	return false
}

func (c ReminderChannel) String() string { return string(c) }

// The two forms of an offset, as the column spells them (0001_init).
const (
	relativeOffsetPrefix = "REL:"
	absoluteOffsetPrefix = "ABS:"
)

// ReminderOffset is when a reminder fires, in one of exactly two forms.
//
// The text travels with the parsed value deliberately. What is stored is what was given - two
// spellings of the same hour are two strings, and rewriting one into the other would be the server
// answering a client with words it did not choose - while the members beside it are what any
// arithmetic uses, so nothing has to re-parse a string to know what it means.
type ReminderOffset struct {
	// Spec is the stored text: `REL:-PT1H` or `ABS:2026-09-01T08:00:00Z`.
	Spec string
	// Relative marks the first form. A relative offset counts from the entry's due date and is
	// negative for before it; an absolute one is a moment no due date moves.
	Relative bool
	Duration time.Duration
	Instant  time.Time
}

// The reasons a spec can fail to be one, separated because the caller answers each with its own
// code: a duration in years is well formed and refused for what it means, which is a different
// sentence from "this is not a duration".
var (
	errOffsetMalformed  = errors.New("the offset is neither REL: nor ABS: followed by a value")
	errOffsetCalendar   = errors.New("the duration counts in years or months")
	errOffsetOutOfRange = errors.New("the duration is longer than any reminder means")
)

// ParseReminderOffset reads an offset spec, or says why it is not one.
//
// The grammar is deliberately narrow: `REL:` and an ISO-8601 duration of weeks, days, hours,
// minutes and seconds, or `ABS:` and an RFC 3339 instant. Years and months are refused - they are
// calendar arithmetic rather than a length of time, and "one month before" would mean two
// different durations in two different months, which is not something an offset can promise.
func ParseReminderOffset(spec string) (ReminderOffset, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ReminderOffset{}, shared.ErrValidation.
			WithDetail("reminders.offset_spec_required").
			WithFields(shared.FieldError{Path: "/offset_spec", Code: "reminders.offset_spec_required"})
	}
	if len([]rune(spec)) > MaxReminderOffsetSpecLength {
		return ReminderOffset{}, offsetInvalid("reminders.offset_spec_too_long", spec)
	}

	switch {
	case strings.HasPrefix(spec, relativeOffsetPrefix):
		duration, err := parseISODuration(strings.TrimPrefix(spec, relativeOffsetPrefix))
		switch {
		case errors.Is(err, errOffsetCalendar):
			return ReminderOffset{}, offsetInvalid("reminders.offset_calendar_unit", spec)
		case errors.Is(err, errOffsetOutOfRange):
			return ReminderOffset{}, offsetInvalid("reminders.offset_out_of_range", spec)
		case err != nil:
			return ReminderOffset{}, offsetInvalid("reminders.offset_spec_invalid", spec)
		}
		return ReminderOffset{Spec: spec, Relative: true, Duration: duration}, nil
	case strings.HasPrefix(spec, absoluteOffsetPrefix):
		instant, err := time.Parse(time.RFC3339, strings.TrimPrefix(spec, absoluteOffsetPrefix))
		if err != nil {
			return ReminderOffset{}, offsetInvalid("reminders.offset_spec_invalid", spec)
		}
		return ReminderOffset{Spec: spec, Instant: instant.UTC()}, nil
	default:
		return ReminderOffset{}, offsetInvalid("reminders.offset_spec_invalid", spec)
	}
}

// offsetInvalid is the one refusal shape, so that every way of getting the spec wrong answers with
// the same field path and carries the value that was refused.
func offsetInvalid(code, spec string) error {
	return shared.ErrValidation.
		WithDetail(code).
		WithParams(map[string]string{"value": spec}).
		WithFields(shared.FieldError{
			Path: "/offset_spec", Code: code, Params: map[string]string{"value": spec},
		})
}

// parseISODuration reads an ISO-8601 duration, signed, of weeks down to seconds.
//
// Written here rather than taken from a library, for the reason the domain has no libraries at all
// (ADR-0001, rule 1): this is twenty lines of arithmetic over a closed grammar, and the one
// dependency this milestone takes is the RRULE expansion D-04 needs, which is not.
func parseISODuration(text string) (time.Duration, error) {
	sign := time.Duration(1)
	switch {
	case strings.HasPrefix(text, "-"):
		sign, text = -1, text[1:]
	case strings.HasPrefix(text, "+"):
		text = text[1:]
	}
	if !strings.HasPrefix(text, "P") {
		return 0, errOffsetMalformed
	}

	date, clock, timed := strings.Cut(text[1:], "T")
	if timed && clock == "" {
		return 0, errOffsetMalformed
	}

	var total time.Duration
	components := 0
	for _, part := range []struct {
		text  string
		units map[byte]time.Duration
	}{
		{date, map[byte]time.Duration{'W': 7 * 24 * time.Hour, 'D': 24 * time.Hour}},
		{clock, map[byte]time.Duration{'H': time.Hour, 'M': time.Minute, 'S': time.Second}},
	} {
		rest := part.text
		for rest != "" {
			count, unit, remainder, err := cutDurationComponent(rest)
			if err != nil {
				return 0, err
			}
			scale, known := part.units[unit]
			if !known {
				// 'Y' and 'M' in the date part are the calendar units, and 'M' after the T is
				// minutes - which is why the two halves are read against two tables rather than
				// one.
				if unit == 'Y' || unit == 'M' {
					return 0, errOffsetCalendar
				}
				return 0, errOffsetMalformed
			}
			total += time.Duration(count) * scale
			if total > MaxReminderOffset {
				return 0, errOffsetOutOfRange
			}
			components++
			rest = remainder
		}
	}
	if components == 0 {
		return 0, errOffsetMalformed
	}
	return sign * total, nil
}

// cutDurationComponent reads one number and its unit letter off the front.
func cutDurationComponent(text string) (count int64, unit byte, rest string, err error) {
	digits := 0
	for digits < len(text) && text[digits] >= '0' && text[digits] <= '9' {
		count = count*10 + int64(text[digits]-'0')
		digits++
		if digits > 6 {
			// Six digits is a hundred thousand days, which the range check refuses anyway; the
			// bound here is what keeps the multiplication above from overflowing on the way there.
			return 0, 0, "", errOffsetOutOfRange
		}
	}
	if digits == 0 || digits == len(text) {
		return 0, 0, "", errOffsetMalformed
	}
	return count, text[digits], text[digits+1:], nil
}

// FireAt answers the moment this offset means for a given due date, or nil when it means none.
//
// Nil is not an error: a relative reminder whose entry has lost its due date has nothing to count
// from, and the reminder stays - the date may come back, and nobody asked for the reminder to go.
// Refusing a relative reminder on an entry that never had one is a different question, decided
// where the reminder is written.
func (o ReminderOffset) FireAt(due *DueDate) (*time.Time, error) {
	if !o.Relative {
		instant := o.Instant.UTC()
		return &instant, nil
	}
	if due == nil {
		return nil, nil
	}

	anchor, err := due.Anchor()
	if err != nil {
		return nil, err
	}
	at := anchor.Add(o.Duration).UTC()
	return &at, nil
}

// NewReminderInput is what a creation needs decided.
type NewReminderInput struct {
	ID       shared.ID
	TenantID shared.ID
	ItemID   shared.ID
	// OffsetSpec is the text as the caller wrote it.
	OffsetSpec string
	// Channels as the caller named them, and empty for the default: an omitted channel list means
	// email, which is what the column's own default says (0001_init).
	Channels   []string
	Recipients []shared.ID
	// Due is the entry's due date, already read - nil for an entry that has none.
	Due *DueDate
	Now time.Time
}

// NewReminder validates and builds a reminder, with the moment it will fire at computed.
func NewReminder(input NewReminderInput) (Reminder, error) {
	offset, err := ParseReminderOffset(input.OffsetSpec)
	if err != nil {
		return Reminder{}, err
	}
	if offset.Relative && input.Due == nil {
		// The backlog's rule, and the reason for it: fire_at would mean nothing. Refused rather
		// than stored dormant, because a person setting "an hour before" on an entry with no
		// deadline has misunderstood what they are getting, and silence would confirm it.
		return Reminder{}, shared.ErrValidation.
			WithDetail("reminders.due_date_required").
			WithFields(shared.FieldError{
				Path: "/offset_spec", Code: "reminders.due_date_required",
			})
	}
	channels, err := validReminderChannels(input.Channels)
	if err != nil {
		return Reminder{}, err
	}
	recipients, err := validReminderRecipients(input.Recipients)
	if err != nil {
		return Reminder{}, err
	}
	fireAt, err := offset.FireAt(input.Due)
	if err != nil {
		return Reminder{}, err
	}

	return Reminder{
		ID:         input.ID,
		TenantID:   input.TenantID,
		ItemID:     input.ItemID,
		Offset:     offset,
		Channels:   channels,
		Recipients: recipients,
		State:      ReminderPending,
		FireAt:     fireAt,
		CreatedAt:  input.Now,
		Version:    1,
	}, nil
}

// EnsureReminderCapacity refuses the reminder that would take an entry past the bound.
func EnsureReminderCapacity(existing int) error {
	if existing < MaxRemindersPerItem {
		return nil
	}
	return shared.ErrValidation.
		WithDetail("reminders.too_many").
		WithParams(map[string]string{"maximum": strconv.Itoa(MaxRemindersPerItem)}).
		WithFields(shared.FieldError{
			Path: "/item_id", Code: "reminders.too_many",
			Params: map[string]string{"maximum": strconv.Itoa(MaxRemindersPerItem)},
		})
}

// ReminderPatch is a merge patch's touch on a reminder: nil means "not sent", exactly as
// ItemAttributes spells it, because only the stored reminder can say what an absent member means.
type ReminderPatch struct {
	OffsetSpec *string
	Channels   *[]string
	Recipients *[]shared.ID
}

// IsEmpty reports whether the patch asks for nothing at all.
func (p ReminderPatch) IsEmpty() bool {
	return p.OffsetSpec == nil && p.Channels == nil && p.Recipients == nil
}

// Patched applies the patch and reports which fields moved, each as its own change.
//
// One change per field, for the reason a scalar update takes one per field (offline-sync.md §4.2):
// a single entry naming the offset and the channels would give the pair one clock, and a merge
// would then decide them together - which is exactly the loss two devices editing different
// members must not suffer. `fire_at`, `version` and `updated_at` are not in it: they are derived,
// and a client that merged them would be merging the server's arithmetic.
//
// Only a pending reminder may be changed. One that has fired is a record of something that
// happened, and one that was cancelled will not fire whatever it says - so both are refused
// rather than silently rewritten into a future they no longer have.
func (r Reminder) Patched(
	patch ReminderPatch, due *DueDate, at time.Time,
) (Reminder, []FieldChange, error) {
	if r.State != ReminderPending {
		return Reminder{}, nil, shared.ErrConflict.
			WithDetail("reminders.not_pending").
			WithParams(map[string]string{"state": r.State.String()}).
			WithFields(shared.FieldError{Path: "/state", Code: "reminders.not_pending"})
	}

	target := r
	if patch.OffsetSpec != nil {
		offset, err := ParseReminderOffset(*patch.OffsetSpec)
		if err != nil {
			return Reminder{}, nil, err
		}
		if offset.Relative && due == nil {
			return Reminder{}, nil, shared.ErrValidation.
				WithDetail("reminders.due_date_required").
				WithFields(shared.FieldError{
					Path: "/offset_spec", Code: "reminders.due_date_required",
				})
		}
		target.Offset = offset
	}
	if patch.Channels != nil {
		channels, err := validReminderChannels(*patch.Channels)
		if err != nil {
			return Reminder{}, nil, err
		}
		target.Channels = channels
	}
	if patch.Recipients != nil {
		recipients, err := validReminderRecipients(*patch.Recipients)
		if err != nil {
			return Reminder{}, nil, err
		}
		target.Recipients = recipients
	}

	changes := reminderChanges(r, target)
	if len(changes) == 0 {
		return r, nil, nil
	}

	fireAt, err := target.Offset.FireAt(due)
	if err != nil {
		return Reminder{}, nil, err
	}
	target.FireAt = fireAt
	target.UpdatedAt = &at
	return target, changes, nil
}

// Rescheduled recomputes the moment a reminder fires at against a due date that has moved, and
// reports whether it moved with it.
//
// Only relative reminders and only pending ones: an absolute reminder is a moment somebody named
// and no deadline moves it, and a reminder that has already fired cannot be given a new future.
// A due date that is gone leaves the reminder with no moment at all rather than removing it (see
// FireAt).
func (r Reminder) Rescheduled(due *DueDate) (Reminder, bool, error) {
	if !r.Offset.Relative || r.State != ReminderPending {
		return r, false, nil
	}

	fireAt, err := r.Offset.FireAt(due)
	if err != nil {
		return Reminder{}, false, err
	}
	if equalInstant(r.FireAt, fireAt) {
		return r, false, nil
	}
	r.FireAt = fireAt
	return r, true, nil
}

// reminderChanges diffs two reminders field by field. A list spells as its members joined by
// commas: the whole list is one field here, because channels and recipients are chosen wholesale
// in one gesture rather than added one at a time - which is what makes them scalars for the merge
// and not the OR-sets labels and members are (offline-sync.md §4.2).
func reminderChanges(from, to Reminder) []FieldChange {
	var changes []FieldChange
	appendChange := func(field, before, after string) {
		if before != after {
			changes = append(changes, FieldChange{Field: field, From: before, To: after})
		}
	}
	appendChange(FieldOffsetSpec, from.Offset.Spec, to.Offset.Spec)
	appendChange(FieldChannels, channelList(from.Channels), channelList(to.Channels))
	appendChange(FieldRecipients, recipientList(from.Recipients), recipientList(to.Recipients))
	return changes
}

// ChannelList spells the channels the way a change and the audit entry carry them.
func ChannelList(channels []ReminderChannel) string { return channelList(channels) }

func channelList(channels []ReminderChannel) string {
	names := make([]string, 0, len(channels))
	for _, channel := range channels {
		names = append(names, channel.String())
	}
	return strings.Join(names, ",")
}

// RecipientList spells the recipients the way a change carries them, and the empty string for the
// empty list - which is the list that means the assignee and the members.
func RecipientList(recipients []shared.ID) string { return recipientList(recipients) }

func recipientList(recipients []shared.ID) string {
	names := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		names = append(names, recipient.String())
	}
	return strings.Join(names, ",")
}

// validReminderChannels reads the channel list: the default for an empty one, every member
// checked, and each named at most once.
func validReminderChannels(given []string) ([]ReminderChannel, error) {
	if len(given) == 0 {
		return []ReminderChannel{ReminderChannelEmail}, nil
	}

	channels := make([]ReminderChannel, 0, len(given))
	seen := make(map[ReminderChannel]bool, len(given))
	for _, name := range given {
		channel := ReminderChannel(strings.TrimSpace(name))
		if !channel.Valid() {
			return nil, shared.ErrValidation.
				WithDetail("reminders.channel_unknown").
				WithParams(map[string]string{"value": string(channel)}).
				WithFields(shared.FieldError{
					Path: "/channels", Code: "reminders.channel_unknown",
					Params: map[string]string{"value": string(channel)},
				})
		}
		if seen[channel] {
			// Named twice is one channel, not two sends. Deduplicated rather than refused: the
			// caller asked for email, twice, and refusing would be pedantry about a request whose
			// meaning is not in doubt.
			continue
		}
		seen[channel] = true
		channels = append(channels, channel)
	}
	return channels, nil
}

// validReminderRecipients reads the recipient list: bounded, deduplicated, and never carrying the
// zero identifier. Who may be named at all is the application layer's question (rule 2) - it is
// the same reachability an assignment asks about, and it needs the memberships to answer.
func validReminderRecipients(given []shared.ID) ([]shared.ID, error) {
	if len(given) > MaxReminderRecipients {
		return nil, shared.ErrValidation.
			WithDetail("reminders.too_many_recipients").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxReminderRecipients)}).
			WithFields(shared.FieldError{
				Path: "/recipients", Code: "reminders.too_many_recipients",
				Params: map[string]string{"maximum": strconv.Itoa(MaxReminderRecipients)},
			})
	}

	recipients := make([]shared.ID, 0, len(given))
	seen := make(map[shared.ID]bool, len(given))
	for _, recipient := range given {
		if recipient.IsZero() {
			return nil, shared.ErrValidation.
				WithDetail("reminders.recipient_required").
				WithFields(shared.FieldError{
					Path: "/recipients", Code: "reminders.recipient_required",
				})
		}
		if seen[recipient] {
			continue
		}
		seen[recipient] = true
		recipients = append(recipients, recipient)
	}
	return recipients, nil
}

// EnsureRemindable refuses what cannot be reminded about: a type whose profile does not carry
// REMINDER, and a trashed or archived entry (I-W4) - the capability first, for the reason
// EnsureCommentable asks it first.
func (i WorkItem) EnsureRemindable(profile CapabilityProfile) error {
	if err := profile.Require(CapabilityReminder, "/item_id"); err != nil {
		return err
	}
	return i.EnsureEditable()
}
