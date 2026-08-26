// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package audit declares how the evidence trail is read back.
//
// The writing side is not here. It is `core/port/audit.Sink`, called by every use case inside the
// transaction it belongs to, and keeping the two apart is the same decision the item history makes:
// a single interface would put the reading half in the dependency list of every use case that
// writes, and make each of them look able to read what it must not (audit.md §7).
//
// What is read back is more than what was written. An entry gains its sequence number and its two
// hashes when it is appended, and a reader has to see them: the chain is what `:verify` walks, and
// a record without its hashes could not be checked against anything.
package audit

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
)

// Record is one stored entry: what a use case wrote, and what the chain added to it.
//
// The entry is the port's own type rather than a copy of its fields. Verification recomputes the
// hash over exactly the bytes the sink hashed, so the shape a verifier reads has to be the shape
// the writer wrote - two structurally identical types would agree until somebody added a field to
// one of them (infrastructure/audit.Canonical).
type Record struct {
	ID    shared.ID
	Seq   int64
	Entry port.Entry
	// PrevHash is the hash of the entry before this one in this tenant's chain, and is empty for
	// the first entry there has ever been. Empty rather than absent is the difference between the
	// start of a chain and a chain somebody truncated, which is why it is read back at all.
	PrevHash []byte
	Hash     []byte
}

// Filter is what a caller asks the trail for (audit.md §5).
//
// Every field is optional and every one of them narrows. The zero filter is "everything this
// actor may see", which is the honest default for a trail: an auditor opening it has no idea yet
// what they are looking for.
type Filter struct {
	// From and To bound the period, To exclusive. Zero on either side means unbounded there.
	From, To time.Time
	// ActionPrefix matches a code by its beginning, so that `auth.` is every authentication event
	// and `membership.role_changed` is one kind of event. A prefix rather than a pattern: the
	// codes are a dotted hierarchy, and matching them as patterns would make a caller's `%` a
	// wildcard nobody asked for.
	ActionPrefix string
	// ActorID is who did it. It is also how a member's own events are narrowed to them - the
	// application sets it rather than trusting the request, which is the whole of §5's second row.
	ActorID shared.ID
	// TargetType and TargetID are the object the entry is about. The filter §5 requires and the
	// specification lacked: "what happened to this object" is the question that brings somebody to
	// the trail in the first place.
	TargetType string
	TargetID   shared.ID
	// Outcome is SUCCESS, DENIED or FAILED. Empty is all three.
	Outcome port.Outcome
	Page    Page
}

// Page is where to continue and how much to read. The cursor is opaque and adapter-owned
// (api-guidelines.md §4): the application passes back what it was given and never looks inside.
type Page struct {
	Cursor string
	Size   int
}

// PageInfo is the walk's own state, as the contract's page carries it.
type PageInfo struct {
	NextCursor string
	HasMore    bool
}

// RecordPage is one page of the trail.
type RecordPage struct {
	Records []Record
	Info    PageInfo
}

// Period is an interval of the trail, To exclusive. Both ends may be zero, which is unbounded
// there - a verification of "everything there has ever been" is a legitimate question.
type Period struct{ From, To time.Time }

// Anchor is the last chain end this tenant exported to an append-only target outside the database
// (audit.md §3).
//
// Nothing writes one yet, and the zero value is what every installation therefore reads. It is
// asked for all the same, because `:verify` proves the chain intact *inside* the database and only
// an anchor says anything against somebody who can rewrite the whole of it - so the answer has to
// be able to say "nothing is sealed" rather than leave the question unasked.
type Anchor struct {
	AnchoredAt time.Time
	LastSeq    int64
	ChainHash  []byte
}

// IsZero reports whether this tenant has never anchored anything.
func (a Anchor) IsZero() bool { return a.AnchoredAt.IsZero() }

// Trail reads the entries back.
//
// Read-only, and there is no writing counterpart anywhere in this package. The application role
// holds no UPDATE and no DELETE on `audit_log`, a trigger refuses both, and a port that offered
// either would be a promise this system deliberately cannot keep (audit.md §3).
type Trail interface {
	// Query answers one page, newest first.
	Query(ctx context.Context, filter Filter) (RecordPage, error)

	// Walk hands over every entry of a period, oldest first, one at a time.
	//
	// A callback rather than a slice, for the reason the archive's source is one: a verification
	// over four hundred days is as large as the tenant was busy, and a method returning []Record
	// would read a year of evidence into memory before checking the first link (T-17). What the
	// implementation does behind it is page on the same keyset the list uses - never OFFSET - so
	// that a walk can neither repeat nor skip an entry.
	//
	// An error from yield stops the walk and is returned as it is: a verification that has found
	// its break has no reason to read the rest, and neither has an export whose target has gone.
	Walk(ctx context.Context, period Period, yield func(Record) error) error

	// LatestAnchor answers the last anchored chain end, or the zero anchor.
	LatestAnchor(ctx context.Context) (Anchor, error)
}
