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
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	AddMemberName    = "AddMember"
	RemoveMemberName = "RemoveMember"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	ItemMemberAddedAction   audit.Action = "item.member_added"
	ItemMemberRemovedAction audit.Action = "item.member_removed"
)

// ItemMemberWriter is what both use cases share.
//
// The same shape ItemLabelWriter has, because the member list is the same kind of thing as the
// label set: an OR-set beside the entry rather than a field on it. What differs is the question
// asked before the write - a label has to belong to the entry's collection, an account has to be
// able to see the entry - and the capability that gates it.
type ItemMemberWriter struct {
	Items       repository.Items
	ItemMembers repository.ItemMembers
	Containers  repository.Containers
	Profiles    metarepo.CapabilityProfiles
	Authorizer  Authorizer
	Visibility  Visibility
	Events      outbox.Events
	Changes     changelog.ChangeLog
	Audit       audit.Sink
	Activity    ActivityJournal
	UnitOfWork  persistence.UnitOfWork
	Clock       clock.Clock
	IDs         clock.IDGenerator
	HLC         clock.HLCSource
}

// AddMember puts an account on an entry's member list.
type AddMember struct {
	Writer ItemMemberWriter
}

// RemoveMember takes an account off an entry's member list.
type RemoveMember struct {
	Writer ItemMemberWriter
}

// MemberCommand is the input of both directions, typed.
//
// No expected version, for the reason LabelCommand spends none: a member lives beside the entry
// rather than on its row, so there is no version of the entry an addition would be racing. Two
// devices adding two different people at once is the case the OR-set exists to serve, and an
// optimistic lock on the entry would make one of them fail for no reason (offline-sync.md §4.2).
type MemberCommand struct {
	ItemID    shared.ID
	AccountID shared.ID
}

// ItemMemberSet is the members an entry carries, as every channel reports them.
type ItemMemberSet struct {
	ItemID     shared.ID
	AccountIDs []shared.ID
}

// Execute puts the account on the entry and returns the members it now carries.
func (h AddMember) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd MemberCommand,
) (ItemMemberSet, error) {
	return h.Writer.change(ctx, actor, cmd, addingMember)
}

// Execute takes the account off the entry and returns the members it now carries.
func (h RemoveMember) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd MemberCommand,
) (ItemMemberSet, error) {
	return h.Writer.change(ctx, actor, cmd, removingMember)
}

// memberDirection is which way the caller asked for. Not a boolean, for the reason labelDirection
// is not: it is the parameter that decides which of two audit trails is written.
type memberDirection bool

const (
	addingMember   memberDirection = true
	removingMember memberDirection = false
)

func (d memberDirection) action() audit.Action {
	if d == addingMember {
		return ItemMemberAddedAction
	}
	return ItemMemberRemovedAction
}

func (d memberDirection) verb() activity.Verb {
	if d == addingMember {
		return activity.ItemMemberAdded
	}
	return activity.ItemMemberRemoved
}

// change is the whole of both use cases.
func (w ItemMemberWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd MemberCommand, want memberDirection,
) (ItemMemberSet, error) {
	if cmd.ItemID.IsZero() {
		return ItemMemberSet{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}
	if cmd.AccountID.IsZero() {
		return ItemMemberSet{}, shared.ErrValidation.
			WithDetail("items.account_id_required").
			WithFields(shared.FieldError{Path: "/account_id", Code: "items.account_id_required"})
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards - the state that decides the write is read again
	// inside the transaction.
	subject, collection, err := w.readCollectionOf(ctx, actor, cmd.ItemID)
	if err != nil {
		return ItemMemberSet{}, err
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
		On:         changing(subject),
	}); err != nil {
		return ItemMemberSet{}, err
	}

	// Only on the way in. Somebody who has lost access has to be removable - that is precisely the
	// tidying up a revoked membership leaves behind, and refusing it would strand them on the entry
	// for good.
	if want == addingMember {
		if err := ensureAccountCanSee(
			ctx, w.Visibility, actor, cmd.AccountID, collection,
		); err != nil {
			return ItemMemberSet{}, err
		}
	}

	var result ItemMemberSet
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		item, err := findItem(ctx, w.Items, cmd.ItemID)
		if err != nil {
			return err
		}
		if err := w.ensureMembersAllowed(ctx, item); err != nil {
			return err
		}
		// A trashed or archived entry is read-only (I-W4). Asked after the capability, exactly as
		// the labels ask it and for the same reason: "an activity has no member list" is true of
		// the type whatever state one particular activity is in, and answering with the state first
		// would send a client off to unarchive an entry whose members would still be refused.
		if err := item.EnsureEditable(); err != nil {
			return err
		}

		now := w.Clock.Now()
		// The tag is the clock reading the OR-set merges on. Taken here rather than derived from
		// `now`, because a merge orders changes against other devices' readings and a wall clock
		// cannot do that (offline-sync.md §4.1).
		tag := w.HLC.Next()

		changed, err := w.apply(ctx, cmd, want, tag)
		if err != nil {
			return err
		}

		carried, err := w.ItemMembers.List(ctx, cmd.ItemID)
		if err != nil {
			return err
		}
		result = ItemMemberSet{ItemID: cmd.ItemID, AccountIDs: carried}

		if !changed {
			// The entry already carries the account, or already does not. The tag has been written
			// all the same - a device that decided this has made a decision another replica has to
			// merge against - but nothing is announced, because nothing about the entry changed.
			return nil
		}
		return w.announce(ctx, actor, item, collection, cmd.AccountID, want, tag, now)
	})
	if err != nil {
		return ItemMemberSet{}, err
	}
	return result, nil
}

