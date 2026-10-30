// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// UC-SVC-07 check 4: a queue's satisfaction is the average of its entries' scores - the view's
// aggregation over `satisfaction` - and a queue keeps the unhappy ones by filtering it.
func TestSatisfactionIsAQueuesAverageAndAFilter(t *testing.T) {
	search := searchOf(t, map[string]any{"field": "is_completed", "op": "EQ", "value": true}, view.Spec{})
	statement, err := Aggregate(search, aggregationOf(t, map[string]any{"function": "AVG", "field": "satisfaction"}, nil), "Europe/Berlin", 5001)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	if want := `avg(` + satisfactionScore + `)::numeric`; !strings.Contains(statement.SQL, want) {
		t.Errorf("missing %q in\n%s", want, statement.SQL)
	}
	got := compile(t, map[string]any{"field": "satisfaction", "op": "LTE", "value": 2}, view.Spec{})
	if !strings.Contains(got.SQL, satisfactionScore+` <= $`) {
		t.Errorf("compiled to\n  %s", got.SQL)
	}
	if _, err := view.ParseFilter(map[string]any{"field": "satisfaction", "op": "CONTAINS", "value": "x"}, "/filter"); err == nil {
		t.Error("a text operator was accepted on a score")
	}
}
