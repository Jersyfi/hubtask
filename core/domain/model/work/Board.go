// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// StateMode is how an entry type in a collection says where an entry stands (domain-model.md
// §3.5a). Derived from what the collection has - a workflow in force, board columns - and never
// stored, so no door can set one that disagrees with the other.
type StateMode string

const (
	// StateModeNone is a list: open or done, nothing else (L0).
	StateModeNone StateMode = "NONE"
	// StateModeColumns is a board without a workflow: the columns are the states, one of them
	// the done column (L1).
	StateModeColumns StateMode = "COLUMNS"
	// StateModeWorkflow is a workflow: the status decides, the columns show statuses (L2).
	StateModeWorkflow StateMode = "WORKFLOW"
)

// StateModeOf answers the level of one entry type: a workflow wins, then columns - which only a
// task can stand in (I-W6) -, else a list.
func StateModeOf(level ItemType, workflowRuns bool, columns int) StateMode {
	switch {
	case workflowRuns:
		return StateModeWorkflow
	case level == ItemTask && columns > 0:
		return StateModeColumns
	default:
		return StateModeNone
	}
}

// ColumnStatus is one row of a board's mapping: the column shows the status (L2).
type ColumnStatus struct {
	BucketID shared.ID
	Status   string
}

// Board is a collection's live columns, left to right, and their mapping to statuses.
type Board struct {
	Columns []Bucket
	Mapping []ColumnStatus
}

// Column answers a live column by identifier.
func (b Board) Column(id shared.ID) (Bucket, bool) {
	for _, column := range b.Columns {
		if column.ID == id {
			return column, true
		}
	}
	return Bucket{}, false
}

// DoneColumn answers the column that means finished (L1); false where the board has none.
func (b Board) DoneColumn() (Bucket, bool) {
	for _, column := range b.Columns {
		if column.IsDoneBucket {
			return column, true
		}
	}
	return Bucket{}, false
}

// FirstColumn answers the leftmost column that is not the done column: where an open entry
// without a column reads, and where a reopened one goes.
func (b Board) FirstColumn() (Bucket, bool) {
	for _, column := range b.Columns {
		if !column.IsDoneBucket {
			return column, true
		}
	}
	return Bucket{}, false
}

// ColumnOfEntry answers the column an entry reads as at L1: its own, else the done column when
// completed and the first column when open.
func (b Board) ColumnOfEntry(item WorkItem) shared.ID {
	if _, found := b.Column(item.BucketID); found {
		return item.BucketID
	}
	if item.Completion.IsCompleted {
		if done, found := b.DoneColumn(); found {
			return done.ID
		}
	}
	if first, found := b.FirstColumn(); found {
		return first.ID
	}
	return ""
}

// EnsureOneDone refuses a board at L1 that does not have exactly one done column.
func (b Board) EnsureOneDone() error {
	count := 0
	for _, column := range b.Columns {
		if column.IsDoneBucket {
			count++
		}
	}
	if count == 1 || len(b.Columns) == 0 {
		return nil
	}
	return shared.ErrConflict.WithDetail("buckets.done_column_required").
		WithFields(shared.FieldError{Path: "/is_done_bucket", Code: "buckets.done_column_required"})
}

// Coupled answers the entry with column and completion agreeing at L1, after a write that took
// it from before to after (UC-WRK-05 check 6). A column chosen wins: entering the done column
// completes, leaving it reopens. Otherwise a change of completion moves the column: a completed
// entry into the done column, a reopened one out of it into the first column.
func (b Board) Coupled(before, after WorkItem, by shared.ID, at time.Time) WorkItem {
	if after.Type != ItemTask || len(b.Columns) == 0 {
		return after
	}
	done, hasDone := b.DoneColumn()
	if after.BucketID != before.BucketID && !after.BucketID.IsZero() {
		inDone := hasDone && after.BucketID == done.ID
		switch {
		case inDone && !after.Completion.IsCompleted:
			return after.Completed(by, at)
		case !inDone && after.Completion.IsCompleted:
			return after.Reopened(at)
		}
		return after
	}
	if after.Completion.IsCompleted == before.Completion.IsCompleted {
		return after
	}
	if after.Completion.IsCompleted {
		if hasDone {
			after.BucketID = done.ID
		}
		return after
	}
	if hasDone && after.BucketID == done.ID {
		after.BucketID = ""
		if first, found := b.FirstColumn(); found {
			after.BucketID = first.ID
		}
	}
	return after
}

