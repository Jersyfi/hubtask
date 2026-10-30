// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The bounds of baselines: how many one plan keeps, how many entries one holds, a name's length.
const (
	MaxBaselinesPerPlan = 20
	MaxBaselineEntries  = 10000
	MaxNameLength       = 120
)

// Baseline is a plan's dates as they were agreed: a named copy of every entry's start and due
// under a hub or a collection, kept to compare today's plan with (PRINCE2's and PMBOK's baseline).
// It is taken and deleted, never changed: a new agreement is a new baseline.
type Baseline struct {
	ID       shared.ID
	TenantID shared.ID
	// PlanID is the hub or collection whose entries it holds.
	PlanID  shared.ID
	Name    string
	TakenAt time.Time
	TakenBy shared.ID
	Entries []BaselineEntry
	// EntryCount is how many entries it holds, for a list that does not read them.
	EntryCount int
}

// BaselineEntry is one entry's dates in a baseline.
type BaselineEntry struct {
	EntryID shared.ID
	StartAt *time.Time
	DueAt   *time.Time
}

// NewBaselineInput is what taking a baseline takes.
type NewBaselineInput struct {
	ID, TenantID, PlanID, TakenBy shared.ID
	Name                          string
	Now                           time.Time
	Text                          text.Normalizer
}

// NewBaseline checks a baseline's own fields; its entries are copied by the store in the same
// transaction, so that the copy is the plan at one moment.
func NewBaseline(in NewBaselineInput) (Baseline, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.PlanID.IsZero() {
		return Baseline{}, shared.ErrInternal.WithDetail("baselines.identity_incomplete")
	}
	name := strings.TrimSpace(in.Name)
	if in.Text != nil {
		name = in.Text.NFC(name)
	}
	switch {
	case name == "":
		return Baseline{}, invalid("/name", "baselines.name_required")
	case utf8.RuneCountInString(name) > MaxNameLength || strings.ContainsAny(name, "\r\n"):
		return Baseline{}, invalid("/name", "baselines.name_invalid")
	}
	return Baseline{
		ID: in.ID, TenantID: in.TenantID, PlanID: in.PlanID, Name: name, TakenAt: in.Now, TakenBy: in.TakenBy,
	}, nil
}

// Variance is one entry's slip against a baseline, in calendar days; positive is later.
type Variance struct {
	EntryID shared.ID
	// InBaseline is false for an entry added since; InPlan false for one gone since.
	InBaseline, InPlan                    bool
	BaselineStart, BaselineDue            *time.Time
	CurrentStart, CurrentDue              *time.Time
	StartVarianceDays, FinishVarianceDays *int
}

// Compare answers each entry's variance between a baseline and today's dates, the baseline's
// entries first in its order, then those added since. Days are read in the zone given.
func Compare(baseline []BaselineEntry, current []BaselineEntry, zone *time.Location) []Variance {
	now := make(map[shared.ID]BaselineEntry, len(current))
	for _, c := range current {
		now[c.EntryID] = c
	}
	seen := make(map[shared.ID]bool, len(baseline))
	out := make([]Variance, 0, len(baseline))
	for _, b := range baseline {
		seen[b.EntryID] = true
		c, inPlan := now[b.EntryID]
		v := Variance{
			EntryID: b.EntryID, InBaseline: true, InPlan: inPlan,
			BaselineStart: b.StartAt, BaselineDue: b.DueAt, CurrentStart: c.StartAt, CurrentDue: c.DueAt,
		}
		if inPlan {
			v.StartVarianceDays = daysBetween(b.StartAt, c.StartAt, zone)
			v.FinishVarianceDays = daysBetween(b.DueAt, c.DueAt, zone)
		}
		out = append(out, v)
	}
	for _, c := range current {
		if !seen[c.EntryID] {
			out = append(out, Variance{EntryID: c.EntryID, InPlan: true, CurrentStart: c.StartAt, CurrentDue: c.DueAt})
		}
	}
	return out
}

func daysBetween(from, to *time.Time, zone *time.Location) *int {
	if from == nil || to == nil {
		return nil
	}
	n := int(DayOf(*to, zone) - DayOf(*from, zone))
	return &n
}
