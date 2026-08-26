// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"strconv"
	"time"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	DuplicateWorkItemName = "DuplicateWorkItem"

	// ItemDuplicatedAction is the audit code. One entry per duplication rather than one per entry
	// it produced: the act an auditor asks about is "who copied this, and to where", and the
	// entries the copy consists of are what that act produced rather than acts of their own
	// (audit.md §2).
	ItemDuplicatedAction audit.Action = "item.duplicated"

	// maxCopiedEntries bounds one duplication, the copied entry included.
	//
	// The same number as a bulk, and for the same reason: a copy is one transaction, and a
	// transaction whose length is decided by how much somebody happens to have below an entry is a
	// connection held for as long as that takes. A subtree over the bound is refused rather than
	// copied halfway - `items.subtree_too_large` names the bound, so a client can say what it
	// would take to do it in parts.
	maxCopiedEntries = 500
)

// DuplicateWorkItem copies an entry, and everything below it when asked for.
//
// The copy is a new entry rather than a fork of a conversation: what travels is the description -
// the title, the notes, the column, the labels, the members, the assignee, the cover, the
// attachments and the custom field values, as far as each type's capability profile allows - and
// what does not is the discussion and the history. A comment is an exchange between people about
// one entry, and there is no sense in which a copy has already had it.
//
// What the destination cannot resolve is reported rather than dropped in silence (I-W6), exactly as
// a move reports it: a label belongs to one collection's vocabulary, a column to one board, a
// custom field key to whatever definition that collection gives it, and an account may see one
// collection and not another. A person who spent an afternoon labelling learns what to redo.
type DuplicateWorkItem struct {
	Items       repository.Items
	ItemLabels  repository.ItemLabels
	ItemMembers repository.ItemMembers
	Labels      repository.Labels
	Buckets     repository.Buckets
	Fields      repository.CustomFields
	Containers  repository.Containers
	Attachments mediarepo.Attachments
	Media       MediaReferences
	Profiles    metarepo.CapabilityProfiles
	Authorizer  Authorizer
	// Ownership is the same question the create path asks: does the role the actor holds write only
	// what is assigned to them (C-04). A copy is a create, so it has to ask it too.
	Ownership  Ownership
	Visibility Visibility
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	Activity   ActivityJournal
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// DuplicateWorkItemCommand is the input, typed.
type DuplicateWorkItemCommand struct {
	ItemID shared.ID
	// IncludeSubtree copies everything below the entry with it. Without it the entry is copied
	// alone and its children stay where they are.
	IncludeSubtree bool
	// TargetParentID is the entry the copy sits under, meaningful only together with ParentGiven:
	// the zero value is both "the top level" and "not asked for", and those are different requests.
	TargetParentID shared.ID
	ParentGiven    bool
	// TargetCollectionID may be omitted when a parent is given: an entry's collection is the one
	// its parent is in, and making a client repeat it is making it possible to contradict.
	TargetCollectionID shared.ID
	// Title is the copy's title, and empty keeps the original's. A server that invented "Copy of …"
	// would be writing display text (ADR-0011).
	Title string
}

// DuplicateResult is what a duplication answers with: the copy of the entry that was asked about,
// how many entries the copy consists of, and what the destination could not carry over (I-W6).
type DuplicateResult struct {
	Item domain.WorkItem
	// Copied is the size of the copy, the entry itself included.
	Copied int
	// DroppedReferences is what could not be carried over, from every entry that lost something.
	// Empty rather than nil: a client that iterates the losses should not have to nil-check the
	// field.
	DroppedReferences []domain.DroppedReference
}

// Execute copies the entry and returns the copy.
func (h DuplicateWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd DuplicateWorkItemCommand,
) (DuplicateResult, error) {
	if cmd.ItemID.IsZero() {
		return DuplicateResult{}, itemIDRequired()
	}

	plan, err := h.plan(ctx, actor, cmd)
	if err != nil {
		return DuplicateResult{}, err
	}
	return h.perform(ctx, actor, plan)
}

// duplication is a decided copy: what is being copied, and where it lands.
type duplication struct {
	source domain.WorkItem
	// origin is the collection the entry is copied from, and destination the one it lands in. Equal
	// for the ordinary copy, which is the one nothing has to be resolved for.
	origin      domain.Container
	parent      *domain.WorkItem
	destination domain.Container
	command     DuplicateWorkItemCommand
	// ownEntriesOnly says the role the actor holds at the destination writes only what is assigned
	// to them, so every entry the copy produces has to land on them (C-04).
	ownEntriesOnly bool
	// dueOverride replaces the root copy's due date, and is nil for an ordinary duplicate. The
	// materialisation sets it: an occurrence is the same entry on another date, and that date is
	// the one the rule computed (D-05). Only the root - a child's date is a fact about the child,
	// and a series that rewrote its subtree's dates would be inventing the relative dates a
	// template owns (D-06).
	dueOverride *domain.DueDate
	// series is the rule an occurrence belongs to, and empty for an ordinary duplicate: a copy
	// belongs to no series (db/queries/Work.sql, CopyWorkItem).
	series shared.ID
}

// plan reads what the copy depends on and asks the permission questions.
//
// Read-only and outside the write transaction, because a refusal writes an audit entry and an entry
// written inside the caller's transaction would be rolled back with the refusal (audit.md §7).
// Nothing read here is trusted afterwards: the state the write is decided from is read again inside
// the transaction.
func (h DuplicateWorkItem) plan(
	ctx context.Context, actor appshared.ActorContext, cmd DuplicateWorkItemCommand,
) (duplication, error) {
	var plan duplication

	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		plan, err = h.read(ctx, cmd)
		return err
	})
	if err != nil {
		return duplication{}, err
	}

	// The source first, because a copy reads an entry whole - its notes, its labels, everything a
	// person put into it - and reading it is the permission that has to hold before anything else
	// is considered. A caller who may write in the destination but may not see the source would
	// otherwise be handed its contents by asking for a copy of it.
	//
	// Reading is the *permission*, so somebody who may only read a collection can still copy out of
	// it into one they may write; the *scope* is the write scope all the same, because a scope is
	// the credential's licence for the operation and the operation is a write. Asking for a second
	// scope here would make a credential the catalogue declares as sufficient fail halfway through.
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(plan.origin),
		Action:     ItemDuplicatedAction,
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		TargetID:   cmd.ItemID,
		On:         reading(plan.source),
	}); err != nil {
		return duplication{}, err
	}

	// And the destination, where the entry comes into being. The subject names no entry: the copy
	// does not exist yet, so there is nothing assigned and nothing shared, exactly as a create
	// names none (access.ItemSubject).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(plan.destination),
		Action:     ItemDuplicatedAction,
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		TargetID:   plan.destination.ID,
		On:         access.ItemSubject{Does: service.ItemCreate},
	}); err != nil {
		return duplication{}, err
	}

	// Whose the copy has to be. A role narrowed to what is assigned to it copies onto itself, or
	// the entries it produced would be out of its reach the moment they existed - which is the
	// decision the create path takes for the same reason, and what keeps "assigned only" true at
	// every moment rather than suspended for one call.
	if plan.ownEntriesOnly, err = h.Ownership.WritesOnlyWhatIsAssigned(
		ctx, actor, containerPath(plan.destination),
	); err != nil {
		return duplication{}, err
	}
	return plan, nil
}

