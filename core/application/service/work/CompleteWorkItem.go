// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"strconv"
	"time"

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
	CompleteWorkItemName = "CompleteWorkItem"
	ReopenWorkItemName   = "ReopenWorkItem"

	// ItemCompletedAction and ItemReopenedAction are the audit codes. Stable: an auditor filters on
	// them and a SIEM rule matches on them (audit.md §2).
	ItemCompletedAction audit.Action = "item.completed"
	ItemReopenedAction  audit.Action = "item.reopened"

	// rolledUp is what the history says of a completion that was a consequence rather than an act.
	// A value rather than a second verb, because what happened is the same thing - the difference
	// is who asked for it (I-W5).
	rolledUp = "ROLL_UP"
)

// CompletionWriter is what both directions of completion need.
//
// One dependency set held by both use cases rather than two copies of eleven fields. The two operations
// are the same walk in opposite directions - the same guards, the same roll-up, the same three records
// per item touched - and the only thing that differs is which state is being asked for. Keeping the
// machinery in one place is what makes "reopening rolls up exactly as completing does" true by
// construction rather than by two people remembering.
type CompletionWriter struct {
	Items      repository.Items
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Authorizer Authorizer
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	Activity   ActivityJournal
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// CompleteWorkItem marks a task, work package or activity done.
//
// Where the collection's policy rolls up, completing the last open child completes the item above it, and
// so on upwards until something is still open or the top is reached (invariant I-W5).
type CompleteWorkItem struct {
	Completion CompletionWriter
}

// ReopenWorkItem marks a completed item open again, and reopens the completed items above it where the
// policy rolls up.
type ReopenWorkItem struct {
	Completion CompletionWriter
}

// CompletionCommand is the input of both directions, typed.
type CompletionCommand struct {
	ItemID shared.ID
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read none and
	// accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute completes the item and returns it.
func (h CompleteWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CompletionCommand,
) (domain.WorkItem, error) {
	return h.Completion.change(ctx, actor, cmd, completing)
}

// Execute reopens the item and returns it.
func (h ReopenWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CompletionCommand,
) (domain.WorkItem, error) {
	return h.Completion.change(ctx, actor, cmd, reopening)
}

// direction is which way the caller asked for. Not a boolean: `change(ctx, actor, cmd, true)` at a call
// site says nothing, and this is the parameter that decides which of two audit trails is written.
type direction bool

const (
	completing direction = true
	reopening  direction = false
)

func (d direction) action() audit.Action {
	if d == completing {
		return ItemCompletedAction
	}
	return ItemReopenedAction
}

// change is the whole of both use cases.
func (w CompletionWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd CompletionCommand, want direction,
) (domain.WorkItem, error) {
	if cmd.ItemID.IsZero() {
		return domain.WorkItem{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}

	// The item and its collection are read before the permission question, because the answer depends on
	// the path: a membership at the hub applies downwards, and a path that named only the collection
	// would refuse somebody who does hold the right (domain-model.md §3.2). Nothing read here is trusted
	// afterwards - the state that decides the write is read again inside the transaction that writes.
	collection, err := w.collectionOf(ctx, actor, cmd.ItemID)
	if err != nil {
		return domain.WorkItem{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written inside
	// this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     want.action(),
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		TargetID:   cmd.ItemID,
	}); err != nil {
		return domain.WorkItem{}, err
	}

	var changed domain.WorkItem
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// One reading of the clock for the whole write, including every item the roll-up touches. Two
		// would let a parent say it was completed before the child that completed it.
		now := w.Clock.Now()

		item, err := w.findItem(ctx, cmd.ItemID)
		if err != nil {
			return err
		}
		collection, err := w.findCollection(ctx, item.CollectionID)
		if err != nil {
			return err
		}
		// I-C3: an archived or trashed collection is read-only, and its items inherit that.
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}
		if err := w.completable(ctx, item); err != nil {
			return err
		}

		wanted := item.Completed(actor.AccountID, now)
		if want == reopening {
			wanted = item.Reopened(now)
		}
		if wanted.Completion.IsCompleted == item.Completion.IsCompleted {
			// Already in the state asked for. Nothing is written, no version is spent and nothing is
			// announced - which is what makes a repeat harmless rather than merely accepted, and what
			// lets the roll-up reach one item twice without it moving twice.
			//
			// The If-Match is still honoured: a caller that wrote against a version somebody else has
			// moved on is told so, even when its own change would have been a no-op, because the state it
			// was reasoning about is not the state that is there.
			if err := ensureExpectedVersion(item, cmd.ExpectedVersion); err != nil {
				return err
			}
			changed = item
			return nil
		}

		written, announcement, err := w.write(ctx, actor, item, wanted, cmd.ExpectedVersion, want, now, event.Cause{})
		if err != nil {
			return err
		}
		changed = written

		return w.rollUp(ctx, actor, written, collection.CompletionPolicy, now, announcement)
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return changed, nil
}

// rollUp propagates a change upwards for as far as the policy and the tree allow (I-W5).
//
// It walks rather than recurses, and it stops the moment an item does not move: if a work package did not
// change, nothing above it can have either, and continuing would be a query per level for an answer
// already known. That is also what bounds the walk - along with the depth, which the capability profile
// caps.
//
// Each step reads the parent's children again rather than reasoning from the child that just changed.
// Completing one activity while another is open must complete nothing, and only the count knows that.
func (w CompletionWriter) rollUp(
	ctx context.Context, actor appshared.ActorContext, child domain.WorkItem,
	policy domain.CompletionPolicy, now time.Time, because event.Envelope,
) error {
	for parentID := child.ParentID; !parentID.IsZero(); {
		parent, err := w.findItem(ctx, parentID)
		if err != nil {
			return err
		}

		// An archived or trashed parent is not editable (I-W4). The child's own change stands - the
		// person's action succeeded - and the automatic consequence stops where it is not allowed rather
		// than failing the whole request.
		if err := w.completable(ctx, parent); err != nil {
			return nil //nolint:nilerr // the refusal bounds the roll-up; it does not undo the child's change
		}

		summary, err := w.Items.ChildCompletion(ctx, parent.ID)
		if err != nil {
			return err
		}

		var wanted domain.WorkItem
		switch service.RollUp(policy, parent.Completion, summary) {
		case service.ParentComplete:
			// Attributed to whoever completed the child. The roll-up is their action's consequence, and
			// an entry naming the system would hide who caused it.
			wanted = parent.Completed(actor.AccountID, now)
		case service.ParentReopen:
			wanted = parent.Reopened(now)
		case service.ParentUnchanged:
			return nil
		}

		// The parent's event is caused by the child's - the same correlation, one level deeper. That chain
		// is how a consumer tells a roll-up from a click without a second event type, and it is what the
		// loop protection reads: a tree deeper than the causation limit stops here rather than emitting an
		// event nobody may deliver (automation.md §2).
		written, announcement, err := w.write(
			ctx, actor, parent, wanted, parent.Version, directionOf(wanted), now, because.CausedBy())
		if err != nil {
			return err
		}

		parentID, because = written.ParentID, announcement
	}
	return nil
}

// directionOf reads the direction back off the state that was decided, so the roll-up cannot announce a
// completion while writing a reopening.
func directionOf(item domain.WorkItem) direction {
	if item.Completion.IsCompleted {
		return completing
	}
	return reopening
}

// write stores one item's completion and records what the change owes: the event outwards, the change log
// for offline clients, and the audit entry - all inside the caller's transaction (test AT-5).
func (w CompletionWriter) write(
	ctx context.Context, actor appshared.ActorContext, before, after domain.WorkItem,
	expectedVersion int, want direction, now time.Time, cause event.Cause,
) (domain.WorkItem, event.Envelope, error) {
	expected := expectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the check:
		// the version in hand is still the one the update matches on, so a concurrent write between the
		// read and here is still caught.
		expected = before.Version
	}
	if err := w.Items.SetCompletion(ctx, after, expected); err != nil {
		return domain.WorkItem{}, event.Envelope{}, err
	}
	after.Version = expected + 1

	// The announcement is built before it is appended and returned afterwards, because the roll-up chains
	// the next event's causation off it. Built from `after` rather than from the command, so what the
	// event says and what the row holds cannot disagree.
	announcement, err := w.announce(after, actor, want, now, cause)
	if err != nil {
		return domain.WorkItem{}, event.Envelope{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return domain.WorkItem{}, event.Envelope{}, err
	}
	if err := w.recordChange(ctx, after, actor, announcement.Payload); err != nil {
		return domain.WorkItem{}, event.Envelope{}, err
	}
	if err := w.recordAudit(ctx, before, after, actor, want, now); err != nil {
		return domain.WorkItem{}, event.Envelope{}, err
	}
	if err := w.recordActivity(ctx, after, actor, want, cause, now); err != nil {
		return domain.WorkItem{}, event.Envelope{}, err
	}
	return after, announcement, nil
}

// recordActivity writes the step of the item's own history.
//
// The change set carries one thing and only for the items above: whether this completion was
// somebody's act or the consequence of one. "Why did this task become done?" is the question the
// roll-up makes worth asking, and without it the history of a parent would show a completion its
// reader never performed and cannot account for (observability-reliability.md §4).
//
// No form question here. A direct completion moves no field, so a compact history and a full one
// would record the same thing - and a roll-up never reaches an activity, which has no children to
// be rolled up from (domain-model.md §2).
func (w CompletionWriter) recordActivity(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	want direction, cause event.Cause, now time.Time,
) error {
	verb := activity.ItemCompleted
	if want == reopening {
		verb = activity.ItemReopened
	}

	changeSet := verbIsTheChange()
	if !cause.CausationID.IsZero() {
		changeSet = activity.ChangeSet(activity.Full,
			activity.Field{Name: "cause", Detail: activity.WithValues, To: rolledUp})
	}
	return w.Activity.record(ctx, actor, item, verb, changeSet, now)
}

func (w CompletionWriter) announce(
	item domain.WorkItem, actor appshared.ActorContext, want direction, now time.Time, cause event.Cause,
) (event.Envelope, error) {
	eventActor := event.Actor{Kind: actor.Kind, ID: actor.AccountID}
	if want == completing {
		return event.NewItemCompleted(w.IDs.NewID(), item, eventActor, now, cause)
	}
	return event.NewItemReopened(w.IDs.NewID(), item, eventActor, now, cause)
}

// recordChange writes what an offline client has to be told (offline-sync.md §3.1).
//
// `completion` is the status field with meaning rather than a scalar attribute: a reopen is never silently
// discarded in favour of a concurrent completion, which is why it merges server-side and not by last
// writer wins. `version` and `updated_at` are derived and never merged.
func (w CompletionWriter) recordChange(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext, snapshot map[string]any,
) error {
	return w.Changes.Record(ctx, changelog.Change{
		TenantID:    item.TenantID,
		Entity:      itemTarget,
		EntityID:    item.ID,
		Op:          changelog.Upsert,
		ContainerID: item.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         w.HLC.Next(),
		Payload:     snapshot,
	})
}

// recordAudit writes the evidence.
//
// The completion state is `OPEN` in the data catalogue's sense - a status, in clear text, which is exactly
// the example audit.md §4 gives. No title and no notes: user content stays out of the trail (rule 10), and
// this entry answers "who closed this, and when" without any of it.
func (w CompletionWriter) recordAudit(
	ctx context.Context, before, after domain.WorkItem, actor appshared.ActorContext,
	want direction, now time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   after.TenantID,
		OccurredAt: now,
		Action:     want.action(),
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: itemTarget,
		TargetID:   after.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{
				Field: "is_completed", Classification: audit.Open,
				From: strconv.FormatBool(before.Completion.IsCompleted),
				To:   strconv.FormatBool(after.Completion.IsCompleted),
			},
			audit.Change{Field: "type", Classification: audit.Open, To: string(after.Type)},
			audit.Change{
				Field: "collection_id", Classification: audit.Open,
				To: after.CollectionID.String(),
			},
		),
	})
}

