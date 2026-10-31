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
	// GoalNextResponse waits for the team's answer to the requester's words: armed by a comment of
	// theirs once the first response is met, met by the next public reply or the completion, and
	// armed again by their next comment.
	GoalNextResponse GoalKind = "NEXT_RESPONSE"
	// GoalResolution is met by the entry's completion.
	GoalResolution GoalKind = "RESOLUTION"
)

var goalKinds = [...]GoalKind{GoalFirstResponse, GoalNextResponse, GoalResolution}

// GoalKinds answers every kind, in the order a screen draws them.
func GoalKinds() []GoalKind { return goalKinds[:] }

// Valid reports whether the kind is one of the set.
func (k GoalKind) Valid() bool { return slices.Contains(goalKinds[:], k) }

// WaitsForRequester reports whether the goal is armed by the requester's words rather than by the
// entry's appearing.
func (k GoalKind) WaitsForRequester() bool { return k == GoalNextResponse }

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
	// MaxPolicyOrganisations bounds the requester organisations one policy is for.
	MaxPolicyOrganisations = 20
	MaxPriorityTargets     = 20
	MaxPauseStatuses       = 20
	// MaxTargetMinutes is the timer's own bound: a year of working time.
	MaxTargetMinutes = int(calendar.MaxBudget / time.Minute)
)

// SLAGoal is one promise of a policy: a target in working minutes, narrower per priority, the
// statuses in which its clock stands still (waiting for the requester), and the share of the
// target at which it is announced at risk.
type SLAGoal struct {
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
func (g SLAGoal) Target(priority string) time.Duration {
	if minutes, ok := g.ByPriority[priority]; ok && priority != "" {
		return time.Duration(minutes) * time.Minute
	}
	return time.Duration(g.Minutes) * time.Minute
}

// Warning answers the working time after which the goal is at risk; false when it announces none.
func (g SLAGoal) Warning(priority string) (time.Duration, bool) {
	if g.WarnPercent == 0 {
		return 0, false
	}
	warning := g.Target(priority) * time.Duration(g.WarnPercent) / 100
	return warning, warning > 0
}

// Pauses reports whether the clock stands still while the entry is in the status.
func (g SLAGoal) Pauses(status string) bool {
	return status != "" && slices.Contains(g.PauseStatuses, status)
}

// SLAPolicy is a workspace's promise for entries of some types: its goals, counted on one of the
// workspace's business calendars - or around the clock when it names none.
type SLAPolicy struct {
	ID          shared.ID
	TenantID    shared.ID
	Key         string
	Name        string
	CalendarKey string
	EntryTypes  []string
	// Organisations are the requester organisations' keys whose entries the policy measures;
	// empty measures every requester's (UC-SVC-02 check 9).
	Organisations []string
	// PriorityField is the custom field whose option value picks a goal's target; empty takes the
	// goals' own.
	PriorityField string
	Goals         []SLAGoal
	Position      int
	Enabled       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int
}

// NewSLAPolicyInput is what defining a policy takes.
type NewSLAPolicyInput struct {
	ID, TenantID  shared.ID
	Key, Name     string
	CalendarKey   string
	EntryTypes    []string
	Organisations []string
	PriorityField string
	Goals         []SLAGoal
	Position      int
	Enabled       bool
	Now           time.Time
	Text          text.Normalizer
}

// NewSLAPolicy checks a policy: a key, a name, at least one type and one goal, each kind once,
// targets inside a timer's bound.
func NewSLAPolicy(in NewSLAPolicyInput) (SLAPolicy, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return SLAPolicy{}, shared.ErrInternal.WithDetail("sla_policies.identity_incomplete")
	}
	key := strings.TrimSpace(in.Key)
	if !keyPattern.MatchString(key) {
		return SLAPolicy{}, invalid("/key", "sla_policies.key_malformed")
	}
	name, err := checkName(in.Name, in.Text, "sla_policies")
	if err != nil {
		return SLAPolicy{}, err
	}
	calendarKey := strings.TrimSpace(in.CalendarKey)
	if calendarKey != "" && !keyPattern.MatchString(calendarKey) {
		return SLAPolicy{}, invalid("/calendar", "sla_policies.calendar_malformed")
	}
	priorityField := strings.TrimSpace(in.PriorityField)
	if priorityField != "" && !keyPattern.MatchString(priorityField) {
		return SLAPolicy{}, invalid("/priority_field", "sla_policies.priority_field_malformed")
	}
	types, err := checkKeys(in.EntryTypes, "/entry_types", MaxPolicyEntryTypes)
	if err != nil {
		return SLAPolicy{}, err
	}
	if len(types) == 0 {
		return SLAPolicy{}, invalid("/entry_types", "sla_policies.entry_types_required")
	}
	organisations, err := checkKeys(in.Organisations, "/organisations", MaxPolicyOrganisations)
	if err != nil {
		return SLAPolicy{}, err
	}
	if len(in.Goals) == 0 {
		return SLAPolicy{}, invalid("/goals", "sla_policies.goals_required")
	}
	goals := make([]SLAGoal, 0, len(in.Goals))
	for i, goal := range in.Goals {
		checked, err := checkGoal(goal, "/goals/"+strconv.Itoa(i))
		if err != nil {
			return SLAPolicy{}, err
		}
		if slices.ContainsFunc(goals, func(g SLAGoal) bool { return g.Kind == checked.Kind }) {
			return SLAPolicy{}, invalid("/goals/"+strconv.Itoa(i)+"/kind", "sla_policies.goal_twice")
		}
		goals = append(goals, checked)
	}
	slices.SortFunc(goals, func(a, b SLAGoal) int {
		return cmp.Compare(slices.Index(goalKinds[:], a.Kind), slices.Index(goalKinds[:], b.Kind))
	})
	return SLAPolicy{
		ID: in.ID, TenantID: in.TenantID, Key: key, Name: name, CalendarKey: calendarKey,
		EntryTypes: types, Organisations: organisations, PriorityField: priorityField, Goals: goals, Position: in.Position,
		Enabled: in.Enabled, CreatedAt: in.Now, UpdatedAt: in.Now, Version: 1,
	}, nil
}