// Arrived answers an entry that came onto this board from elsewhere - moved in, imported - with
// its column agreeing (domain-model.md §3.5a): under a workflow the column showing its status; at
// L1 a column it was put in decides its completion, and a completed one without a column goes
// into the done column.
func (b Board) Arrived(item WorkItem, by shared.ID, at time.Time) WorkItem {
	if item.Type != ItemTask || len(b.Columns) == 0 {
		return item
	}
	if item.Status != "" {
		item.BucketID, _ = b.ColumnOfStatus(item.Status)
		return item
	}
	if _, live := b.Column(item.BucketID); !live {
		item.BucketID = ""
	}
	if item.BucketID.IsZero() {
		if done, found := b.DoneColumn(); found && item.Completion.IsCompleted {
			item.BucketID = done.ID
		}
		return item
	}
	before := item
	before.BucketID = ""
	return b.Coupled(before, item, by, at)
}

// Copied answers a copy - open, as every copy and every entry from a template is - in a column
// that agrees: under a workflow the one showing its status, else out of the done column into the
// first.
func (b Board) Copied(item WorkItem) WorkItem {
	if item.Type != ItemTask || len(b.Columns) == 0 {
		return item
	}
	if item.Status != "" {
		item.BucketID, _ = b.ColumnOfStatus(item.Status)
		return item
	}
	if done, found := b.DoneColumn(); found && item.BucketID == done.ID {
		item.BucketID = ""
		if first, found := b.FirstColumn(); found {
			item.BucketID = first.ID
		}
	}
	return item
}

// MarkDone answers the other columns whose mark changes when column is marked done or cleared:
// marking takes the mark off the others, and clearing the only done column of a board is refused
// as buckets.done_column_required (one done column per board, §3.5a).
func (b Board) MarkDone(column shared.ID, done bool) ([]Bucket, error) {
	current, hasDone := b.DoneColumn()
	if !done {
		if hasDone && current.ID == column {
			return nil, shared.ErrConflict.WithDetail("buckets.done_column_required").
				WithFields(shared.FieldError{Path: "/is_done_bucket", Code: "buckets.done_column_required"})
		}
		return nil, nil
	}
	var changed []Bucket
	for _, other := range b.Columns {
		if other.IsDoneBucket && other.ID != column {
			other.IsDoneBucket = false
			changed = append(changed, other)
		}
	}
	return changed, nil
}

// DoneSuccessor answers the column that takes the done mark when the done column deleted goes:
// the rightmost column left. False where deleted is not the done column or nothing is left.
func (b Board) DoneSuccessor(deleted shared.ID) (Bucket, bool) {
	if done, found := b.DoneColumn(); !found || done.ID != deleted {
		return Bucket{}, false
	}
	for i := len(b.Columns) - 1; i >= 0; i-- {
		if b.Columns[i].ID != deleted {
			return b.Columns[i], true
		}
	}
	return Bucket{}, false
}

// ChooseDoneSuccessor answers the column that takes the done mark when deleted goes, as the person
// deleting it chose (domain-model.md §3.5a): at L1 the done column of a board with other columns
// left is refused without a choice as buckets.done_successor_required, the suggestion
// (DoneSuccessor) in its parameters; under a mapping the suggestion is taken. Zero where deleted is
// not the done column or nothing is left.
func (b Board) ChooseDoneSuccessor(deleted, chosen shared.ID) (shared.ID, error) {
	suggested, found := b.DoneSuccessor(deleted)
	if !found {
		return "", nil
	}
	if len(b.Mapping) > 0 {
		return suggested.ID, nil
	}
	if chosen.IsZero() {
		return "", shared.ErrConflict.WithDetail("buckets.done_successor_required").
			WithParams(map[string]string{"suggested_bucket_id": suggested.ID.String()}).
			WithFields(shared.FieldError{Path: "/new_done_bucket_id", Code: "buckets.done_successor_required"})
	}
	if _, live := b.Column(chosen); !live || chosen == deleted {
		return "", shared.ErrNotFound.WithDetail("buckets.not_found").
			WithParams(map[string]string{"bucket_id": chosen.String()})
	}
	return chosen, nil
}