// completable refuses an item whose type or state does not allow its completion to change.
func (w CompletionWriter) completable(ctx context.Context, item domain.WorkItem) error {
	profile, err := profileOf(ctx, w.Profiles, item.Type)
	if err != nil {
		return err
	}
	return item.EnsureCompletable(profile)
}

// collectionOf reads the collection an item lives in, read-only and outside the write transaction, because
// the permission check needs it first.
func (w CompletionWriter) collectionOf(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (domain.Container, error) {
	var collection domain.Container

	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		item, err := w.findItem(ctx, itemID)
		if err != nil {
			return err
		}
		collection, err = w.findCollection(ctx, item.CollectionID)
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return collection, nil
}

func (w CompletionWriter) findItem(ctx context.Context, id shared.ID) (domain.WorkItem, error) {
	return findItem(ctx, w.Items, id)
}

func (w CompletionWriter) findCollection(ctx context.Context, id shared.ID) (domain.Container, error) {
	return findCollection(ctx, w.Containers, id)
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through REST, MCP
// and automation at once (arc42 §4) - and completion is the automation trigger and action that matters
// most, so all three doors leading to this one handler is the point rather than a nicety.
func (h CompleteWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CompleteWorkItemName,
		Summary: "Marks a task, work package or activity done. Idempotent: completing an item that is " +
			"already done succeeds and changes nothing. Where the collection is configured to roll up, " +
			"completing the last open child also completes the item above it, and so on upwards until " +
			"something is still open.",
		SideEffects: "Writes the completion, announces " + string(event.ItemCompleted) +
			" for every item it changes, records a change for offline clients, and writes an audit entry.",
		TokenScope: itemsWrite,
		Input:      completionInput("The item to complete."),
		Audit: usecase.AuditDeclaration{
			Action: ItemCompletedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemCompleted},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h ReopenWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReopenWorkItemName,
		Summary: "Marks a completed task, work package or activity open again. Idempotent: reopening an " +
			"item that is already open succeeds and changes nothing. Where the collection is configured " +
			"to roll up, reopening an item also reopens the completed items above it.",
		SideEffects: "Writes the completion, announces " + string(event.ItemReopened) +
			" for every item it changes, records a change for offline clients, and writes an audit entry.",
		TokenScope: itemsWrite,
		Input:      completionInput("The item to reopen."),
		Audit: usecase.AuditDeclaration{
			Action: ItemReopenedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemReopened},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// completionInput is the input both directions declare. One list, because the two take the same thing and
// a client that learned one from /meta/capabilities should not find the other spelled differently.
func completionInput(itemDescription string) []usecase.Field {
	return []usecase.Field{
		{Name: "item_id", Kind: usecase.KindID, Required: true, Description: itemDescription},
		{
			Name: "expected_version", Kind: usecase.KindInt,
			Description: "The version last read, from the If-Match header over REST. Omitted means the " +
				"caller read none and accepts whatever is there; a version that has moved on since is " +
				"refused rather than overwritten.",
		},
		{
			Name: "cascade_children", Kind: usecase.KindBool,
			Description: "Reserved. Only false is accepted: completing a whole subtree in one call is not " +
				"implemented on this installation, and sending true is refused rather than silently " +
				"ignored.",
		},
	}
}

// invoke is the adapter between the catalogue's untyped input and the typed command, for all three
// channels at once.
func (h CompleteWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := completionCommand(in)
	if err != nil {
		return nil, err
	}
	item, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}

func (h ReopenWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := completionCommand(in)
	if err != nil {
		return nil, err
	}
	item, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}

// completionCommand reads the untyped input, and refuses the one field the contract declares that this
// installation does not serve.
//
// `cascade_children` is in api/openapi.yaml and completing a whole subtree in one call is not part of
// B-07. It is declared here and refused when true rather than left out of the declaration entirely,
// because the two failures are different: a field the catalogue does not know comes back as
// `usecase.field_unknown`, which tells a client it misspelled something, and a client sending the
// documented default `false` would then be refused for asking for exactly what it gets. Refusing only
// `true` says the true thing - this installation cannot do that yet.
func completionCommand(in usecase.Input) (CompletionCommand, error) {
	if in.Bool("cascade_children") {
		return CompletionCommand{}, shared.ErrValidation.
			WithDetail("items.cascade_not_supported").
			WithFields(shared.FieldError{
				Path: "/cascade_children", Code: "items.cascade_not_supported",
			})
	}

	itemID, err := in.ID("item_id")
	if err != nil {
		return CompletionCommand{}, err
	}
	return CompletionCommand{ItemID: itemID, ExpectedVersion: in.Int("expected_version")}, nil
}
