// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// GoalKind is what a promise measures - a closed set, because each has its own moment that
// meets it.
type GoalKind string

const (
	// GoalFirstResponse is met by the first comment of somebody other than the person who raised
	// the entry, or by its completion.
	GoalFirstResponse GoalKind = "FIRST_RESPONSE"
	// GoalResolution is met by the entry's completion.
	GoalResolution GoalKind = "RESOLUTION"
)

var goalKinds = [...]GoalKind{GoalFirstResponse, GoalResolution}

// GoalKinds answers every kind, in the order a screen draws them.
func GoalKinds() []GoalKind { return goalKinds[:] }

// Valid reports whether the kind is one of the set.
func (k GoalKind) Valid() bool { return slices.Contains(goalKinds[:], k) }

// TimerName is the durable timer that counts down the goal.
func (k GoalKind) TimerName() string { return strings.ToLower(string(k)) }

// WarnTimerName is the timer that runs out at the goal's warning share.
func (k GoalKind) WarnTimerName() string { return k.TimerName() + warnSuffix }

const warnSuffix = "_warn"

// GoalOfTimer reads which goal a timer counts down, and whether it is the warning.
func GoalOfTimer(name string) (GoalKind, bool, bool) {
	base, warn := strings.CutSuffix(name, warnSuffix)
	for _, kind := range goalKinds {
		if kind.TimerName() == base {
			return kind, warn, true
		}
	}
	return "", false, false
}

// Bounds of a policy, so that a policy stays a small document every write can read whole.
const (
	MaxPolicyEntryTypes = 50
	MaxPriorityTargets  = 20
	MaxPauseStatuses    = 20
	// MaxTargetMinutes is the timer's own bound: a year of working time.
	MaxTargetMinutes = int(calendar.MaxBudget / time.Minute)
)

// SlaGoal is one promise of a policy: a target in working minutes, narrower per priority, the
// statuses in which its clock stands still (waiting for the requester), and the share of the
// target at which it is announced at risk.
type SlaGoal struct {
	Kind       GoalKind
	Minutes    int
	ByPriority map[string]int
	// PauseStatuses are workflow status keys; an entry in one of them spends none of the target.
	PauseStatuses []string
	// WarnPercent is the share of the target after which the goal is at risk; 0 announces none.
	WarnPercent int
}

// Target answers the goal's working time for an entry of a priority; a priority without a target
// of its own takes the goal's.
func (g SlaGoal) Target(priority string) time.Duration {
	if minutes, ok := g.ByPriority[priority]; ok && priority != "" {
		return time.Duration(minutes) * time.Minute
	}
	return time.Duration(g.Minutes) * time.Minute
}

// Warning answers the working time after which the goal is at risk; false when it announces none.
func (g SlaGoal) Warning(priority string) (time.Duration, bool) {
	if g.WarnPercent == 0 {
		return 0, false
	}
	warning := g.Target(priority) * time.Duration(g.WarnPercent) / 100
	return warning, warning > 0
}

// Pauses reports whether the clock stands still while the entry is in the status.
func (g SlaGoal) Pauses(status string) bool {
	return status != "" && slices.Contains(g.PauseStatuses, status)
}

// SlaPolicy is a workspace's promise for entries of some types: its goals, counted on one of the
// workspace's business calendars - or around the clock when it names none.
type SlaPolicy struct {
	ID          shared.ID
	TenantID    shared.ID
	Key         string
	Name        string
	CalendarKey string
	EntryTypes  []string
	// PriorityField is the custom field whose option value picks a goal's target; empty takes the
	// goals' own.
	PriorityField string
	Goals         []SlaGoal
	Position      int
	Enabled       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int
}

// NewSlaPolicyInput is what defining a policy takes.
type NewSlaPolicyInput struct {
	ID, TenantID  shared.ID
	Key, Name     string
	CalendarKey   string
	EntryTypes    []string
	PriorityField string
	Goals         []SlaGoal
	Position      int
	Enabled       bool
	Now           time.Time
	Text          text.Normalizer
}

