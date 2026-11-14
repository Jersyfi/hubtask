// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	configrepo "github.com/Jersyfi/hubtask/core/application/repository/config"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const quarterPeriod = "0190a000-0000-7000-8000-0000000000e1"

// workspaceTemplateWorld is the catalogue the workspace's goal sets and kinds of work reach: Goals
// and Time switched on, the kinds of work by key, the periods a goal set opened with their goals.
type workspaceTemplateWorld struct {
	catalogue
	kinds   map[string]map[string]any
	periods []usecase.Input
	goals   []usecase.Input
	marks   map[string]configrepo.GoalSet
	// open are the periods ListGoalPeriods answers.
	open []usecase.Output
	// desk answers AddGoalSet, as the registry routes the apply's operation to it.
	desk *Desk
}

func newWorkspaceTemplateWorld(f fixture) *workspaceTemplateWorld {
	return &workspaceTemplateWorld{catalogue: catalogue{f.world}, kinds: map[string]map[string]any{}, marks: map[string]configrepo.GoalSet{}}
}

func (w *workspaceTemplateWorld) Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	switch name {
	case "ListModules":
		return usecase.Output{"modules": []any{
			map[string]any{"module": "WORKFLOWS", "on": true, "source": "WORKSPACE"},
			map[string]any{"module": "TIME", "on": true, "source": "WORKSPACE"},
			map[string]any{"module": "GOALS", "on": true, "source": "WORKSPACE"},
		}}, nil
	case "ListWorkTypes":
		var rows []any
		for _, key := range slices.Sorted(maps.Keys(w.kinds)) {
			rows = append(rows, w.kinds[key])
		}
		return usecase.Output{"data": rows}, nil
	case "DefineWorkType":
		row := map[string]any{"billable": false, "position": 0}
		maps.Copy(row, in)
		w.kinds[in.String("key")] = row
		return usecase.Output(row), nil
	case ListGoalSetsName:
		return listedGoalSets(w.marks), nil
	case AddGoalSetName:
		return AddGoalSet{*w.desk}.Descriptor().Handler.Invoke(ctx, actor, in)
	case "ListGoalPeriods":
		var rows []any
		for _, p := range w.open {
			rows = append(rows, map[string]any(p))
		}
		return usecase.Output{"data": rows}, nil
	case "DefineGoalPeriod":
		w.periods = append(w.periods, in)
		period := usecase.Output{"id": quarterPeriod, "name": in["name"], "start_on": in["start_on"], "end_on": in["end_on"], "state": "OPEN"}
		w.open = append(w.open, period)
		return period, nil
	case "CreateGoal":
		w.goals = append(w.goals, in)
		return usecase.Output{"id": "0190a000-0000-7000-8000-0000000000e2"}, nil
	}
	return w.catalogue.Invoke(ctx, name, actor, in)
}

func (w *workspaceTemplateWorld) Mark(_ context.Context, period shared.ID, key string, _ time.Time) (bool, error) {
	if _, held := w.marks[key]; held {
		return false, nil
	}
	for _, p := range w.open {
		if p["id"] == period.String() {
			start, _ := time.Parse(time.DateOnly, text(p["start_on"]))
			end, _ := time.Parse(time.DateOnly, text(p["end_on"]))
			w.marks[key] = configrepo.GoalSet{Key: key, PeriodID: period, Name: text(p["name"]), StartOn: start, EndOn: end}
		}
	}
	return true, nil
}

func (w *workspaceTemplateWorld) List(context.Context) ([]configrepo.GoalSet, error) {
	return slices.Collect(maps.Values(w.marks)), nil
}

// listed is what ListGoalSets answers for the marks, as its handler writes the rows.
func listedGoalSets(marks map[string]configrepo.GoalSet) usecase.Output {
	rows := []any{}
	for _, key := range slices.Sorted(maps.Keys(marks)) {
		s := marks[key]
		rows = append(rows, goalSetRow(s.Key, s.PeriodID.String(), s.Name, s.StartOn.Format(time.DateOnly), s.EndOn.Format(time.DateOnly)))
	}
	return usecase.Output{"data": rows}
}

