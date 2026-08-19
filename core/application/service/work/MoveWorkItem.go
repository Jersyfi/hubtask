// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	MoveWorkItemName    = "MoveWorkItem"
	ReorderWorkItemName = "ReorderWorkItem"

	// ItemMovedAction is the audit code for both operations. One code, because domain-model.md §4 gives
	// movement one event and an auditor asking "who moved this" means both - a reorder is recognisable in the
	// entry by the parent being unchanged (audit.md §2).
	ItemMovedAction audit.Action = "item.moved"
)

// PlacementWriter is what moving and reordering share.
//
// One dependency set held by both use cases, on the reasoning that keeps them one event type: a reorder is a
// move that keeps its parent. What differs is how much has to be rewritten, and that is one branch rather than
// two implementations.
type PlacementWriter struct {
	Items      repository.Items
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Authorizer Authorizer
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// MoveWorkItem moves an item, and its subtree with it, to a new parent or a new collection.
type MoveWorkItem struct {
	Placement PlacementWriter
}

// ReorderWorkItem changes an item's rank among the items it already sits with.
type ReorderWorkItem struct {
	Placement PlacementWriter
}

// MoveWorkItemCommand is the input, typed.
type MoveWorkItemCommand struct {
	ItemID shared.ID
	// TargetParentID is the item to move under. Meaningful only together with ParentGiven: the zero value is
	// both "the top level" and "not asked for", and those are different requests.
	TargetParentID shared.ID
	// ParentGiven says the caller named a parent, null included. Without it, a request that omitted the field
	// entirely would be read as "move to the top level" - which is how an item ends up somewhere nobody asked
	// for.
	ParentGiven bool
	// TargetCollectionID may be omitted when a parent is given: an item's collection is the one its parent is
	// in, and making a client repeat it is making it possible to contradict.
	TargetCollectionID shared.ID
	// BeforeItemID is the sibling to land in front of at the destination. Empty appends.
	BeforeItemID    shared.ID
	ExpectedVersion int
}

// ReorderWorkItemCommand is the input, typed.
type ReorderWorkItemCommand struct {
	ItemID          shared.ID
	BeforeItemID    shared.ID
	ExpectedVersion int
}

// MoveResult is what a move answers with: the item, and the references a change of collection could not carry
// over (I-W6).
type MoveResult struct {
	Item domain.WorkItem
	// SubtreeSize is how many rows moved, the item included. The event reports it so that a client knows how
	// much of its own copy to rewrite.
	SubtreeSize int
	// DroppedReferences is always empty on this version. Labels, buckets and members arrive with B-09 and
	// 0.3.0, so a change of collection has nothing yet that could fail to resolve - the field exists because
	// the contract declares it and a client should not have to change shape when it fills.
	DroppedReferences []DroppedReference
}

// DroppedReference is one reference a move could not carry into the destination collection.
type DroppedReference struct {
	Kind string
	ID   shared.ID
	Code string
}

// Execute moves the item and returns it.
func (h MoveWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd MoveWorkItemCommand,
) (MoveResult, error) {
	if cmd.ItemID.IsZero() {
		return MoveResult{}, itemIDRequired()
	}

	plan, err := h.Placement.plan(ctx, actor, cmd)
	if err != nil {
		return MoveResult{}, err
	}
	return h.Placement.perform(ctx, actor, plan)
}

// Execute reorders the item and returns it.
//
// A reorder is a move that keeps its parent, and it is expressed as exactly that: the same plan, with the
// destination read off the item rather than from the request. What it does not do is rewrite the subtree -
// nothing moved, so no path changed, and a statement over the subtree would spend a version on every
// descendant for no reason.
func (h ReorderWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ReorderWorkItemCommand,
) (domain.WorkItem, error) {
	if cmd.ItemID.IsZero() {
		return domain.WorkItem{}, itemIDRequired()
	}

	plan, err := h.Placement.plan(ctx, actor, MoveWorkItemCommand{
		ItemID:          cmd.ItemID,
		BeforeItemID:    cmd.BeforeItemID,
		ExpectedVersion: cmd.ExpectedVersion,
	})
	if err != nil {
		return domain.WorkItem{}, err
	}

	result, err := h.Placement.perform(ctx, actor, plan)
	if err != nil {
		return domain.WorkItem{}, err
	}
	return result.Item, nil
}

func itemIDRequired() error {
	return shared.ErrValidation.
		WithDetail("items.item_id_required").
		WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
}

// placement is a decided move: what is being moved, to where, and whether anything actually changes.
type placement struct {
	item   domain.WorkItem
	parent *domain.WorkItem
	// source and destination are the collections either side. Equal for a move within one collection, which
	// is the ordinary case.
	source      domain.Container
	destination domain.Container
	command     MoveWorkItemCommand
}

// reparents reports whether the tree changes shape, as opposed to only the order changing. It is what decides
// whether the subtree's paths have to be rewritten at all.
func (p placement) reparents() bool {
	return p.item.ParentID != p.command.TargetParentID || p.source.ID != p.destination.ID
}

// plan reads everything the move depends on and asks the permission questions.
//
// Read-only and outside the write transaction, because the answers are needed before the permission check and
// the permission check has to happen before the transaction: a refusal writes an audit entry, and an entry
// written inside the caller's transaction would be rolled back with the refusal (audit.md §7). Nothing read
// here is trusted afterwards - the state that decides the write is read again inside the transaction.
func (w PlacementWriter) plan(
	ctx context.Context, actor appshared.ActorContext, cmd MoveWorkItemCommand,
) (placement, error) {
	var plan placement

	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		plan, err = w.read(ctx, cmd)
		return err
	})
	if err != nil {
		return placement{}, err
	}

	// The destination first, because that is where the item ends up and the commoner refusal.
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(plan.destination),
		Action:     ItemMovedAction,
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		TargetID:   cmd.ItemID,
	}); err != nil {
		return placement{}, err
	}

	// And the source, when the two differ. Moving an item out of a collection is a change to that collection
	// as much as to the destination, and a member of only the destination must not be able to reach into a
	// collection they cannot write to and take something out of it.
	if plan.source.ID != plan.destination.ID {
		if err := w.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: service.PermissionWriteItems,
			Path:       containerPath(plan.source),
			Action:     ItemMovedAction,
			TokenScope: itemsWrite,
			TargetType: itemTarget,
			TargetID:   cmd.ItemID,
		}); err != nil {
			return placement{}, err
		}
	}
	return plan, nil
}

