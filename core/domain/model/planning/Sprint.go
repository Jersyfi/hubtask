// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// SprintState is where a sprint stands: planned, running or closed (PLN-06).
type SprintState string

const (
	SprintPlanned SprintState = "PLANNED"
	SprintActive  SprintState = "ACTIVE"
	SprintClosed  SprintState = "CLOSED"
)

// SprintOutcome is what became of an entry when its sprint closed.
type SprintOutcome string

const (
	// OutcomeDone: finished within the sprint.
	OutcomeDone SprintOutcome = "DONE"
	// OutcomeCarried: unfinished, moved into the next sprint.
	OutcomeCarried SprintOutcome = "CARRIED"
	// OutcomeReturned: unfinished, back in the backlog.
	OutcomeReturned SprintOutcome = "RETURNED"
)

// The bounds of sprints: a goal's length, a sprint's length in days, its entries, and how many
// closed sprints a velocity reads.
const (
	MaxGoalLength      = 500
	MaxSprintDays      = 92
	MaxSprintEntries   = 1000
	MaxVelocitySprints = 20
	// VelocityAverageOf is how many of the latest closed sprints the average velocity reads.
	VelocityAverageOf = 3
)

// Sprint is a time box of one collection - a team's board - with a goal and the entries the team
// commits to in it. Starting it records the commitment; closing it records what was done and sends
// every unfinished entry into the next sprint or back to the backlog, as the closer chooses.
type Sprint struct {
	ID           shared.ID
	TenantID     shared.ID
	CollectionID shared.ID
	Name         string
	Goal         string
	StartOn      Day
	EndOn        Day
	State        SprintState
	// Unit is what the commitment and the velocity count: POINTS (sizes among them) or HOURS.
	Unit EstimateUnit
	// Committed and Completed are the estimate sums at start and at close, and how many entries.
	CommittedValue float64
	CommittedCount int
	CompletedValue float64
	CompletedCount int
	StartedAt      *time.Time
	ClosedAt       *time.Time
	CreatedAt      time.Time
	CreatedBy      shared.ID
	UpdatedAt      time.Time
	Version        int
}

// SprintFields are the fields a person sets on a sprint.
type SprintFields struct {
	Name    string
	Goal    string
	StartOn Day
	EndOn   Day
	Unit    EstimateUnit
}

// NewSprintInput is what planning a sprint takes.
type NewSprintInput struct {
	ID, TenantID, CollectionID, CreatedBy shared.ID
	Fields                                SprintFields
	Now                                   time.Time
	Text                                  text.Normalizer
}

// NewSprint checks a planned sprint.
func NewSprint(in NewSprintInput) (Sprint, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.CollectionID.IsZero() {
		return Sprint{}, shared.ErrInternal.WithDetail("sprints.identity_incomplete")
	}
	s := Sprint{
		ID: in.ID, TenantID: in.TenantID, CollectionID: in.CollectionID, State: SprintPlanned,
		CreatedAt: in.Now, CreatedBy: in.CreatedBy, UpdatedAt: in.Now, Version: 1,
	}
	if err := s.set(in.Fields, in.Text); err != nil {
		return Sprint{}, err
	}
	return s, nil
}

// Change sets a sprint's name, goal, dates or unit. A closed sprint is history and does not
// change; a running one keeps its start and its unit, which its commitment was counted on.
func (s *Sprint) Change(fields SprintFields, now time.Time, normalizer text.Normalizer) error {
	switch s.State {
	case SprintClosed:
		return shared.ErrConflict.WithDetail("sprints.closed")
	case SprintActive:
		if fields.StartOn != s.StartOn || fields.Unit != s.Unit {
			return invalid("/start_on", "sprints.running_start_fixed")
		}
	}
	if err := s.set(fields, normalizer); err != nil {
		return err
	}
	s.UpdatedAt = now
	return nil
}

