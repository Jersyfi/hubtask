// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A ZIP of an issue tracker's answers may carry, beside the search answers, the workflow search
// answer with its transitions and the boards' configurations: the transitions become the
// workflow's steps, a board's columns the order of its statuses and a board view of the
// collection. The issues' sprint field becomes the collection's sprints and no field, their story
// points the entries' estimates beside the field (UC-IMP-07).

type trackerWorkflows struct {
	Values []trackerWorkflow `json:"values"`
}

type trackerWorkflow struct {
	ID struct {
		Name string `json:"name"`
	} `json:"id"`
	Statuses    []trackerNamed `json:"statuses"`
	Transitions []struct {
		ID   string   `json:"id"`
		Name string   `json:"name"`
		From []string `json:"from"`
		To   string   `json:"to"`
		Type string   `json:"type"`
	} `json:"transitions"`
}

type trackerBoard struct {
	ID       json.Number `json:"id"`
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Location struct {
		ProjectKey string `json:"projectKey"`
	} `json:"location"`
	ColumnConfig *struct {
		Columns []struct {
			Name     string `json:"name"`
			Statuses []struct {
				ID string `json:"id"`
			} `json:"statuses"`
		} `json:"columns"`
	} `json:"columnConfig"`
}

type trackerSprint struct {
	ID           json.Number `json:"id"`
	Name         string      `json:"name"`
	State        string      `json:"state"`
	Goal         string      `json:"goal"`
	StartDate    string      `json:"startDate"`
	EndDate      string      `json:"endDate"`
	CompleteDate string      `json:"completeDate"`
}

// readPlanning reads a ZIP member that is no search answer: a workflow search answer or a board's
// configuration, alone or as an array. It answers whether the member was one.
func (a *trackerAnswer) readPlanning(content []byte) bool {
	var workflows trackerWorkflows
	if json.Unmarshal(content, &workflows) == nil && len(workflows.Values) > 0 && workflows.Values[0].Transitions != nil {
		a.workflows = append(a.workflows, workflows.Values...)
		return true
	}
	var boards []trackerBoard
	if json.Unmarshal(content, &boards) != nil {
		var one trackerBoard
		if json.Unmarshal(content, &one) != nil {
			return false
		}
		boards = []trackerBoard{one}
	}
	found := false
	for _, board := range boards {
		if board.ColumnConfig != nil && board.Name != "" {
			a.boards = append(a.boards, board)
			found = true
		}
	}
	return found
}

func (r *trackerRun) sprintField(id string) bool {
	return strings.HasSuffix(r.answer.Schema[id].Custom, ":gh-sprint")
}

func (r *trackerRun) pointsField(id string) bool {
	schema := r.answer.Schema[id]
	if strings.HasSuffix(schema.Custom, ":jsw-story-points") {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(r.answer.Names[id]))
	return schema.Type == "number" && (name == "story points" || name == "story point estimate")
}

// boardsOf answers the boards on the project.
func (r *trackerRun) boardsOf(project string) []trackerBoard {
	var out []trackerBoard
	for _, board := range r.answer.boards {
		if strings.EqualFold(board.Location.ProjectKey, project) {
			out = append(out, board)
		}
	}
	return out
}

// orderByBoard orders the statuses as the project's first board shows them, column by column; a
// status no column shows follows in its category's place.
func orderByBoard(statuses []work.WorkflowStatus, boards []trackerBoard, idKeys map[string]string) bool {
	if len(boards) == 0 || boards[0].ColumnConfig == nil {
		return false
	}
	rank := map[string]int{}
	for c, column := range boards[0].ColumnConfig.Columns {
		for _, status := range column.Statuses {
			if key, ok := idKeys[status.ID]; ok {
				if _, seen := rank[key]; !seen {
					rank[key] = c
				}
			}
		}
	}
	if len(rank) == 0 {
		return false
	}
	category := map[work.StatusCategory]int{work.CategoryBacklog: 0, work.CategoryTodo: 1, work.CategoryInProgress: 2, work.CategoryDone: 3}
	place := func(s work.WorkflowStatus) int {
		if at, ok := rank[s.Key]; ok {
			return at * 10
		}
		return category[s.Category]*10 + 5
	}
	sort.SliceStable(statuses, func(i, j int) bool { return place(statuses[i]) < place(statuses[j]) })
	return true
}