// read resolves the entry, the destination parent and both collections.
//
// Where the copy lands is read exactly as a move reads it, and the three cases are the same three:
// a parent decides the collection, a named collection alone is the top level of it, and neither is
// the place the entry already sits - which is what makes "duplicate this" a request that needs no
// body at all.
func (h DuplicateWorkItem) read(
	ctx context.Context, cmd DuplicateWorkItemCommand,
) (duplication, error) {
	source, err := findItem(ctx, h.Items, cmd.ItemID)
	if err != nil {
		return duplication{}, err
	}

	origin, err := findCollection(ctx, h.Containers, source.CollectionID)
	if err != nil {
		return duplication{}, err
	}

	targetParentID := source.ParentID
	if cmd.ParentGiven {
		targetParentID = cmd.TargetParentID
	}

	var parent *domain.WorkItem
	if !targetParentID.IsZero() {
		found, err := findParentItem(ctx, h.Items, targetParentID)
		if err != nil {
			return duplication{}, err
		}
		parent = &found
	}

	destination := origin
	switch {
	case parent != nil:
		if !cmd.TargetCollectionID.IsZero() && cmd.TargetCollectionID != parent.CollectionID {
			return duplication{}, shared.ErrValidation.
				WithDetail("items.collection_contradicts_parent").
				WithParams(map[string]string{
					"collection_id": cmd.TargetCollectionID.String(),
					"parent_id":     parent.ID.String(),
				}).
				WithFields(shared.FieldError{
					Path: "/target_collection_id", Code: "items.collection_contradicts_parent",
				})
		}
		if parent.CollectionID != origin.ID {
			if destination, err = findCollection(ctx, h.Containers, parent.CollectionID); err != nil {
				return duplication{}, err
			}
		}
	case !cmd.TargetCollectionID.IsZero():
		if destination, err = findTargetCollection(ctx, h.Containers, cmd.TargetCollectionID); err != nil {
			return duplication{}, err
		}
	case cmd.ParentGiven:
		// The top level, and no collection named. There is nothing to place the copy against.
		return duplication{}, shared.ErrValidation.
			WithDetail("items.collection_or_parent_required").
			WithFields(shared.FieldError{
				Path: "/target_collection_id", Code: "items.collection_or_parent_required",
			})
	}

	cmd.TargetParentID = targetParentID
	return duplication{
		source: source, origin: origin, parent: parent, destination: destination, command: cmd,
	}, nil
}