// read resolves the item, the destination parent and both collections.
func (w PlacementWriter) read(ctx context.Context, cmd MoveWorkItemCommand) (placement, error) {
	item, err := w.findItem(ctx, cmd.ItemID)
	if err != nil {
		return placement{}, err
	}

	source, err := w.findCollection(ctx, item.CollectionID)
	if err != nil {
		return placement{}, err
	}

	// What the caller asked for, resolved into a parent and a collection. Absent means unchanged, which is
	// what makes a reorder the same plan as a move.
	targetParentID := item.ParentID
	if cmd.ParentGiven {
		targetParentID = cmd.TargetParentID
	}

	var parent *domain.WorkItem
	if !targetParentID.IsZero() {
		found, err := w.findParent(ctx, targetParentID)
		if err != nil {
			return placement{}, err
		}
		parent = &found
	}

	destination := source
	switch {
	case parent != nil:
		// The parent decides the collection. A client may name it as well, and it has to agree.
		if !cmd.TargetCollectionID.IsZero() && cmd.TargetCollectionID != parent.CollectionID {
			return placement{}, shared.ErrValidation.
				WithDetail("items.collection_contradicts_parent").
				WithParams(map[string]string{
					"collection_id": cmd.TargetCollectionID.String(),
					"parent_id":     parent.ID.String(),
				}).
				WithFields(shared.FieldError{
					Path: "/target_collection_id", Code: "items.collection_contradicts_parent",
				})
		}
		if parent.CollectionID != source.ID {
			if destination, err = w.findCollection(ctx, parent.CollectionID); err != nil {
				return placement{}, err
			}
		}
	case !cmd.TargetCollectionID.IsZero():
		// The top level of a named collection.
		if destination, err = w.findNamedCollection(ctx, cmd.TargetCollectionID); err != nil {
			return placement{}, err
		}
	case cmd.ParentGiven:
		// The top level, and no collection named. There is nothing to place the item against: it is leaving
		// its parent and the request does not say where to.
		return placement{}, shared.ErrValidation.
			WithDetail("items.collection_or_parent_required").
			WithFields(shared.FieldError{
				Path: "/target_collection_id", Code: "items.collection_or_parent_required",
			})
	}

	cmd.TargetParentID = targetParentID
	return placement{
		item: item, parent: parent, source: source, destination: destination, command: cmd,
	}, nil
}

