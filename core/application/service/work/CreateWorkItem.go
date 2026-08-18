// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
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
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateWorkItemName = "CreateWorkItem"
	itemTarget         = "item"
	itemsWrite         = "items:write"

	// ItemCreatedAction is the audit code. Stable: an auditor filters on it and a SIEM rule
	// matches on it (audit.md §2).
	ItemCreatedAction audit.Action = "item.created"
)

// CreateWorkItemCommand is the input, typed.
//
// The fields are the ones this use case owns. Everything else the contract's WorkItemCreate
// declares - the bucket, the labels, the members, the assignee, the due date, the cover, the
// custom fields - belongs to a use case that has not landed yet, and is refused rather than
// accepted and dropped: the catalogue does not declare those fields, so a request carrying one
// comes back naming it (usecase.field_unknown) instead of returning a 201 for an item that is
// not what the caller asked for.
type CreateWorkItemCommand struct {
	Type domain.ItemType
	// CollectionID may be left empty when ParentID is given: an item's collection is the one its
	// parent is in, and making a client repeat it is making it possible to contradict.
	CollectionID shared.ID
	ParentID     shared.ID
	Title        string
	Notes        string
}

// CreateWorkItem creates a task, a work package, or an activity.
//
// It is the second walking skeleton, and it owes what CreateContainer owes: the permission check
// before the transaction, the invariants in the domain, and inside one transaction the row, the
// event, the change log entry for offline clients and the audit entry.
//
// What is new here is that the rules are data. Which type may sit under which, and how deep, come
// from the capability profiles through the hierarchy service (ADR-0006) - so this file, like that
// one, names none of the three levels.
type CreateWorkItem struct {
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

// Execute creates the item and returns it.
func (h CreateWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateWorkItemCommand,
) (domain.WorkItem, error) {
	// The collection has to be known before the permission question can be asked, because the
	// answer depends on it: a membership held at the hub applies downwards, and a path that named
	// only the collection would ignore it and refuse somebody who does have the right
	// (domain-model.md §3.2). So this read comes first - it decides nothing, it only says which
	// path the question is about.
	collectionID, path, err := h.scopeOf(ctx, actor, cmd)
	if err != nil {
		return domain.WorkItem{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       path,
		Action:     ItemCreatedAction,
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		// The item does not exist yet, so the refusal names the collection it would have been
		// created in.
		TargetID: collectionID,
	}); err != nil {
		return domain.WorkItem{}, err
	}

	var created domain.WorkItem
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// One reading of the clock for the whole write. Two would let the row say it was created
		// at one moment and the event say another, and the difference would show up as an item
		// whose `created_at` is not the `time` of the event announcing it.
		now := h.Clock.Now()

		item, err := h.build(ctx, actor, cmd, collectionID, now)
		if err != nil {
			return err
		}
		if err := h.Items.Insert(ctx, item); err != nil {
			return err
		}

		// One snapshot, two recipients - the event outwards as a public contract, the change log
		// to synchronising clients (offline-sync.md §10). They describe the same state, and
		// building it twice is how the two come to disagree.
		announcement, err := event.NewItemCreated(
			h.IDs.NewID(), item, event.Actor{Kind: actor.Kind, ID: actor.AccountID},
			now, event.Cause{})
		if err != nil {
			return err
		}
		if err := h.Events.Append(ctx, announcement); err != nil {
			return err
		}
		if err := h.recordChange(ctx, item, actor, announcement.Payload); err != nil {
			return err
		}
		if err := h.recordAudit(ctx, item, actor, now); err != nil {
			return err
		}

		created = item
		return nil
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return created, nil
}

// scopeOf resolves which collection the item will live in, and the authorisation path to it.
//
// Read-only and outside the write transaction, because its answer is needed before the permission
// check. Nothing it reads is trusted afterwards: the state that decides whether the write may
// happen is read again inside the transaction that writes, since anything read before it can have
// changed by the time it commits.
func (h CreateWorkItem) scopeOf(
	ctx context.Context, actor appshared.ActorContext, cmd CreateWorkItemCommand,
) (shared.ID, []identity.Scope, error) {
	var collection domain.Container

	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		collectionID := cmd.CollectionID

		if collectionID.IsZero() {
			if cmd.ParentID.IsZero() {
				// Neither was given, so there is nothing to place the item against. Its own code
				// rather than the one for a collection that is really a hub: the fix here is to
				// send a field, not to send a different value.
				return shared.ErrValidation.
					WithDetail("items.collection_or_parent_required").
					WithFields(shared.FieldError{
						Path: "/collection_id", Code: "items.collection_or_parent_required",
					})
			}
			parent, err := h.findParent(ctx, cmd.ParentID)
			if err != nil {
				return err
			}
			collectionID = parent.CollectionID
		}

		found, err := h.findCollection(ctx, collectionID)
		if err != nil {
			return err
		}
		collection = found
		return nil
	})
	if err != nil {
		return "", nil, err
	}

	// Built by containerPath, which the completion use cases need for the same reason: two copies of
	// this would eventually disagree about one level.
	return collection.ID, containerPath(collection), nil
}