// perform writes the whole copy inside one transaction.
//
// One transaction, and that is what the bound on the subtree is for: a copy that committed its
// first half would leave a subtree whose root exists and whose children do not, which is neither
// what was asked for nor something a client could undo by repeating the call.
func (h DuplicateWorkItem) perform(
	ctx context.Context, actor appshared.ActorContext, plan duplication,
) (DuplicateResult, error) {
	var result DuplicateResult

	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// One reading of the clock for the whole copy, so that every entry it produces agrees about
		// when it came into being.
		now := h.Clock.Now()

		var err error
		result, err = h.copyInto(ctx, actor, plan, now)
		return err
	})
	if err != nil {
		return DuplicateResult{}, err
	}
	return result, nil
}

// copyInto is the copy itself, without the permission questions and without a transaction of its
// own: everything perform does inside the caller's.
//
// It exists as its own method because the materialisation reuses it (D-05, "the copy machinery is
// C-11's duplicate, reused"). A series copying its own template is the system acting on a decision
// somebody already made - when they wrote the rule - so there is no second person to ask about,
// and asking the authorisation service about an actor with no memberships would refuse the only
// caller that has the right to be here. What is *not* skipped is everything that makes a copy
// correct: the fresh read, the lifecycle guards, the placement, and the records each entry owes.
func (h DuplicateWorkItem) copyInto(
	ctx context.Context, actor appshared.ActorContext, plan duplication, now time.Time,
) (DuplicateResult, error) {
	var result DuplicateResult

	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// Read again inside the transaction: everything the plan was decided from can have changed
		// since, and what decides the write has to be true at the moment the rows are written.
		fresh, err := h.read(ctx, plan.command)
		if err != nil {
			return err
		}
		fresh.ownEntriesOnly = plan.ownEntriesOnly
		fresh.dueOverride = plan.dueOverride
		fresh.series = plan.series
		if err := fresh.destination.EnsureAcceptsItems(); err != nil {
			return err
		}
		if fresh.source.IsTrashed() || fresh.source.IsArchived() {
			// A trashed entry is on its way out and a copy of it would put back what somebody
			// deleted; an archived one has been put away, and a live copy of it would take it back
			// out without anybody asking. Both are the answer I-W4 gives every other write, and the
			// fix is the same: restore or unarchive it first.
			return notEditable(fresh.source)
		}

		hierarchy, err := h.hierarchy(ctx)
		if err != nil {
			return err
		}
		// Where the copy may sit, asked of the destination parent rather than of the entry's own
		// place: the profiles may have been narrowed since the entry was created, and the copy is a
		// new entry that has to be permitted where it is going.
		//
		// The question is asked once, about the root. The shape below it is carried whole, exactly
		// as a move carries it: it is a tree that already exists, and re-deciding each level of it
		// would mean refusing to copy a structure the workspace is still living with. What the root
		// answers covers the reach - the depth budget is per type - and what it deliberately does
		// not cover is a child type a narrowed profile no longer permits under its parent, which is
		// the same thing a move of the same subtree does not re-decide either.
		spot, err := hierarchy.Place(fresh.parent, fresh.source.Type)
		if err != nil {
			return err
		}

		entries, err := h.entriesToCopy(ctx, fresh)
		if err != nil {
			return err
		}

		vocabulary, err := h.vocabularyOf(ctx, actor, fresh)
		if err != nil {
			return err
		}

		result, err = h.write(ctx, actor, fresh, spot, entries, vocabulary, hierarchy, now)
		return err
	})
	if err != nil {
		return DuplicateResult{}, err
	}
	return result, nil
}

// entriesToCopy is the entry and, when the request asked for it, everything below it.
func (h DuplicateWorkItem) entriesToCopy(
	ctx context.Context, plan duplication,
) ([]domain.WorkItem, error) {
	entries := []domain.WorkItem{plan.source}
	if !plan.command.IncludeSubtree {
		return entries, nil
	}

	below, err := h.Items.Subtree(ctx, plan.source, maxCopiedEntries-1)
	if err != nil {
		return nil, err
	}
	if len(entries)+len(below) > maxCopiedEntries {
		// The repository reads one row past the bound, which is how a subtree at the bound is told
		// apart from one over it.
		return nil, shared.ErrValidation.
			WithDetail("items.subtree_too_large").
			WithParams(map[string]string{
				"item_id": plan.source.ID.String(),
				"limit":   strconv.Itoa(maxCopiedEntries),
			}).
			WithFields(shared.FieldError{Path: "/include_subtree", Code: "items.subtree_too_large"})
	}
	return append(entries, below...), nil
}

// vocabulary is what the destination collection can resolve: the labels it defines, the columns on
// its board, the custom fields in force there, and the accounts it lets somebody see.
type vocabulary struct {
	// sameCollection is the ordinary copy, where nothing has to be resolved again: every reference
	// the entry carries is a reference of the collection it is being copied inside.
	sameCollection bool
	labels         map[shared.ID]bool
	buckets        map[shared.ID]bool
	fields         map[string]domain.CustomFieldDefinition
	// visible caches the account question, which is the expensive one: a subtree can carry the same
	// person on every entry, and the answer is one answer about one collection.
	visible map[shared.ID]bool
}

