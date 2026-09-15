// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package sync declares what offline clients need recorded about a change.
//
// The change log is deliberately not the event outbox (offline-sync.md §10). The outbox carries
// business integration events outwards - CloudEvents, versioned, a public contract - while the
// change log carries state deltas to clients. Different recipients, different retention,
// different compatibility commitments; mixing them would damage both.
package sync

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Operation is what happened to an entity from a synchronising client's point of view.
type Operation string

const (
	// Upsert covers creation and change alike: a client that has the entity applies the payload,
	// one that does not adds it. Two operations would make the client decide which case it is in,
	// and it is exactly the client that cannot know.
	Upsert Operation = "UPSERT"
	Delete Operation = "DELETE"
	// AccessRevoked tells a device to delete its local copy because the actor may no longer see
	// the entity. Distinct from a deletion: the entity still exists, just not for this client
	// (offline-sync.md §6).
	AccessRevoked Operation = "ACCESS_REVOKED"
)

// Change is one entry of the log. The sequence number is assigned by the database, because it has
// to be gapless per tenant and monotonic - it is the cursor `:pull` pages on, and a value chosen
// in the application would leave holes whenever a transaction rolled back.
type Change struct {
	TenantID shared.ID
	Entity   string
	EntityID shared.ID
	Op       Operation
	// ContainerID is the visibility filter a pull applies: a device that subscribes to one hub
	// gets the changes below it and nothing else. For a container it is the container itself.
	ContainerID shared.ID
	ActorID     shared.ID
	// DeviceID is the device that caused the change, so that the device which made it can skip
	// its own echo. Empty for a change made through the API rather than through a sync push.
	DeviceID shared.ID
	// HLC orders the change against concurrent ones on other devices (offline-sync.md §4.1).
	HLC shared.HLC
	// Field names the one field this entry moves, for a scalar change - empty for a creation,
	// a deletion or a set change, which are not about one field. A named field is what the
	// server's clock per field is kept for (N-05, §4.2): the adapter stamps `field_clock` with
	// this entry's reading in the same transaction, and a push compares its own reading against
	// that row. When a push is applying, the reading is the device's rather than the writer's -
	// carried in the context (appshared.ContextWithReadings), the way the device is, so that
	// fifty-five writers need not know a push exists.
	Field string
	// Payload is the changed fields. Nil on a deletion - there is nothing left to describe, and a
	// tombstone carries no content by design.
	Payload map[string]any
}

// Recorded is one entry as it comes back out: the change, and the position it holds.
//
// The position is not on Change, because a writer does not have one - `seq` is the database's, and
// it is assigned when the row lands. A type that carried it on the way in would be a type with a
// field every writer has to leave empty and hope nobody reads.
type Recorded struct {
	Change
	// Seq is the position in the log, and the cursor a reader resumes from. Monotonic per tenant
	// and sparse: the identity is table-wide, so a gap between two of one tenant's entries is
	// somebody else's entry rather than one of theirs that went missing.
	Seq int64
	// OccurredAt is when the change was recorded. Not the cursor - ADR-0021 is explicit that a
	// timestamp is never one - but what a client shows and what an operator reads.
	OccurredAt time.Time
}

// ChangeLog records what a client has to be told about.
type ChangeLog interface {
	// Record writes one change inside the caller's transaction. A change that reached the tables
	// but not the log would be invisible to every offline client until something else touched the
	// same row - which is a data loss that looks like a caching bug.
	Record(ctx context.Context, change Change) error
}

// Changes reads the log back. The half `:pull` and the stream share: the same records, the same
// order, the same cursor - which is what makes the stream an accelerator rather than a second
// source of truth (ADR-0021).
type Changes interface {
	// After returns up to batch entries past the cursor, oldest first. A short page is the end of
	// the log rather than the end of a page: there is no `has_more` to compute, because the cursor
	// of the last entry is the answer to "what next".
	After(ctx context.Context, after int64, batch int) ([]Recorded, error)

	// Latest is where the log stands now, and where a client with no cursor starts. Zero for a
	// workspace nothing has happened in.
	Latest(ctx context.Context) (int64, error)
}

// Devices is what the server keeps about the devices that synchronise (offline-sync.md §6, §10,
// N-03).
type Devices interface {
	// Touch registers a device on its first contact and records every contact after that: the
	// last position, the last moment, the credential, and what the device said about itself. An
	// identifier another account holds - in this workspace or any other - is refused with
	// `sync.device_foreign`, and a forgotten one with `sync.device_revoked`; neither is written.
	Touch(ctx context.Context, contact syncdomain.Contact) (syncdomain.Device, error)
	// ForAccount lists one account's devices, most recently seen first, forgotten ones included.
	ForAccount(ctx context.Context, accountID shared.ID) ([]syncdomain.Device, error)
	// Forget marks the account's device blocked and answers it as it was; false when the device
	// is not the account's or is already forgotten, which the caller tells apart by asking again.
	Forget(ctx context.Context, id, accountID shared.ID, now time.Time) (syncdomain.Device, bool, error)
	// Find answers one device of this workspace, or ErrNotFound.
	Find(ctx context.Context, id shared.ID) (syncdomain.Device, error)
}