// DoneAfterWorkflow answers the column that is the done column once the workflow leaves the board:
// the one marked, else the rightmost whose statuses are all done, else the rightmost.
func (b Board) DoneAfterWorkflow(shape WorkflowShape) (Bucket, bool) {
	if done, found := b.DoneColumn(); found {
		return done, true
	}
	for i := len(b.Columns) - 1; i >= 0; i-- {
		if ColumnCompletes(b.StatusesOf(b.Columns[i].ID), shape) {
			return b.Columns[i], true
		}
	}
	if len(b.Columns) == 0 {
		return Bucket{}, false
	}
	return b.Columns[len(b.Columns)-1], true
}

// BoardCell counts a board's top-level tasks of one status, column and completion.
type BoardCell struct {
	Status    string
	BucketID  shared.ID
	Completed bool
	Count     int
}

// BoardChange is what a change of the workflow in force does to one board (UC-FLW-02 check 4).
type BoardChange struct {
	Before, After StateMode
	// Board is the board after: its columns and, under a workflow, its mapping.
	Board Board
	// DoneColumn is the done column after, at L1; it takes the mark where it lacks it.
	DoneColumn shared.ID
	// Unmapped are the statuses no column shows after, each with a suggested column.
	Unmapped []MappingSuggestion
	// Moved counts the entries whose column changes; ColumnOnly those of them whose status stays.
	Moved, ColumnOnly int
}

// PlanBoardChange answers what a board goes through when the workflow in force for its tasks
// changes from was to will - nil for none -, the board after carrying the mapping chosen. Under a
// workflow an entry stands in the column of the status it enters; without one a completed entry
// in the done column, an open one in its own column, else the first.
func PlanBoardChange(before, after Board, cells []BoardCell, was, will *WorkflowShape) BoardChange {
	change := BoardChange{
		Before: StateModeOf(ItemTask, was != nil, len(before.Columns)),
		After:  StateModeOf(ItemTask, will != nil, len(after.Columns)),
		Board:  after,
	}
	if len(after.Columns) == 0 {
		return change
	}
	if will == nil {
		change.Board.Mapping = nil
		done, _ := before.DoneAfterWorkflow(shapeOrEmpty(was))
		change.DoneColumn = done.ID
	} else {
		change.Unmapped = change.Board.Suggestions(*will)
	}
	for _, cell := range cells {
		status, column := "", shared.ID("")
		if will != nil {
			status = will.Entering(cell.Status, cell.Completed)
			column, _ = change.Board.ColumnOfStatus(status)
		} else {
			column = change.columnWithout(cell)
		}
		if column == cell.BucketID {
			continue
		}
		change.Moved += cell.Count
		if status == cell.Status {
			change.ColumnOnly += cell.Count
		}
	}
	return change
}

// columnWithout is the column of a cell once no workflow runs: completed into the done column,
// open in its own column where that is not the done column, else the first.
func (c BoardChange) columnWithout(cell BoardCell) shared.ID {
	if cell.Completed {
		return c.DoneColumn
	}
	if _, live := c.Board.Column(cell.BucketID); live && cell.BucketID != c.DoneColumn {
		return cell.BucketID
	}
	for _, column := range c.Board.Columns {
		if column.ID != c.DoneColumn {
			return column.ID
		}
	}
	return ""
}

func shapeOrEmpty(shape *WorkflowShape) WorkflowShape {
	if shape == nil {
		return WorkflowShape{}
	}
	return *shape
}