// vocabularyOf reads what the destination can resolve, once for the whole copy.
//
// Nothing is read for a copy inside one collection, and that is not an optimisation but the rule:
// a reference of that collection resolves there because it is already there, and asking again would
// be asking a question whose answer cannot have changed between the entry and its copy.
func (h DuplicateWorkItem) vocabularyOf(
	ctx context.Context, actor appshared.ActorContext, plan duplication,
) (*vocabulary, error) {
	known := &vocabulary{
		sameCollection: plan.origin.ID == plan.destination.ID,
		labels:         map[shared.ID]bool{},
		buckets:        map[shared.ID]bool{},
		fields:         map[string]domain.CustomFieldDefinition{},
		visible:        map[shared.ID]bool{},
	}
	// The definitions are read whichever collection the copy lands in: a value has to name the
	// definition it stands behind, and a copy inside one collection needs that name as much as a
	// copy into another one does (repository.Copy).
	definitions, err := h.Fields.ListInScope(ctx, plan.destination.ID)
	if err != nil {
		return nil, err
	}
	for _, definition := range definitions {
		// The collection's own wins over the workspace-wide one under the same key, which is the
		// order ListInScope answers in (C-07).
		if _, taken := known.fields[definition.Key]; !taken {
			known.fields[definition.Key] = definition
		}
	}

	if known.sameCollection {
		return known, nil
	}

	labels, err := h.Labels.List(ctx, plan.destination.ID)
	if err != nil {
		return nil, err
	}
	for _, label := range labels {
		known.labels[label.ID] = true
	}

	buckets, err := h.Buckets.List(ctx, plan.destination.ID)
	if err != nil {
		return nil, err
	}
	for _, bucket := range buckets {
		known.buckets[bucket.ID] = true
	}
	return known, nil
}

// canSee answers whether an account reaches the destination, once per account.
//
// Asked at all only when the collection changes: an account that could see the entry can see a copy
// of it beside it. It is the question C-01 asks before an assignment, and asking it here is what
// keeps "an entry is only ever on somebody who can see it" true through a copy as well.
func (h DuplicateWorkItem) canSee(
	ctx context.Context, actor appshared.ActorContext, known *vocabulary,
	destination domain.Container, accountID shared.ID,
) (bool, error) {
	if known.sameCollection {
		return true, nil
	}
	if answer, asked := known.visible[accountID]; asked {
		return answer, nil
	}

	permitted, err := h.Visibility.CanSee(ctx, actor, accountID, containerPath(destination))
	if err != nil {
		return false, err
	}
	known.visible[accountID] = permitted
	return permitted, nil
}

// copies is the state one duplication carries while it writes: what each source entry became, and
// what the last sibling written under each new parent was ranked.
type copies struct {
	// newIDs maps a source entry to its copy, which is how a child finds the parent it now hangs
	// from. The rows arrive parents first, so the mapping is always already there (Items.Subtree).
	newIDs map[shared.ID]domain.WorkItem
	// lastRank is the rank of the last entry written at one level, keyed by the new parent - the
	// zero identifier for the top level of the destination collection.
	lastRank map[shared.ID]string
	dropped  []domain.DroppedReference
}

// write produces every entry of the copy, and the records each one owes.
//
// One event, one change log entry and one history entry per copied entry, deliberately: a client
// synchronising the destination has to learn about every row, and one record covering a subtree
// would leave it with a root whose children it has never heard of. The audit entry is the
// exception - one act, one entry (audit.md §2).
func (h DuplicateWorkItem) write(
	ctx context.Context, actor appshared.ActorContext, plan duplication,
	spot service.Placement, entries []domain.WorkItem, known *vocabulary,
	hierarchy service.Hierarchy, now time.Time,
) (DuplicateResult, error) {
	made := copies{
		newIDs:   make(map[shared.ID]domain.WorkItem, len(entries)),
		lastRank: map[shared.ID]string{},
		dropped:  []domain.DroppedReference{},
	}

	// Where the copy of the entry itself lands: at the end of the level it is asked for, which is
	// the one rank this operation has to measure against entries it did not write.
	previous, err := h.Items.LastOrderKey(ctx, plan.destination.ID, spot.ParentID)
	if err != nil {
		return DuplicateResult{}, err
	}
	made.lastRank[spot.ParentID] = previous

	var root domain.WorkItem
	for index, source := range entries {
		profile, err := hierarchy.Profile(source.Type)
		if err != nil {
			return DuplicateResult{}, err
		}

		copied, err := h.copyOf(ctx, actor, plan, spot, source, index == 0, profile, &made, known, now)
		if err != nil {
			return DuplicateResult{}, err
		}
		if index == 0 {
			root = copied
		}
	}

	if err := h.recordAudit(ctx, actor, plan, root, len(entries), now); err != nil {
		return DuplicateResult{}, err
	}
	return DuplicateResult{Item: root, Copied: len(entries), DroppedReferences: made.dropped}, nil
}

