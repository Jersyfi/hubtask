// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Occurrence is one moment a RELATIVE_DATE rule owes one entry (G-08, automation.md §1.1).
//
// D-02's shape rather than a new one: a reminder is "this entry, this moment, this person", and
// this is the same fact with a rule in place of the person. What makes it worth storing at all is
// that the anchor moves - a due date pushed by a day moves every rule that measures from it, and a
// system that recomputed at firing time would have to look at every entry to find out.
type Occurrence struct {
	ID       shared.ID
	TenantID shared.ID
	RuleID   shared.ID
	// ItemID is the entry the offset was measured from, and the entry the run will be about.
	ItemID shared.ID
	// FireAt is the instant, with the offset already applied. Stored resolved rather than as an
	// anchor and an offset, so that nothing downstream has to know what "24 hours before" meant -
	// and so that the due index is an index on a moment rather than on an expression.
	FireAt time.Time
}

// OccurrenceAt works out when a rule owes an entry, given the instant its anchor names.
//
// The anchor is a pointer because "there is no due date" and "the due date is the zero time" are
// two different facts, and only the first of them means the rule owes nothing. A cleared anchor
// answers false, which is what makes "does not fire for an anchor that was cleared" a property of
// this function rather than of a caller that remembered to check.
func OccurrenceAt(trigger Trigger, anchor *time.Time) (time.Time, bool, error) {
	if trigger.Kind != TriggerRelativeDate || anchor == nil || anchor.IsZero() {
		return time.Time{}, false, nil
	}

	offset, err := parseOffset(trigger.Offset)
	if err != nil {
		// Unreachable through the aggregate, which parses the same text when the rule is written.
		// Reported rather than swallowed all the same: a rule stored before an offset grammar
		// changed would otherwise fire at the anchor itself.
		return time.Time{}, false, shared.ErrValidation.
			WithDetail("automation.offset_invalid").
			WithParams(map[string]string{"offset": trigger.Offset})
	}
	return anchor.Add(offset).UTC(), true, nil
}
