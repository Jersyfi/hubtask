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
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
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

// Ownership is the slice of the authorisation service the create path needs beyond Authorizer:
// whether the role the actor holds writes only what is assigned to them.
//
// Its own interface rather than a method on Authorizer, for the reason that split Visibility off: a
// use case declaring a dependency it does not use is a use case whose test has to satisfy it
// anyway, and creation is the only write that has to know this.
type Ownership interface {
	WritesOnlyWhatIsAssigned(
		ctx context.Context, actor appshared.ActorContext, path []identity.Scope,
	) (bool, error)
}

// CreateWorkItemCommand is the input, typed.
//
// The fields are the ones this use case owns. Everything else the contract's WorkItemCreate
// declares - the labels, the members, the due date, the cover, the custom fields - belongs to a
// use case that has not landed yet, and is refused rather than accepted and dropped: the
// catalogue does not declare those fields, so a request carrying one comes back naming it
// (usecase.field_unknown) instead of returning a 201 for an item that is not what the caller
// asked for. The assignee and auto_assign joined with C-02, which is the task C-01 left them to.
type CreateWorkItemCommand struct {
	Type domain.ItemType
	// CollectionID may be left empty when ParentID is given: an item's collection is the one its
	// parent is in, and making a client repeat it is making it possible to contradict.
	CollectionID shared.ID
	ParentID     shared.ID
	Title        string
	Notes        string
	// BucketID is the column of the collection's board the entry starts in, empty for none.
	BucketID shared.ID
	// AssigneeID creates the entry already on somebody (C-02): the same checks and the same
	// records as :assign, in the same transaction as the creation. Empty for nobody.
	AssigneeID shared.ID
	// AutoAssign asks the collection's assignment policy to hand the new entry out, explicitly -
	// an enabled policy applies itself without this flag. Contradicts AssigneeID and is refused
	// beside it: a named person and a policy cannot both decide one scalar.
	AutoAssign bool
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
	Buckets    repository.Buckets
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Authorizer Authorizer
	Ownership  Ownership
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	Activity   ActivityJournal
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
	// AutoAssign is the machinery C-02 built for :auto-assign, reused whole: the create path is
	// its second caller - the policy that applies itself to what a collection creates, and the
	// explicit `auto_assign` ask - and its Assignment writer is also how an explicit assignee is
	// written, with the same guards and the same four records as :assign.
	AutoAssign AutoAssignWorkItem
}

// Execute creates the item and returns it, together with what automatic assignment did when it
// ran - nil when it did not, which is a different answer from "it ran and found nobody".
func (h CreateWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateWorkItemCommand,
) (domain.WorkItem, *AutoAssignOutcome, error) {
	// The collection has to be known before the permission question can be asked, because the
	// answer depends on it: a membership held at the hub applies downwards, and a path that named
	// only the collection would ignore it and refuse somebody who does have the right
	// (domain-model.md §3.2). So this read comes first - it decides nothing, it only says which
	// path the question is about.
	collection, path, err := h.scopeOf(ctx, actor, cmd)
	if err != nil {
		return domain.WorkItem{}, nil, err
	}
	collectionID := collection.ID

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       path,
		Action:     ItemCreatedAction,
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		// The item does not exist yet, so the refusal names the collection it would have been
		// created in, and the subject names no entry: there is nothing to share and nothing
		// assigned, so the path ends at the container (access.ItemSubject).
		TargetID: collectionID,
		On:       access.ItemSubject{Does: service.ItemCreate},
	}); err != nil {
		return domain.WorkItem{}, nil, err
	}

	// Whose the new entry has to be. A role narrowed to what is assigned to it creates on itself,
	// or the entry would be out of its reach the moment it existed - which is the decision on issue
	// #84, and what keeps "assigned only" true at every moment rather than suspended for one call.
	cmd, err = h.assignToCreator(ctx, actor, path, cmd)
	if err != nil {
		return domain.WorkItem{}, nil, err
	}

	// Whether and how the new entry gets an assignee is decided before the transaction, for the
	// reasons the manual path decides it there: the second person's visibility and the pool's
	// eligibility read through the authorisation service, which opens transactions of its own.
	plan, err := h.assignmentPlan(ctx, actor, collection, cmd)
	if err != nil {
		return domain.WorkItem{}, nil, err
	}

	var created domain.WorkItem
	var outcome *AutoAssignOutcome
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
		// The first step of the entry's own history. No change set: nothing moved, the entry came
		// into being, and everything a reader would want beside the verb is on the entry itself.
		err = h.Activity.record(ctx, actor, item, activity.ItemCreated, verbIsTheChange(), now)
		if err != nil {
			return err
		}

		created = item
		if plan == nil {
			return nil
		}

		// The entry exists; now it lands on somebody, with the same records the standalone
		// assignment writes - the second event is what a notification reacts to (C-09 subscribes
		// to item.assigned, not to every create), and it is why the assignment is not baked into
		// the inserted row.
		created, outcome, err = plan.apply(ctx, actor, item, now)
		return err
	})
	if err != nil {
		return domain.WorkItem{}, nil, err
	}
	return created, outcome, nil
}

