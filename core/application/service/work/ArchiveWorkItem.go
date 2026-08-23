// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

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
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ArchiveWorkItemName   = "ArchiveWorkItem"
	UnarchiveWorkItemName = "UnarchiveWorkItem"

	// The audit codes. Two, not one: an auditor asking what was taken out of use must not have to
	// read a change list to find out whether the entry means it went or came back (audit.md §2).
	ItemArchivedAction   audit.Action = "item.archived"
	ItemUnarchivedAction audit.Action = "item.unarchived"
)

// LifecycleWriter is what every verb that moves an entry between the archive and the trash needs.
//
// One dependency set held by all of them rather than a copy per verb. They are the same walk with a
// different transition in the middle - read the entry and the collection it is in, ask whether the
// actor may, apply the transition, write it, and record the three things a change owes - and keeping
// the machinery in one place is what makes "restoring records what trashing records" true by
// construction rather than by somebody remembering.
type LifecycleWriter struct {
	Items      repository.Items
	Containers repository.Containers
	Authorizer Authorizer
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	Activity   ActivityJournal
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
	// Queue is where a deletion asks for the tenant's cleanup to be scheduled.
	//
	// A deletion scheduling its own cleanup is not only convenient, it is the only shape available:
	// nothing in this system may enumerate tenants - the `tenant` table is behind row level security
	// with no bypass for the application role - so no scheduler can create one sweep job per tenant.
	// The job that a deletion creates reschedules itself forever afterwards (queue.KindRetentionSweep).
	Queue queue.Queue
}

// ArchiveWorkItem takes an entry out of use without deleting it: kept, and read-only.
//
// Nothing below it is archived with it. An entry's children are entries in their own right, unlike
// the collections of a hub, so there is no inherited stamp here and no I-C3 to enforce - a work
// package under an archived task stays writable, and that is the model rather than an oversight.
type ArchiveWorkItem struct {
	Lifecycle LifecycleWriter
}

// UnarchiveWorkItem makes an archived entry writable again.
type UnarchiveWorkItem struct {
	Lifecycle LifecycleWriter
}

