// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The database stores no zero byte: words holding one are refused here, never left to fail the
// write as unavailable - an error a client retries for ever.
func TestWordsHoldingAZeroByteAreRefused(t *testing.T) {
	result := func(r KeyResult) []KeyResult {
		if r.ID.IsZero() {
			r.ID = goalA
		}
		if r.Kind == "" {
			r.Kind, r.Target = ResultNumber, 10
		}
		if r.Title == "" {
			r.Title = "Customers"
		}
		return []KeyResult{r}
	}
	goal := func(f GoalFields) error {
		if f.Title == "" {
			f.Title = "Grow"
		}
		f.PeriodID = goalB
		_, err := CheckGoal(f, nil)
		return err
	}
	for _, tc := range []struct {
		name, want string
		err        error
	}{
		{"a period's name", "goal_periods.name_invalid", func() error {
			_, err := CheckPeriod(Period{Name: "Q4\x00", StartOn: 1, EndOn: 2}, nil)
			return err
		}()},
		{"a goal's title", "goals.title_invalid", goal(GoalFields{Title: "Grow\x00"})},
		{"a goal's title with a tab", "goals.title_invalid", goal(GoalFields{Title: "Grow\tfast"})},
		{"a goal's description", "goals.description_malformed", goal(GoalFields{Description: "Why\x00"})},
		{"a key result's title", "key_results.title_invalid", goal(GoalFields{KeyResults: result(KeyResult{Title: "Cu\x00"})})},
		{"a key result's unit", "key_results.unit_invalid", goal(GoalFields{KeyResults: result(KeyResult{Unit: "€\x00"})})},
		{"a check-in's note", "check_ins.note_malformed", func() error {
			_, err := CheckCheckIn(CheckIn{Status: StatusOnTrack, Confidence: 5, Note: "fine\x00"}, Goal{}, nil)
			return err
		}()},
		{"a portfolio's name", "portfolios.name_invalid", func() error {
			_, err := CheckPortfolio(Portfolio{Name: "Web\x00"}, nil)
			return err
		}()},
		{"a portfolio's description", "portfolios.description_malformed", func() error {
			_, err := CheckPortfolio(Portfolio{Name: "Web", Description: "all\x00"}, nil)
			return err
		}()},
		{"a status update's summary", "portfolio_updates.summary_malformed", func() error {
			_, err := CheckStatusUpdate(StatusUpdate{Status: StatusAtRisk, Summary: "late\x00"}, nil)
			return err
		}()},
		{"a dashboard's name", "dashboards.name_invalid", func() error {
			_, err := CheckDashboard(Dashboard{Name: "Ops\x00"}, nil)
			return err
		}()},
		{"a widget's title", "dashboards.widget_title_invalid", func() error {
			_, err := CheckDashboard(Dashboard{Name: "Ops", Widgets: []Widget{{
				ID: shared.ID(goalA), Title: "Open\x00", Chart: ChartNumber, Source: WidgetView, ViewID: goalB,
			}}}, nil)
			return err
		}()},
	} {
		if code(tc.err) != tc.want {
			t.Errorf("%s: %v, want %s", tc.name, tc.err, tc.want)
		}
	}
	if err := goal(GoalFields{Description: "Line one\nline two\ttabbed"}); err != nil {
		t.Fatalf("a description with a line break and a tab is refused: %v", err)
	}
}