// TargetAfterDeleting answers where the entries of a deleted column go: under a mapped workflow
// none - their statuses are unmapped now, *Ohne Spalte* -; the done column's to its successor,
// where they stay completed; an open column's to the first open column left. Zero where nothing
// fits.
func (b Board) TargetAfterDeleting(deleted shared.ID) shared.ID {
	if len(b.Mapping) > 0 {
		return ""
	}
	if successor, found := b.DoneSuccessor(deleted); found {
		return successor.ID
	}
	for _, column := range b.Columns {
		if column.ID != deleted && !column.IsDoneBucket {
			return column.ID
		}
	}
	return ""
}

// StatusesOf answers the statuses a column shows (L2), in mapping order.
func (b Board) StatusesOf(column shared.ID) []string {
	var out []string
	for _, row := range b.Mapping {
		if row.BucketID == column {
			out = append(out, row.Status)
		}
	}
	return out
}

// ColumnOfStatus answers the column showing a status; false where none does, which is the
// virtual column *Ohne Spalte*.
func (b Board) ColumnOfStatus(status string) (shared.ID, bool) {
	for _, row := range b.Mapping {
		if row.Status == status {
			if _, live := b.Column(row.BucketID); live {
				return row.BucketID, true
			}
		}
	}
	return "", false
}

// Unmapped answers the shape's statuses no live column shows, in the shape's order.
func (b Board) Unmapped(shape WorkflowShape) []string {
	var out []string
	for _, status := range shape.Statuses {
		if _, mapped := b.ColumnOfStatus(status.Key); !mapped {
			out = append(out, status.Key)
		}
	}
	return out
}

// PlaceOfStatus answers the order keys a new column for a status stands between, either empty for
// an open end: right of the column showing the nearest status before it in the workflow's order,
// leftmost where none does. A status that does not complete never stands right of a done column,
// so the done column stays last.
func (b Board) PlaceOfStatus(shape WorkflowShape, status string) (after, before string) {
	at := -1
	for _, candidate := range shape.Statuses {
		if candidate.Key == status {
			break
		}
		if column, mapped := b.ColumnOfStatus(candidate.Key); mapped {
			at = b.columnIndex(column)
		}
	}
	if !shape.Completes(status) {
		for index, column := range b.Columns {
			if column.IsDoneBucket || ColumnCompletes(b.StatusesOf(column.ID), shape) {
				at = min(at, index-1)
				break
			}
		}
	}
	if at >= 0 {
		after = b.Columns[at].OrderKey
	}
	if at+1 < len(b.Columns) {
		before = b.Columns[at+1].OrderKey
	}
	return after, before
}

func (b Board) columnIndex(id shared.ID) int {
	for index, column := range b.Columns {
		if column.ID == id {
			return index
		}
	}
	return -1
}

// ColumnCompletes reports whether a column is done at L2: it shows statuses and every one of
// them is of the DONE category.
func ColumnCompletes(statuses []string, shape WorkflowShape) bool {
	if len(statuses) == 0 {
		return false
	}
	for _, status := range statuses {
		if !shape.Completes(status) {
			return false
		}
	}
	return true
}

// NewBoardMapping checks a mapping against the board and the shape: every column is live, every
// status the shape's, and no status in more than one column.
func NewBoardMapping(columns []Bucket, mapping []ColumnStatus, shape WorkflowShape) ([]ColumnStatus, error) {
	board := Board{Columns: columns}
	seen := map[string]bool{}
	out := make([]ColumnStatus, 0, len(mapping))
	for _, row := range mapping {
		if _, live := board.Column(row.BucketID); !live {
			return nil, mappingInvalid("buckets.not_found", map[string]string{"bucket_id": row.BucketID.String()})
		}
		if _, known := shape.Status(row.Status); !known {
			return nil, mappingInvalid("workflows.status_unknown", map[string]string{"key": row.Status})
		}
		if seen[row.Status] {
			return nil, mappingInvalid("buckets.status_mapped_twice", map[string]string{"key": row.Status})
		}
		seen[row.Status] = true
		out = append(out, row)
	}
	return out, nil
}

func mappingInvalid(code string, params map[string]string) error {
	return shared.ErrValidation.WithDetail(code).WithParams(params).
		WithFields(shared.FieldError{Path: "/columns", Code: code})
}