// LifecycleCommand is the input every lifecycle verb takes: which entry, and the version the caller
// read. The same shape for all of them, because they all take exactly that.
type LifecycleCommand struct {
	ItemID shared.ID
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read none
	// and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute archives the entry and returns it as it now stands.
func (h ArchiveWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd LifecycleCommand,
) (domain.WorkItem, error) {
	return h.Lifecycle.change(ctx, actor, cmd, archiving)
}

// Execute unarchives the entry and returns it as it now stands.
func (h UnarchiveWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd LifecycleCommand,
) (domain.WorkItem, error) {
	return h.Lifecycle.change(ctx, actor, cmd, unarchiving)
}

// itemVerb is one lifecycle transition, as the writer needs to know it.
//
// A value rather than a boolean or a string at the call site: this is the parameter that decides
// which audit trail is written and which event leaves the installation, and `change(ctx, actor, cmd,
// true)` would say nothing about either.
type itemVerb struct {
	action audit.Action
	// step is what the entry's own history records. Beside the audit code rather than derived from
	// it: the two vocabularies are different by design - the trail is filtered on by an auditor, the
	// history is rendered to a person - and a mapping between them would be a third place for them
	// to drift (audit.md §1).
	step activity.Verb
	// batch names the deletion this verb acts on: a fresh identifier on the way into the trash, the
	// one already on the row on the way out, and none at all for the archive verbs, which are not
	// about a deletion. Nil where there is none - the writer then passes the zero identifier, and
	// the transitions that do not read it never look.
	batch func(domain.WorkItem, clock.IDGenerator) shared.ID
	// apply is the domain transition. It reports no changes when the entry already says what the
	// caller asked it to say, which is what makes every one of these verbs idempotent.
	apply func(domain.WorkItem, time.Time, shared.ID) (domain.WorkItem, []domain.FieldChange, error)
	// store writes the decided state and reports how many rows it touched. One for a stamp on a
	// single entry; a subtree's size for the verbs that take one with them.
	store func(context.Context, repository.Items, repository.ItemTrash) (int, error)
	// announce builds the event. It is handed the row count because the trash events carry it - a
	// consumer cannot derive how much went, since nothing below the entry is announced separately.
	announce func(shared.ID, domain.WorkItem, int, event.Actor, time.Time) (event.Envelope, error)
	// op is what an offline client is told: an upsert for a state change it should apply, a deletion
	// for an entry it should drop (offline-sync.md §3.1).
	op changelog.Operation
	// guard is what this verb refuses beyond what the transition itself does. Nil where the
	// transition is the whole of it.
	guard func(context.Context, LifecycleWriter, domain.WorkItem) error
	// schedulesRetention marks the verb that puts something on a clock. Only the way into the trash
	// does: a restore takes something off the clock, and archiving never puts anything on one.
	schedulesRetention bool
}

var (
	archiving = itemVerb{
		action: ItemArchivedAction,
		step:   activity.ItemArchived,
		apply: func(item domain.WorkItem, now time.Time, _ shared.ID) (domain.WorkItem, []domain.FieldChange, error) {
			return item.Archived(now)
		},
		store:    storeArchiveStamp,
		announce: announceItem(event.NewItemArchived),
		op:       changelog.Upsert,
	}
	unarchiving = itemVerb{
		action: ItemUnarchivedAction,
		step:   activity.ItemUnarchived,
		apply: func(item domain.WorkItem, now time.Time, _ shared.ID) (domain.WorkItem, []domain.FieldChange, error) {
			return item.Unarchived(now)
		},
		store:    storeArchiveStamp,
		announce: announceItem(event.NewItemUnarchived),
		op:       changelog.Upsert,
	}
)

// storeArchiveStamp writes the stamp on one row. One row is the whole of it, so the count is one.
func storeArchiveStamp(
	ctx context.Context, items repository.Items, trash repository.ItemTrash,
) (int, error) {
	if err := items.SetArchived(ctx, trash.Item, trash.ExpectedVersion); err != nil {
		return 0, err
	}
	return 1, nil
}

// announceItem and announceSubtree adapt the event constructors to the one shape the writer calls.
//
// Two adapters rather than one signature everywhere, because the difference is real: the archive
// verbs are about one entry and have no count for their payload to carry, and the trash verbs have
// one that a consumer cannot derive - nothing below the entry is announced separately.
//
// The cause is empty at both call sites. These are things a person did; a lifecycle change caused by
// another event will pass its own chain when there is one to pass (automation.md §2).
func announceItem(
	build func(shared.ID, domain.WorkItem, event.Actor, time.Time, event.Cause) (event.Envelope, error),
) func(shared.ID, domain.WorkItem, int, event.Actor, time.Time) (event.Envelope, error) {
	return func(id shared.ID, item domain.WorkItem, _ int, actor event.Actor, at time.Time,
	) (event.Envelope, error) {
		return build(id, item, actor, at, event.Cause{})
	}
}

func announceSubtree(
	build func(shared.ID, domain.WorkItem, int, event.Actor, time.Time, event.Cause) (event.Envelope, error),
) func(shared.ID, domain.WorkItem, int, event.Actor, time.Time) (event.Envelope, error) {
	return func(id shared.ID, item domain.WorkItem, touched int, actor event.Actor, at time.Time,
	) (event.Envelope, error) {
		return build(id, item, touched, actor, at, event.Cause{})
	}
}

// change is the whole of what a lifecycle verb owes, once.
func (w LifecycleWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd LifecycleCommand, verb itemVerb,
) (domain.WorkItem, error) {
	if cmd.ItemID.IsZero() {
		return domain.WorkItem{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership at the hub applies downwards, and a path naming only the
	// collection would refuse somebody who does hold the right (domain-model.md §3.2). Nothing read
	// here is trusted afterwards - the state that decides the write is read again inside the
	// transaction that writes.
	subject, collection, err := w.collectionOf(ctx, actor, cmd.ItemID)
	if err != nil {
		return domain.WorkItem{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     verb.action,
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		TargetID:   cmd.ItemID,
		On:         changing(subject),
	}); err != nil {
		return domain.WorkItem{}, err
	}

	var changed domain.WorkItem
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		item, err := findItem(ctx, w.Items, cmd.ItemID)
		if err != nil {
			return err
		}
		collection, err := findCollection(ctx, w.Containers, item.CollectionID)
		if err != nil {
			return err
		}
		// I-C3: an archived or trashed collection is read-only, and its entries inherit that. The
		// archive of an entry inside one is a write into a subtree nobody may write to.
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}

		if verb.guard != nil {
			if err := verb.guard(ctx, w, item); err != nil {
				return err
			}
		}

		// The batch is decided before the transition, because the transition is what writes it onto
		// the row - and on the way out it has to be read off the row before being cleared.
		var batch shared.ID
		if verb.batch != nil {
			batch = verb.batch(item, w.IDs)
		}

		wanted, changes, err := verb.apply(item, now, batch)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// The entry already says what the caller asked it to say. Nothing is written, no version
			// is spent and nothing is announced - which is what makes a retry after a lost response
			// harmless rather than merely accepted.
			//
			// The If-Match is still honoured: a caller writing against a version somebody else has
			// moved on is told so even when its own change would have been a no-op, because the
			// state it was reasoning about is not the state that is there.
			if err := ensureExpectedVersion(item, cmd.ExpectedVersion); err != nil {
				return err
			}
			changed = item
			return nil
		}

		changed, err = w.write(ctx, actor, verb, repository.ItemTrash{
			Item: wanted, Prefix: item.Path, BatchID: batch,
		}, item.Version, cmd.ExpectedVersion, now)
		return err
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return changed, nil
}

// write stores the transition and records what it owes: the event outwards, the change log for
// offline clients, and the audit entry - all inside the caller's transaction (test AT-5).
func (w LifecycleWriter) write(
	ctx context.Context, actor appshared.ActorContext, verb itemVerb,
	stored repository.ItemTrash, currentVersion, expectedVersion int, now time.Time,
) (domain.WorkItem, error) {
	stored.ExpectedVersion = expectedVersion
	if stored.ExpectedVersion == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still the one the update matches on, so a concurrent write
		// between the read and here is still caught.
		stored.ExpectedVersion = currentVersion
	}

	touched, err := verb.store(ctx, w.Items, stored)
	if err != nil {
		return domain.WorkItem{}, err
	}
	after := stored.Item
	after.Version = stored.ExpectedVersion + 1

	// Built from the stored state rather than from the command, so that what the event says and what
	// the row holds cannot disagree.
	announcement, err := verb.announce(
		w.IDs.NewID(), after, touched, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now)
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordChange(ctx, after, actor, verb, announcement.Payload); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordAudit(ctx, after, actor, verb, touched, now); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordActivity(ctx, after, actor, verb, now); err != nil {
		return domain.WorkItem{}, err
	}
	if verb.schedulesRetention {
		if err := scheduleRetention(ctx, w.Queue, after.TenantID); err != nil {
			return domain.WorkItem{}, err
		}
	}
	return after, nil
}