// build reads the state the placement depends on and turns the command into an item. It runs
// inside the write transaction, so what it checked is still true when the row is written.
func (h CreateWorkItem) build(
	ctx context.Context, actor appshared.ActorContext, cmd CreateWorkItemCommand,
	collectionID shared.ID, now time.Time,
) (domain.WorkItem, error) {
	collection, err := h.findCollection(ctx, collectionID)
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := collection.EnsureAcceptsItems(); err != nil {
		return domain.WorkItem{}, err
	}

	parent, err := h.parentOf(ctx, cmd.ParentID, collection.ID)
	if err != nil {
		return domain.WorkItem{}, err
	}

	hierarchy, err := h.hierarchy(ctx)
	if err != nil {
		return domain.WorkItem{}, err
	}
	profile, err := hierarchy.Profile(cmd.Type)
	if err != nil {
		return domain.WorkItem{}, err
	}
	placement, err := hierarchy.Place(parent, cmd.Type)
	if err != nil {
		return domain.WorkItem{}, err
	}

	orderKey, err := h.nextItemOrderKey(ctx, collection.ID, placement.ParentID)
	if err != nil {
		return domain.WorkItem{}, err
	}

	id := h.IDs.NewID()
	return domain.NewWorkItem(domain.NewWorkItemInput{
		ID:           id,
		TenantID:     actor.TenantID,
		CollectionID: collection.ID,
		Type:         cmd.Type,
		ParentID:     placement.ParentID,
		Title:        cmd.Title,
		Notes:        cmd.Notes,
		Profile:      profile,
		Path:         placement.PathOf(id),
		Depth:        placement.Depth,
		OrderKey:     orderKey,
		CreatedBy:    actor.AccountID,
		Now:          now,
	})
}

// hierarchy builds the rules in force: the profiles this tenant sees - the system defaults with
// its own overrides where it has them - and the defaults themselves as the topology (ADR-0006).
//
// Both are needed rather than one. See NewHierarchy: read off a narrowed set alone, "which types
// sit directly under a collection" comes out wrong, and a tenant that took a task's children away
// would find work packages allowed at the top level.
func (h CreateWorkItem) hierarchy(ctx context.Context) (service.Hierarchy, error) {
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

// parentOf reads the item the new one will sit under, or nil when it will sit under the
// collection directly.
func (h CreateWorkItem) parentOf(
	ctx context.Context, parentID, collectionID shared.ID,
) (*domain.WorkItem, error) {
	if parentID.IsZero() {
		return nil, nil
	}

	parent, err := h.findParent(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if parent.CollectionID != collectionID {
		// Invariant I-W3: an item's references live in the same collection. It is also what keeps
		// the permission check honest - the right was asked for at one collection, and a parent
		// somewhere else would place the item under a different one.
		return nil, shared.ErrValidation.
			WithDetail("items.parent_not_in_collection").
			WithParams(map[string]string{"parent_id": parentID.String()}).
			WithFields(shared.FieldError{Path: "/parent_id", Code: "items.parent_not_in_collection"})
	}
	return &parent, nil
}

// findCollection reads the container the item belongs to. Not found and belonging to another
// tenant are one answer, because anything else confirms that another tenant's data exists
// (multi-tenancy.md §2).
func (h CreateWorkItem) findCollection(ctx context.Context, id shared.ID) (domain.Container, error) {
	collection, err := h.Containers.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.Container{}, shared.ErrNotFound.
				WithDetail("items.collection_not_found").
				WithParams(map[string]string{"collection_id": id.String()}).
				WithFields(shared.FieldError{
					Path: "/collection_id", Code: "items.collection_not_found",
				})
		}
		return domain.Container{}, err
	}
	return collection, nil
}

func (h CreateWorkItem) findParent(ctx context.Context, id shared.ID) (domain.WorkItem, error) {
	parent, err := h.Items.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.WorkItem{}, shared.ErrNotFound.
				WithDetail("items.parent_not_found").
				WithParams(map[string]string{"parent_id": id.String()}).
				WithFields(shared.FieldError{Path: "/parent_id", Code: "items.parent_not_found"})
		}
		return domain.WorkItem{}, err
	}
	return parent, nil
}

// nextItemOrderKey ranks the new item after its last sibling.
func (h CreateWorkItem) nextItemOrderKey(
	ctx context.Context, collectionID, parentID shared.ID,
) (string, error) {
	last, err := h.Items.LastOrderKey(ctx, collectionID, parentID)
	if err != nil {
		return "", err
	}
	return service.OrderKeyAfter(last)
}