// NewSlaPolicy checks a policy: a key, a name, at least one type and one goal, each kind once,
// targets inside a timer's bound.
func NewSlaPolicy(in NewSlaPolicyInput) (SlaPolicy, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return SlaPolicy{}, shared.ErrInternal.WithDetail("sla_policies.identity_incomplete")
	}
	key := strings.TrimSpace(in.Key)
	if !keyPattern.MatchString(key) {
		return SlaPolicy{}, invalid("/key", "sla_policies.key_malformed")
	}
	name, err := checkName(in.Name, in.Text, "sla_policies")
	if err != nil {
		return SlaPolicy{}, err
	}
	calendarKey := strings.TrimSpace(in.CalendarKey)
	if calendarKey != "" && !keyPattern.MatchString(calendarKey) {
		return SlaPolicy{}, invalid("/calendar", "sla_policies.calendar_malformed")
	}
	priorityField := strings.TrimSpace(in.PriorityField)
	if priorityField != "" && !keyPattern.MatchString(priorityField) {
		return SlaPolicy{}, invalid("/priority_field", "sla_policies.priority_field_malformed")
	}
	types, err := checkKeys(in.EntryTypes, "/entry_types", MaxPolicyEntryTypes)
	if err != nil {
		return SlaPolicy{}, err
	}
	if len(types) == 0 {
		return SlaPolicy{}, invalid("/entry_types", "sla_policies.entry_types_required")
	}
	if len(in.Goals) == 0 {
		return SlaPolicy{}, invalid("/goals", "sla_policies.goals_required")
	}
	goals := make([]SlaGoal, 0, len(in.Goals))
	for i, goal := range in.Goals {
		checked, err := checkGoal(goal, "/goals/"+strconv.Itoa(i))
		if err != nil {
			return SlaPolicy{}, err
		}
		if slices.ContainsFunc(goals, func(g SlaGoal) bool { return g.Kind == checked.Kind }) {
			return SlaPolicy{}, invalid("/goals/"+strconv.Itoa(i)+"/kind", "sla_policies.goal_twice")
		}
		goals = append(goals, checked)
	}
	slices.SortFunc(goals, func(a, b SlaGoal) int {
		return cmp.Compare(slices.Index(goalKinds[:], a.Kind), slices.Index(goalKinds[:], b.Kind))
	})
	return SlaPolicy{
		ID: in.ID, TenantID: in.TenantID, Key: key, Name: name, CalendarKey: calendarKey,
		EntryTypes: types, PriorityField: priorityField, Goals: goals, Position: in.Position,
		Enabled: in.Enabled, CreatedAt: in.Now, UpdatedAt: in.Now, Version: 1,
	}, nil
}

func checkGoal(goal SlaGoal, at string) (SlaGoal, error) {
	if !goal.Kind.Valid() {
		return SlaGoal{}, invalid(at+"/kind", "sla_policies.goal_kind_unknown")
	}
	if goal.Minutes <= 0 || goal.Minutes > MaxTargetMinutes {
		return SlaGoal{}, invalid(at+"/minutes", "sla_policies.target_invalid")
	}
	if len(goal.ByPriority) > MaxPriorityTargets {
		return SlaGoal{}, invalid(at+"/by_priority", "sla_policies.too_many_targets")
	}
	byPriority := make(map[string]int, len(goal.ByPriority))
	for _, priority := range slices.Sorted(maps.Keys(goal.ByPriority)) {
		minutes := goal.ByPriority[priority]
		if strings.TrimSpace(priority) == "" || len(priority) > MaxNameLength {
			return SlaGoal{}, invalid(at+"/by_priority", "sla_policies.priority_malformed")
		}
		if minutes <= 0 || minutes > MaxTargetMinutes {
			return SlaGoal{}, invalid(at+"/by_priority/"+priority, "sla_policies.target_invalid")
		}
		byPriority[priority] = minutes
	}
	pauses, err := checkKeys(goal.PauseStatuses, at+"/pause_statuses", MaxPauseStatuses)
	if err != nil {
		return SlaGoal{}, err
	}
	if goal.WarnPercent < 0 || goal.WarnPercent > 99 {
		return SlaGoal{}, invalid(at+"/warn_percent", "sla_policies.warn_percent_invalid")
	}
	return SlaGoal{
		Kind: goal.Kind, Minutes: goal.Minutes, ByPriority: byPriority, PauseStatuses: pauses,
		WarnPercent: goal.WarnPercent,
	}, nil
}