// createAssignment is the decided plan: exactly one of the two fields is set.
type createAssignment struct {
	h        CreateWorkItem
	assignee shared.ID
	pool     *eligiblePool
}

// assignToCreator makes the new entry the creator's, where the role they hold requires it.
//
// Not optional, and not the caller's to override: re-assigning is a write on an entry that is not
// yet theirs, and it is not a permission this role holds. So naming somebody else is refused rather
// than quietly corrected - a create that silently landed somewhere other than where the client
// asked would be worse than a refusal - and asking a policy to hand the entry out is refused for
// the same reason, since a policy can land it on anybody.
//
// Where the type's profile does not carry ASSIGNMENT, the write below refuses: such a type cannot
// be owned, and a role that may only write what it owns therefore cannot create one. That falls out
// of the capability profile rather than being decided here (I-W3, ADR-0006).
func (h CreateWorkItem) assignToCreator(
	ctx context.Context, actor appshared.ActorContext, path []identity.Scope,
	cmd CreateWorkItemCommand,
) (CreateWorkItemCommand, error) {
	onlyOwn, err := h.Ownership.WritesOnlyWhatIsAssigned(ctx, actor, path)
	if err != nil || !onlyOwn {
		return cmd, err
	}

	if cmd.AutoAssign || (!cmd.AssigneeID.IsZero() && cmd.AssigneeID != actor.AccountID) {
		return CreateWorkItemCommand{}, shared.ErrForbidden.
			WithDetail("items.assignee_must_be_the_creator").
			WithFields(shared.FieldError{
				Path: "/assignee_id", Code: "items.assignee_must_be_the_creator",
			})
	}

	cmd.AssigneeID = actor.AccountID
	return cmd, nil
}

// assignmentPlan reads the command's two assignment fields against the collection's policy and
// decides what - if anything - will put an assignee on the new entry. Nil means the entry is
// created on nobody, which is what a plain create has always meant.
func (h CreateWorkItem) assignmentPlan(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container,
	cmd CreateWorkItemCommand,
) (*createAssignment, error) {
	if cmd.AutoAssign && !cmd.AssigneeID.IsZero() {
		return nil, shared.ErrValidation.
			WithDetail("items.assignee_conflicts_auto_assign").
			WithFields(shared.FieldError{
				Path: "/auto_assign", Code: "items.assignee_conflicts_auto_assign",
			})
	}

	if !cmd.AssigneeID.IsZero() {
		// The person named must be able to see what they are being given - the check that makes
		// an assignment mean anything, asked here exactly as :assign asks it (C-01).
		if err := ensureAccountCanSee(
			ctx, h.AutoAssign.Assignment.Visibility, actor, cmd.AssigneeID, collection,
		); err != nil {
			return nil, err
		}
		return &createAssignment{h: h, assignee: cmd.AssigneeID}, nil
	}

	policy := collection.AutoAssign
	if cmd.AutoAssign {
		if policy == nil {
			return nil, autoAssignUnavailableError()
		}
	} else if policy == nil || !policy.Enabled {
		// No explicit ask and no policy that applies itself: the two ways a policy is reached
		// (C-02), and neither is this create.
		return nil, nil
	}

	pool, err := h.AutoAssign.eligible(ctx, actor, collection, *policy)
	if err != nil {
		return nil, err
	}
	return &createAssignment{h: h, pool: &pool}, nil
}

// apply runs inside the create's transaction, on the row the create just wrote.
func (p *createAssignment) apply(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem, now time.Time,
) (domain.WorkItem, *AutoAssignOutcome, error) {
	if p.pool != nil {
		written, outcome, err := p.h.AutoAssign.apply(
			ctx, actor, AssignmentCommand{ItemID: item.ID}, *p.pool)
		if err != nil {
			return domain.WorkItem{}, nil, err
		}
		return written, &outcome, nil
	}

	w := p.h.AutoAssign.Assignment
	profile, err := profileOf(ctx, w.Profiles, item.Type)
	if err != nil {
		return domain.WorkItem{}, nil, err
	}
	if err := item.EnsureAssignable(profile); err != nil {
		return domain.WorkItem{}, nil, err
	}
	written, err := w.write(ctx, actor, item, item.Assigned(p.assignee, now),
		0, profile, assigning, now, byHand)
	return written, nil, err
}