// stepsOf answers the steps the tracker's workflow allows between the statuses, from the workflow
// whose statuses the project's issues share the most: a directed transition from the statuses it
// names, a global one from any. A transition to a status not carried is counted. No workflow
// answer keeps every status reachable from any other.
func (r *trackerRun) stepsOf(statuses []work.WorkflowStatus, idKeys map[string]string) ([]work.WorkflowStep, string, bool) {
	carried := map[string]bool{}
	for _, status := range statuses {
		carried[status.Key] = true
	}
	best, score := -1, 0
	for i, workflow := range r.answer.workflows {
		common := 0
		for _, status := range workflow.Statuses {
			if _, ok := idKeys[status.ID]; ok {
				common++
			}
		}
		if common > score {
			best, score = i, common
		}
	}
	if best < 0 {
		return nil, "", false
	}
	var steps []work.WorkflowStep
	initial := ""
	taken := map[string]bool{}
	for _, transition := range r.answer.workflows[best].Transitions {
		to, ok := idKeys[transition.To]
		if !ok || !carried[to] {
			r.unmapped["transitions"]++
			continue
		}
		if strings.EqualFold(transition.Type, "initial") {
			initial = to
			continue
		}
		step := work.WorkflowStep{Name: clip(strings.TrimSpace(transition.Name), 120), To: to}
		if step.Name == "" {
			step.Name = to
		}
		if !strings.EqualFold(transition.Type, "global") {
			for _, from := range transition.From {
				if key, ok := idKeys[from]; ok && carried[key] && key != to {
					step.From = append(step.From, key)
				}
			}
			if len(step.From) == 0 {
				r.unmapped["transitions"]++
				continue
			}
		}
		key := keyOf(step.Name)
		if key == "" || len(key) > 50 || taken[key] {
			key = clip(keyOf("step_"+transition.ID+"_"+strconv.Itoa(len(steps))), 50)
		}
		taken[key] = true
		step.Key = key
		steps = append(steps, step)
		if len(steps) == work.MaxWorkflowSteps {
			break
		}
	}
	if len(steps) == 0 {
		return nil, "", false
	}
	return steps, initial, true
}

// boardViews adds a board view of the collection for each of the project's boards, its columns
// the statuses.
func (r *trackerRun) boardViews(project string, collection shared.ID) {
	b := r.b
	for _, board := range r.boardsOf(project) {
		id := b.id("saved_views", "board:"+project+":"+board.ID.String())
		name := clip(strings.TrimSpace(board.Name), 120)
		if name == "" {
			continue
		}
		b.add("saved_views", id, map[string]any{
			"id": id.String(), "scope_type": "COLLECTION", "scope_id": collection.String(), "owner_id": b.source.Actor.String(),
			"name": name, "layout": "KANBAN",
			"query":    map[string]any{"scope_container_id": collection.String(), "include_descendants": true},
			"grouping": map[string]any{"field": "status"}, "visible_fields": []string{}, "sharing": "SCOPE",
			"created_at": stamp(b.source.Now), "version": 1,
		})
	}
}

