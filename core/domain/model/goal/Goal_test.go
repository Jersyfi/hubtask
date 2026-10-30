// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	goalA   = shared.MustParseID("0192f000-0000-7000-8000-000000000b01")
	goalB   = shared.MustParseID("0192f000-0000-7000-8000-000000000b02")
	resultA = shared.MustParseID("0192f000-0000-7000-8000-000000000b03")
	resultB = shared.MustParseID("0192f000-0000-7000-8000-000000000b04")
	period  = shared.MustParseID("0192f000-0000-7000-8000-000000000b05")
	entry   = shared.MustParseID("0192f000-0000-7000-8000-000000000b06")
	hub     = shared.MustParseID("0192f000-0000-7000-8000-000000000b07")
)

func code(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

func TestAKeyResultIsCheckedByItsKind(t *testing.T) {
	cases := []struct {
		name string
		in   KeyResult
		want string
	}{
		{"a number", KeyResult{ID: resultA, Title: "Revenue", Kind: ResultNumber, Start: 10, Target: 20}, ""},
		{"a number falling", KeyResult{ID: resultA, Title: "Churn", Kind: ResultNumber, Start: 8, Target: 2}, ""},
		{"no title", KeyResult{ID: resultA, Kind: ResultNumber, Target: 1}, "key_results.title_required"},
		{"start is target", KeyResult{ID: resultA, Title: "x", Kind: ResultNumber, Start: 1, Target: 1}, "key_results.target_is_start"},
		{"percent above 100", KeyResult{ID: resultA, Title: "x", Kind: ResultPercent, Target: 120}, "key_results.percent_out_of_range"},
		{"binary not 0 or 1", KeyResult{ID: resultA, Title: "x", Kind: ResultBinary, Current: 0.5}, "key_results.binary_value"},
		{"milestone without entry", KeyResult{ID: resultA, Title: "x", Kind: ResultMilestone}, "key_results.milestone_entry_required"},
		{"milestone", KeyResult{ID: resultA, Title: "x", Kind: ResultMilestone, EntryID: entry}, ""},
		{"view without view", KeyResult{ID: resultA, Title: "x", Kind: ResultNumber, Target: 5, Source: SourceView}, "key_results.view_required"},
		{"binary from a view", KeyResult{ID: resultA, Title: "x", Kind: ResultBinary, Source: SourceView, ViewID: entry}, "key_results.source_not_measurable"},
		{"not a number", KeyResult{ID: resultA, Title: "x", Kind: ResultNumber, Target: math.NaN()}, "key_results.value_invalid"},
		{"unknown kind", KeyResult{ID: resultA, Title: "x", Kind: "SCORE"}, "key_results.kind_invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := CheckKeyResult(c.in, "/key_results/0", nil)
			if got := code(err); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestAKeyResultsProgressRunsFromStartToTarget(t *testing.T) {
	rising := KeyResult{Kind: ResultNumber, Start: 10, Target: 20}
	falling := KeyResult{Kind: ResultNumber, Start: 8, Target: 2}
	cases := []struct {
		name    string
		r       KeyResult
		current float64
		want    float64
	}{
		{"half way up", rising, 15, 0.5},
		{"past the target", rising, 30, 1},
		{"behind the start", rising, 5, 0},
		{"half way down", falling, 5, 0.5},
		{"reached down", falling, 1, 1},
	}
	for _, c := range cases {
		if got := c.r.Progress(c.current); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAGoalsProgressIsTheMeanOfItsParts(t *testing.T) {
	if _, ok := Progress(nil); ok {
		t.Fatal("a goal with no part has a progress")
	}
	entries, ok := EntriesPart(3, 4)
	if !ok || entries.Progress != 0.75 {
		t.Fatalf("entries part: %v %v", entries, ok)
	}
	if _, ok := EntriesPart(0, 0); ok {
		t.Fatal("no linked entries is a part")
	}
	got, _ := Progress([]Part{{Kind: PartKeyResult, Progress: 0.5}, {Kind: PartChild, Progress: 1}, entries})
	if math.Abs(got-0.75) > 1e-9 {
		t.Fatalf("got %v, want 0.75", got)
	}
}

func TestAGoalNeverSitsUnderItself(t *testing.T) {
	if err := Parenting(goalA, []shared.ID{goalB}); err != nil {
		t.Fatalf("a parent refused: %v", err)
	}
	if code(Parenting(goalA, []shared.ID{goalB, goalA})) != "goals.parent_cycle" {
		t.Fatal("a cycle was accepted")
	}
	deep := make([]shared.ID, MaxDepth)
	for i := range deep {
		deep[i] = goalB
	}
	if code(Parenting(goalA, deep)) != "goals.too_deep" {
		t.Fatal("a tree past its depth was accepted")
	}
}

func TestACheckInHoldsToItsGoal(t *testing.T) {
	g := Goal{ID: goalA, KeyResults: []KeyResult{
		{ID: resultA, Title: "a", Kind: ResultNumber, Start: 0, Target: 10, Source: SourceManual},
		{ID: resultB, Title: "b", Kind: ResultNumber, Start: 0, Target: 10, Source: SourceView, ViewID: entry},
	}}
	cases := []struct {
		name string
		in   CheckIn
		want string
	}{
		{"a value", CheckIn{Status: StatusOnTrack, Confidence: 7, Values: map[shared.ID]float64{resultA: 4}}, ""},
		{"no judgement", CheckIn{Status: StatusNone, Confidence: 7}, "check_ins.status_invalid"},
		{"confidence too high", CheckIn{Status: StatusAtRisk, Confidence: 11}, "check_ins.confidence_out_of_range"},
		{"another goal's result", CheckIn{Status: StatusAtRisk, Values: map[shared.ID]float64{goalB: 1}}, "check_ins.key_result_unknown"},
		{"a view's result", CheckIn{Status: StatusAtRisk, Values: map[shared.ID]float64{resultB: 1}}, "check_ins.value_not_manual"},
	}
	for _, c := range cases {
		if got := code(func() error { _, err := CheckCheckIn(c.in, g, nil); return err }()); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	g.Apply(CheckIn{Status: StatusOnTrack, Confidence: 7, Values: map[shared.ID]float64{resultA: 4}})
	if g.KeyResults[0].Current != 4 || g.Status != StatusOnTrack || g.Confidence != 7 {
		t.Fatalf("the check-in did not apply: %+v", g)
	}
	grade := 0.5
	g.Grade = &grade
	if _, err := CheckCheckIn(CheckIn{Status: StatusOnTrack}, g, nil); code(err) != "goals.graded" {
		t.Fatalf("a graded goal took a check-in: %v", err)
	}
}

func TestAGoalNeedsAPeriodAndKeepsItsResultsApart(t *testing.T) {
	ok := GoalFields{PeriodID: period, Title: "Grow", KeyResults: []KeyResult{
		{ID: resultA, Title: "a", Kind: ResultBinary},
	}}
	if _, err := CheckGoal(ok, nil); err != nil {
		t.Fatalf("a goal refused: %v", err)
	}
	noPeriod := ok
	noPeriod.PeriodID = ""
	if code(func() error { _, err := CheckGoal(noPeriod, nil); return err }()) != "goals.period_required" {
		t.Fatal("a goal without a period was accepted")
	}
	twice := GoalFields{PeriodID: period, Title: "Grow", KeyResults: []KeyResult{
		{ID: resultA, Title: "a", Kind: ResultBinary}, {ID: resultA, Title: "b", Kind: ResultBinary},
	}}
	if code(func() error { _, err := CheckGoal(twice, nil); return err }()) != "key_results.identity_duplicate" {
		t.Fatal("a key result twice was accepted")
	}
}

func TestAPeriodRunsForward(t *testing.T) {
	if _, err := CheckPeriod(Period{Name: "Q4", StartOn: 10, EndOn: 100}, nil); err != nil {
		t.Fatalf("a period refused: %v", err)
	}
	if code(func() error { _, err := CheckPeriod(Period{Name: "Q4", StartOn: 10, EndOn: 9}, nil); return err }()) != "goal_periods.range_reversed" {
		t.Fatal("a reversed period was accepted")
	}
	if code(func() error {
		_, err := CheckPeriod(Period{Name: "x", StartOn: 0, EndOn: MaxPeriodDays}, nil)
		return err
	}()) != "goal_periods.range_too_long" {
		t.Fatal("a period past two years was accepted")
	}
	if CheckGrade(1.2) == nil || CheckGrade(0.7) != nil {
		t.Fatal("grades are not 0 to 1")
	}
}

func TestAPortfolioHoldsEachProjectOnce(t *testing.T) {
	start, end := Day(20), Day(10)
	cases := []struct {
		name string
		in   Portfolio
		want string
	}{
		{"a project", Portfolio{Name: "Programme", Items: []PortfolioItem{{ContainerID: hub}}}, ""},
		{"twice", Portfolio{Name: "P", Items: []PortfolioItem{{ContainerID: hub}, {ContainerID: hub}}}, "portfolios.item_duplicate"},
		{"reversed", Portfolio{Name: "P", Items: []PortfolioItem{{ContainerID: hub, StartOn: &start, EndOn: &end}}}, "portfolios.range_reversed"},
		{"done is no health", Portfolio{Name: "P", Status: StatusDone}, "portfolios.status_invalid"},
	}
	for _, c := range cases {
		if got := code(func() error { _, err := CheckPortfolio(c.in, nil); return err }()); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if code(PortfolioNesting(goalA, []shared.ID{goalA})) != "portfolios.parent_cycle" {
		t.Fatal("a portfolio under itself was accepted")
	}
	if _, err := CheckStatusUpdate(StatusUpdate{Status: StatusAtRisk}, nil); code(err) != "portfolio_updates.summary_required" {
		t.Fatalf("an empty update was accepted: %v", err)
	}
}

func TestADashboardsWidgetsFitTheGrid(t *testing.T) {
	widget := func(w Widget) Dashboard {
		if w.ID.IsZero() {
			w.ID = resultA
		}
		if w.Title == "" {
			w.Title = "Open"
		}
		return Dashboard{Name: "Team", Widgets: []Widget{w}}
	}
	cases := []struct {
		name string
		in   Dashboard
		want string
	}{
		{"a view", widget(Widget{Chart: ChartBar, Source: WidgetView, ViewID: entry}), ""},
		{"a query", widget(Widget{Chart: ChartLine, Source: WidgetQuery, Query: map[string]any{"aggregate": map[string]any{}}}), ""},
		{"goals", widget(Widget{Chart: ChartBar, Source: WidgetGoals, GoalIDs: []shared.ID{goalA}}), ""},
		{"a view without one", widget(Widget{Chart: ChartBar, Source: WidgetView}), "dashboards.view_required"},
		{"a query without one", widget(Widget{Chart: ChartBar, Source: WidgetQuery}), "dashboards.query_required"},
		{"progress of a query", widget(Widget{Chart: ChartProgress, Source: WidgetQuery, Query: map[string]any{"a": 1}}), "dashboards.chart_invalid"},
		{"past the grid", widget(Widget{Chart: ChartBar, Source: WidgetView, ViewID: entry, X: 8, W: 6}), "dashboards.widget_outside_grid"},
		{"an unknown sharing", Dashboard{Name: "x", Sharing: "GROUP"}, "dashboards.sharing_invalid"},
		{"groups without one", Dashboard{Name: "x", Sharing: SharingGroups}, "dashboards.groups_required"},
		{"a group", Dashboard{Name: "x", Sharing: SharingGroups, GroupIDs: []shared.ID{goalA}}, ""},
	}
	for _, c := range cases {
		if got := code(func() error { _, err := CheckDashboard(c.in, nil); return err }()); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// UC-GOL-04 check 14: a dashboard shared with groups names each once; any other sharing names none.
func TestADashboardsGroupsAreItsSharingsAlone(t *testing.T) {
	d, err := CheckDashboard(Dashboard{Name: "x", Sharing: SharingGroups, GroupIDs: []shared.ID{goalA, goalA, ""}}, nil)
	if err != nil || len(d.GroupIDs) != 1 {
		t.Fatalf("groups = %v, %v", d.GroupIDs, err)
	}
	if d, _ = CheckDashboard(Dashboard{Name: "x", Sharing: SharingScope, GroupIDs: []shared.ID{goalA}}, nil); d.GroupIDs != nil {
		t.Fatalf("a scope dashboard kept groups: %v", d.GroupIDs)
	}
	many := make([]shared.ID, MaxDashboardGroups+1)
	for i := range many {
		many[i] = shared.ID(fmt.Sprintf("018f0000-0000-7000-8000-%012d", i+1))
	}
	if _, err := CheckDashboard(Dashboard{Name: "x", Sharing: SharingGroups, GroupIDs: many}, nil); code(err) != "dashboards.too_many_groups" {
		t.Fatalf("21 groups: %v", err)
	}
}