// copyOf writes one entry of the copy: the row, its sets, and the three records it owes.
func (h DuplicateWorkItem) copyOf(
	ctx context.Context, actor appshared.ActorContext, plan duplication, spot service.Placement,
	source domain.WorkItem, isRoot bool, profile domain.CapabilityProfile,
	made *copies, known *vocabulary, now time.Time,
) (domain.WorkItem, error) {
	copied := source
	copied.ID = h.IDs.NewID()
	copied.CollectionID = plan.destination.ID
	copied.CreatedBy = actor.AccountID
	copied.CreatedAt, copied.UpdatedAt = now, now
	copied.Version = 1
	copied.DeletedAt, copied.TrashBatchID = nil, noID
	// The copy is open. `completed_at` and `completed_by` name a person and a moment, and a copy
	// carrying them would say that person completed an entry they never touched.
	copied.Completion = domain.Completion{}

	// A copy belongs to no series, whatever the entry it was copied from belongs to: duplicating a
	// recurring task gives somebody a task like it rather than a second template. The
	// materialisation is the one caller that says otherwise, for the root it creates (D-05).
	copied.RecurrenceRuleID = noID

	switch {
	case isRoot:
		copied.ParentID = spot.ParentID
		copied.Path = spot.PathOf(copied.ID)
		copied.Depth = spot.Depth
		if plan.command.Title != "" {
			copied.Title = plan.command.Title
		}
		if plan.dueOverride != nil {
			copied.Due = plan.dueOverride
		}
		if !plan.series.IsZero() {
			copied.RecurrenceRuleID = plan.series
		}
	default:
		parent, found := made.newIDs[source.ParentID]
		if !found {
			// Unreachable: the rows arrive parents first (Items.Subtree). A defect rather than
			// input, and one that would otherwise write an entry hanging from nothing.
			return domain.WorkItem{}, shared.ErrInternal.
				WithDetail("items.copy_parent_missing").
				WithParams(map[string]string{"item_id": source.ID.String()})
		}
		copied.ParentID = parent.ID
		copied.Path = parent.ChildPath(copied.ID)
		copied.Depth = parent.Depth + 1
	}

	rank, err := service.OrderKeyBetween(made.lastRank[copied.ParentID], "")
	if err != nil {
		return domain.WorkItem{}, err
	}
	copied.OrderKey = rank
	made.lastRank[copied.ParentID] = rank

	copied.BucketID = h.bucketFor(source, copied, profile, known, made)
	copied.AssigneeID, err = h.assigneeFor(ctx, actor, plan, source, copied, profile, known, made)
	if err != nil {
		return domain.WorkItem{}, err
	}
	fields, references := h.fieldsFor(source, copied, profile, known, made)
	copied.CustomFields = fields
	if !profile.Allows(domain.CapabilityCover) {
		copied.Cover = nil
	}
	if !profile.Allows(domain.CapabilityNotes) {
		copied.Notes = ""
	}

	if err := h.Items.InsertCopy(ctx, repository.Copy{Item: copied, FieldDefinitions: references}); err != nil {
		return domain.WorkItem{}, err
	}
	if err := h.copySets(ctx, actor, plan, source, copied, profile, known, made); err != nil {
		return domain.WorkItem{}, err
	}
	if err := h.announce(ctx, actor, copied, now); err != nil {
		return domain.WorkItem{}, err
	}

	made.newIDs[source.ID] = copied
	return copied, nil
}

// bucketFor is the column the copy lands in: the one the entry was in, when the destination's board
// has it. A board belongs to a collection, so a copy into another one leaves the column behind
// (I-W6), and a type whose profile no longer carries a board leaves it behind as well.
func (h DuplicateWorkItem) bucketFor(
	source, copied domain.WorkItem, profile domain.CapabilityProfile,
	known *vocabulary, made *copies,
) shared.ID {
	if source.BucketID.IsZero() {
		return noID
	}
	if !profile.Allows(domain.CapabilityBucket) {
		made.dropped = append(made.dropped, domain.DroppedCapability(
			copied.ID, domain.ReferenceBucket, source.BucketID.String()))
		return noID
	}
	if !known.sameCollection && !known.buckets[source.BucketID] {
		made.dropped = append(made.dropped, domain.DroppedBucket(copied.ID, source.BucketID))
		return noID
	}
	return source.BucketID
}