// day answers the calendar day of a moment in the import's zone.
func (r *trackerRun) day(at time.Time) time.Time {
	local := at.In(r.zone)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

type sprintRow struct {
	key   string
	data  map[string]any
	start time.Time
	state string
}

// sprints adds the project's sprints as its issues name them and each issue's place in them: a
// closed sprint's issue DONE when it was done there, CARRIED when a later sprint took it and
// RETURNED otherwise; an open sprint holds it still. One sprint of the collection stays active,
// and into a collection that runs its own none is.
func (r *trackerRun) sprints(project string, indexes []int, collection shared.ID) {
	b := r.b
	var sprintID, pointsID string
	for id := range r.answer.Names {
		if r.sprintField(id) && sprintID == "" {
			sprintID = id
		}
		if r.pointsField(id) && pointsID == "" {
			pointsID = id
		}
	}
	for id := range r.answer.Schema {
		if r.sprintField(id) && sprintID == "" {
			sprintID = id
		}
	}
	points := map[shared.ID]float64{}
	if pointsID != "" {
		for _, index := range indexes {
			issue := r.answer.Issues[index]
			entry, ok := r.entries[issue.Key]
			value, read := readNumber(issue.Fields[pointsID]).(float64)
			if !ok || !read || value < 0 || value > 10000 {
				continue
			}
			points[entry] = value
			b.records["entry_estimates"] = append(b.records["entry_estimates"], archive.Record{
				ID: entry.String(), Op: archive.OpUpsert, UpdatedAt: b.source.Now,
				Data: map[string]any{
					"entry_id": entry.String(), "unit": "POINTS", "value": value, "size": nil,
					"updated_at": stamp(b.source.Now), "updated_by": b.source.Actor.String(), "version": 1,
				},
			})
		}
	}
	if sprintID == "" {
		return
	}
	rows := map[string]*sprintRow{}
	var order []*sprintRow
	type place struct {
		entry   shared.ID
		done    bool
		sprints []*sprintRow
	}
	var places []place
	for _, index := range indexes {
		issue := r.answer.Issues[index]
		entry, ok := r.entries[issue.Key]
		if !ok {
			continue
		}
		var named []trackerSprint
		if json.Unmarshal(issue.Fields[sprintID], &named) != nil || len(named) == 0 {
			continue
		}
		p := place{entry: entry, done: categoryOf(r.status(index).StatusCategory.Key) == work.CategoryDone}
		for _, sprint := range named {
			row := rows[sprint.ID.String()]
			if row == nil {
				row = r.sprintRow(project, sprint, collection)
				if row == nil {
					r.unmapped["sprints"]++
					continue
				}
				rows[sprint.ID.String()] = row
				order = append(order, row)
			}
			p.sprints = append(p.sprints, row)
		}
		sort.SliceStable(p.sprints, func(i, j int) bool { return p.sprints[i].start.Before(p.sprints[j].start) })
		places = append(places, p)
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].start.Before(order[j].start) })
	active := false
	for _, row := range order {
		if row.state == "ACTIVE" && (active || b.source.Into != nil) {
			row.state = "PLANNED"
		}
		if row.state == "ACTIVE" {
			active = true
		}
		row.data["state"] = row.state
		if row.state != "CLOSED" {
			row.data["closed_at"] = nil
		}
		if row.state == "PLANNED" {
			row.data["started_at"] = nil
		}
	}
	for _, p := range places {
		open := false
		for i := len(p.sprints) - 1; i >= 0; i-- {
			row := p.sprints[i]
			outcome := any(nil)
			if row.state == "CLOSED" {
				switch {
				case i < len(p.sprints)-1:
					outcome = "CARRIED"
				case p.done:
					outcome = "DONE"
				default:
					outcome = "RETURNED"
				}
			} else if open {
				// An issue is open in one sprint at a time: the latest holds it.
				continue
			} else {
				open = true
			}
			committed := row.state != "PLANNED"
			if committed {
				row.data["committed_count"] = row.data["committed_count"].(int) + 1
				row.data["committed_value"] = row.data["committed_value"].(float64) + points[p.entry]
			}
			if outcome == "DONE" {
				row.data["completed_count"] = row.data["completed_count"].(int) + 1
				row.data["completed_value"] = row.data["completed_value"].(float64) + points[p.entry]
			}
			b.records["sprint_entries"] = append(b.records["sprint_entries"], archive.Record{
				ID: row.data["id"].(string) + "/" + p.entry.String(), Op: archive.OpUpsert, UpdatedAt: b.source.Now,
				Data: map[string]any{
					"sprint_id": row.data["id"], "entry_id": p.entry.String(), "added_at": stamp(row.start),
					"removed_at": nil, "committed": committed, "outcome": outcome,
				},
			})
		}
	}
	for _, row := range order {
		b.records["sprints"] = append(b.records["sprints"], archive.Record{
			ID: row.data["id"].(string), Op: archive.OpUpsert, UpdatedAt: b.source.Now, Data: row.data,
		})
	}
}

// sprintRow reads one sprint: its days in the import's zone, a sprint without them starting
// today for two weeks, one longer than a sprint may be cut at the bound.
func (r *trackerRun) sprintRow(project string, sprint trackerSprint, collection shared.ID) *sprintRow {
	b := r.b
	name := clip(strings.TrimSpace(sprint.Name), 120)
	if name == "" || sprint.ID.String() == "" {
		return nil
	}
	state := map[string]string{"closed": "CLOSED", "active": "ACTIVE", "future": "PLANNED"}[strings.ToLower(sprint.State)]
	if state == "" {
		state = "PLANNED"
	}
	start := r.day(b.source.Now)
	startAt, started := r.instant(jsonString(sprint.StartDate))
	if started {
		start = r.day(startAt)
	}
	end := start.AddDate(0, 0, 13)
	if endAt, ok := r.instant(jsonString(sprint.EndDate)); ok && !r.day(endAt).Before(start) {
		end = r.day(endAt)
	}
	if end.Sub(start) >= 92*24*time.Hour {
		end = start.AddDate(0, 0, 91)
	}
	id := b.id("sprints", "sprint:"+project+":"+sprint.ID.String())
	data := map[string]any{
		"id": id.String(), "collection_id": collection.String(), "name": name, "goal": clip(strings.TrimSpace(sprint.Goal), 500),
		"start_on": start.Format("2006-01-02"), "end_on": end.Format("2006-01-02"), "state": state, "unit": "POINTS",
		"committed_value": 0.0, "committed_count": 0, "completed_value": 0.0, "completed_count": 0,
		"started_at": nil, "closed_at": nil,
		"created_at": stamp(b.source.Now), "created_by": b.source.Actor.String(), "updated_at": stamp(b.source.Now), "version": 1,
	}
	if started {
		data["started_at"] = stamp(startAt)
	} else if state != "PLANNED" {
		data["started_at"] = stamp(start)
	}
	if done, ok := r.instant(jsonString(sprint.CompleteDate)); ok {
		data["closed_at"] = stamp(done)
	} else if state == "CLOSED" {
		data["closed_at"] = stamp(end)
	}
	return &sprintRow{key: sprint.ID.String(), data: data, start: start, state: state}
}