// MappingSuggestion is where a status the default mapping left unmapped would fit.
type MappingSuggestion struct {
	Status   string
	BucketID shared.ID
}

// DefaultMapping proposes a mapping of a board's columns to a shape's statuses (UC-FLW-02 check
// 4): a column takes the status of its own name first; the done column then takes the DONE
// statuses, the first column the initial status, and the next column still empty the first
// IN_PROGRESS status. What is left stays unmapped, each with a suggestion: DONE beside the done
// column, the rest beside the first column.
func DefaultMapping(columns []Bucket, shape WorkflowShape) ([]ColumnStatus, []MappingSuggestion) {
	board := Board{Columns: columns}
	taken := map[string]bool{}
	filled := map[shared.ID]bool{}
	var mapping []ColumnStatus
	put := func(column shared.ID, status string) {
		if column.IsZero() || status == "" || taken[status] {
			return
		}
		taken[status] = true
		filled[column] = true
		mapping = append(mapping, ColumnStatus{BucketID: column, Status: status})
	}
	for _, column := range columns {
		for _, status := range shape.Statuses {
			if sameWord(column.Name, status.Name) || sameWord(column.Name, status.Key) {
				put(column.ID, status.Key)
			}
		}
	}
	done, hasDone := board.DoneColumn()
	if hasDone && !filled[done.ID] {
		for _, status := range shape.Statuses {
			if status.Category.Completes() {
				put(done.ID, status.Key)
			}
		}
	}
	first, hasFirst := board.FirstColumn()
	if hasFirst && !filled[first.ID] {
		put(first.ID, shape.Initial)
	}
	for _, column := range columns {
		if filled[column.ID] || column.IsDoneBucket {
			continue
		}
		for _, status := range shape.Statuses {
			if status.Category == CategoryInProgress && !taken[status.Key] {
				put(column.ID, status.Key)
				break
			}
		}
		break
	}
	return mapping, Board{Columns: columns, Mapping: mapping}.Suggestions(shape)
}

// Suggestions answers the shape's statuses no column shows, each with where it would fit: a done
// status beside the done column, the rest beside the first column.
func (b Board) Suggestions(shape WorkflowShape) []MappingSuggestion {
	done, hasDone := b.DoneColumn()
	first, hasFirst := b.FirstColumn()
	var suggestions []MappingSuggestion
	for _, key := range b.Unmapped(shape) {
		suggestion := MappingSuggestion{Status: key}
		switch {
		case shape.Completes(key) && hasDone:
			suggestion.BucketID = done.ID
		case hasFirst:
			suggestion.BucketID = first.ID
		}
		suggestions = append(suggestions, suggestion)
	}
	return suggestions
}