// Snapshot reads the current state a device starting from nothing is told about, one kind at a
// time and in pages by identifier (offline-sync.md §3.1, N-02).
//
// Tenant-wide rather than per container, because the walk is one sequence with one cursor: a
// page is "the next batch of this kind after this identifier", and a device that stops halfway
// resumes at exactly that identifier. Every method reads only what is live - the trash and the
// deleted are tombstones in the log, not state to hand out - and the permission on each row is
// the reader's to decide, by the container the row belongs to.
type Snapshot interface {
	// Containers pages every live hub and collection, by identifier.
	Containers(ctx context.Context, after shared.ID, batch int) ([]work.Container, error)
	// Buckets pages every live column.
	Buckets(ctx context.Context, after shared.ID, batch int) ([]work.Bucket, error)
	// Labels pages every live label.
	Labels(ctx context.Context, after shared.ID, batch int) ([]work.Label, error)
	// Items pages every live entry - archived ones included, because an archive is an UPSERT
	// carrying `archived_at` and not a deletion.
	Items(ctx context.Context, after shared.ID, batch int) ([]work.WorkItem, error)
	// SetElements pages every tag of every set on every live entry, in the order of the set
	// element's key: entry, set, element. The key is what a page resumes after.
	SetElements(ctx context.Context, after SetElementKey, batch int) ([]ItemSetElement, error)
	// Comments pages every live comment with the collection its entry is in.
	Comments(ctx context.Context, after shared.ID, batch int) ([]InCollection[work.Comment], error)
	// Reminders pages every reminder of a live entry with the collection its entry is in.
	Reminders(ctx context.Context, after shared.ID, batch int) ([]InCollection[work.Reminder], error)
	// Recurrences pages every recurrence rule of a live entry with the collection its entry is in.
	Recurrences(ctx context.Context, after shared.ID, batch int) ([]InCollection[work.RecurrenceRule], error)
	// Templates pages every live template.
	Templates(ctx context.Context, after shared.ID, batch int) ([]work.Template, error)
}

// SetElementKey is where a page of set elements resumes: the primary key of the row.
type SetElementKey struct {
	ItemID    shared.ID
	Set       work.SetName
	ElementID shared.ID
}

// ItemSetElement is one tag row with what the reader needs beside it: the entry it belongs to and
// the collection that decides who may see it.
type ItemSetElement struct {
	ItemID       shared.ID
	CollectionID shared.ID
	Set          work.SetName
	Element      work.SetElement
}

// InCollection is a row of a kind that belongs to an entry, with the entry's collection beside it
// - the container the permission is decided by, joined rather than looked up row by row.
type InCollection[T any] struct {
	Value        T
	CollectionID shared.ID
}

// OpRecord is what a push did with one mutation, kept so that a repeated push takes effect
// exactly once (offline-sync.md §3.2, §7): the answer is served from here, and nothing is applied
// a second time.
type OpRecord struct {
	OpID     shared.ID
	DeviceID shared.ID
	Result   syncdomain.ResultKind
	EntityID shared.ID
	// Response is the result as it was answered, so that the repeat answers the same thing.
	Response  map[string]any
	AppliedAt time.Time
}

// OpLog remembers processed operations for the offline window (N-04). Rows age out with the
// window: a device may be away for the whole of it and then push its queue.
type OpLog interface {
	// Find answers the record of an operation this workspace has already processed, and false
	// when it has not.
	Find(ctx context.Context, opID shared.ID) (OpRecord, bool, error)
	// Record writes the record inside the caller's transaction - the one that applied the
	// mutation, so that a push that dies halfway leaves neither the effect nor the record.
	Record(ctx context.Context, record OpRecord) error
}

// Tombstones answers whether an entity has been purged (offline-sync.md §7): a mutation naming
// one is refused rather than applied to nothing, and a creation under its identifier is refused
// rather than bringing it back.
type Tombstones interface {
	Holds(ctx context.Context, entity string, id shared.ID) (bool, error)
}

// FieldClocks reads the server's clock per field (N-05, offline-sync.md §4.2): the reading of the
// write that last landed on each field, which a push's reading is compared against. Written by the
// change log adapter beside the entry that names a field, never directly.
type FieldClocks interface {
	// Of answers every field of the entity that has a reading. A field with none has never been
	// written since the clocks exist, and loses to any reading.
	Of(ctx context.Context, entity string, id shared.ID) (map[string]shared.HLC, error)
}