// apply writes the membership and the tag, and reports whether the set actually moved.
func (w ItemMemberWriter) apply(
	ctx context.Context, cmd MemberCommand, want memberDirection, tag shared.HLC,
) (bool, error) {
	if want == removingMember {
		return w.ItemMembers.Remove(ctx, cmd.ItemID, cmd.AccountID, tag)
	}

	carried, err := w.ItemMembers.List(ctx, cmd.ItemID)
	if err != nil {
		return false, err
	}
	for _, id := range carried {
		if id == cmd.AccountID {
			// Already carried. The addition is written anyway, so that the tag moves forward and a
			// concurrent removal on another device does not win a merge it should not - but nothing
			// is announced, because the set did not move.
			return false, w.ItemMembers.Add(ctx, cmd.ItemID, cmd.AccountID, tag)
		}
	}
	return true, w.ItemMembers.Add(ctx, cmd.ItemID, cmd.AccountID, tag)
}

// announce records what a change to an entry's members owes: the event outwards, the change log for
// offline clients, the audit entry, and the step of the entry's history - all inside the caller's
// transaction (test AT-5).
func (w ItemMemberWriter) announce(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem,
	collection domain.Container, accountID shared.ID, want memberDirection,
	tag shared.HLC, now time.Time,
) error {
	by := event.Actor{Kind: actor.Kind, ID: actor.AccountID}

	announcement, err := event.NewItemMemberAdded(w.IDs.NewID(), item, accountID, by, now, event.Cause{})
	if want == removingMember {
		announcement, err = event.NewItemMemberRemoved(
			w.IDs.NewID(), item, accountID, by, now, event.Cause{})
	}
	if err != nil {
		return err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return err
	}
	if err := w.recordChange(ctx, item, collection, actor, accountID, want, tag); err != nil {
		return err
	}
	if err := w.recordAudit(ctx, item, actor, accountID, want, now); err != nil {
		return err
	}
	return w.recordActivity(ctx, item, actor, accountID, want, now)
}

// recordActivity writes the step of the entry's own history.
//
// The account travels as the side it moved to: `to` for an addition, `from` for a removal, so that
// the change set reads the same way round as the verb. No form question here - a type without the
// MEMBERS capability has no member list to add to, so an entry that reaches this point is one whose
// history keeps its detail (domain-model.md §2).
func (w ItemMemberWriter) recordActivity(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	accountID shared.ID, want memberDirection, now time.Time,
) error {
	field := activity.Field{Name: "account_id", Detail: activity.WithValues, To: accountID.String()}
	if want == removingMember {
		field = activity.Field{
			Name: "account_id", Detail: activity.WithValues, From: accountID.String(),
		}
	}

	return w.Activity.record(
		ctx, actor, item, want.verb(), activity.ChangeSet(activity.Full, field), now)
}

// recordChange writes what an offline client has to be told.
//
// The payload is the one element that moved and the tag that decides it, not the whole set - the
// merge rule for a set, written down (offline-sync.md §4.2). It is the shape the labels already
// use, with `set` naming which of the two this is: that field has been in the payload since B-09
// for exactly this second caller.
func (w ItemMemberWriter) recordChange(
	ctx context.Context, item domain.WorkItem, collection domain.Container,
	actor appshared.ActorContext, accountID shared.ID, want memberDirection, tag shared.HLC,
) error {
	operation := "add"
	if want == removingMember {
		operation = "remove"
	}

	return w.Changes.Record(ctx, changelog.Change{
		TenantID: item.TenantID,
		Entity:   itemTarget,
		EntityID: item.ID,
		Op:       changelog.Upsert,
		// The visibility filter a pull applies: the hub above the collection, so that a device
		// subscribed to the hub sees the change (offline-sync.md §3.1).
		ContainerID: firstNonZero(collection.ParentID, item.CollectionID),
		ActorID:     actor.AccountID,
		HLC:         tag,
		Payload: map[string]any{
			"set":        string(domain.SetMembers),
			"element_id": accountID.String(),
			"op":         operation,
		},
	})
}