// configuration.md §6: a goal set is a period opened with the workspace's goals and their key
// results in one write, marked under its key; a key taken is refused and nothing is marked twice.
func TestAGoalSetIsAPeriodWithItsGoalsMarkedByItsKey(t *testing.T) {
	f := newFixture()
	w := newWorkspaceTemplateWorld(f)
	f.desk.Catalogue, f.desk.GoalSets = w, w
	add := AddGoalSet{f.desk}.Descriptor()
	goals := []any{map[string]any{"title": "Raise customer satisfaction", "key_results": []any{
		map[string]any{"title": "Satisfaction score", "kind": "PERCENT", "start": 70, "target": 85},
	}}}
	out := invoke(t, add, person, usecase.Input{
		"key": "customer_satisfaction", "name": "Q4 2026", "start_on": "2026-10-01", "end_on": "2026-12-31", "goals": goals,
	})
	if out["period_id"] != quarterPeriod || out["key"] != "customer_satisfaction" || out["name"] != "Q4 2026" || out["start_on"] != "2026-10-01" {
		t.Fatalf("added %v", out)
	}
	if len(w.goals) != 1 || w.goals[0]["period_id"] != quarterPeriod || w.goals[0]["title"] != "Raise customer satisfaction" || w.goals[0]["key_results"] == nil {
		t.Errorf("the goals written %v", w.goals)
	}
	if !slices.Contains(f.trail.actions, GoalSetAddedAction) {
		t.Errorf("no trail entry: %v", f.trail.actions)
	}
	// A second set of the same days takes its goals into the same period, never a twin of it.
	health := invoke(t, add, person, usecase.Input{"key": "team_health", "name": "Q4 2026", "start_on": "2026-10-01", "end_on": "2026-12-31",
		"goals": []any{map[string]any{"title": "Sustainable pace"}}})
	if health["period_id"] != quarterPeriod || len(w.periods) != 1 || len(w.goals) != 2 {
		t.Errorf("a second set of the quarter: %v, periods %v", health, w.periods)
	}
	_, err := add.Handler.Invoke(t.Context(), person, usecase.Input{"key": "customer_satisfaction", "name": "Again", "start_on": "2027-01-01", "end_on": "2027-03-31"})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.goal_set_taken" {
		t.Errorf("a key taken twice: %v", err)
	}
	_, err = add.Handler.Invoke(t.Context(), person, usecase.Input{"key": "Bad Key", "name": "X", "start_on": "2027-01-01", "end_on": "2027-03-31"})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.key_invalid" {
		t.Errorf("a key that is no key: %v", err)
	}
	listed := invoke(t, ListGoalSets{f.desk}.Descriptor(), person, nil)
	if rows := listed["data"].([]any); len(rows) != 2 || rows[0].(map[string]any)["end_on"] != "2026-12-31" {
		t.Errorf("listed %v", listed)
	}
}

// refusing answers every permission question with no, as a member without STRUCTURE is answered.
type refusing struct{}

func (refusing) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return shared.ErrForbidden.WithDetail("access.not_permitted")
}

// configuration.md §6, rule 2: a goal set is refused to whoever may not change the workspace's
// structure, before any period is opened.
func TestAGoalSetIsRefusedWithoutTheRightToAdministerGoals(t *testing.T) {
	f := newFixture()
	w := newWorkspaceTemplateWorld(f)
	f.desk.Catalogue, f.desk.GoalSets, f.desk.Authorizer = w, w, refusing{}
	_, err := AddGoalSet{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"key": "team_health", "name": "Q4 2026", "start_on": "2026-10-01", "end_on": "2026-12-31",
	})
	if !errors.Is(err, shared.ErrForbidden) || len(w.periods) != 0 || len(w.marks) != 0 {
		t.Errorf("refused %v, opened %v, marked %v", err, w.periods, w.marks)
	}
}

// configuration.md §6: the document reads a goal set back by its period's name and days alone;
// every attribute is fixed and a block is never removed - a period is closed, never deleted.
func TestAWorkspacesDocumentCarriesItsGoalSets(t *testing.T) {
	s := goalSetsSection()
	key, block := s.read(map[string]any{"key": "team_health", "period_id": quarterPeriod, "name": "Q4 2026", "start_on": "2026-10-01", "end_on": "2026-12-31"})
	if key != "team_health" || block["name"] != "Q4 2026" || block["end_on"] != "2026-12-31" || block["goals"] != nil {
		t.Fatalf("read %q %v", key, block)
	}
	goals := []any{map[string]any{"title": "Sustainable pace"}}
	ops, err := s.ops(domain.BlockChange{Section: sectionGoalSets, Key: "team_health", Op: domain.OpAdd,
		After: domain.Block{"name": "Q4 2026", "start_on": "2026-10-01", "end_on": "2026-12-31", "goals": goals}}, nil)
	if err != nil || len(ops) != 1 || ops[0].UseCase != AddGoalSetName || ops[0].Input["key"] != "team_health" || ops[0].Input["goals"] == nil {
		t.Fatalf("add %+v, %v", ops, err)
	}
	if _, err := s.ops(domain.BlockChange{Section: sectionGoalSets, Key: "team_health", Op: domain.OpChange, Fields: []string{"name"}}, nil); refusalsIn(err) == nil || refusalsIn(err)[0].Code != "config.attribute_fixed" {
		t.Errorf("a change rewrote a period: %v", err)
	}
	if _, err := s.ops(domain.BlockChange{Section: sectionGoalSets, Key: "team_health", Op: domain.OpRemove}, nil); refusalsIn(err) == nil || refusalsIn(err)[0].Code != "config.not_removable" {
		t.Errorf("a removal deleted a period: %v", err)
	}
}