// perform writes the move inside one transaction.
func (w PlacementWriter) perform(
	ctx context.Context, actor appshared.ActorContext, plan placement,
) (MoveResult, error) {
	var result MoveResult

	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// One reading of the clock for the whole write, so the item and its subtree agree about when they
		// moved.
		now := w.Clock.Now()

		// Read again inside the transaction: everything the placement was decided from can have changed since,
		// and the cycle check in particular has to be true at the moment the rows are written.
		fresh, err := w.read(ctx, plan.command)
		if err != nil {
			return err
		}
		if err := fresh.destination.EnsureAcceptsItems(); err != nil {
			return err
		}
		if fresh.source.ID != fresh.destination.ID {
			if err := fresh.source.EnsureAcceptsItems(); err != nil {
				return err
			}
		}
		if fresh.item.IsTrashed() || fresh.item.IsArchived() {
			// I-W4: a trashed or archived item is not editable except through Restore or Unarchive.
			return notEditable(fresh.item)
		}

		hierarchy, err := w.hierarchy(ctx)
		if err != nil {
			return err
		}
		spot, err := hierarchy.Move(fresh.item, fresh.parent)
		if err != nil {
			return err
		}

		orderKey, err := w.rankAt(ctx, fresh, spot)
		if err != nil {
			return err
		}

		result, err = w.write(ctx, actor, fresh, spot, orderKey, now)
		return err
	})
	if err != nil {
		return MoveResult{}, err
	}
	return result, nil
}

// rankAt works out the rank the item takes at its destination.
//
// The bounds come from the database and the key from the ordering service, which is what makes an insertion
// between two neighbours renumber nothing: a fractional index needs only the two keys either side
// (offline-sync.md §4.2).
func (w PlacementWriter) rankAt(
	ctx context.Context, plan placement, spot service.Placement,
) (string, error) {
	level := repository.Level{CollectionID: plan.destination.ID, ParentID: spot.ParentID}

	previous, next, err := w.Items.Neighbours(ctx, level, plan.command.BeforeItemID, plan.item.ID)
	if err != nil {
		return "", err
	}
	if !plan.command.BeforeItemID.IsZero() && next == "" {
		// The sibling named is not at this level. Its own answer rather than a silent append: a client that
		// asked for a position and got the end of the list has been ignored.
		return "", shared.ErrValidation.
			WithDetail("items.before_item_not_in_level").
			WithParams(map[string]string{"before_item_id": plan.command.BeforeItemID.String()}).
			WithFields(shared.FieldError{
				Path: "/before_item_id", Code: "items.before_item_not_in_level",
			})
	}
	return service.OrderKeyBetween(previous, next)
}

