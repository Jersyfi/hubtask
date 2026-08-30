// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation_test

import (
	"testing"
	"time"

	automation "github.com/Jersyfi/hubtask/core/domain/model/automation"
)

// The arithmetic a RELATIVE_DATE trigger is: an anchor plus a signed offset, or nothing at all.
//
// "Nothing at all" is the half worth stating. A cleared anchor is not an anchor of zero, and the
// distinction is what makes "does not fire for an anchor that was cleared" a property of this
// function rather than of a caller that remembered to check.
func TestAnOccurrenceIsTheAnchorPlusTheOffsetOrNothing(t *testing.T) {
	anchor := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
	relative := func(offset string) automation.Trigger {
		return automation.Trigger{
			Kind: automation.TriggerRelativeDate, Anchor: automation.AnchorDueDate, Offset: offset,
		}
	}

	cases := map[string]struct {
		trigger automation.Trigger
		anchor  *time.Time
		want    time.Time
		owes    bool
	}{
		"24 hours before": {
			trigger: relative("-PT24H"), anchor: &anchor,
			want: anchor.Add(-24 * time.Hour), owes: true,
		},
		"three days after": {
			trigger: relative("P3D"), anchor: &anchor,
			want: anchor.Add(72 * time.Hour), owes: true,
		},
		"an unsigned offset is forward": {
			trigger: relative("PT90M"), anchor: &anchor,
			want: anchor.Add(90 * time.Minute), owes: true,
		},
		"a cleared anchor": {
			trigger: relative("-PT24H"), anchor: nil,
		},
		"an anchor of the zero time": {
			trigger: relative("-PT24H"), anchor: &time.Time{},
		},
		"another kind of trigger": {
			trigger: automation.Trigger{Kind: automation.TriggerManual}, anchor: &anchor,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			at, owes, err := automation.OccurrenceAt(tc.trigger, tc.anchor)
			if err != nil {
				t.Fatalf("working it out: %v", err)
			}
			if owes != tc.owes {
				t.Fatalf("owes=%v, want %v", owes, tc.owes)
			}
			if owes && !at.Equal(tc.want) {
				t.Errorf("owes at %v, want %v", at, tc.want)
			}
			if !owes && !at.IsZero() {
				t.Errorf("a rule that owes nothing answered %v", at)
			}
		})
	}
}

// Unreachable through the aggregate, which parses the same text when the rule is written. Reported
// rather than swallowed all the same: a rule stored before an offset grammar changed would
// otherwise fire at the anchor itself, which is a run at a moment nobody chose.
func TestAnOffsetThisBuildCannotReadIsAnError(t *testing.T) {
	anchor := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)

	_, owes, err := automation.OccurrenceAt(automation.Trigger{
		Kind: automation.TriggerRelativeDate, Anchor: automation.AnchorDueDate, Offset: "P1M",
	}, &anchor)
	if err == nil {
		t.Fatal("a month-long offset was read as a length of time")
	}
	if owes {
		t.Error("a rule owes a moment it could not work out")
	}
}
