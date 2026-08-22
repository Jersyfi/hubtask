// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
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
	AssignWorkItemName   = "AssignWorkItem"
	UnassignWorkItemName = "UnassignWorkItem"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	ItemAssignedAction   audit.Action = "item.assigned"
	ItemUnassignedAction audit.Action = "item.unassigned"
)

// Visibility is the slice of the authorisation service this package needs beyond Authorizer: the
// same membership question asked about somebody other than the actor.
//
// Its own interface rather than a second method on Authorizer, on the reasoning that split Reader
// off: a use case declaring a dependency it does not use is a use case whose test has to satisfy it
// anyway, and most writers here never ask about a second person.
type Visibility interface {
	CanSee(
		ctx context.Context, actor appshared.ActorContext, accountID shared.ID,
		path []identity.Scope,
	) (bool, error)
}

// AssignmentWriter is what both directions of assignment need.
//
// One dependency set held by both use cases rather than two copies of thirteen fields, for the
// reason CompletionWriter is one: the two operations are the same walk in opposite directions -
// the same guards, the same four records - and the only thing that differs is whether a person
// arrives or leaves.
type AssignmentWriter struct {
	Items      repository.Items
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Authorizer Authorizer
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

// AssignWorkItem puts one person on an entry.
//
// Handing an entry from one person to another is this use case rather than an unassignment followed
// by an assignment: the field is a scalar, so it is one decision, one version and one step of the
// history - and doing it in two calls would leave a moment in which the work belongs to nobody.
type AssignWorkItem struct {
	Assignment AssignmentWriter
}

// UnassignWorkItem takes the assignee off an entry.
type UnassignWorkItem struct {
	Assignment AssignmentWriter
}

// AssignmentCommand is the input of both directions, typed. The account is empty for an
// unassignment, which is the one field the two do not share.
type AssignmentCommand struct {
	ItemID    shared.ID
	AccountID shared.ID
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read none
	// and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute puts the account on the entry and returns it.
func (h AssignWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd AssignmentCommand,
) (domain.WorkItem, error) {
	return h.Assignment.change(ctx, actor, cmd, assigning)
}

// Execute takes the assignee off the entry and returns it.
func (h UnassignWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd AssignmentCommand,
) (domain.WorkItem, error) {
	return h.Assignment.change(ctx, actor, cmd, unassigning)
}

// assignmentDirection is which way the caller asked for. Not a boolean, for the reason
// labelDirection is not: `change(ctx, actor, cmd, true)` at a call site says nothing, and this is
// the parameter that decides which of two audit trails is written.
type assignmentDirection bool

const (
	assigning   assignmentDirection = true
	unassigning assignmentDirection = false
)

func (d assignmentDirection) action() audit.Action {
	if d == assigning {
		return ItemAssignedAction
	}
	return ItemUnassignedAction
}

func (d assignmentDirection) verb() activity.Verb {
	if d == assigning {
		return activity.ItemAssigned
	}
	return activity.ItemUnassigned
}

// change is the whole of both use cases.
func (w AssignmentWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd AssignmentCommand,
	want assignmentDirection,
) (domain.WorkItem, error) {
	if cmd.ItemID.IsZero() {
		return domain.WorkItem{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}
	if want == assigning && cmd.AccountID.IsZero() {
		return domain.WorkItem{}, shared.ErrValidation.
			WithDetail("items.account_id_required").
			WithFields(shared.FieldError{Path: "/account_id", Code: "items.account_id_required"})
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards - the state that decides the write is read again
	// inside the transaction.
	collection, err := w.readCollectionOf(ctx, actor, cmd.ItemID)
	if err != nil {
		return domain.WorkItem{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
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

	// And the second person's, which is a different question: the actor may write here, and the
	// account they named still has to be able to see what they are being given. Outside the
	// transaction for the reason the check above is outside it - it reads through the
	// authorisation service, which opens its own.
	if want == assigning {
		if err := ensureAccountCanSee(
			ctx, w.Visibility, actor, cmd.AccountID, collection,
		); err != nil {
			return domain.WorkItem{}, err
		}
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
		// I-C3: an archived or trashed collection is read-only, and its entries inherit that.
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}
		profile, err := profileOf(ctx, w.Profiles, item.Type)
		if err != nil {
			return err
		}
		if err := item.EnsureAssignable(profile); err != nil {
			return err
		}

		wanted := item.Assigned(cmd.AccountID, now)
		if want == unassigning {
			wanted = item.Unassigned(now)
		}
		if wanted.AssigneeID == item.AssigneeID {
			// Already in the state asked for. Nothing is written, no version is spent and nothing
			// is announced - which is what makes a repeat harmless rather than merely accepted, and
			// what makes two devices assigning the same person converge on one version.
			//
			// The If-Match is still honoured: a caller that wrote against a version somebody else
			// has moved on is told so, even when its own change would have been a no-op, because
			// the state it was reasoning about is not the state that is there.
			if err := ensureExpectedVersion(item, cmd.ExpectedVersion); err != nil {
				return err
			}
			changed = item
			return nil
		}

		written, err := w.write(ctx, actor, item, wanted, cmd.ExpectedVersion, profile, want, now)
		changed = written
		return err
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return changed, nil
}

// write stores the assignee and records what the change owes: the event outwards, the change log
// for offline clients, the audit entry, and the step of the entry's own history - all inside the
// caller's transaction (test AT-5).
func (w AssignmentWriter) write(
	ctx context.Context, actor appshared.ActorContext, before, after domain.WorkItem,
	expectedVersion int, profile domain.CapabilityProfile, want assignmentDirection, now time.Time,
) (domain.WorkItem, error) {
	expected := expectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still the one the update matches on, so a concurrent write
		// between the read and here is still caught.
		expected = before.Version
	}
	if err := w.Items.SetAssignee(ctx, after, expected); err != nil {
		return domain.WorkItem{}, err
	}
	after.Version = expected + 1

	// The person the event is about: whoever now carries the entry, or whoever no longer does.
	// Neither direction announces nobody, which is what makes the payload always a reference.
	about := after.AssigneeID
	if want == unassigning {
		about = before.AssigneeID
	}

	if err := w.announce(ctx, actor, after, about, want, now); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordChange(ctx, after, actor, about, want); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordAudit(ctx, before, after, actor, now); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordActivity(ctx, before, after, actor, profile, want, now); err != nil {
		return domain.WorkItem{}, err
	}
	return after, nil
}

func (w AssignmentWriter) announce(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem, about shared.ID,
	want assignmentDirection, now time.Time,
) error {
	by := event.Actor{Kind: actor.Kind, ID: actor.AccountID}

	announcement, err := event.NewItemAssigned(w.IDs.NewID(), item, about, by, now, event.Cause{})
	if want == unassigning {
		announcement, err = event.NewItemUnassigned(
			w.IDs.NewID(), item, about, by, now, event.Cause{})
	}
	if err != nil {
		return err
	}
	return w.Events.Append(ctx, announcement)
}

// recordActivity writes the step of the entry's own history.
//
// Both sides travel, so that handing an entry on reads as what it was: `from` is who had it and
// `to` is who has it now, and an entry showing only the new name would leave a reader unable to see
// that it changed hands at all. An unassignment carries only the side that moved, which is the same
// shape the label removal takes.
//
// The form is the type's. An activity's history is compact (domain-model.md §2), and an activity is
// exactly the type that carries an assignee and nothing else - so this is the one verb where the
// compact form is reached in practice rather than in principle.
func (w AssignmentWriter) recordActivity(
	ctx context.Context, before, after domain.WorkItem, actor appshared.ActorContext,
	profile domain.CapabilityProfile, want assignmentDirection, now time.Time,
) error {
	field := activity.Field{
		Name:   domain.FieldAssigneeID,
		Detail: activity.WithValues,
		From:   before.AssigneeID.String(),
		To:     after.AssigneeID.String(),
	}

	return w.Activity.record(ctx, actor, after, want.verb(),
		activity.ChangeSet(historyForm(profile), field), now)
}

// recordChange writes what an offline client has to be told.
//
// One entry naming one field, which is the merge rule for a scalar written down: the assignee
// merges as last writer wins per field, so it takes an HLC of its own and carries nothing else
// (offline-sync.md §4.2). An unassignment names the field as empty rather than leaving it out - an
// absent field means "not touched", and a device that read it that way would keep an assignee
// somebody removed.
func (w AssignmentWriter) recordChange(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext, about shared.ID,
	want assignmentDirection,
) error {
	assignee := about.String()
	if want == unassigning {
		assignee = ""
	}

	return w.Changes.Record(ctx, changelog.Change{
		TenantID:    item.TenantID,
		Entity:      itemTarget,
		EntityID:    item.ID,
		Op:          changelog.Upsert,
		ContainerID: item.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         w.HLC.Next(),
		Payload:     map[string]any{domain.FieldAssigneeID: assignee},
	})
}

// recordAudit writes the evidence.
//
// Both accounts are recorded by identifier and in clear text. An account identifier is
// PERSONAL_BASIC rather than content - it is the same class the membership entries record - and
// "who was this given to, by whom, and when" is not answerable without it (audit.md §4, rule 10).
// No title and no notes: user content stays out of the trail.
func (w AssignmentWriter) recordAudit(
	ctx context.Context, before, after domain.WorkItem, actor appshared.ActorContext, now time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   after.TenantID,
		OccurredAt: now,
		Action:     directionOfAssignment(after).action(),
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
				Field: domain.FieldAssigneeID, Classification: audit.Open,
				From: before.AssigneeID.String(), To: after.AssigneeID.String(),
			},
			audit.Change{Field: "type", Classification: audit.Open, To: string(after.Type)},
			audit.Change{
				Field: "collection_id", Classification: audit.Open,
				To: after.CollectionID.String(),
			},
		),
	})
}

// directionOfAssignment reads the direction back off the state that was decided, so that the trail
// cannot record an assignment while the row says nobody.
func directionOfAssignment(item domain.WorkItem) assignmentDirection {
	if item.AssigneeID.IsZero() {
		return unassigning
	}
	return assigning
}

// readCollectionOf reads the collection an entry belongs to, outside the write transaction, because
// the permission check needs its path first. Read-only, so it may be served by a replica
// (multi-tenancy.md §7).
func (w AssignmentWriter) readCollectionOf(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (domain.Container, error) {
	var collection domain.Container

	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		item, err := findItem(ctx, w.Items, itemID)
		if err != nil {
			return err
		}
		found, err := findCollection(ctx, w.Containers, item.CollectionID)
		collection = found
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return collection, nil
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h AssignWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AssignWorkItemName,
		Summary: "Puts one person on an entry. Every type carries an assignee; an activity carries " +
			"this and no member list. The account has to be able to see the entry - it has to hold " +
			"a membership somewhere on the path above it - and one that cannot is refused rather " +
			"than stored, with the same answer for an account of another tenant and one that does " +
			"not exist. Handing an entry on is this call rather than an unassignment followed by " +
			"an assignment. Idempotent: assigning the person who already has it succeeds and " +
			"announces nothing.",
		SideEffects: "Writes the assignee, announces " + string(event.ItemAssigned) +
			", records the change for offline clients, writes an audit entry and a step of the " +
			"entry's history.",
		TokenScope: itemsWrite,
		Input: append(
			[]usecase.Field{{
				Name: "account_id", Kind: usecase.KindID, Required: true,
				Description: "The account to give the entry to.",
			}},
			assignmentInput("The entry to assign.")...,
		),
		Audit: usecase.AuditDeclaration{
			Action: ItemAssignedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemAssigned},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h UnassignWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UnassignWorkItemName,
		Summary: "Takes the assignee off an entry. Idempotent: an entry nobody is on succeeds and " +
			"announces nothing.",
		SideEffects: "Clears the assignee, announces " + string(event.ItemUnassigned) +
			", records the change for offline clients, writes an audit entry and a step of the " +
			"entry's history.",
		TokenScope: itemsWrite,
		Input:      assignmentInput("The entry to take the assignee off."),
		Audit: usecase.AuditDeclaration{
			Action: ItemUnassignedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemUnassigned},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// assignmentInput is what both directions take beyond the account. One list, so that a client which
// learned one of them from /meta/capabilities does not find the other spelled differently.
func assignmentInput(itemDescription string) []usecase.Field {
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

func (h AssignWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	cmd, err := assignmentCommand(in)
	if err != nil {
		return nil, err
	}
	cmd.AccountID = accountID

	item, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}

func (h UnassignWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := assignmentCommand(in)
	if err != nil {
		return nil, err
	}

	item, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}

// assignmentCommand is the adapter between the catalogue's untyped input and the typed command, for
// both directions and all three channels.
func assignmentCommand(in usecase.Input) (AssignmentCommand, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return AssignmentCommand{}, err
	}
	return AssignmentCommand{ItemID: itemID, ExpectedVersion: in.Int("expected_version")}, nil
}