// assigneeFor is the person the copy is on: the one the entry was on, when they can see where the
// copy lands. An entry is only ever on somebody who can see it (C-01), and a copy into another
// collection is the moment that can stop being true.
func (h DuplicateWorkItem) assigneeFor(
	ctx context.Context, actor appshared.ActorContext, plan duplication,
	source, copied domain.WorkItem, profile domain.CapabilityProfile,
	known *vocabulary, made *copies,
) (shared.ID, error) {
	if plan.ownEntriesOnly {
		if !profile.Allows(domain.CapabilityAssignment) {
			// A type that cannot be owned cannot be copied by a role that may only write what it
			// owns: the copy would exist and be out of its reach, which is the refusal the create
			// path gives for the same reason.
			return noID, shared.ErrForbidden.
				WithDetail("items.assignee_must_be_the_creator").
				WithParams(map[string]string{"item_type": string(copied.Type)})
		}
		if !source.AssigneeID.IsZero() && source.AssigneeID != actor.AccountID {
			// Reported rather than silently replaced: somebody who copies an entry that was on a
			// colleague should be able to see that the copy is on them instead.
			made.dropped = append(made.dropped, domain.DroppedReference{
				ItemID: copied.ID, Kind: domain.ReferenceAssignee,
				ID: source.AssigneeID.String(), Code: "items.assignee_must_be_the_creator",
			})
		}
		return actor.AccountID, nil
	}

	if source.AssigneeID.IsZero() {
		return noID, nil
	}
	if !profile.Allows(domain.CapabilityAssignment) {
		made.dropped = append(made.dropped, domain.DroppedCapability(
			copied.ID, domain.ReferenceAssignee, source.AssigneeID.String()))
		return noID, nil
	}

	permitted, err := h.canSee(ctx, actor, known, plan.destination, source.AssigneeID)
	if err != nil {
		return noID, err
	}
	if !permitted {
		made.dropped = append(made.dropped, domain.DroppedAssignee(copied.ID, source.AssigneeID))
		return noID, nil
	}
	return source.AssigneeID, nil
}

// fieldsFor resolves the custom field values against the destination: the definition under the same
// key there, which has to exist, carry this type, and accept the value.
//
// A key that is text in one collection and a number in another is not the same field, and a value
// written under a definition that will not have it would be invisible to every read while occupying
// the key. So the value is validated against the destination's definition rather than carried over
// on the strength of its key (C-07).
func (h DuplicateWorkItem) fieldsFor(
	source, copied domain.WorkItem, profile domain.CapabilityProfile,
	known *vocabulary, made *copies,
) (map[string]any, map[string]shared.ID) {
	if len(source.CustomFields) == 0 {
		return nil, nil
	}
	if !profile.Allows(domain.CapabilityCustomFields) {
		for key := range source.CustomFields {
			made.dropped = append(made.dropped, domain.DroppedCapability(
				copied.ID, domain.ReferenceCustomField, key))
		}
		return nil, nil
	}
	values := map[string]any{}
	references := map[string]shared.ID{}
	for key, value := range source.CustomFields {
		definition, defined := known.fields[key]
		if !defined {
			made.dropped = append(made.dropped, domain.DroppedCustomField(
				copied.ID, key, "fields.not_in_collection"))
			continue
		}
		if !definition.Carries(copied.Type) {
			made.dropped = append(made.dropped, domain.DroppedCustomField(
				copied.ID, key, "fields.not_for_type"))
			continue
		}
		accepted, err := definition.ValidateValue(value)
		if err != nil {
			made.dropped = append(made.dropped, domain.DroppedCustomField(
				copied.ID, key, "fields.value_not_accepted"))
			continue
		}
		values[key], references[key] = accepted, definition.ID
	}
	if len(values) == 0 {
		return nil, nil
	}
	return values, references
}

// copySets carries the sets beside the row: the labels, the members and the attachments.
//
// Each element is written with a tag of its own, because each is an addition a merge has to be able
// to see: an OR-set is decided by its tags, and elements written without them would merge as one
// value and lose whatever another device did concurrently (offline-sync.md §4.2).
func (h DuplicateWorkItem) copySets(
	ctx context.Context, actor appshared.ActorContext, plan duplication,
	source, copied domain.WorkItem, profile domain.CapabilityProfile,
	known *vocabulary, made *copies,
) error {
	if err := h.copyLabels(ctx, actor, plan, source, copied, profile, known, made); err != nil {
		return err
	}
	if err := h.copyMembers(ctx, actor, plan, source, copied, profile, known, made); err != nil {
		return err
	}
	return h.copyAttachments(ctx, actor, plan, source, copied, profile, made)
}

