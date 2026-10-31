// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package planning is the planning module's domain (M2): how a plan's entries are scheduled along
// their dependencies on a working calendar - automatic entries placed as early as their
// predecessors allow, manual ones kept where a person put them and named where a dependency
// cannot hold (P-11) - which chain decides the end date, the float of every entry, the roll-up of
// a parent from its children, and the baselines a plan is compared with.
//
// Scheduling is a pure function of its inputs: the same plan on the same calendar answers the same
// dates, and nothing here reads a clock (rule 4). It proposes; a person applies (P-04).
package planning

import (
	"slices"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Mode says whether the scheduler may move an entry.
type Mode string

const (
	// Manual keeps the entry's dates; a dependency it breaks is reported, never repaired. What
	// every entry is until a planner says otherwise, so that nothing moves by surprise.
	Manual Mode = "MANUAL"
	// Automatic places the entry as early as its predecessors allow, its duration kept.
	Automatic Mode = "AUTOMATIC"
)

// Valid reports whether the mode is one of the two.
func (m Mode) Valid() bool { return m == Manual || m == Automatic }

// DependencyKind is how a successor's dates hang on its predecessor's.
type DependencyKind string

const (
	// FinishToStart: the successor starts once the predecessor has finished. The default.
	FinishToStart DependencyKind = "FS"
	// StartToStart: the successor starts once the predecessor has started.
	StartToStart DependencyKind = "SS"
	// FinishToFinish: the successor finishes once the predecessor has finished.
	FinishToFinish DependencyKind = "FF"
	// StartToFinish: the successor finishes once the predecessor has started.
	StartToFinish DependencyKind = "SF"
)

// Valid reports whether the kind is one of the four.
func (k DependencyKind) Valid() bool {
	return k == FinishToStart || k == StartToStart || k == FinishToFinish || k == StartToFinish
}

// The bounds of one scheduling run (rule 7): a lag, the entries of a plan, how far the plan may
// reach from its earliest date.
const (
	MaxLagDays    = 999
	MaxEntries    = 2000
	maxReachDays  = 40 * 366
	backwardRoom  = 3 * 366
	minPlanLength = 1
)

// Timing is a dependency's kind and lag. The lag counts working days and may be negative - a lead.
type Timing struct {
	Kind    DependencyKind
	LagDays int
}

// NewTiming checks a dependency's timing; an empty kind is finish-to-start.
func NewTiming(kind DependencyKind, lagDays int) (Timing, error) {
	if kind == "" {
		kind = FinishToStart
	}
	if !kind.Valid() {
		return Timing{}, invalid("/dependency_kind", "planning.dependency_kind_unknown")
	}
	if lagDays < -MaxLagDays || lagDays > MaxLagDays {
		return Timing{}, shared.ErrValidation.WithDetail("planning.lag_out_of_range").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxLagDays)}).
			WithFields(shared.FieldError{Path: "/lag_days", Code: "planning.lag_out_of_range"})
	}
	return Timing{Kind: kind, LagDays: lagDays}, nil
}

// Day is a civil date, counted in days from 1970-01-01. A plan is scheduled in whole days.
type Day int

// DayOf answers the local date of an instant in a zone.
func DayOf(at time.Time, zone *time.Location) Day {
	local := at.In(zone)
	return DayFrom(local.Year(), local.Month(), local.Day())
}