func sameWord(a, b string) bool {
	return a != "" && b != "" && strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// DropOutcome is what a drop onto a column does.
type DropOutcome string

const (
	// DropMoved: the entry moves into the column (L1), or already stands there.
	DropMoved DropOutcome = "MOVED"
	// DropStep: exactly one step reaches the column and is taken.
	DropStep DropOutcome = "STEP_TAKEN"
	// DropChoose: several steps reach the column; the person chooses one.
	DropChoose DropOutcome = "CHOOSE"
	// DropRefused: nothing reaches the column; Reason says why.
	DropRefused DropOutcome = "REFUSED"
)

// DropPlan is what a drop resolves to.
type DropPlan struct {
	Outcome DropOutcome
	// Steps are the step taken, or the steps to choose among.
	Steps []WorkflowStep
	// Reason is the code of a refusal: board.no_columns, board.column_without_status,
	// board.no_step or board.step_blocked.
	Reason string
}

// PlanDrop resolves a drop of an entry onto a column - zero for *Ohne Spalte* - (UC-WRK-05,
// UC-FLW-06). At L1 it moves. At L2 the target's statuses are the column's, or the unmapped
// ones for *Ohne Spalte*; a status the entry already has is no move; else the steps from where
// it stands into one of them that the person may take decide: one is taken, several are offered,
// none is refused - blocked where the workflow has such a step but the person may not take it.
func (b Board) PlanDrop(mode StateMode, item WorkItem, target shared.ID, shape WorkflowShape, offered []WorkflowStep) DropPlan {
	switch mode {
	case StateModeColumns:
		if target.IsZero() {
			return DropPlan{Outcome: DropRefused, Reason: "board.column_required"}
		}
		return DropPlan{Outcome: DropMoved}
	case StateModeWorkflow:
	default:
		return DropPlan{Outcome: DropRefused, Reason: "board.no_columns"}
	}
	targets := b.StatusesOf(target)
	if target.IsZero() {
		targets = b.Unmapped(shape)
	}
	if len(targets) == 0 {
		return DropPlan{Outcome: DropRefused, Reason: "board.column_without_status"}
	}
	for _, status := range targets {
		if status == item.Status {
			return DropPlan{Outcome: DropMoved}
		}
	}
	reaches := func(step WorkflowStep) bool {
		for _, status := range targets {
			if step.To == status {
				return true
			}
		}
		return false
	}
	existing := 0
	for _, step := range shape.StepsFrom(item.Status) {
		if reaches(step) {
			existing++
		}
	}
	var steps []WorkflowStep
	for _, step := range offered {
		if step.LeavesFrom(item.Status) && reaches(step) {
			steps = append(steps, step)
		}
	}
	switch {
	case len(steps) == 1:
		return DropPlan{Outcome: DropStep, Steps: steps}
	case len(steps) > 1:
		return DropPlan{Outcome: DropChoose, Steps: steps}
	case existing > 0:
		return DropPlan{Outcome: DropRefused, Reason: "board.step_blocked"}
	default:
		return DropPlan{Outcome: DropRefused, Reason: "board.no_step"}
	}
}

// BoardTypes answers the entry types whose entries stand on a board (I-W6: tasks): the task's
// system type, every task-level type of the registry, and every type a scheme names that the
// registry does not place at another level.
func BoardTypes(registry EntryTypes, schemes []WorkflowScheme) []string {
	types := []string{SystemEntryTypeKey(ItemTask)}
	add := func(key string) {
		if key != "" && !slices.Contains(types, key) {
			types = append(types, key)
		}
	}
	for _, t := range registry {
		if t.Level == ItemTask {
			add(t.Key)
		}
	}
	for _, scheme := range schemes {
		if known, found := registry.Find(scheme.EntryType); found && known.Level != ItemTask {
			continue
		}
		if scheme.EntryType == SystemEntryTypeKey(ItemWorkPackage) || scheme.EntryType == SystemEntryTypeKey(ItemActivity) {
			continue
		}
		add(scheme.EntryType)
	}
	return types
}

// OnBoard answers what a collection's board shows under the schemes: the workflows in force for
// the types on it, as one shape (BoardShape), and their keys joined in order to tell one union from
// another; nil and "" where none runs.
func (ws Workflows) OnBoard(schemes []WorkflowScheme, collectionID, hubID shared.ID, types []string) (*WorkflowShape, string) {
	var keys []string
	var shapes []WorkflowShape
	for _, entryType := range types {
		key, found := ResolveWorkflow(schemes, collectionID, hubID, entryType)
		if !found || slices.Contains(keys, key) {
			continue
		}
		workflow, found := ws.Find(key)
		if !found {
			continue
		}
		keys = append(keys, key)
		shapes = append(shapes, workflow.Published)
	}
	if len(shapes) == 0 {
		return nil, ""
	}
	shape := BoardShape(shapes)
	return &shape, strings.Join(keys, ",")
}

// BoardShape answers what a board shows when its entry types run several workflows: their statuses
// in one shape, each key once - the first workflow's where two share a key -, so that a status key
// stands in at most one column. The first workflow's initial status is the board's; the shape
// carries no steps, which stay each workflow's own.
func BoardShape(shapes []WorkflowShape) WorkflowShape {
	if len(shapes) == 0 {
		return WorkflowShape{}
	}
	out := WorkflowShape{Initial: shapes[0].Initial}
	for _, shape := range shapes {
		for _, status := range shape.Statuses {
			if _, known := out.Status(status.Key); !known {
				out.Statuses = append(out.Statuses, status)
			}
		}
	}
	return out
}
