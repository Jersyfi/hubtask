// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RelativeDatesConsumer identifies this subscriber. Stable across versions: renaming it makes every
// event it has already seen look new.
//
// Hyphenated rather than dotted, so that it cannot be mistaken for a message code by the gate that
// finds unregistered ones - a consumer name and a message code are two different vocabularies that
// happen to look alike.
const RelativeDatesConsumer = "automation-relative-dates"

// RelativeDates keeps the moments a RELATIVE_DATE rule owes in step with the anchors it measures
// from (G-08, automation.md §1.1).
//
// **The recompute is the substance, not the firing.** "24 hours before it is due" is a moment that
// moves whenever the due date does, and a system that worked it out at firing time would have to
// look at every entry in the workspace to find out what is due. So the moment is stored per (rule,
// entry) - D-02's shape - and this subscriber is what writes it: an entry whose anchor changed is
// an event, and an event is where the recompute belongs.
//
// **A cleared anchor owes nothing.** Somebody who removes a due date has removed the thing the rule
// measures from, and a row left behind would fire at a deadline that no longer exists. The same is
// true of an entry that goes.
//
// **A rule takes effect for what happens after it is switched on.** Nothing here walks the entries
// a rule would have matched before it existed: that walk is a scan of the workspace, and a
// subscriber inside the dispatcher's transaction is the worst possible place for one. An entry that
// is touched after the rule is enabled gets its moment; one that is never touched again does not.
// That is a real limit and it is written here rather than discovered.
//
// It runs inside the dispatcher's transaction, which is the reliability argument (ADR-0007): the
// moments it writes and the record that says this event was handled commit together.
//
// **It never takes a replay**, for MatchRules' reason: `eventbus.TakesReplays` is opt-in and this
// type deliberately does not implement it. A restore's events are already-old states, and
// backup-restore.md §8.4 is unambiguous that no rule acts because of one.
type RelativeDates struct {
	Rules       repository.Matching
	Occurrences repository.Occurrences
	Entries     Entries
	Containers  Containers
	Jobs        Queue
	Clock       clock.Clock
	IDs         clock.IDGenerator
}

var _ eventbus.Subscriber = RelativeDates{}

// Name identifies the subscriber.
func (d RelativeDates) Name() string { return RelativeDatesConsumer }

// Wants reports whether an event can have moved an anchor.
//
// The two anchors are the due date and the creation instant, so what matters is an entry appearing,
// its deadline moving, and an entry ending. A narrow list rather than every type, because unlike
// MatchRules this subscriber does a read per event and a rule's existence cannot make an unrelated
// event relevant - `comment.created` moves no anchor whatever anybody has written.
func (d RelativeDates) Wants(eventType event.Type) bool {
	switch eventType {
	case event.ItemCreated, event.ItemDueChanged, event.ItemUpdated,
		event.ItemTrashed, event.ItemPurged, event.ItemRestored:
		return true
	default:
		return false
	}
}

// Deliver brings every relative-date rule's moment for this entry up to date.
func (d RelativeDates) Deliver(ctx context.Context, envelope event.Envelope) error {
	itemID := itemOf(envelope)
	if itemID.IsZero() {
		return nil
	}

	rules, err := d.Rules.ByTriggerKind(ctx, domain.TriggerRelativeDate)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		// The common case in every workspace that has written no such rule, and it costs one
		// indexed read rather than a lookup of the entry.
		return nil
	}

	item, found, err := d.entry(ctx, itemID)
	if err != nil {
		return err
	}
	if !found {
		// Purged, or gone between the event and the delivery. Every rule's moment for it goes.
		return d.Occurrences.ForgetItem(ctx, itemID)
	}

	at := location{CollectionID: item.CollectionID}
	if at.HubID, err = d.hubOf(ctx, item.CollectionID); err != nil {
		return err
	}

	earliest := d.Clock.Now()
	owed := false
	for _, rule := range rules {
		if !covers(rule.Scope, at) {
			continue
		}
		moment, owes, err := d.settle(ctx, rule, item)
		if err != nil {
			return err
		}
		if owes && (!owed || moment.Before(earliest)) {
			earliest, owed = moment, true
		}
	}
	if !owed {
		return nil
	}
	return d.wake(ctx, envelope.TenantID, earliest)
}