// DayFrom answers the day of a civil date.
func DayFrom(year int, month time.Month, day int) Day {
	return Day(time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

// Date answers the civil date of the day.
func (d Day) Date() (int, time.Month, int) {
	t := time.Unix(int64(d)*86400, 0).UTC()
	return t.Year(), t.Month(), t.Day()
}

// In answers the day's local midnight in a zone.
func (d Day) In(zone *time.Location) time.Time {
	y, m, day := d.Date()
	return time.Date(y, m, day, 0, 0, 0, 0, zone)
}

// String is the day as YYYY-MM-DD.
func (d Day) String() string { return d.In(time.UTC).Format(time.DateOnly) }

// Workdays answers whether a local day holds working time (calendar.BusinessCalendar).
type Workdays interface {
	WorksOn(year int, month time.Month, day int) bool
}

// Entry is one entry of a plan as the scheduler reads it. Start and Finish are inclusive days;
// an entry with neither is unscheduled and left out of every chain.
type Entry struct {
	ID shared.ID
	// ParentID is the entry it rolls up into, zero where the parent is not part of the plan.
	ParentID  shared.ID
	Mode      Mode
	Milestone bool
	// Done entries are never moved: finished work happened when it happened.
	Done   bool
	Start  *Day
	Finish *Day
}

// Dependency is "Successor waits for Predecessor" with its timing.
type Dependency struct {
	Predecessor shared.ID
	Successor   shared.ID
	Timing      Timing
}

// Plan is what one run schedules.
type Plan struct {
	Entries      []Entry
	Dependencies []Dependency
	Calendar     Workdays
}

// Placement is where the run puts one scheduled entry, and how much room it has.
type Placement struct {
	ID     shared.ID
	Start  Day
	Finish Day
	// Duration is in working days; a milestone has none.
	Duration int
	// Moved says the run proposes dates other than the entry's own.
	Moved bool
	// TotalFloat is how many working days the entry may slip before the plan's end slips;
	// FreeFloat before any successor must. Negative on a chain a manual entry breaks.
	TotalFloat int
	FreeFloat  int
	Critical   bool
	// Summary marks a parent whose dates are its children's span.
	Summary bool
}

// Conflict is a dependency the run could not keep: its successor is manual or done, or a summary.
type Conflict struct {
	Predecessor shared.ID
	Successor   shared.ID
	Kind        DependencyKind
	// Earliest is the earliest start (FS, SS) or finish (FF, SF) the dependency allows.
	Earliest Day
}

// Result is a run's whole answer.
type Result struct {
	// Placements are the scheduled entries, in the order the plan listed them.
	Placements  []Placement
	Unscheduled []shared.ID
	Conflicts   []Conflict
	// Finish is the plan's last day; CriticalPath its critical entries, earliest first.
	Finish       Day
	CriticalPath []shared.ID
}

// Moves are the placements the run proposes to change.
func (r Result) Moves() []Placement {
	var out []Placement
	for _, p := range r.Placements {
		if p.Moved {
			out = append(out, p)
		}
	}
	return out
}

// task is an entry on the working-day axis: [S, E) in working-day indices.
type task struct {
	entry         Entry
	s, e, d       int
	ls, le        int
	preds, succs  []int
	summary       bool
	originalStart Day
	originalEnd   Day
}

// Schedule runs the plan: a forward pass places the automatic entries, a backward pass measures
// the float, and the parents are rolled up from their children. A loop of dependencies is
// refused with `planning.cycle`.
func Schedule(plan Plan) (Result, error) {
	if len(plan.Entries) > MaxEntries {
		return Result{}, shared.ErrValidation.WithDetail("planning.too_many_entries").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxEntries)})
	}
	if plan.Calendar == nil {
		return Result{}, shared.ErrInternal.WithDetail("planning.calendar_missing")
	}
	index := map[shared.ID]int{}
	parents := map[shared.ID]bool{}
	for _, entry := range plan.Entries {
		if !entry.ParentID.IsZero() {
			parents[entry.ParentID] = true
		}
	}
	var (
		tasks       []*task
		unscheduled []shared.ID
		origin      Day
		first       = true
	)
	for _, entry := range plan.Entries {
		if _, seen := index[entry.ID]; seen {
			continue
		}
		start, finish := entry.Start, entry.Finish
		if start == nil && finish == nil {
			unscheduled = append(unscheduled, entry.ID)
			continue
		}
		if start == nil {
			start = finish
		}
		if finish == nil || *finish < *start {
			finish = start
		}
		if entry.Milestone {
			start = finish
		}
		index[entry.ID] = len(tasks)
		tasks = append(tasks, &task{entry: entry, summary: parents[entry.ID], originalStart: *start, originalEnd: *finish})
		if first || *start < origin {
			origin, first = *start, false
		}
	}
	result := Result{Unscheduled: unscheduled, Placements: []Placement{}, Conflicts: []Conflict{}, CriticalPath: []shared.ID{}}
	if len(tasks) == 0 {
		return result, nil
	}
	axis := newAxis(plan.Calendar, origin-backwardRoom)
	for _, t := range tasks {
		s, err := axis.atOrAfter(t.originalStart)
		if err != nil {
			return Result{}, err
		}
		t.s = s
		if t.entry.Milestone {
			// A milestone is the end of its day: what finishes that day meets it.
			last, err := axis.atOrBefore(t.originalEnd)
			if err != nil {
				return Result{}, err
			}
			t.s, t.d = last+1, 0
		} else {
			last, err := axis.atOrBefore(t.originalEnd)
			if err != nil {
				return Result{}, err
			}
			t.d = max(last-t.s+1, minPlanLength)
		}
		t.e = t.s + t.d
	}
	var deps []Dependency
	for _, dep := range plan.Dependencies {
		p, okP := index[dep.Predecessor]
		s, okS := index[dep.Successor]
		if !okP || !okS || p == s || tasks[p].summary || tasks[s].summary {
			// A summary follows its children; a dependency on it is drawn, not scheduled.
			continue
		}
		if dep.Timing.Kind == "" {
			dep.Timing.Kind = FinishToStart
		}
		deps = append(deps, dep)
		tasks[p].succs = append(tasks[p].succs, len(deps)-1)
		tasks[s].preds = append(tasks[s].preds, len(deps)-1)
	}
	order, err := topological(tasks, deps, index)
	if err != nil {
		return Result{}, err
	}

	for _, i := range order {
		t := tasks[i]
		earliest, bound := 0, false
		for _, k := range t.preds {
			need := earliestStart(tasks[index[deps[k].Predecessor]], t, deps[k].Timing)
			if !bound || need > earliest {
				earliest, bound = need, true
			}
		}
		if !bound {
			continue
		}
		movable := t.entry.Mode == Automatic && !t.entry.Done && !t.summary
		if movable {
			t.s, t.e = earliest, earliest+t.d
			continue
		}
		if t.s < earliest {
			for _, k := range t.preds {
				dep := deps[k]
				need := earliestStart(tasks[index[dep.Predecessor]], t, dep.Timing)
				if t.s >= need {
					continue
				}
				at := need
				if dep.Timing.Kind == FinishToFinish || dep.Timing.Kind == StartToFinish {
					at = need + t.d - 1
				}
				day, err := axis.day(max(at, 0))
				if err != nil {
					return Result{}, err
				}
				result.Conflicts = append(result.Conflicts, Conflict{
					Predecessor: dep.Predecessor, Successor: dep.Successor, Kind: dep.Timing.Kind, Earliest: day,
				})
			}
		}
	}

	rollUp(tasks, index)

	end := tasks[0].e
	for _, t := range tasks {
		end = max(end, t.e)
	}
	for n := len(order) - 1; n >= 0; n-- {
		t := tasks[order[n]]
		t.le = end
		for _, k := range t.succs {
			succ := tasks[index[deps[k].Successor]]
			t.le = min(t.le, latestEnd(t, succ, deps[k].Timing))
		}
		t.ls = t.le - t.d
	}
	for _, t := range tasks {
		start, err := axis.day(t.s)
		if err != nil {
			return Result{}, err
		}
		var finish Day
		if t.d > 0 {
			if finish, err = axis.day(t.e - 1); err != nil {
				return Result{}, err
			}
		} else if finish, err = axis.day(t.s - 1); err != nil {
			return Result{}, err
		} else {
			start = finish
		}
		free := end - t.e
		for _, k := range t.succs {
			succ := tasks[index[deps[k].Successor]]
			free = min(free, slack(t, succ, deps[k].Timing))
		}
		total := t.ls - t.s
		placement := Placement{
			ID: t.entry.ID, Start: start, Finish: finish, Duration: t.d,
			Moved:      start != t.originalStart || finish != t.originalEnd,
			TotalFloat: total, FreeFloat: free, Critical: total <= 0 && !t.summary, Summary: t.summary,
		}
		if t.entry.Mode != Automatic || t.entry.Done {
			placement.Moved = false
		}
		result.Placements = append(result.Placements, placement)
	}
	result.Finish, err = axis.day(end - 1)
	if err != nil {
		return Result{}, err
	}
	critical := slices.Clone(result.Placements)
	slices.SortStableFunc(critical, func(a, b Placement) int { return int(a.Start) - int(b.Start) })
	for _, p := range critical {
		if p.Critical {
			result.CriticalPath = append(result.CriticalPath, p.ID)
		}
	}
	return result, nil
}