// scheduleRetention asks for the tenant's sweep, in the transaction that made the deletion.
//
// The same transaction on purpose: a deletion whose cleanup was never scheduled would sit in the
// trash until somebody deleted something else, and nothing would say why. The dedupe key is the
// tenant, so a request that deletes twenty entries leaves one job - and a tenant whose sweep is
// already waiting has it pulled forward rather than duplicated (queue.Request.DedupeKey).
//
// A nil queue is a build without one, which is a state the composition root does not produce and a
// test may: nothing to schedule is better than a panic on the deletion path.
func scheduleRetention(ctx context.Context, jobs queue.Queue, tenantID shared.ID) error {
	if jobs == nil {
		return nil
	}
	return jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindRetentionSweep,
		TenantID:  tenantID,
		DedupeKey: tenantID.String(),
	})
}

// recordChange writes what an offline client has to be told (offline-sync.md §3.1).
//
// One entry for the whole transition rather than one per field, unlike a rename: the two stamps are
// not independently editable fields that merge per field, they are a state the server decides. A
// device that archived an entry while another edited its title keeps both, because the title's own
// entry is a different change with its own tag.
//
// A deletion carries no payload. There is nothing left to describe, and a tombstone with content
// would be a copy of the deleted entry living on in the log.
func (w LifecycleWriter) recordChange(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	verb itemVerb, snapshot map[string]any,
) error {
	if verb.op == changelog.Delete {
		snapshot = nil
	}
	return w.Changes.Record(ctx, changelog.Change{
		TenantID:    item.TenantID,
		Entity:      itemTarget,
		EntityID:    item.ID,
		Op:          verb.op,
		ContainerID: item.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         w.HLC.Next(),
		Payload:     snapshot,
	})
}

// recordActivity writes the step of the entry's own history.
//
// The verb is the whole of it: archiving an entry is what happened, and a diff of the stamp it
// wrote would say the same thing a second time in a worse way. That also makes the form question
// moot - a compact history and a full one record the same step here (domain-model.md §2).
//
// One step for the entry the caller named, and none for the subtree a deletion took with it. The
// subtree went as one act, and a history entry on each of four hundred descendants would be four
// hundred steps nobody performed - the entry above them is where the act is recorded, and it is
// the one somebody will look at.
func (w LifecycleWriter) recordActivity(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	verb itemVerb, now time.Time,
) error {
	return w.Activity.record(ctx, actor, item, verb.step, verbIsTheChange(), now)
}

