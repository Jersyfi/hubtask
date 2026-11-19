// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// asked parses a filter, resolves its group words with the people given and answers its asker's
// keys as the use case would, and queries one collection.
func asked(
	ctx context.Context, t *testing.T, tenant, collection shared.ID, filter map[string]any,
	groups map[string][]shared.ID, answer view.Asked,
) []shared.ID {
	t.Helper()
	search := searchIn(t, collection, filter, view.Spec{})
	resolved, err := search.Spec.Filter.Resolve(view.Resolution{Now: time.Now(), ActorID: authorA, Groups: groups}, "/filter")
	if err != nil {
		t.Fatal(err)
	}
	answered := resolved.Answer(answer)
	search.Spec.Filter = &answered
	return idsOfItems(queried(ctx, t, tenant, search).Items)
}

// time.logged counts the minutes the use case found the asker may see (UC-TIM-04): their own, the
// others' only in a collection they approve time in, and where the workspace shows only sums only
// on an entry three people worked on. Another workspace's records never count.
func TestTimeLoggedCountsOnlyTheTimeTheAskerMaySee(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection, entries := seedPlan(ctx, t, tenantA)
	colleague := seedAccount(ctx, t, tenantA)
	admin := adminPool(ctx, t)
	record := func(tenant, account, entry shared.ID, minutes int) {
		t.Helper()
		if _, err := admin.Exec(ctx, `INSERT INTO time_record (id, tenant_id, account_id, entry_id, day, minutes, source, created_at, updated_at)
			VALUES ($1, $2, $3, $4, '2026-11-02', $5, 'MANUAL', now(), now())`,
			freshID(t).String(), tenant.String(), account.String(), entry.String(), minutes); err != nil {
			t.Fatalf("seeding a record: %v", err)
		}
	}
	record(tenantA, authorA, entries[0], 60)
	record(tenantA, colleague, entries[0], 30)
	record(tenantA, colleague, entries[1], 45)
	// Tenant B's record on tenant A's entry identifier is another workspace's and never counts.
	record(tenantB, authorB, entries[1], 600)

	member := view.Asked{Sight: view.TimeSight{Reader: authorA, Shown: []shared.ID{}}}
	lead := view.Asked{Sight: view.TimeSight{Reader: authorA, Shown: []shared.ID{collection}}}
	sumsOnly := view.Asked{Sight: view.TimeSight{Reader: authorA, Shown: []shared.ID{collection}, Withhold: true}}
	cases := []struct {
		name   string
		filter map[string]any
		answer view.Asked
		want   []shared.ID
	}{
		{"a member sees their own hour", map[string]any{"field": "time_logged", "op": "GTE", "value": 60}, member, []shared.ID{entries[0]}},
		{"a member does not see the colleague's", map[string]any{"field": "time_logged", "op": "GT", "value": 60}, member, nil},
		{"a member sees nothing on the colleague's entry", map[string]any{"field": "time_logged", "op": "GT", "value": 0}, member, []shared.ID{entries[0]}},
		{"a lead sees everybody's", map[string]any{"field": "time_logged", "op": "EQ", "value": 90}, lead, []shared.ID{entries[0]}},
		{"a lead sees the colleague's alone", map[string]any{"field": "time_logged", "op": "BETWEEN", "value": []any{1, 50}}, lead, []shared.ID{entries[1]}},
		{"sums only withhold two people", map[string]any{"field": "time_logged", "op": "GT", "value": 0}, sumsOnly, []shared.ID{entries[0]}},
		{"sums only leave the asker's own", map[string]any{"field": "time_logged", "op": "EQ", "value": 60}, sumsOnly, []shared.ID{entries[0]}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := asked(ctx, t, tenantA, collection, c.filter, nil, c.answer); !sameIDs(got, c.want) {
				t.Errorf("found %v, want %v", got, c.want)
			}
		})
	}
}

