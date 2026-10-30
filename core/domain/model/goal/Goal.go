// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package goal is the goals and insight module's model (M8): goals of a period with key results,
// check-ins with a confidence, portfolios of hubs and collections with their status updates, and
// dashboards whose widgets are the query's own summaries.
//
// Progress is never typed in: a key result's comes from its values, a goal's from its key results,
// the goals below it and the entries linked to it, each counted once (UC-GOL-01). A number a reader
// sees is computed from what that reader may see (P-05).
package goal

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// Day is a civil date, as a plan's.
type Day = planning.Day

// The bounds of the model.
const (
	MaxTitleLength       = 200
	MaxDescriptionLength = 10000
	MaxNoteLength        = 5000
	MaxKeyResults        = 20
	MaxLinkedEntries     = 500
	MaxPeriodDays        = 731
	MaxUnitLength        = 16
	// MaxDepth bounds the alignment tree: a goal and its parents, this many deep.
	MaxDepth = 8
)

// PeriodState is whether a period is still being worked on.
type PeriodState string

const (
	PeriodOpen   PeriodState = "OPEN"
	PeriodClosed PeriodState = "CLOSED"
)

// Period is a cycle goals are set for - a quarter, a year -, closed with every goal's grade.
type Period struct {
	ID      shared.ID
	Name    string
	StartOn Day
	EndOn   Day
	State   PeriodState
	Version int
}

// CheckPeriod checks a period's name and dates.
func CheckPeriod(p Period, normalizer text.Normalizer) (Period, error) {
	name, err := title(p.Name, normalizer, "/name", "goal_periods.name")
	if err != nil {
		return Period{}, err
	}
	p.Name = name
	if p.EndOn < p.StartOn {
		return Period{}, invalid("/end_on", "goal_periods.range_reversed")
	}
	if int(p.EndOn-p.StartOn) >= MaxPeriodDays {
		return Period{}, invalid("/end_on", "goal_periods.range_too_long")
	}
	if p.State == "" {
		p.State = PeriodOpen
	}
	return p, nil
}

// Status is how a goal, or a portfolio, stands as its owner judges it at a check-in.
type Status string

const (
	StatusNone     Status = "NONE"
	StatusOnTrack  Status = "ON_TRACK"
	StatusAtRisk   Status = "AT_RISK"
	StatusOffTrack Status = "OFF_TRACK"
	// StatusDone and StatusDropped end a goal before its period closes.
	StatusDone    Status = "DONE"
	StatusDropped Status = "DROPPED"
)

// Valid reports whether a status is one of the set.
func (s Status) Valid() bool {
	switch s {
	case StatusNone, StatusOnTrack, StatusAtRisk, StatusOffTrack, StatusDone, StatusDropped:
		return true
	}
	return false
}

// Health is a status a person may give a portfolio or report at a check-in: the judgement, not
// an end.
func (s Status) Health() bool {
	return s == StatusOnTrack || s == StatusAtRisk || s == StatusOffTrack
}

// ResultKind is how a key result is measured.
type ResultKind string

const (
	// ResultNumber runs from a start value to a target value, either way.
	ResultNumber ResultKind = "NUMBER"
	// ResultPercent is a number between 0 and 100.
	ResultPercent ResultKind = "PERCENT"
	// ResultBinary is reached or not: its current value is 0 or 1.
	ResultBinary ResultKind = "BINARY"
	// ResultMilestone is reached when its entry is completed.
	ResultMilestone ResultKind = "MILESTONE"
)

// ResultSource is where a key result's current value comes from.
type ResultSource string

const (
	// SourceManual: the value a check-in reported.
	SourceManual ResultSource = "MANUAL"
	// SourceView: the total a saved summary view answers the reader - a count of entries or a
	// sum - so the number is the list's (P-11).
	SourceView ResultSource = "VIEW"
)

// KeyResult is one measurable result of a goal.
type KeyResult struct {
	ID      shared.ID
	Title   string
	Kind    ResultKind
	Start   float64
	Target  float64
	Current float64
	Unit    string
	Source  ResultSource
	// ViewID is the saved summary view of a VIEW result.
	ViewID shared.ID
	// EntryID is the entry a MILESTONE result waits for.
	EntryID shared.ID
}