// write stores the move and records what it owes: the event outwards, the change log for offline clients, and
// the audit entry - all inside the caller's transaction (test AT-5).
func (w PlacementWriter) write(
	ctx context.Context, actor appshared.ActorContext, plan placement,
	spot service.Placement, orderKey string, now time.Time,
) (MoveResult, error) {
	before := plan.item
	expected := plan.command.ExpectedVersion
	if expected == 0 {
		expected = before.Version
	}

	after := before
	after.ParentID = spot.ParentID
	after.CollectionID = plan.destination.ID
	after.Path = before.SubtreePathUnder(spot.ParentPath())
	after.Depth = spot.Depth
	after.OrderKey = orderKey
	after.UpdatedAt = now

	size := 1
	if plan.reparents() {
		moved, err := w.Items.MoveSubtree(ctx, repository.Move{
			Item:            before,
			TargetParentID:  spot.ParentID,
			CollectionID:    plan.destination.ID,
			OldPrefix:       before.Path,
			NewPrefix:       after.Path,
			DepthDelta:      after.Depth - before.Depth,
			OrderKey:        orderKey,
			UpdatedAt:       now,
			ExpectedVersion: expected,
		})
		if err != nil {
			return MoveResult{}, err
		}
		size = moved
	} else {
		// Nothing moved, so no path changed and the subtree is untouched. Writing one row rather than the
		// whole subtree is the difference between a reorder and a move.
		if err := w.Items.SetOrderKey(ctx, after, expected); err != nil {
			return MoveResult{}, err
		}
	}
	after.Version = expected + 1

	announcement, err := event.NewItemMoved(w.IDs.NewID(), after, event.Movement{
		FromParentID:     before.ParentID,
		FromPath:         before.Path,
		FromCollectionID: before.CollectionID,
	}, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return MoveResult{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return MoveResult{}, err
	}
	if err := w.recordChange(ctx, after, actor, announcement.Payload); err != nil {
		return MoveResult{}, err
	}
	if err := w.recordAudit(ctx, before, after, actor, now); err != nil {
		return MoveResult{}, err
	}

	// Empty, and present: there is nothing yet that a change of collection could fail to carry over (I-W6).
	return MoveResult{Item: after, SubtreeSize: size, DroppedReferences: []DroppedReference{}}, nil
}

// recordChange writes what an offline client has to be told (offline-sync.md §3.1).
//
// `order_key` is a fractional index and merges by itself: two devices that inserted into the same list both
// keep their position, which is the whole reason the rank is a key rather than a number. `parent_id`, `path`
// and `depth` are the hierarchy, which is last writer wins with cycle detection on the server - a merge that
// would make a cycle is rejected rather than merged (offline-sync.md §4.2). The payload carries the path the
// item came from, so a client rewrites its own copy of the subtree from one entry.
func (w PlacementWriter) recordChange(
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

// recordAudit writes the evidence. All of it is structure - identifiers and a rank - so all of it is OPEN in
// the data catalogue's sense. No title: user content stays out of the trail (rule 10), and "who moved this
// where" needs none of it.
func (w PlacementWriter) recordAudit(
	ctx context.Context, before, after domain.WorkItem, actor appshared.ActorContext, now time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   after.TenantID,
		OccurredAt: now,
		Action:     ItemMovedAction,
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
				Field: "parent_id", Classification: audit.Open,
				From: idOrNil(before.ParentID), To: idOrNil(after.ParentID),
			},
			audit.Change{
				Field: "collection_id", Classification: audit.Open,
				From: before.CollectionID.String(), To: after.CollectionID.String(),
			},
			audit.Change{
				Field: "order_key", Classification: audit.Open,
				From: before.OrderKey, To: after.OrderKey,
			},
		),
	})
}

// hierarchy builds the rules in force, the same way CreateWorkItem does and for the same reason: read off a
// narrowed set alone, the topology comes out wrong (NewHierarchy).
func (w PlacementWriter) hierarchy(ctx context.Context) (service.Hierarchy, error) {
	inForce, err := w.Profiles.List(ctx)
	if err != nil {
		return service.Hierarchy{}, err
	}
	system, err := w.Profiles.ListSystem(ctx)
	if err != nil {
		return service.Hierarchy{}, err
	}
	return service.NewHierarchy(inForce, system)
}

func (w PlacementWriter) findItem(ctx context.Context, id shared.ID) (domain.WorkItem, error) {
	item, err := w.Items.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.WorkItem{}, shared.ErrNotFound.
				WithDetail("items.not_found").
				WithParams(map[string]string{"item_id": id.String()})
		}
		return domain.WorkItem{}, err
	}
	return item, nil
}

func (w PlacementWriter) findParent(ctx context.Context, id shared.ID) (domain.WorkItem, error) {
	parent, err := w.Items.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.WorkItem{}, shared.ErrNotFound.
				WithDetail("items.parent_not_found").
				WithParams(map[string]string{"parent_id": id.String()}).
				WithFields(shared.FieldError{Path: "/target_parent_id", Code: "items.parent_not_found"})
		}
		return domain.WorkItem{}, err
	}
	return parent, nil
}

// findCollection reads a collection the item already belongs to. A missing one is a defect rather than a
// client's mistake: a tenant-scoped foreign key makes it unreachable (ADR-0024).
func (w PlacementWriter) findCollection(ctx context.Context, id shared.ID) (domain.Container, error) {
	collection, err := w.Containers.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.Container{}, shared.ErrInternal.
				WithDetail("items.collection_missing").WithCause(err)
		}
		return domain.Container{}, err
	}
	return collection, nil
}

// findNamedCollection reads a collection the caller named, where absent is the caller's mistake and says so.
func (w PlacementWriter) findNamedCollection(ctx context.Context, id shared.ID) (domain.Container, error) {
	collection, err := w.Containers.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.Container{}, shared.ErrNotFound.
				WithDetail("items.collection_not_found").
				WithParams(map[string]string{"collection_id": id.String()}).
				WithFields(shared.FieldError{
					Path: "/target_collection_id", Code: "items.collection_not_found",
				})
		}
		return domain.Container{}, err
	}
	return collection, nil
}

