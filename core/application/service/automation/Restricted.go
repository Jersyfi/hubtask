// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	privacy "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The two codes a run withheld by a restriction of processing ends with (data-protection.md §4,
// UC-PRV-04 check 4). SKIPPED rather than failed: the rule did nothing wrong, and a rule must not
// switch itself off over somebody's right - it runs again once the restriction is lifted.
const (
	// CodeRunAsRestricted: the account the rule acts as is restricted.
	CodeRunAsRestricted = "automation.run_as_restricted"
	// CodeSubjectRestricted: the run is about a restricted person - the event's actor, the entry's
	// creator or assignee, or the sender of an intake entry.
	CodeSubjectRestricted = "automation.subject_restricted"
)

// restricted answers why a run may not act, or nothing: no rule runs as a restricted person, and no
// rule runs about one. Read once per run, in the runner's transaction, before the conditions - a
// condition is already a reading of the person's data.
func (h RunRule) restricted(
	ctx context.Context, rule domain.Rule, envelope event.Envelope, cmd Command, values condition.Values,
) (string, error) {
	if h.Restrictions == nil {
		return "", nil
	}
	people, err := h.Restrictions.RestrictedPeople(ctx)
	if err != nil || len(people) == 0 {
		return "", err
	}
	set := privacy.NewRestricted(people)
	if set.Has(rule.RunAs) {
		return CodeRunAsRestricted, nil
	}
	if set.Has(envelope.Actor.ID) || set.Has(envelope.Actor.OnBehalfOf) {
		return CodeSubjectRestricted, nil
	}
	about, err := h.about(ctx, set, cmd, values)
	if err != nil || !about {
		return "", err
	}
	return CodeSubjectRestricted, nil
}

// about reports whether the run's subject belongs to a restricted person: an entry they created or
// are on, or an intake entry they sent. A subject gone since is about nobody.
func (h RunRule) about(
	ctx context.Context, set privacy.Restricted, cmd Command, values condition.Values,
) (bool, error) {
	if entry := values.EntryID(); !entry.IsZero() && h.Entries != nil {
		item, err := h.Entries.Find(ctx, entry)
		if errors.Is(err, shared.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if set.Has(item.CreatedBy) || set.Has(item.AssigneeID) {
			return true, nil
		}
	}
	if cmd.Trigger == domain.TriggerJumbleEntry && !cmd.SubjectID.IsZero() && h.Jumble != nil {
		entry, err := h.Jumble.Find(ctx, cmd.SubjectID)
		if errors.Is(err, shared.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return set.HasAddress(entry.Sender), nil
	}
	return false, nil
}