// recordAudit writes the evidence.
//
// The account is recorded by identifier and in clear text: an account identifier is PERSONAL_BASIC
// rather than content - the same class the membership entries record - and "who was put on this
// work, by whom, and when" is not answerable without it (audit.md §4, rule 10).
func (w ItemMemberWriter) recordAudit(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	accountID shared.ID, want memberDirection, now time.Time,
) error {
	change := audit.Change{
		Field: "account_id", Classification: audit.Open, To: accountID.String(),
	}
	if want == removingMember {
		change = audit.Change{
			Field: "account_id", Classification: audit.Open, From: accountID.String(),
		}
	}

	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   item.TenantID,
		OccurredAt: now,
		Action:     want.action(),
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: itemTarget,
		TargetID:   item.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(change, audit.Change{
			Field: "collection_id", Classification: audit.Open, To: item.CollectionID.String(),
		}),
	})
}

// ensureMembersAllowed refuses a type whose profile does not carry MEMBERS.
//
// An activity has none: it carries exactly one assignee and no member list, which is the one row of
// the capability matrix where the two part company (domain-model.md §2). Refused rather than
// silently ignored - a client that put somebody on an activity and received a 200 would believe
// they were on it.
func (w ItemMemberWriter) ensureMembersAllowed(ctx context.Context, item domain.WorkItem) error {
	profile, err := profileOf(ctx, w.Profiles, item.Type)
	if err != nil {
		return err
	}
	return profile.Require(domain.CapabilityMembers, "/account_id")
}

// readCollectionOf reads the collection an entry belongs to, outside the write transaction, because
// the permission check needs its path first. Read-only, so it may be served by a replica
// (multi-tenancy.md §7).
func (w ItemMemberWriter) readCollectionOf(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (domain.WorkItem, domain.Container, error) {
	return readItemScope(ctx, w.UnitOfWork, w.Items, w.Containers, actor, itemID)
}

// memberSetOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema ItemMembers).
func memberSetOutput(set ItemMemberSet) usecase.Output {
	ids := make([]string, 0, len(set.AccountIDs))
	for _, id := range set.AccountIDs {
		ids = append(ids, id.String())
	}
	return usecase.Output{"item_id": set.ItemID.String(), "member_ids": ids}
}

// Descriptor is the catalogue entry.
func (h AddMember) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AddMemberName,
		Summary: "Puts an account on an entry's member list. The account has to be able to see the " +
			"entry - it has to hold a membership somewhere on the path above it - and the entry's " +
			"type has to carry a member list at all: an activity has one assignee and no members. " +
			"Idempotent: an entry that already carries the account succeeds and announces nothing.",
		SideEffects: "Writes the membership and its merge tag, announces " +
			string(event.ItemMemberAdded) + ", records the change for offline clients, writes an " +
			"audit entry and a step of the entry's history.",
		TokenScope: itemsWrite,
		Input:      memberInput("The entry to put the account on.", "The account to add."),
		Audit: usecase.AuditDeclaration{
			Action: ItemMemberAddedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemMemberAdded},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h RemoveMember) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RemoveMemberName,
		Summary: "Takes an account off an entry's member list. Idempotent: an entry that does not " +
			"carry it succeeds and announces nothing. The removal is recorded either way, so that " +
			"a device which never saw the account added still merges the decision. Somebody who " +
			"has lost access can still be removed - that is the tidying up a revoked membership " +
			"leaves behind.",
		SideEffects: "Removes the membership, writes its merge tag, announces " +
			string(event.ItemMemberRemoved) + ", records the change for offline clients, writes an " +
			"audit entry and a step of the entry's history.",
		TokenScope: itemsWrite,
		Input:      memberInput("The entry to take the account off.", "The account to remove."),
		Audit: usecase.AuditDeclaration{
			Action: ItemMemberRemovedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemMemberRemoved},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// memberInput is the input both directions declare. One list, so that a client which learned one of
// them from /meta/capabilities does not find the other spelled differently.
func memberInput(itemDescription, accountDescription string) []usecase.Field {
	return []usecase.Field{
		{Name: "item_id", Kind: usecase.KindID, Required: true, Description: itemDescription},
		{Name: "account_id", Kind: usecase.KindID, Required: true, Description: accountDescription},
	}
}

func (h AddMember) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := memberCommandOf(in)
	if err != nil {
		return nil, err
	}

	set, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return memberSetOutput(set), nil
}

func (h RemoveMember) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := memberCommandOf(in)
	if err != nil {
		return nil, err
	}

	set, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return memberSetOutput(set), nil
}

// memberCommandOf is the adapter between the catalogue's untyped input and the typed command, for
// both directions and all three channels.
func memberCommandOf(in usecase.Input) (MemberCommand, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return MemberCommand{}, err
	}
	accountID, err := in.ID("account_id")
	if err != nil {
		return MemberCommand{}, err
	}
	return MemberCommand{ItemID: itemID, AccountID: accountID}, nil
}
