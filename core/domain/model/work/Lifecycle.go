// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The trash and the archive, for the item and the container alike.
//
// They live together rather than beside the two aggregates they belong to because the interesting
// part of both is one rule that spans them: a container's deletion takes its subtree with it, and
// what makes that reversible is the batch identifier every row of one deletion shares (I-C2). Split
// across two files, the half of the rule that says "and never restamp a row that was already in the
// trash" would be written twice and agreed on once.
//
// The two stamps are deliberately independent columns rather than one status:
//
//   - archived means kept and read-only; a decision about how the item is worked with;
//   - deleted means on its way out, with a retention period running against it.
//
// An item can be both, and the state machine in domain-model.md §3.4 says so - Archived --> Trashed
// is a documented edge. That is why trashing never touches the archive stamp and restoring never
// restores it: an archived item that went into the trash comes back archived, because coming back
// undoes the deletion and nothing else. Anything narrower would silently make Restore an unarchive
// as well, which is the one thing a person restoring a deletion did not ask for.
//
// The two field names these transitions report under - FieldArchivedAt and FieldDeletedAt - are
// declared with the rest of their aggregates' field names, in Container.go and Structure.go. They
// are the spelling the API, the change set and the change log all have to agree on, so each is
// written once wherever it first appeared rather than a second time here.

// Archived stamps the item read-only.
//
// Idempotent: an item already archived comes back untouched, keeping its original timestamp, which
// is what makes a retry after a lost response harmless rather than a silent re-dating.
func (i WorkItem) Archived(at time.Time) (WorkItem, []FieldChange, error) {
	if err := i.ensureLifecycleChangeable(); err != nil {
		return WorkItem{}, nil, err
	}
	if i.IsArchived() {
		return i, nil, nil
	}

	changes := []FieldChange{{Field: FieldArchivedAt, From: "", To: instant(at)}}
	i.ArchivedAt = &at
	i.UpdatedAt = at
	return i, changes, nil
}

// Unarchived makes the item writable again. Idempotent in the same way.
func (i WorkItem) Unarchived(at time.Time) (WorkItem, []FieldChange, error) {
	if err := i.ensureLifecycleChangeable(); err != nil {
		return WorkItem{}, nil, err
	}
	if !i.IsArchived() {
		return i, nil, nil
	}

	changes := []FieldChange{{Field: FieldArchivedAt, From: instant(*i.ArchivedAt), To: ""}}
	i.ArchivedAt = nil
	i.UpdatedAt = at
	return i, changes, nil
}

// Trashed puts the item into the trash as part of one deletion, named by batch.
//
// An item already in the trash comes back untouched - not merely as politeness towards a retry, but
// because that is the invariant a subtree deletion rests on. Trashing a subtree walks rows that may
// already be in the trash from an earlier, separate deletion, and restamping those would fold them
// into the new batch: restoring it would then bring back something nobody deleted this time, and
// its own retention period would have been quietly restarted. Its deletion stays its own.
//
// Archived is not an obstacle. Deleting something that was taken out of use is the ordinary case,
// and the archive stamp survives the trip so that Restored can put it back the way it was.
func (i WorkItem) Trashed(at time.Time, batch shared.ID) (WorkItem, []FieldChange, error) {
	if batch.IsZero() {
		// A batch identifier the caller did not generate is a defect in this process, not something
		// a client did: every entry point into the trash goes through the ID generator port.
		return WorkItem{}, nil, shared.ErrInternal.WithDetail("items.trash_batch_missing")
	}
	if i.IsTrashed() {
		return i, nil, nil
	}

	changes := []FieldChange{{Field: FieldDeletedAt, From: "", To: instant(at)}}
	i.DeletedAt = &at
	i.TrashBatchID = batch
	i.UpdatedAt = at
	return i, changes, nil
}

// Restored takes the item out of the trash.
//
// It clears the batch along with the stamp, so that an item deleted again later belongs to that
// deletion and not to the one it came back from. The archive stamp is left exactly as it was, for
// the reason the file header gives.
//
// No lifecycle gate: the state this refuses to act on is the only state it is ever called in.
func (i WorkItem) Restored(at time.Time) (WorkItem, []FieldChange, error) {
	if !i.IsTrashed() {
		return i, nil, nil
	}

	changes := []FieldChange{{Field: FieldDeletedAt, From: instant(*i.DeletedAt), To: ""}}
	i.DeletedAt = nil
	i.TrashBatchID = ""
	i.UpdatedAt = at
	return i, changes, nil
}

// ensureLifecycleChangeable is what archiving and unarchiving an item both need: not on its way
// out already.
//
// It does not read IsArchived the way EnsureEditable does, because these two verbs own that stamp -
// a check that refused an archived item would make unarchiving impossible. What it does refuse is
// an item in the trash: the archive is a decision about work that is still there, and taking a
// deletion out of use is not a state the machine in domain-model.md §3.4 has an arrow for.
func (i WorkItem) ensureLifecycleChangeable() error {
	if i.IsTrashed() {
		return shared.ErrConflict.
			WithDetail("items.trashed").
			WithParams(map[string]string{"item_id": i.ID.String()})
	}
	return nil
}

// Trashed puts the container into the trash as part of one deletion, named by batch.
//
// The same batch as every row beneath it: the collections of a hub and the items of a collection go
// in with it, and one identifier over all of them is what makes the way back a single decision
// rather than a walk that has to guess what belonged together (I-C2).
//
// The gate is the one archiving uses, and it refuses the same thing: a container inside an archived
// hub. That subtree is read-only, and deleting out of it is a write into it - unarchive the hub
// first, and the answer names it. A hub archived in its own right is not refused: Archived -->
// Trashed is a documented edge, and the archive stamp survives so that restoring gives back what
// was deleted rather than something subtly different.
func (c Container) Trashed(at time.Time, batch shared.ID) (Container, []FieldChange, error) {
	if batch.IsZero() {
		return Container{}, nil, shared.ErrInternal.WithDetail("containers.trash_batch_missing")
	}
	// Before the gate, not after it. A container already in the trash is the state the caller asked
	// for, and the gate would refuse exactly that state - which would make a retry after a lost
	// response an error rather than a no-op, and would put a second deletion of the same subtree
	// beyond reach for no reason.
	if c.IsTrashed() {
		return c, nil, nil
	}
	if err := c.ensureLifecycleChangeable(); err != nil {
		return Container{}, nil, err
	}

	changes := []FieldChange{{Field: FieldDeletedAt, From: "", To: instant(at)}}
	c.DeletedAt = &at
	c.TrashBatchID = batch
	c.UpdatedAt = at
	return c, changes, nil
}

// Restored takes the container out of the trash, clearing the batch with the stamp.
func (c Container) Restored(at time.Time) (Container, []FieldChange, error) {
	if !c.IsTrashed() {
		return c, nil, nil
	}

	changes := []FieldChange{{Field: FieldDeletedAt, From: instant(*c.DeletedAt), To: ""}}
	c.DeletedAt = nil
	c.TrashBatchID = ""
	c.UpdatedAt = at
	return c, changes, nil
}