// settle writes, moves or removes one rule's moment for this entry, and answers the moment when
// there is one.
func (d RelativeDates) settle(
	ctx context.Context, rule domain.Rule, item anchoredItem,
) (time.Time, bool, error) {
	at, owes, err := domain.OccurrenceAt(rule.Trigger, item.anchor(rule.Trigger.Anchor))
	if err != nil {
		// An offset this build cannot read, on a rule that was written when it could. The rule
		// stops owing rather than firing at the anchor itself, and it stays visible and editable.
		return time.Time{}, false, d.Occurrences.Forget(ctx, rule.ID, item.ID)
	}
	if !owes {
		// The anchor was cleared, or the entry is in the trash. Either way the deadline this rule
		// measured from no longer exists.
		return time.Time{}, false, d.Occurrences.Forget(ctx, rule.ID, item.ID)
	}

	err = d.Occurrences.Upsert(ctx, domain.Occurrence{
		ID: d.IDs.NewID(), TenantID: rule.TenantID, RuleID: rule.ID, ItemID: item.ID, FireAt: at,
	})
	return at, err == nil, err
}

// wake seeds or pulls forward this tenant's poller - the same one the schedules use, because the
// question it answers is "what does this tenant owe now" and there is one answer to it.
func (d RelativeDates) wake(ctx context.Context, tenantID shared.ID, at time.Time) error {
	if d.Jobs == nil || tenantID.IsZero() {
		return nil
	}
	_, err := d.Jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindAutomationSchedule,
		TenantID:  tenantID,
		DedupeKey: tenantID.String(),
		RunAt:     at.UTC(),
	})
	return err
}

// anchoredItem is the entry as this subscriber reads it: where it sits, and the two instants a
// relative-date rule can measure from.
type anchoredItem struct {
	ID           shared.ID
	CollectionID shared.ID
	CreatedAt    time.Time
	DueAt        *time.Time
	// Trashed marks an entry in the trash. It has both anchors still and owes nothing: a rule that
	// escalated a deleted entry's deadline would act on something its author had thrown away.
	Trashed bool
}

// anchor answers the instant the trigger names, and nil when there is none. A cleared due date and
// an entry in the trash are both "none", which is what makes "does not fire for an anchor that was
// cleared" a property of this type rather than of a caller that remembered.
func (i anchoredItem) anchor(kind domain.DateAnchor) *time.Time {
	if i.Trashed {
		return nil
	}
	switch kind {
	case domain.AnchorDueDate:
		return i.DueAt
	case domain.AnchorCreatedAt:
		created := i.CreatedAt
		return &created
	default:
		return nil
	}
}

func (d RelativeDates) entry(
	ctx context.Context, id shared.ID,
) (anchoredItem, bool, error) {
	if d.Entries == nil {
		return anchoredItem{}, false, nil
	}

	item, err := d.Entries.Find(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		return anchoredItem{}, false, nil
	}
	if err != nil {
		return anchoredItem{}, false, err
	}

	anchored := anchoredItem{
		ID: item.ID, CollectionID: item.CollectionID,
		CreatedAt: item.CreatedAt, Trashed: item.DeletedAt != nil,
	}
	if item.Due != nil {
		due := item.Due.At
		anchored.DueAt = &due
	}
	return anchored, true, nil
}

// hubOf is the one container read this subscriber makes, and only so that a rule scoped to a hub
// can be matched against an entry in one of its collections.
func (d RelativeDates) hubOf(ctx context.Context, collectionID shared.ID) (shared.ID, error) {
	if collectionID.IsZero() || d.Containers == nil {
		return "", nil
	}

	container, err := d.Containers.Find(ctx, collectionID)
	if errors.Is(err, shared.ErrNotFound) {
		// Gone between the event and the delivery. A hub-scoped rule then does not match, which is
		// the honest answer rather than a guess.
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return container.ParentID, nil
}