// recordChange writes what an offline client has to be told (offline-sync.md §3.1), and states
// the merge rule for every field it carries - the Definition of Done asks for one per new field.
//
// `title` and `notes` are scalar attributes: last writer wins per field, decided by the HLC.
// `order_key` is a fractional index and merges by itself. `parent_id`, `path` and `depth` are the
// hierarchy, which is last writer wins with cycle detection on the server - a move that would
// make a cycle is rejected rather than merged (offline-sync.md §4.2). `completion` is the status
// field with meaning, where a reopen is never silently discarded. `version` and `collection_id`
// are decided server-side and never merged: one is derived, the other changes only through a move,
// which is a use case of its own.
func (h CreateWorkItem) recordChange(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext, snapshot map[string]any,
) error {
	return h.Changes.Record(ctx, changelog.Change{
		TenantID: item.TenantID,
		Entity:   itemTarget,
		EntityID: item.ID,
		Op:       changelog.Upsert,
		// The visibility filter a pull applies. An item is visible to a device subscribed to its
		// collection, at every level of the subtree - which is why the collection is denormalised
		// onto the row rather than resolved through the parent chain.
		ContainerID: item.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         h.HLC.Next(),
		Payload:     snapshot,
	})
}

// recordAudit writes the evidence, inside the same transaction as the change (test AT-5).
//
// The title is user content, so it is recorded as a fingerprint rather than as itself (rule 10,
// audit.md §4): enough to see that two entries concern the same title, not enough to read it. The
// notes are not recorded at all - a fingerprint of a free text field answers no question an
// auditor asks, and hashing it is a claim that it might.
func (h CreateWorkItem) recordAudit(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext, now time.Time,
) error {
	var parent any
	if !item.ParentID.IsZero() {
		parent = item.ParentID.String()
	}

	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   item.TenantID,
		OccurredAt: now,
		Action:     ItemCreatedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: itemTarget,
		TargetID:   item.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "type", Classification: audit.Open, To: string(item.Type)},
			audit.Change{Field: "collection_id", Classification: audit.Open, To: item.CollectionID.String()},
			audit.Change{Field: "parent_id", Classification: audit.Open, To: parent},
			audit.Change{Field: "depth", Classification: audit.Open, To: strconv.Itoa(item.Depth)},
			audit.Change{Field: "title", Classification: audit.Sensitive, To: item.Title},
		),
	})
}

// itemOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema WorkItem), so that a REST response, an MCP tool result and an
// automation action result describe the item in the same words.
func itemOutput(item domain.WorkItem) usecase.Output {
	out := usecase.Output{
		"id":            item.ID.String(),
		"type":          string(item.Type),
		"collection_id": item.CollectionID.String(),
		"parent_id":     nil,
		"path":          item.Path,
		"depth":         item.Depth,
		"title":         item.Title,
		"completion": map[string]any{
			"is_completed": item.Completion.IsCompleted,
			"completed_at": nil,
			"completed_by": nil,
		},
		"order_key":  item.OrderKey,
		"created_by": item.CreatedBy.String(),
		"created_at": item.CreatedAt,
		"updated_at": item.UpdatedAt,
		"version":    item.Version,
	}
	if !item.ParentID.IsZero() {
		out["parent_id"] = item.ParentID.String()
	}
	if item.Notes != "" {
		out["notes"] = item.Notes
	}
	return out
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h CreateWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateWorkItemName,
		Summary: "Creates a task, a work package or an activity. A task sits directly in a " +
			"collection; a work package sits in a task, and an activity in a work package. " +
			"Which combinations are permitted is configured per workspace rather than fixed, " +
			"so a refusal names the reason rather than the rule.",
		SideEffects: "Writes the item, announces " + string(event.ItemCreated) +
			", records a change for offline clients, and writes an audit entry.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{
				Name: "type", Kind: usecase.KindString, Required: true,
				Enum: []string{
					string(domain.ItemTask), string(domain.ItemWorkPackage), string(domain.ItemActivity),
				},
				Description: "The level: TASK in a collection, WORK_PACKAGE in a task, ACTIVITY in a work package.",
			},
			{
				Name: "title", Kind: usecase.KindString, Required: true,
				Description: "Up to 500 characters, on one line. Longer text belongs in the notes.",
			},
			{
				Name: "collection_id", Kind: usecase.KindID,
				Description: "The collection the item belongs to. May be omitted when parent_id " +
					"is given: the collection is then the parent's.",
			},
			{
				Name: "parent_id", Kind: usecase.KindID,
				Description: "The item this one sits in. Omitted for a task, which sits in the collection.",
			},
			{
				Name: "notes", Kind: usecase.KindString,
				Description: "Markdown, not rendered by the server. Only for types whose profile " +
					"carries the NOTES capability; on one that does not, sending it is refused " +
					"rather than ignored.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemCreatedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// invoke is the adapter between the catalogue's untyped input and the typed command. It is the
// only place that mapping happens, for all three channels.
func (h CreateWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	collectionID, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}
	parentID, err := in.ID("parent_id")
	if err != nil {
		return nil, err
	}

	item, err := h.Execute(ctx, actor, CreateWorkItemCommand{
		Type:         domain.ItemType(in.String("type")),
		CollectionID: collectionID,
		ParentID:     parentID,
		Title:        in.String("title"),
		Notes:        in.String("notes"),
	})
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}
