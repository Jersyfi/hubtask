// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
)

// The entry's type as a query field against the database (UC-FLW-01): an entry without a type of
// its own is its level's system type, the rows carry the type they hold, and a grouping by type -
// and by assignee - cuts the board where the values change.
func TestTheQueryFiltersAndGroupsByTheEntrysType(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	typed := seedTask(ctx, t, tenantA, authorA, collection)
	plain := seedTask(ctx, t, tenantA, authorA, collection)

	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`INSERT INTO entry_type (id, tenant_id, key, level, name, icon, color_token, created_at, updated_at) ` +
			`VALUES ('01936f2a-7c1e-7000-8000-00000000fe31', '` + tenantA.String() + `', 'qbug', 'TASK', ` +
			`'Bug', 'bug', 'red', now(), now()) ON CONFLICT (tenant_id, key) DO NOTHING`,
		`UPDATE work_item SET entry_type = 'qbug', assignee_id = '` + authorA.String() + `' ` +
			`WHERE id = '` + typed.String() + `'`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	bugs := typedQuery(ctx, t, tenantA, collection,
		map[string]any{"field": "entry_type", "op": "EQ", "value": "qbug"}, nil, nil)
	if len(bugs.Items) != 1 || bugs.Items[0].ID != typed || bugs.Items[0].EntryType != "qbug" {
		t.Fatalf("the filter by type answered %v", entryIDs(bugs.Items))
	}
	if bugs.Items[0].Number == 0 {
		t.Error("a queried entry carries no number")
	}

	tasks := typedQuery(ctx, t, tenantA, collection,
		map[string]any{"field": "entry_type", "op": "EQ", "value": "task"}, nil, nil)
	if len(tasks.Items) != 1 || tasks.Items[0].ID != plain {
		t.Fatalf("the level's own type answered %v", entryIDs(tasks.Items))
	}

	byType := typedQuery(ctx, t, tenantA, collection, nil, nil, map[string]any{"field": "entry_type"})
	if len(byType.Groups) != 2 || byType.Groups[0].Key != "qbug" || byType.Groups[1].Key != "task" {
		t.Fatalf("the groups by type are %+v", byType.Groups)
	}
	if byType.Groups[0].Total != 1 || byType.Groups[1].Total != 1 {
		t.Errorf("the groups by type count %d and %d", byType.Groups[0].Total, byType.Groups[1].Total)
	}

	byAssignee := typedQuery(ctx, t, tenantA, collection, nil, nil, map[string]any{"field": "assignee_id"})
	if len(byAssignee.Groups) != 2 || byAssignee.Groups[0].Key != authorA.String() ||
		!byAssignee.Groups[1].Absent {
		t.Fatalf("the groups by assignee are %+v", byAssignee.Groups)
	}
}