func checkGoal(goal SLAGoal, at string) (SLAGoal, error) {
	if !goal.Kind.Valid() {
		return SLAGoal{}, invalid(at+"/kind", "sla_policies.goal_kind_unknown")
	}
	if goal.Minutes <= 0 || goal.Minutes > MaxTargetMinutes {
		return SLAGoal{}, invalid(at+"/minutes", "sla_policies.target_invalid")
	}
	if len(goal.ByPriority) > MaxPriorityTargets {
		return SLAGoal{}, invalid(at+"/by_priority", "sla_policies.too_many_targets")
	}
	byPriority := make(map[string]int, len(goal.ByPriority))
	for _, priority := range slices.Sorted(maps.Keys(goal.ByPriority)) {
		minutes := goal.ByPriority[priority]
		if strings.TrimSpace(priority) == "" || len(priority) > MaxNameLength {
			return SLAGoal{}, invalid(at+"/by_priority", "sla_policies.priority_malformed")
		}
		if minutes <= 0 || minutes > MaxTargetMinutes {
			return SLAGoal{}, invalid(at+"/by_priority/"+priority, "sla_policies.target_invalid")
		}
		byPriority[priority] = minutes
	}
	pauses, err := checkKeys(goal.PauseStatuses, at+"/pause_statuses", MaxPauseStatuses)
	if err != nil {
		return SLAGoal{}, err
	}
	if goal.WarnPercent < 0 || goal.WarnPercent > 99 {
		return SLAGoal{}, invalid(at+"/warn_percent", "sla_policies.warn_percent_invalid")
	}
	return SLAGoal{
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
func (p SLAPolicy) Change(in NewSLAPolicyInput) (SLAPolicy, error) {
	in.ID, in.TenantID, in.Key = p.ID, p.TenantID, p.Key
	changed, err := NewSLAPolicy(in)
	if err != nil {
		return SLAPolicy{}, err
	}
	changed.CreatedAt, changed.Version = p.CreatedAt, p.Version
	return changed, nil
}

// Covers reports whether the policy measures entries of the type raised by a requester of the
// organisation (its key, empty for none).
func (p SLAPolicy) Covers(entryType, organisation string) bool {
	return p.Enabled && entryType != "" && slices.Contains(p.EntryTypes, entryType) &&
		(len(p.Organisations) == 0 || organisation != "" && slices.Contains(p.Organisations, organisation))
}

// ForOrganisations reports whether any of the policies is for some organisations only, so that
// a caller reads the requester's organisation only where it can matter.
func ForOrganisations(policies []SLAPolicy) bool {
	return slices.ContainsFunc(policies, func(p SLAPolicy) bool { return p.Enabled && len(p.Organisations) > 0 })
}

// Goal answers the policy's goal of a kind.
func (p SLAPolicy) Goal(kind GoalKind) (SLAGoal, bool) {
	for _, goal := range p.Goals {
		if goal.Kind == kind {
			return goal, true
		}
	}
	return SLAGoal{}, false
}

// Choose answers the policy an entry of the type, raised by a requester of the organisation, is
// measured against: the first enabled one that covers both, by position and then key - one policy
// per entry, so its clocks never disagree.
func Choose(policies []SLAPolicy, entryType, organisation string) (SLAPolicy, bool) {
	ordered := slices.Clone(policies)
	slices.SortFunc(ordered, func(a, b SLAPolicy) int {
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.Key, b.Key))
	})
	for _, policy := range ordered {
		if policy.Covers(entryType, organisation) {
			return policy, true
		}
	}
	return SLAPolicy{}, false
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

// EntrySLA is what became of one goal on one entry: the record a report reads and the archive
// carries, beside the timer that is its clock.
type EntrySLA struct {
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
func (r EntrySLA) Follow(timer calendar.Timer, now time.Time) EntrySLA {
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

// Rearm starts a met goal's next round at now, its marks of the last round cleared; false for one
// still live or breached - a round is not cut short, and a broken promise stays on the entry.
func (r EntrySLA) Rearm(target time.Duration, now time.Time) (EntrySLA, bool) {
	if r.Standing != StandingMet {
		return r, false
	}
	r.Target, r.ArmedAt, r.Standing = target, now, StandingRunning
	r.DueAt, r.AtRiskAt, r.MetAt = nil, nil, nil
	return r, true
}

// ReportRow counts the goals of one policy and kind in one standing.
type ReportRow struct {
	PolicyKey string
	Goal      GoalKind
	Standing  Standing
	Entries   int
}