func (s *Sprint) set(f SprintFields, normalizer text.Normalizer) error {
	name, goal := strings.TrimSpace(f.Name), strings.TrimSpace(f.Goal)
	if normalizer != nil {
		name, goal = normalizer.NFC(name), normalizer.NFC(goal)
	}
	unit := f.Unit
	if unit == "" {
		unit = Points
	}
	switch {
	case name == "":
		return invalid("/name", "sprints.name_required")
	case utf8.RuneCountInString(name) > MaxNameLength || strings.ContainsAny(name, "\r\n"):
		return invalid("/name", "sprints.name_invalid")
	case utf8.RuneCountInString(goal) > MaxGoalLength:
		return shared.ErrValidation.WithDetail("sprints.goal_too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxGoalLength)}).
			WithFields(shared.FieldError{Path: "/goal", Code: "sprints.goal_too_long"})
	case f.EndOn < f.StartOn:
		return invalid("/end_on", "sprints.ends_before_start")
	case int(f.EndOn-f.StartOn) >= MaxSprintDays:
		return shared.ErrValidation.WithDetail("sprints.too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxSprintDays)}).
			WithFields(shared.FieldError{Path: "/end_on", Code: "sprints.too_long"})
	case unit != Points && unit != Hours:
		return invalid("/unit", "sprints.unit_unknown")
	}
	s.Name, s.Goal, s.StartOn, s.EndOn, s.Unit = name, goal, f.StartOn, f.EndOn, unit
	return nil
}

// Start runs a planned sprint and records the commitment: the estimate sum of its entries now.
func (s *Sprint) Start(entries []SprintEntry, estimates map[shared.ID]Estimate, now time.Time) error {
	if s.State != SprintPlanned {
		return shared.ErrConflict.WithDetail("sprints.not_planned")
	}
	s.CommittedValue, s.CommittedCount = s.sum(entries, estimates, func(SprintEntry) bool { return true })
	s.State, s.StartedAt, s.UpdatedAt = SprintActive, &now, now
	return nil
}

// Close ends a running sprint: what is done is its result, the rest is carried or returned. It
// answers each open entry's outcome.
func (s *Sprint) Close(entries []SprintEntry, estimates map[shared.ID]Estimate, carry bool, now time.Time) (map[shared.ID]SprintOutcome, error) {
	if s.State != SprintActive {
		return nil, shared.ErrConflict.WithDetail("sprints.not_active")
	}
	s.CompletedValue, s.CompletedCount = s.sum(entries, estimates, func(e SprintEntry) bool { return e.Done })
	unfinished := OutcomeReturned
	if carry {
		unfinished = OutcomeCarried
	}
	out := make(map[shared.ID]SprintOutcome, len(entries))
	for _, e := range entries {
		if e.Done {
			out[e.EntryID] = OutcomeDone
		} else {
			out[e.EntryID] = unfinished
		}
	}
	s.State, s.ClosedAt, s.UpdatedAt = SprintClosed, &now, now
	return out, nil
}

func (s Sprint) sum(entries []SprintEntry, estimates map[shared.ID]Estimate, keep func(SprintEntry) bool) (float64, int) {
	var (
		value float64
		count int
	)
	for _, e := range entries {
		if !keep(e) {
			continue
		}
		count++
		if v, ok := estimates[e.EntryID].CountsIn(s.Unit); ok {
			value += v
		}
	}
	return value, count
}

// Open reports whether entries may still join or leave the sprint.
func (s Sprint) Open() bool { return s.State != SprintClosed }

// SprintEntry is one entry's membership in a sprint: when it joined and left, whether it was in
// the commitment, and what became of it at the close.
type SprintEntry struct {
	SprintID  shared.ID
	EntryID   shared.ID
	AddedAt   time.Time
	RemovedAt *time.Time
	Committed bool
	Outcome   SprintOutcome
	// Done and CompletedAt are the entry's own completion, read beside the membership.
	Done        bool
	CompletedAt *time.Time
}

// BurndownPoint is one day of a sprint: the work left at its end, the whole scope, and the ideal.
type BurndownPoint struct {
	Day       Day
	Remaining float64
	Scope     float64
	Ideal     float64
}

// Burndown answers a sprint's work left day by day, from its first day to its last or to today,
// whichever comes first: an entry counts from the day it joined until it left or was finished.
// The ideal line falls from the commitment to zero over the working days. Each entry counts at its
// estimate now.
func Burndown(s Sprint, entries []SprintEntry, estimates map[shared.ID]Estimate, calendar Workdays, zone *time.Location, today Day) []BurndownPoint {
	if s.State == SprintPlanned {
		return nil
	}
	last := s.EndOn
	if s.ClosedAt != nil {
		if closed := DayOf(*s.ClosedAt, zone); closed < last {
			last = closed
		}
	}
	if today < last {
		last = today
	}
	if last < s.StartOn {
		return nil
	}
	working := 0
	for d := s.StartOn; d <= s.EndOn; d++ {
		if worksOn(calendar, d) {
			working++
		}
	}
	commitment := s.CommittedValue
	out := make([]BurndownPoint, 0, int(last-s.StartOn)+1)
	elapsed := 0
	for d := s.StartOn; d <= last; d++ {
		if worksOn(calendar, d) {
			elapsed++
		}
		end := (d + 1).In(zone)
		var remaining, scope float64
		for _, e := range entries {
			if !e.AddedAt.Before(end) || (e.RemovedAt != nil && e.RemovedAt.Before(end)) {
				continue
			}
			value, _ := estimates[e.EntryID].CountsIn(s.Unit)
			scope += value
			if e.CompletedAt == nil || !e.CompletedAt.Before(end) {
				remaining += value
			}
		}
		ideal := 0.0
		if working > 0 {
			ideal = commitment * float64(working-elapsed) / float64(working)
		}
		out = append(out, BurndownPoint{Day: d, Remaining: round2(remaining), Scope: round2(scope), Ideal: round2(ideal)})
	}
	return out
}

func worksOn(calendar Workdays, d Day) bool {
	if calendar == nil {
		return true
	}
	y, m, day := d.Date()
	return calendar.WorksOn(y, m, day)
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// Velocity is how much a team got done in its latest closed sprints, the newest first, and the
// average of the latest VelocityAverageOf in the unit most of them counted.
type Velocity struct {
	Unit    EstimateUnit
	Sprints []Sprint
	Average float64
}

// VelocityOf reads a collection's closed sprints, newest first.
func VelocityOf(closed []Sprint) Velocity {
	v := Velocity{Unit: Points, Sprints: closed}
	if len(closed) == 0 {
		return v
	}
	v.Unit = closed[0].Unit
	var (
		sum float64
		n   int
	)
	for _, s := range closed {
		if n == VelocityAverageOf {
			break
		}
		if s.Unit != v.Unit {
			continue
		}
		sum += s.CompletedValue
		n++
	}
	if n > 0 {
		v.Average = round2(sum / float64(n))
	}
	return v
}