// configuration.md §6: a template's goal set names its span; planned, it is the quarter, half-year
// or year around today with its name filled in.
func TestAGoalSetsSpanIsThePeriodAroundToday(t *testing.T) {
	today := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct{ span, name, start, end, said string }{
		{"QUARTER", "Q{{quarter}} {{year}}", "2026-10-01", "2026-12-31", "Q4 2026"},
		{"HALF_YEAR", "H{{half}} {{year}}", "2026-07-01", "2026-12-31", "H2 2026"},
		{"YEAR", "{{year}}", "2026-01-01", "2026-12-31", "2026"},
	} {
		resolved := resolvedGoalSets(domain.Document{sectionGoalSets: {"set": {"span": c.span, "name": c.name}}}, today)
		block := resolved[sectionGoalSets]["set"]
		if block["start_on"] != c.start || block["end_on"] != c.end || block["name"] != c.said || block["span"] != nil {
			t.Errorf("%s: %v", c.span, block)
		}
	}
	feb := resolvedGoalSets(domain.Document{sectionGoalSets: {"set": {"span": "QUARTER", "name": "Q{{quarter}}"}}}, time.Date(2027, 2, 28, 0, 0, 0, 0, time.UTC))
	if block := feb[sectionGoalSets]["set"]; block["start_on"] != "2027-01-01" || block["end_on"] != "2027-03-31" || block["name"] != "Q1" {
		t.Errorf("the first quarter: %v", block)
	}
}

// configuration.md §6: the document reads a kind of work by key with its name, order and whether
// its time starts billable; an archived kind is none, and removing a block archives its kind.
func TestAWorkspacesDocumentCarriesItsKindsOfWork(t *testing.T) {
	s := timeWorkTypesSection()
	key, block := s.read(map[string]any{"key": "support", "name": "Support", "billable": false, "archived": false, "position": 3, "version": 1})
	if key != "support" || block["name"] != "Support" || block["position"] != 3 || block["billable"] != nil {
		t.Fatalf("read %q %v", key, block)
	}
	if key, _ := s.read(map[string]any{"key": "old", "name": "Old", "archived": true}); key != "" {
		t.Errorf("an archived kind is a block: %q", key)
	}
	ops, err := s.ops(domain.BlockChange{Section: sectionTimeWorkTypes, Key: "consulting", Op: domain.OpAdd,
		After: domain.Block{"name": "Consulting", "billable": true, "position": 1}}, nil)
	if err != nil || len(ops) != 1 || ops[0].UseCase != "DefineWorkType" || ops[0].Input["billable"] != true || ops[0].Input["archived"] != false {
		t.Fatalf("add %+v, %v", ops, err)
	}
	ops, _ = s.ops(domain.BlockChange{Section: sectionTimeWorkTypes, Key: "consulting", Op: domain.OpRemove, Before: domain.Block{"name": "Consulting"}}, nil)
	if len(ops) != 1 || ops[0].Input["archived"] != true || ops[0].Input["name"] != "Consulting" {
		t.Errorf("remove %+v", ops)
	}
}

// configuration.md §6: a workspace template of kinds of work and a goal set is taken by the
// workspace, not by making a hub; taking it again makes nothing new.
func TestAWorkspaceTemplateIsTakenOnceByKey(t *testing.T) {
	f := newFixture()
	w := newWorkspaceTemplateWorld(f)
	f.desk.Catalogue, f.desk.GoalSets = w, w
	w.desk = &f.desk
	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "office", "name": "Office",
		"document": map[string]any{
			"time_work_types": map[string]any{
				"meeting": map[string]any{"name": "Meeting", "position": 1},
				"support": map[string]any{"name": "Support", "billable": true, "position": 2},
			},
			"goal_sets": map[string]any{
				"team_health": map[string]any{"span": "QUARTER", "name": "Q{{quarter}} {{year}}", "goals": []any{map[string]any{"title": "Sustainable pace"}}},
			},
		},
	})
	take := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "office"})
	if refusals := take["refusals"].([]usecase.Output); len(refusals) > 0 {
		t.Fatalf("refused %v", refusals)
	}
	set := take["document"].(map[string]any)["goal_sets"].(map[string]any)["team_health"].(map[string]any)
	if set["name"] != "Q4 2026" || set["start_on"] != "2026-10-01" || set["end_on"] != "2026-12-31" {
		t.Errorf("the goal set planned %v", set)
	}
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": take["document"], "plan_hash": take["plan_hash"], "template": take["template"],
	})
	if len(w.kinds) != 2 || w.kinds["support"]["billable"] != true || len(w.periods) != 1 || len(w.goals) != 1 {
		t.Fatalf("applied kinds %v, periods %v, goals %v", w.kinds, w.periods, w.goals)
	}
	again := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "office"})
	if changes := again["changes"].([]usecase.Output); len(changes) != 0 {
		t.Errorf("taking the template again changes %v", changes)
	}
}