func notEditable(item domain.WorkItem) error {
	detail := "items.archived"
	if item.IsTrashed() {
		detail = "items.trashed"
	}
	return shared.ErrConflict.
		WithDetail(detail).
		WithParams(map[string]string{"item_id": item.ID.String()})
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through REST, MCP and
// automation at once (arc42 §4) - and a move is the action a kanban rule performs, so the automation door is
// not a nicety here.
func (h MoveWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: MoveWorkItemName,
		Summary: "Moves an item, and everything below it, under a different parent or into a different " +
			"collection. Which placements are permitted is configured per workspace rather than fixed, so a " +
			"refusal names the reason. An item can never be moved into its own subtree.",
		SideEffects: "Writes the item and rewrites its subtree's paths, announces " + string(event.ItemMoved) +
			", records a change for offline clients, and writes an audit entry.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The item to move. Its subtree moves with it.",
			},
			{
				Name: "target_parent_id", Kind: usecase.KindID,
				Description: "The item to move it under. Send it as null to move to the top level of a " +
					"collection, which then has to be named. Omit it to keep the parent it has.",
			},
			{
				Name: "target_collection_id", Kind: usecase.KindID,
				Description: "The destination collection. May be omitted when a parent is given: the " +
					"collection is then the parent's, and naming a different one is refused rather than " +
					"quietly preferred.",
			},
			{
				Name: "before_item_id", Kind: usecase.KindID,
				Description: "The sibling to land in front of at the destination. Omitted appends to the end.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read. Omitted means the caller read none and accepts whatever " +
					"is there; a version that has moved on since is refused rather than overwritten.",
			},
			{
				Name: "target_bucket_id", Kind: usecase.KindID,
				Description: "Reserved. Buckets arrive with their own use case, and sending one is refused " +
					"rather than silently ignored.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemMovedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReorderWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReorderWorkItemName,
		Summary: "Changes an item's position among the items it already sits with. The position is given as " +
			"the sibling to place it before; omitting it moves the item to the end. The rank is a fractional " +
			"index, so nothing else is renumbered and two offline devices can reorder the same list without " +
			"either one's order being lost.",
		SideEffects: "Writes the item's rank, announces " + string(event.ItemMoved) +
			" with the parent unchanged, records a change for offline clients, and writes an audit entry.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The item to reorder."},
			{
				Name: "before_item_id", Kind: usecase.KindID,
				Description: "The sibling to place it before. Omitted moves it to the end of its level.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read, as on a move.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemMovedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// invoke is the adapter between the catalogue's untyped input and the typed command, for all three channels at
// once.
func (h MoveWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if in.Present("target_bucket_id") {
		// In the contract and served by nothing yet. Refused by name rather than dropped: a client that moved
		// a card into a bucket and got a 200 believes the card is in that bucket (B-09).
		return nil, shared.ErrValidation.
			WithDetail("items.bucket_not_supported").
			WithFields(shared.FieldError{Path: "/target_bucket_id", Code: "items.bucket_not_supported"})
	}

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
	beforeID, err := in.ID("before_item_id")
	if err != nil {
		return nil, err
	}

	result, err := h.Execute(ctx, actor, MoveWorkItemCommand{
		ItemID:         itemID,
		TargetParentID: parentID,
		// Present, not non-empty: a client that sends `"target_parent_id": null` is asking for the top level,
		// and one that omits the field is asking for nothing to change. Those are different requests and the
		// difference is invisible in the value.
		ParentGiven:        in.Present("target_parent_id"),
		TargetCollectionID: collectionID,
		BeforeItemID:       beforeID,
		ExpectedVersion:    in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return moveOutput(result), nil
}

func (h ReorderWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	beforeID, err := in.ID("before_item_id")
	if err != nil {
		return nil, err
	}

	item, err := h.Execute(ctx, actor, ReorderWorkItemCommand{
		ItemID:          itemID,
		BeforeItemID:    beforeID,
		ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}

// moveOutput is the shape a move answers with: the contract's MoveResult, whose `dropped_references` is present
// and empty until there is something a change of collection could fail to carry over (I-W6).
func moveOutput(result MoveResult) usecase.Output {
	dropped := make([]usecase.Output, 0, len(result.DroppedReferences))
	for _, reference := range result.DroppedReferences {
		dropped = append(dropped, usecase.Output{
			"kind": reference.Kind, "id": reference.ID.String(), "code": reference.Code,
		})
	}
	return usecase.Output{
		"item":               itemOutput(result.Item),
		"dropped_references": dropped,
		"subtree_size":       result.SubtreeSize,
	}
}