// checkKeys answers a set of keys trimmed, without repeats, in their order.
func checkKeys(raw []string, at string, bound int) ([]string, error) {
	if len(raw) > bound {
		return nil, invalid(at, "sla_policies.too_many")
	}
	out := make([]string, 0, len(raw))
	for i, key := range raw {
		key = strings.TrimSpace(key)
		if !keyPattern.MatchString(key) {
			return nil, invalid(at+"/"+strconv.Itoa(i), "sla_policies.key_malformed")
		}
		if !slices.Contains(out, key) {
			out = append(out, key)
		}
	}
	return out, nil
}

// Change answers the policy with every decision replaced, its key and identity kept.
func (p SlaPolicy) Change(in NewSlaPolicyInput) (SlaPolicy, error) {
	in.ID, in.TenantID, in.Key = p.ID, p.TenantID, p.Key
	changed, err := NewSlaPolicy(in)
	if err != nil {
		return SlaPolicy{}, err
	}
	changed.CreatedAt, changed.Version = p.CreatedAt, p.Version
	return changed, nil
}

// Covers reports whether the policy measures entries of the type.
func (p SlaPolicy) Covers(entryType string) bool {
	return p.Enabled && entryType != "" && slices.Contains(p.EntryTypes, entryType)
}

// Goal answers the policy's goal of a kind.
func (p SlaPolicy) Goal(kind GoalKind) (SlaGoal, bool) {
	for _, goal := range p.Goals {
		if goal.Kind == kind {
			return goal, true
		}
	}
	return SlaGoal{}, false
}

// Choose answers the policy an entry of the type is measured against: the first enabled one that
// covers the type, by position and then key - one policy per entry, so its clocks never disagree.
func Choose(policies []SlaPolicy, entryType string) (SlaPolicy, bool) {
	ordered := slices.Clone(policies)
	slices.SortFunc(ordered, func(a, b SlaPolicy) int {
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.Key, b.Key))
	})
	for _, policy := range ordered {
		if policy.Covers(entryType) {
			return policy, true
		}
	}
	return SlaPolicy{}, false
}

// Standing is where a goal of one entry stands, as a handler and a report read it.
type Standing string

const (
	StandingRunning  Standing = "RUNNING"
	StandingPaused   Standing = "PAUSED"
	StandingMet      Standing = "MET"
	StandingBreached Standing = "BREACHED"
)

// StandingOf reads a goal's timer: stopped before it ran out is met, fired is breached.
func StandingOf(timer calendar.Timer) Standing {
	switch timer.State {
	case calendar.TimerFired:
		return StandingBreached
	case calendar.TimerStopped:
		return StandingMet
	case calendar.TimerPaused:
		return StandingPaused
	}
	return StandingRunning
}

// EntrySla is what became of one goal on one entry: the record a report reads and the archive
// carries, beside the timer that is its clock.
type EntrySla struct {
	ID         shared.ID
	TenantID   shared.ID
	ItemID     shared.ID
	Goal       GoalKind
	PolicyKey  string
	Target     time.Duration
	Standing   Standing
	ArmedAt    time.Time
	DueAt      *time.Time
	AtRiskAt   *time.Time
	MetAt      *time.Time
	BreachedAt *time.Time
	Version    int
}

// Follow brings the record in step with its timer at now: its due moment while it runs, met when
// it was stopped, breached when it fired. A breached goal stays breached.
func (r EntrySla) Follow(timer calendar.Timer, now time.Time) EntrySla {
	if r.Standing == StandingBreached {
		return r
	}
	r.Standing, r.DueAt = StandingOf(timer), timer.DueAt
	switch r.Standing {
	case StandingMet:
		at := now
		r.MetAt = &at
	case StandingBreached:
		at := now
		if timer.FiredAt != nil {
			at = *timer.FiredAt
		}
		r.BreachedAt = &at
	}
	return r
}

// ReportRow counts the goals of one policy and kind in one standing.
type ReportRow struct {
	PolicyKey string
	Goal      GoalKind
	Standing  Standing
	Entries   int
}