// recordAudit writes the evidence.
//
// No title and no notes: user content stays out of the trail (rule 10), and this entry answers "who
// took this out of use, and when" without any of it. The row count travels because a deletion that
// took four hundred entries with it is a different event from one that took one, and an auditor
// reading the trail should not have to reconstruct that from the entries themselves.
func (w LifecycleWriter) recordAudit(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	verb itemVerb, touched int, now time.Time,
) error {
	changes := []audit.Change{
		{Field: "type", Classification: audit.Open, To: string(item.Type)},
		{Field: "collection_id", Classification: audit.Open, To: item.CollectionID.String()},
	}
	changes = append(changes, lifecycleStamps(item)...)

	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   item.TenantID,
		OccurredAt: now,
		Action:     verb.action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: itemTarget,
		TargetID:   item.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// lifecycleStamps is the state the trail records: both stamps, as they now stand.
//
// Both on every entry rather than only the one that moved, because an entry can be archived and in
// the trash at once (domain-model.md §3.4) - and an auditor reading one line should be able to see
// what state the entry ended in without reading the line before it. A timestamp is not user content,
// so it goes in as it stands.
func lifecycleStamps(item domain.WorkItem) []audit.Change {
	stamps := []audit.Change{{Field: "archived_at", Classification: audit.Open}}
	if item.ArchivedAt != nil {
		stamps[0].To = item.ArchivedAt.UTC().Format(time.RFC3339Nano)
	}

	deleted := audit.Change{Field: "deleted_at", Classification: audit.Open}
	if item.DeletedAt != nil {
		deleted.To = item.DeletedAt.UTC().Format(time.RFC3339Nano)
	}
	return append(stamps, deleted)
}

// collectionOf reads the collection an entry lives in, read-only and outside the write transaction,
// because the permission check needs it first.
func (w LifecycleWriter) collectionOf(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (domain.WorkItem, domain.Container, error) {
	return readItemScope(ctx, w.UnitOfWork, w.Items, w.Containers, actor, itemID)
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h ArchiveWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ArchiveWorkItemName,
		Summary: "Archives a task, work package or activity: it is kept and becomes read-only. " +
			"Distinct from the trash, which is a deletion with a retention period running against " +
			"it - an archived entry stays until somebody says otherwise. Nothing below it is " +
			"archived with it. Idempotent: archiving an archived entry succeeds and changes nothing.",
		SideEffects: "Writes the archive stamp, announces " + string(event.ItemArchived) +
			", records a change for offline clients, and writes an audit entry.",
		TokenScope: itemsWrite,
		Input:      lifecycleInput("The entry to archive."),
		Audit: usecase.AuditDeclaration{
			Action: ItemArchivedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemArchived},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h UnarchiveWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UnarchiveWorkItemName,
		Summary: "Makes an archived task, work package or activity writable again. Refused for an " +
			"entry in the trash: restore it first, and the answer says so. Idempotent.",
		SideEffects: "Clears the archive stamp, announces " + string(event.ItemUnarchived) +
			", records a change for offline clients, and writes an audit entry.",
		TokenScope: itemsWrite,
		Input:      lifecycleInput("The entry to unarchive."),
		Audit: usecase.AuditDeclaration{
			Action: ItemUnarchivedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemUnarchived},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// lifecycleInput is the input every lifecycle verb declares. One list, because they all take the
// same thing and a client that learned one from /meta/capabilities should not find the next spelled
// differently.
func lifecycleInput(itemDescription string) []usecase.Field {
	return []usecase.Field{
		{Name: "item_id", Kind: usecase.KindID, Required: true, Description: itemDescription},
		{
			Name: "expected_version", Kind: usecase.KindInt,
			Description: "The version last read, from the If-Match header over REST. Omitted means " +
				"the caller read none and accepts whatever is there; a version that has moved on " +
				"since is refused rather than overwritten.",
		},
	}
}

func (h ArchiveWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return invokeLifecycle(ctx, actor, in, h.Execute)
}

func (h UnarchiveWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return invokeLifecycle(ctx, actor, in, h.Execute)
}

// invokeLifecycle is the adapter between the catalogue's untyped input and the typed command, for
// all three channels at once. One implementation, because every lifecycle verb reads the same two
// fields and writes back the same projection.
func invokeLifecycle(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
	execute func(context.Context, appshared.ActorContext, LifecycleCommand) (domain.WorkItem, error),
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	item, err := execute(ctx, actor, LifecycleCommand{
		ItemID: itemID, ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}
