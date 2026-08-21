// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	activityrepo "github.com/Jersyfi/hubtask/core/application/repository/activity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The fourth thing a change to an entry owes, in one place.
//
// The other three are already shared - the event outwards, the change log for offline clients, the
// audit entry - and each writer records them itself. This one is a value every writer holds rather
// than a method each of them repeats, because what an entry's history says about a change must not
// depend on which use case made it: twelve copies of "build the entry, take an identifier, stamp
// the actor" is twelve chances for one of them to record the actor of the request instead of the
// actor of the change.

// ActivityJournal writes the steps of an item's own history (domain-model.md §3.5).
type ActivityJournal struct {
	Entries activityrepo.Journal
	IDs     clock.IDGenerator
}

// record writes one step, inside the caller's transaction.
//
// It fails the transaction when there is nowhere to write rather than carrying on quietly. A change
// whose history is missing one step is a history nobody can trust afterwards, and unlike an event or
// an audit entry there is no second record from which the gap could later be noticed.
func (j ActivityJournal) record(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem,
	verb activity.Verb, changeSet map[string]any, at time.Time,
) error {
	if j.Entries == nil {
		return shared.ErrInternal.
			WithDetail("activity.journal_missing").
			WithParams(map[string]string{"verb": string(verb)})
	}

	return j.Entries.Record(ctx, activity.Entry{
		ID:       j.IDs.NewID(),
		TenantID: item.TenantID,
		ItemID:   item.ID,
		// The collection as it is *after* the change, which is the one the entry is now read
		// through. A move records where it went; where it came from is in the change set.
		CollectionID: item.CollectionID,
		Actor:        activity.Actor{Kind: actor.Kind, ID: actor.AccountID},
		Verb:         verb,
		ChangeSet:    changeSet,
		OccurredAt:   at,
	})
}

// verbIsTheChange is the change set of a step that moved no field.
//
// Archiving an entry is the whole of what happened, and a diff of the stamp it wrote would say the
// same thing a second time in a worse way. Written as a named function rather than an empty map
// literal at seven call sites, so that "this verb has no detail" is a decision a reader can see.
func verbIsTheChange() map[string]any { return map[string]any{} }

// historyForm is the form this item type's history takes (domain-model.md §2).
func historyForm(profile domain.CapabilityProfile) activity.Form {
	if profile.HistoryIsCompact() {
		return activity.Compact
	}
	return activity.Full
}

// historyFields turns the fields a change moved into the fields the history is asked to keep.
//
// The rule is the task's wording: the field names, and "content only where the product needs it".
// A title is content the product needs - "renamed Milk to Oat milk" is the entry somebody reads -
// and a note is content it does not: a note can be a page of text, and what its history is about is
// that somebody edited it. Everything else is an identifier, a rank or a state and is not content
// at all.
//
// A new content field is a decision to take here. The default is the values, which is right for the
// identifiers and states that make up most of them, and wrong for text - so a field carrying text
// belongs in the case above the default the day it arrives.
func historyFields(changes []domain.FieldChange) []activity.Field {
	fields := make([]activity.Field, 0, len(changes))

	for _, change := range changes {
		fields = append(fields, activity.Field{
			Name:   change.Field,
			Detail: historyDetailOf(change.Field),
			From:   change.From,
			To:     change.To,
		})
	}
	return fields
}

func historyDetailOf(field string) activity.Detail {
	switch field {
	case domain.FieldNotes:
		return activity.NameOnly
	default:
		return activity.WithValues
	}
}