// my-groups and members("…") are the people of the asker's groups and of a named group, read in
// the asker's workspace: a group of the same name next door is not read.
func TestTheGroupWordsFindTheGroupsPeople(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection, entries := seedPlan(ctx, t, tenantA)
	colleague, stranger := seedAccount(ctx, t, tenantA), seedAccount(ctx, t, tenantA)
	admin := adminPool(ctx, t)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	name := freshName(t)
	group, groupB := freshID(t), freshID(t)
	exec(`INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, $3)`, group.String(), tenantA.String(), name)
	exec(`INSERT INTO account_group_member (tenant_id, group_id, account_id) VALUES ($1, $2, $3), ($1, $2, $4)`,
		tenantA.String(), group.String(), authorA.String(), colleague.String())
	exec(`INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, $3)`, groupB.String(), tenantB.String(), name+" B")
	exec(`INSERT INTO account_group_member (tenant_id, group_id, account_id) VALUES ($1, $2, $3)`,
		tenantB.String(), groupB.String(), authorB.String())
	exec(`UPDATE work_item SET assignee_id = $1 WHERE id = $2`, colleague.String(), entries[0].String())
	exec(`UPDATE work_item SET assignee_id = $1 WHERE id = $2`, stranger.String(), entries[1].String())

	repo := postgres.GroupPeopleRepository{}
	var mine, named, byID []shared.ID
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if mine, err = repo.PeopleOfGroupsOf(ctx, authorA, 100); err != nil {
			return err
		}
		if named, err = repo.PeopleOfGroup(ctx, name, 100); err != nil {
			return err
		}
		byID, err = repo.PeopleOfGroup(ctx, group.String(), 100)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !sameIDs(mine, []shared.ID{authorA, colleague}) || !sameIDs(named, mine) || !sameIDs(byID, mine) {
		t.Fatalf("mine %v, named %v, by identifier %v", mine, named, byID)
	}
	groups := map[string][]shared.ID{"@my_groups": mine, "@group:" + name: named}
	for _, filter := range []map[string]any{
		{"field": "assignee_id", "op": "EQ", "value": "@my_groups"},
		{"field": "assignee_id", "op": "EQ", "value": "@group:" + name},
	} {
		if got := asked(ctx, t, tenantA, collection, filter, groups, view.Asked{}); !sameIDs(got, []shared.ID{entries[0]}) {
			t.Errorf("%v found %v", filter, got)
		}
	}

	// Tenant B reads neither tenant A's group by its name or identifier nor its people.
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if people, err := repo.PeopleOfGroup(ctx, name, 100); err == nil {
			t.Errorf("tenant B read tenant A's group by name: %v", people)
		}
		if people, err := repo.PeopleOfGroup(ctx, group.String(), 100); err == nil {
			t.Errorf("tenant B read tenant A's group by identifier: %v", people)
		}
		people, err := repo.PeopleOfGroupsOf(ctx, authorA, 100)
		if err == nil && len(people) != 0 {
			t.Errorf("tenant B read tenant A's people: %v", people)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// plan.critical keeps the entries the scheduler put on the critical path, and -plan.critical the
// others; the statement binds the use case's answer and nothing else.
func TestPlanCriticalKeepsTheCriticalPath(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection, entries := seedPlan(ctx, t, tenantA)
	path := view.Asked{Critical: []shared.ID{entries[1]}}
	on := asked(ctx, t, tenantA, collection, map[string]any{"field": "critical", "op": "EQ", "value": true}, nil, path)
	if !sameIDs(on, []shared.ID{entries[1]}) {
		t.Errorf("on the path %v", on)
	}
	off := asked(ctx, t, tenantA, collection, map[string]any{"field": "critical", "op": "NEQ", "value": true}, nil, path)
	if !sameIDs(off, []shared.ID{entries[0]}) {
		t.Errorf("off the path %v", off)
	}
	// The path tenant B's use case would answer names tenant A's entry; tenant B's query still
	// reads only tenant B's rows.
	_, collectionB, _ := seedPlan(ctx, t, tenantB)
	if got := asked(ctx, t, tenantB, collectionB, map[string]any{"field": "critical", "op": "EQ", "value": true}, nil, path); len(got) != 0 {
		t.Errorf("tenant B found %v", got)
	}
}