// scopeOf resolves which collection the item will live in, and the authorisation path to it.
//
// Read-only and outside the write transaction, because its answer is needed before the permission
// check. Nothing it reads is trusted afterwards: the state that decides whether the write may
// happen is read again inside the transaction that writes, since anything read before it can have
// changed by the time it commits.
func (h CreateWorkItem) scopeOf(
	ctx context.Context, actor appshared.ActorContext, cmd CreateWorkItemCommand,
) (domain.Container, []identity.Scope, error) {
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
		return domain.Container{}, nil, err
	}

	// The path runs from the tenant downwards: the hub the collection sits in, then the collection
	// itself. A membership held at any of them counts, which is what "the effective permission is the
	// highest role along the path" means. Built by containerPath, which the read side needs for the
	// same reason - two copies of this would eventually disagree about one level.
	return collection, containerPath(collection), nil
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

	if !cmd.BucketID.IsZero() {
		// Checked against the collection the entry is being created in, before the constructor
		// checks the capability: a column of another collection's board is a reference nothing
		// renders (I-W6).
		if err := ensureBucketOnBoard(ctx, h.Buckets, collection.ID, cmd.BucketID); err != nil {
			return domain.WorkItem{}, err
		}
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
		BucketID:     cmd.BucketID,
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
		// Read off the item rather than written out as three nulls. A create never produces a
		// completed item, and the read side returns this same projection for one that is
		// (B-07) - a completion that reported itself open would be a lie a client acts on.
		"completion": map[string]any{
			"is_completed": item.Completion.IsCompleted,
			"completed_at": timeOrNil(item.Completion.CompletedAt),
			"completed_by": idOrNil(item.Completion.CompletedBy),
		},
		// Always present, as null for an entry on no board. A field that appeared only once
		// somebody had dragged the card into a column is one a client cannot read unconditionally.
		"bucket_id": idOrNil(item.BucketID),
		// The same, for the same reason: null for an entry nobody is on. The member list is not
		// here - it is a set beside the row, read through its own endpoint, exactly as the labels
		// are (C-01).
		"assignee_id": idOrNil(item.AssigneeID),
		"order_key":   item.OrderKey,
		"archived_at": timeOrNil(item.ArchivedAt),
		"deleted_at":  timeOrNil(item.DeletedAt),
		"created_by":  item.CreatedBy.String(),
		"created_at":  item.CreatedAt,
		"updated_at":  item.UpdatedAt,
		"version":     item.Version,
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
			", records a change for offline clients, and writes an audit entry. When an assignee " +
			"is named or an assignment policy applies, additionally writes the assignment with " +
			"everything :assign writes, " + string(event.ItemAssigned) + " included.",
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
			{
				Name: "bucket_id", Kind: usecase.KindID,
				Description: "The column of the collection's board the entry starts in. It has to " +
					"be on this collection's board, and only a type whose profile carries BUCKET " +
					"has one - a board belongs to a collection, so only the entries directly in it " +
					"have a place on it.",
			},
			{
				Name: "assignee_id", Kind: usecase.KindID,
				Description: "The account to put the new entry on, with the same rules as " +
					":assign: the person has to be able to see the entry, and one who cannot is " +
					"refused rather than stored. Cannot be combined with auto_assign - a named " +
					"person and a policy cannot both decide one field.",
			},
			{
				Name: "auto_assign", Kind: usecase.KindBool,
				Description: "Hand the new entry out by the collection's assignment policy, " +
					"explicitly. Refused when the collection has no policy. Without this flag an " +
					"enabled policy applies itself anyway; a policy that is not enabled runs only " +
					"when this asks. The response's auto_assign object says what happened - " +
					"including that nobody was eligible, which leaves the entry unassigned rather " +
					"than failing the creation.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemCreatedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemCreated},
		Handler:  usecase.HandlerFunc(h.invoke),
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
	bucketID, err := in.ID("bucket_id")
	if err != nil {
		return nil, err
	}
	assigneeID, err := in.ID("assignee_id")
	if err != nil {
		return nil, err
	}

	item, outcome, err := h.Execute(ctx, actor, CreateWorkItemCommand{
		Type:         domain.ItemType(in.String("type")),
		CollectionID: collectionID,
		ParentID:     parentID,
		Title:        in.String("title"),
		Notes:        in.String("notes"),
		BucketID:     bucketID,
		AssigneeID:   assigneeID,
		AutoAssign:   in.Bool("auto_assign"),
	})
	if err != nil {
		return nil, err
	}

	out := itemOutput(item)
	if outcome != nil {
		out = outcome.output(out)
	}
	return out, nil
}