// earliestStart is the earliest start index the dependency allows the successor.
func earliestStart(pred, succ *task, timing Timing) int {
	switch timing.Kind {
	case StartToStart:
		return pred.s + timing.LagDays
	case FinishToFinish:
		return pred.e + timing.LagDays - succ.d
	case StartToFinish:
		return pred.s + timing.LagDays - succ.d
	default:
		return pred.e + timing.LagDays
	}
}

// latestEnd is the latest end index of the predecessor that keeps the successor's late dates.
func latestEnd(pred, succ *task, timing Timing) int {
	switch timing.Kind {
	case StartToStart:
		return succ.ls - timing.LagDays + pred.d
	case FinishToFinish:
		return succ.le - timing.LagDays
	case StartToFinish:
		return succ.le - timing.LagDays + pred.d
	default:
		return succ.ls - timing.LagDays
	}
}

// slack is how far the predecessor may slip before it pushes the successor's early dates.
func slack(pred, succ *task, timing Timing) int {
	return succ.s - earliestStart(pred, succ, timing)
}

// topological orders the scheduled entries so that every predecessor comes first; a loop is
// refused, its entries not named - some may be out of the reader's reach.
func topological(tasks []*task, deps []Dependency, index map[shared.ID]int) ([]int, error) {
	waiting := make([]int, len(tasks))
	for i, t := range tasks {
		waiting[i] = len(t.preds)
	}
	queue := make([]int, 0, len(tasks))
	for i := range tasks {
		if waiting[i] == 0 {
			queue = append(queue, i)
		}
	}
	order := make([]int, 0, len(tasks))
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		order = append(order, i)
		for _, k := range tasks[i].succs {
			s := index[deps[k].Successor]
			waiting[s]--
			if waiting[s] == 0 {
				queue = append(queue, s)
			}
		}
	}
	if len(order) != len(tasks) {
		return nil, shared.ErrConflict.WithDetail("planning.cycle")
	}
	return order, nil
}