// CheckKeyResult checks a key result's shape.
func CheckKeyResult(r KeyResult, at string, normalizer text.Normalizer) (KeyResult, error) {
	name, err := title(r.Title, normalizer, at+"/title", "key_results.title")
	if err != nil {
		return KeyResult{}, err
	}
	r.Title = name
	r.Unit = strings.TrimSpace(r.Unit)
	if utf8.RuneCountInString(r.Unit) > MaxUnitLength {
		return KeyResult{}, invalid(at+"/unit", "key_results.unit_too_long")
	}
	if r.Source == "" {
		r.Source = SourceManual
	}
	if r.Source != SourceManual && r.Source != SourceView {
		return KeyResult{}, invalid(at+"/source", "key_results.source_invalid")
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	if !finite(r.Start) || !finite(r.Target) || !finite(r.Current) {
		return KeyResult{}, invalid(at+"/target", "key_results.value_invalid")
	}
	switch r.Kind {
	case ResultNumber:
		if r.Start == r.Target {
			return KeyResult{}, invalid(at+"/target", "key_results.target_is_start")
		}
	case ResultPercent:
		for _, v := range []float64{r.Start, r.Target, r.Current} {
			if v < 0 || v > 100 {
				return KeyResult{}, invalid(at+"/target", "key_results.percent_out_of_range")
			}
		}
		if r.Start == r.Target {
			return KeyResult{}, invalid(at+"/target", "key_results.target_is_start")
		}
	case ResultBinary, ResultMilestone:
		r.Start, r.Target = 0, 1
		if r.Current != 0 && r.Current != 1 {
			return KeyResult{}, invalid(at+"/current", "key_results.binary_value")
		}
		if r.Source == SourceView {
			return KeyResult{}, invalid(at+"/source", "key_results.source_not_measurable")
		}
	default:
		return KeyResult{}, invalid(at+"/kind", "key_results.kind_invalid")
	}
	if r.Kind == ResultMilestone && r.EntryID.IsZero() {
		return KeyResult{}, invalid(at+"/entry_id", "key_results.milestone_entry_required")
	}
	if r.Kind != ResultMilestone {
		r.EntryID = ""
	}
	if r.Source == SourceView && r.ViewID.IsZero() {
		return KeyResult{}, invalid(at+"/view_id", "key_results.view_required")
	}
	if r.Source != SourceView {
		r.ViewID = ""
	}
	return r, nil
}

// Progress is how far a key result has come from its start to its target, 0 to 1: a value past
// the target counts as reached, one behind the start as none.
func (r KeyResult) Progress(current float64) float64 {
	if r.Target == r.Start {
		return 0
	}
	return clamp((current - r.Start) / (r.Target - r.Start))
}

// Goal is an objective of a period, at the workspace or in one hub, under an optional parent goal.
type Goal struct {
	ID          shared.ID
	PeriodID    shared.ID
	HubID       shared.ID
	ParentID    shared.ID
	Title       string
	Description string
	OwnerID     shared.ID
	Status      Status
	// Confidence is the last check-in's, 0 to 10; -1 before the first.
	Confidence int
	KeyResults []KeyResult
	// Grade is what the period's close gave the goal, 0 to 1; nil while it is open.
	Grade   *float64
	Version int
}

// GoalFields are what a person sets on a goal.
type GoalFields struct {
	PeriodID, HubID, ParentID, OwnerID shared.ID
	Title, Description                 string
	Status                             Status
	KeyResults                         []KeyResult
}

// CheckGoal checks a goal's fields; the key results keep the identifiers they came with.
func CheckGoal(f GoalFields, normalizer text.Normalizer) (GoalFields, error) {
	name, err := title(f.Title, normalizer, "/title", "goals.title")
	if err != nil {
		return GoalFields{}, err
	}
	f.Title = name
	f.Description = strings.TrimSpace(f.Description)
	if normalizer != nil {
		f.Description = normalizer.NFC(f.Description)
	}
	if utf8.RuneCountInString(f.Description) > MaxDescriptionLength {
		return GoalFields{}, invalid("/description", "goals.description_too_long")
	}
	if f.PeriodID.IsZero() {
		return GoalFields{}, invalid("/period_id", "goals.period_required")
	}
	if f.Status == "" {
		f.Status = StatusNone
	}
	if !f.Status.Valid() {
		return GoalFields{}, invalid("/status", "goals.status_invalid")
	}
	if len(f.KeyResults) > MaxKeyResults {
		return GoalFields{}, invalid("/key_results", "key_results.too_many")
	}
	seen := map[shared.ID]bool{}
	for i, r := range f.KeyResults {
		checked, err := CheckKeyResult(r, "/key_results/"+itoa(i), normalizer)
		if err != nil {
			return GoalFields{}, err
		}
		if checked.ID.IsZero() || seen[checked.ID] {
			return GoalFields{}, invalid("/key_results/"+itoa(i)+"/id", "key_results.identity_duplicate")
		}
		seen[checked.ID] = true
		f.KeyResults[i] = checked
	}
	return f, nil
}

// Parenting checks that a goal may sit under a parent: not under itself or one of its own goals
// (a cycle), and no deeper than MaxDepth. chain is the parent and its parents, nearest first.
func Parenting(goal shared.ID, chain []shared.ID) error {
	for _, id := range chain {
		if id == goal {
			return invalid("/parent_id", "goals.parent_cycle")
		}
	}
	if len(chain) >= MaxDepth {
		return invalid("/parent_id", "goals.too_deep")
	}
	return nil
}

// Part is one thing a goal's progress is made of, 0 to 1.
type Part struct {
	Kind     string
	Progress float64
}

// The kinds of part.
const (
	PartKeyResult = "KEY_RESULT"
	PartChild     = "CHILD_GOAL"
	PartEntries   = "ENTRIES"
)

// Progress is the mean of a goal's parts: each key result, each goal below it, and the share of
// its linked entries that are completed, counted once as a whole. A goal with no part has none.
// A dropped child goal is not a part: what nobody pursues any more does not hold its parent back.
func Progress(parts []Part) (float64, bool) {
	if len(parts) == 0 {
		return 0, false
	}
	sum := 0.0
	for _, p := range parts {
		sum += clamp(p.Progress)
	}
	return sum / float64(len(parts)), true
}

// EntriesPart is the share of linked entries completed; false with none.
func EntriesPart(done, total int) (Part, bool) {
	if total <= 0 {
		return Part{}, false
	}
	return Part{Kind: PartEntries, Progress: float64(done) / float64(total)}, true
}

// Confidence bounds.
const (
	MinConfidence = 0
	MaxConfidence = 10
)

// CheckIn is a moment somebody said how a goal stands: its status, how confident they are, a
// note, and the key results' values as they reported them. Check-ins are history; none changes.
type CheckIn struct {
	ID         shared.ID
	GoalID     shared.ID
	AuthorID   shared.ID
	Status     Status
	Confidence int
	Note       string
	Values     map[shared.ID]float64
	At         time.Time
}

// CheckCheckIn checks a check-in against its goal: a status that judges, a confidence 0 to 10,
// a note within bounds, values only for the goal's MANUAL key results and within their kind.
func CheckCheckIn(c CheckIn, g Goal, normalizer text.Normalizer) (CheckIn, error) {
	if !c.Status.Valid() || c.Status == StatusNone {
		return CheckIn{}, invalid("/status", "check_ins.status_invalid")
	}
	if c.Confidence < MinConfidence || c.Confidence > MaxConfidence {
		return CheckIn{}, invalid("/confidence", "check_ins.confidence_out_of_range")
	}
	c.Note = strings.TrimSpace(c.Note)
	if normalizer != nil {
		c.Note = normalizer.NFC(c.Note)
	}
	if utf8.RuneCountInString(c.Note) > MaxNoteLength {
		return CheckIn{}, invalid("/note", "check_ins.note_too_long")
	}
	if g.Grade != nil {
		return CheckIn{}, shared.ErrConflict.WithDetail("goals.graded")
	}
	for id, v := range c.Values {
		r, ok := g.result(id)
		if !ok {
			return CheckIn{}, invalid("/values", "check_ins.key_result_unknown")
		}
		if r.Source != SourceManual || r.Kind == ResultMilestone {
			return CheckIn{}, invalid("/values", "check_ins.value_not_manual")
		}
		probe := r
		probe.Current = v
		if _, err := CheckKeyResult(probe, "/values", nil); err != nil {
			return CheckIn{}, err
		}
	}
	return c, nil
}

// Apply writes a check-in's status, confidence and values onto its goal.
func (g *Goal) Apply(c CheckIn) {
	g.Status, g.Confidence = c.Status, c.Confidence
	for i, r := range g.KeyResults {
		if v, ok := c.Values[r.ID]; ok {
			g.KeyResults[i].Current = v
		}
	}
}

func (g Goal) result(id shared.ID) (KeyResult, bool) {
	for _, r := range g.KeyResults {
		if r.ID == id {
			return r, true
		}
	}
	return KeyResult{}, false
}

// CheckGrade checks a grade given at a period's close, 0 to 1.
func CheckGrade(grade float64) error {
	if math.IsNaN(grade) || grade < 0 || grade > 1 {
		return invalid("/grades", "goal_periods.grade_out_of_range")
	}
	return nil
}

func clamp(v float64) float64 {
	switch {
	case math.IsNaN(v) || v < 0:
		return 0
	case v > 1:
		return 1
	}
	return v
}

func title(raw string, normalizer text.Normalizer, path, family string) (string, error) {
	name := strings.TrimSpace(raw)
	if normalizer != nil {
		name = normalizer.NFC(name)
	}
	switch {
	case name == "":
		return "", invalid(path, family+"_required")
	case utf8.RuneCountInString(name) > MaxTitleLength || strings.ContainsAny(name, "\r\n"):
		return "", invalid(path, family+"_invalid")
	}
	return name, nil
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

func itoa(i int) string { return strconv.Itoa(i) }
