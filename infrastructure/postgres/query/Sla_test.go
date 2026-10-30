// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// A queue sorted by the next breach (UC-SVC-03 check 1): sla_due orders by the earliest due moment
// of the goals that still run, nulls last, and the cursor continues it by the same expression.
func TestSlaDueSortsByTheNextRunningGoal(t *testing.T) {
	sort, err := view.ParseSort([]any{map[string]any{"field": "sla_due", "nulls": "LAST"}}, "/sort")
	if err != nil {
		t.Fatalf("the grammar refused the sort: %v", err)
	}
	first := compile(t, nil, view.Spec{Sort: sort})
	want := ` ORDER BY ` + slaDue + ` ASC NULLS LAST, wi.id LIMIT `
	if !strings.Contains(first.SQL, want) {
		t.Fatalf("compiled to\n  %s\nand should contain\n  %s", first.SQL, want)
	}

	moment := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	next, err := Rows(searchOf(t, nil, view.Spec{Sort: sort}),
		Boundary{Keys: []string{DerivedKey(&moment)}, ID: task}, 51)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	if !strings.Contains(next.SQL, slaDue+` > $`) {
		t.Errorf("the keyset does not compare the derived value:\n  %s", next.SQL)
	}
}

// A grouped queue carries the derived value under its alias, so the outer order reads it.
func TestSlaDueInAGroupedQueue(t *testing.T) {
	sort, _ := view.ParseSort([]any{map[string]any{"field": "sla_due"}}, "/sort")
	group, err := view.ParseGroupBy(map[string]any{"field": "assignee_id"}, "/group_by")
	if err != nil {
		t.Fatalf("the grammar refused the grouping: %v", err)
	}
	statement, err := Groups(searchOf(t, nil, view.Spec{Sort: sort, GroupBy: group}), 51)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	for _, want := range []string{`, ` + slaDue + ` AS sk0`, `ASC NULLS LAST, sk0 ASC NULLS LAST, id`} {
		if !strings.Contains(statement.SQL, want) {
			t.Errorf("compiled to\n  %s\nand should contain\n  %s", statement.SQL, want)
		}
	}
}

// sla_standing filters by the worst standing; the value stays a parameter (ADR-0026).
func TestSlaStandingIsTheWorstGoal(t *testing.T) {
	got := compile(t, map[string]any{"field": "sla_standing", "op": "EQ", "value": "AT_RISK"}, view.Spec{})
	if !strings.Contains(got.SQL, slaStanding+` = $`) {
		t.Fatalf("compiled to\n  %s", got.SQL)
	}
	if strings.Count(got.SQL, "AT_RISK") != 1 {
		t.Errorf("the value reached the text:\n  %s", got.SQL)
	}
	if _, err := view.ParseFilter(map[string]any{"field": "sla_standing", "op": "EQ", "value": "LATE"}, "/filter"); err == nil {
		t.Error("an unknown standing was accepted")
	}
}