// rollUp sets each summary to its scheduled children's span, deepest first (three levels at most,
// domain-model.md §2; the loop ends on any depth).
func rollUp(tasks []*task, index map[shared.ID]int) {
	for range len(tasks) {
		changed := false
		for _, t := range tasks {
			if !t.summary {
				continue
			}
			s, e, found := 0, 0, false
			for _, child := range tasks {
				if child.entry.ParentID != t.entry.ID {
					continue
				}
				if _, ok := index[child.entry.ID]; !ok {
					continue
				}
				if !found || child.s < s {
					s = child.s
				}
				if !found || child.e > e {
					e = child.e
				}
				found = true
			}
			if found && (t.s != s || t.e != e) && t.entry.Mode == Automatic && !t.entry.Done {
				t.s, t.e, t.d = s, e, e-s
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

// axis numbers the working days from an origin: index 0 is the first working day on or after it.
type axis struct {
	calendar Workdays
	origin   Day
	days     []Day
}

func newAxis(calendar Workdays, origin Day) *axis { return &axis{calendar: calendar, origin: origin} }

// day answers the working day at an index, growing the axis as far as it is asked.
func (a *axis) day(i int) (Day, error) {
	if i < 0 {
		return 0, shared.ErrValidation.WithDetail("planning.too_far")
	}
	next := a.origin
	if len(a.days) > 0 {
		next = a.days[len(a.days)-1] + 1
	}
	for len(a.days) <= i {
		if int(next-a.origin) > maxReachDays {
			return 0, shared.ErrValidation.WithDetail("planning.too_far")
		}
		y, m, d := next.Date()
		if a.calendar.WorksOn(y, m, d) {
			a.days = append(a.days, next)
		}
		next++
	}
	return a.days[i], nil
}

// atOrAfter answers the index of the first working day on or after a day.
func (a *axis) atOrAfter(target Day) (int, error) {
	for i := 0; ; i++ {
		day, err := a.day(i)
		if err != nil {
			return 0, err
		}
		if day >= target {
			return i, nil
		}
	}
}

// atOrBefore answers the index of the last working day on or before a day.
func (a *axis) atOrBefore(target Day) (int, error) {
	i, err := a.atOrAfter(target)
	if err != nil {
		return 0, err
	}
	if day, _ := a.day(i); day > target {
		i--
	}
	return i, nil
}

// AddWorkdays answers the day n working days after a working day on the calendar (before it for a
// negative n): what moves a finish when a duration is set (M2, duration against dates).
func AddWorkdays(calendar Workdays, from Day, n int) (Day, error) {
	if n < -maxReachDays || n > maxReachDays {
		return 0, shared.ErrValidation.WithDetail("planning.too_far")
	}
	a := newAxis(calendar, from-backwardRoom-Day(max(-n, 0))*2)
	i, err := a.atOrAfter(from)
	if err != nil {
		return 0, err
	}
	return a.day(i + n)
}

// WorkdaysBetween counts the working days from one day to another, both included.
func WorkdaysBetween(calendar Workdays, from, to Day) (int, error) {
	if to < from {
		return 0, nil
	}
	if int(to-from) > maxReachDays {
		return 0, shared.ErrValidation.WithDetail("planning.too_far")
	}
	count := 0
	for d := from; d <= to; d++ {
		y, m, day := d.Date()
		if calendar.WorksOn(y, m, day) {
			count++
		}
	}
	return count, nil
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}
