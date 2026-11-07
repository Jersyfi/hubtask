// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// The status is a query field (UC-FLW-03 check 1): a view filters and groups by it, and a board of
// a workflow collection is one group per status.
func TestTheStatusFiltersAndGroups(t *testing.T) {
	statement := compile(t, map[string]any{"field": "status", "op": "EQ", "value": "in_review"}, view.Spec{})
	if !strings.Contains(statement.SQL, `wi.status = normalize($2::text, NFC)`) {
		t.Errorf("the statement does not compare the status:\n%s", statement.SQL)
	}
	absent := compile(t, map[string]any{"field": "status", "op": "IS_NULL"}, view.Spec{})
	if !strings.Contains(absent.SQL, `wi.status IS NULL`) {
		t.Errorf("an entry where no workflow runs is not asked for:\n%s", absent.SQL)
	}

	grouping, err := view.ParseGroupBy(map[string]any{"field": "status"}, "/group_by")
	if err != nil {
		t.Fatalf("the grouping was refused: %v", err)
	}
	groups, err := Groups(searchOf(t, nil, view.Spec{GroupBy: grouping}), 51)
	if err != nil {
		t.Fatalf("grouping failed: %v", err)
	}
	if !strings.Contains(groups.SQL, `PARTITION BY wi.status`) {
		t.Errorf("the groups are not cut by the status:\n%s", groups.SQL)
	}
}