func (h DuplicateWorkItem) copyLabels(
	ctx context.Context, actor appshared.ActorContext, plan duplication,
	source, copied domain.WorkItem, profile domain.CapabilityProfile,
	known *vocabulary, made *copies,
) error {
	labels, err := h.ItemLabels.List(ctx, source.ID)
	if err != nil {
		return err
	}

	for _, labelID := range labels {
		switch {
		case !profile.Allows(domain.CapabilityLabels):
			made.dropped = append(made.dropped, domain.DroppedCapability(
				copied.ID, domain.ReferenceLabel, labelID.String()))
		case !known.sameCollection && !known.labels[labelID]:
			made.dropped = append(made.dropped, domain.DroppedLabel(copied.ID, labelID))
		default:
			tag := h.HLC.Next()
			if err := h.ItemLabels.Add(ctx, copied.ID, labelID, tag); err != nil {
				return err
			}
			if err := h.recordElement(ctx, actor, plan, copied, domain.SetLabels, labelID, tag); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h DuplicateWorkItem) copyMembers(
	ctx context.Context, actor appshared.ActorContext, plan duplication,
	source, copied domain.WorkItem, profile domain.CapabilityProfile,
	known *vocabulary, made *copies,
) error {
	members, err := h.ItemMembers.List(ctx, source.ID)
	if err != nil {
		return err
	}

	for _, accountID := range members {
		if !profile.Allows(domain.CapabilityMembers) {
			made.dropped = append(made.dropped, domain.DroppedCapability(
				copied.ID, domain.ReferenceMember, accountID.String()))
			continue
		}
		permitted, err := h.canSee(ctx, actor, known, plan.destination, accountID)
		if err != nil {
			return err
		}
		if !permitted {
			made.dropped = append(made.dropped, domain.DroppedMember(copied.ID, accountID))
			continue
		}
		tag := h.HLC.Next()
		if err := h.ItemMembers.Add(ctx, copied.ID, accountID, tag); err != nil {
			return err
		}
		if err := h.recordElement(ctx, actor, plan, copied, domain.SetMembers, accountID, tag); err != nil {
			return err
		}
	}
	return nil
}

// copyAttachments links the copy to the same files and raises their reference counts. The bytes are
// not copied: an attachment is a reference to an object of this tenant, and two entries pointing at
// one file is what the counter exists to describe (C-06).
func (h DuplicateWorkItem) copyAttachments(
	ctx context.Context, actor appshared.ActorContext, plan duplication,
	source, copied domain.WorkItem, profile domain.CapabilityProfile, made *copies,
) error {
	files, err := h.Attachments.MediaIDs(ctx, source.ID)
	if err != nil {
		return err
	}

	for _, mediaID := range files {
		if !profile.Allows(domain.CapabilityAttachments) {
			made.dropped = append(made.dropped, domain.DroppedCapability(
				copied.ID, domain.ReferenceAttachment, mediaID.String()))
			continue
		}
		tag := h.HLC.Next()
		added, err := h.Attachments.Add(ctx, copied.ID, mediaID, tag)
		if err != nil {
			return err
		}
		if !added {
			continue
		}
		if err := h.Media.AdjustRefCount(ctx, mediaID, 1); err != nil {
			return err
		}
		if err := h.recordElement(ctx, actor, plan, copied, domain.SetAttachments, mediaID, tag); err != nil {
			return err
		}
	}

	// The cover is a reference of its own rather than an attachment, and it is counted the same
	// way: the copy points at the same image, so the image has one more entry standing on it.
	if copied.Cover != nil && !copied.Cover.MediaID.IsZero() {
		return h.Media.AdjustRefCount(ctx, copied.Cover.MediaID, 1)
	}
	return nil
}

// recordElement writes what an offline client has to be told about one element of one set.
//
// One record per element, carrying the element and the tag it was written with, exactly as the use
// case that adds that element singly writes it (offline-sync.md §4.2). Not an optional extra: the
// entry's own record is a snapshot of the row, and a set lives beside the row - a client that
// received only the entry would have a copy with no labels, no members and no files, and nothing
// would ever tell it otherwise.
func (h DuplicateWorkItem) recordElement(
	ctx context.Context, actor appshared.ActorContext, plan duplication, copied domain.WorkItem,
	set domain.SetName, elementID shared.ID, tag shared.HLC,
) error {
	return h.Changes.Record(ctx, changelog.Change{
		TenantID: copied.TenantID,
		Entity:   itemTarget,
		EntityID: copied.ID,
		Op:       changelog.Upsert,
		// The hub above the collection, so that a device subscribed to the hub sees it - the
		// visibility filter a pull applies to a set element (offline-sync.md §3.1), and the same
		// choice AddLabel, AddMember and AttachMedia make.
		ContainerID: firstNonZero(plan.destination.ParentID, copied.CollectionID),
		ActorID:     actor.AccountID,
		HLC:         tag,
		Payload: map[string]any{
			"set":        string(set),
			"element_id": elementID.String(),
			"op":         "add",
		},
	})
}

// announce writes what one copied entry owes outwards: the event, and the change log entry a
// synchronising client rebuilds its own copy from.
//
// `item.created`, because that is what happened - an entry came into being, and a consumer that
// reacts to a new entry has to react to this one. One snapshot, two recipients, for the reason
// CreateWorkItem builds one: they describe the same state, and building it twice is how the two
// come to disagree.
func (h DuplicateWorkItem) announce(
	ctx context.Context, actor appshared.ActorContext, copied domain.WorkItem, now time.Time,
) error {
	announcement, err := event.NewItemCreated(
		h.IDs.NewID(), copied, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return err
	}
	if err := h.Events.Append(ctx, announcement); err != nil {
		return err
	}
	if err := h.Changes.Record(ctx, changelog.Change{
		TenantID:    copied.TenantID,
		Entity:      itemTarget,
		EntityID:    copied.ID,
		Op:          changelog.Upsert,
		ContainerID: copied.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         h.HLC.Next(),
		Payload:     announcement.Payload,
	}); err != nil {
		return err
	}
	// The first step of the copy's own history, and the whole of it: it was copied from somewhere,
	// and the conversation and the history of the entry it was copied from stayed where they were.
	// No change set - nothing moved, an entry came into being, and everything a reader would want
	// beside the verb is on the entry itself.
	return h.Activity.record(ctx, actor, copied, activity.ItemDuplicated, verbIsTheChange(), now)
}

// recordAudit writes the evidence: one entry for the act, naming what was copied and where to.
//
// All of it is structure - identifiers and a count - so all of it is OPEN in the data catalogue's
// sense. No title: user content stays out of the trail (rule 10), and "who copied what to where"
// needs none of it.
func (h DuplicateWorkItem) recordAudit(
	ctx context.Context, actor appshared.ActorContext, plan duplication,
	root domain.WorkItem, copied int, now time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   root.TenantID,
		OccurredAt: now,
		Action:     ItemDuplicatedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: itemTarget,
		TargetID:   root.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{
				Field: "source_item_id", Classification: audit.Open,
				From: plan.source.ID.String(), To: root.ID.String(),
			},
			audit.Change{
				Field: "collection_id", Classification: audit.Open,
				From: plan.origin.ID.String(), To: root.CollectionID.String(),
			},
			audit.Change{
				Field: "parent_id", Classification: audit.Open,
				From: idOrNil(plan.source.ParentID), To: idOrNil(root.ParentID),
			},
			audit.Change{
				Field: "copied", Classification: audit.Open, To: strconv.Itoa(copied),
			},
		),
	})
}

// hierarchy builds the rules in force, the same way CreateWorkItem and MoveWorkItem do and for the
// same reason: read off a narrowed set alone, the topology comes out wrong (NewHierarchy).
func (h DuplicateWorkItem) hierarchy(ctx context.Context) (service.Hierarchy, error) {
	inForce, err := h.Profiles.List(ctx)
	if err != nil {
		return service.Hierarchy{}, err
	}
	system, err := h.Profiles.ListSystem(ctx)
	if err != nil {
		return service.Hierarchy{}, err
	}
	return service.NewHierarchy(inForce, system)
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4) - and copying an entry is an action a template or a
// rule performs, so the automation door is not a nicety here.
func (h DuplicateWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DuplicateWorkItemName,
		Summary: "Copies an entry, and everything below it when asked for. The copy carries the " +
			"description - the title, the notes, the column, the labels, the members, the " +
			"assignee, the cover, the attachments and the custom field values, as far as the " +
			"type's capability profile allows - and carries neither the comments nor the history: " +
			"a copy is a new entry rather than a fork of a conversation. What the destination " +
			"cannot resolve is reported back rather than dropped in silence.",
		SideEffects: "Writes one new entry per copied entry, announces " + string(event.ItemCreated) +
			" for each of them, records a change for offline clients and a step of each new " +
			"entry's history, raises the reference count of every file the copy points at, and " +
			"writes one audit entry for the duplication.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry to copy.",
			},
			{
				Name: "include_subtree", Kind: usecase.KindBool,
				Description: "Whether everything below the entry is copied with it. Omitted " +
					"copies the entry alone, and its children stay where they are.",
			},
			{
				Name: "target_parent_id", Kind: usecase.KindID,
				Description: "The entry the copy sits under. Send it as null to put the copy at " +
					"the top level of a collection, which then has to be named. Omit it to put " +
					"the copy beside the original.",
			},
			{
				Name: "target_collection_id", Kind: usecase.KindID,
				Description: "The destination collection. May be omitted when a parent is given: " +
					"the collection is then the parent's, and naming a different one is refused " +
					"rather than quietly preferred.",
			},
			{
				Name: "title", Kind: usecase.KindString,
				Description: "The title of the copy. Omitted keeps the original's - the server " +
					"invents no title, because a title is text somebody reads.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemDuplicatedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemDuplicated},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// invoke is the adapter between the catalogue's untyped input and the typed command, for all three
// channels at once.
func (h DuplicateWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	parentID, err := in.ID("target_parent_id")
	if err != nil {
		return nil, err
	}
	collectionID, err := in.ID("target_collection_id")
	if err != nil {
		return nil, err
	}

	result, err := h.Execute(ctx, actor, DuplicateWorkItemCommand{
		ItemID:         itemID,
		IncludeSubtree: in.Bool("include_subtree"),
		TargetParentID: parentID,
		// Present, not non-empty: a client that sends `"target_parent_id": null` is asking for the
		// top level of a collection, and one that omits the field is asking for the copy to land
		// beside the original. Those are different requests and the difference is invisible in the
		// value.
		ParentGiven:        in.Present("target_parent_id"),
		TargetCollectionID: collectionID,
		Title:              in.String("title"),
	})
	if err != nil {
		return nil, err
	}
	return duplicateOutput(result), nil
}

// duplicateOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema DuplicateResult).
func duplicateOutput(result DuplicateResult) usecase.Output {
	dropped := make([]usecase.Output, 0, len(result.DroppedReferences))
	for _, reference := range result.DroppedReferences {
		dropped = append(dropped, usecase.Output{
			"item_id": reference.ItemID.String(),
			"kind":    string(reference.Kind),
			"id":      reference.ID,
			"code":    reference.Code,
		})
	}
	return usecase.Output{
		"item":               itemOutput(result.Item),
		"dropped_references": dropped,
		"copied":             result.Copied,
	}
}
