// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// The query language against a real database (B-12).
//
// This is where the compiler is actually proved. FuzzCompile shows that no request can reach the
// statement's text; only PostgreSQL can say whether the statement it produces is valid SQL, uses
// the right types, and answers the question that was asked. Every filter here is written the way a
// client writes it - as a document - and goes through the same grammar the endpoint uses, so a
// change to either side shows up here.

// queried runs one query as the use case would, and fails the test if it cannot.
func queried(
	ctx context.Context, t *testing.T, tenant shared.ID, search repository.ItemSearch,
) repository.ItemQueryResult {
	t.Helper()

	var result repository.ItemQueryResult
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		result, err = itemRepo().Query(ctx, search)
		return err
	}); err != nil {
		t.Fatalf("querying: %v", err)
	}
	return result
}

// searchIn builds a query over one collection from a filter document, through the real grammar.
func searchIn(t *testing.T, collection shared.ID, filter any, spec view.Spec) repository.ItemSearch {
	t.Helper()

	node, err := view.ParseFilter(filter, "/filter")
	if err != nil {
		t.Fatalf("the grammar refused the filter: %v", err)
	}
	if spec.Sort == nil {
		if spec.Sort, err = view.ParseSort(nil, "/sort"); err != nil {
			t.Fatalf("the default sort does not parse: %v", err)
		}
	}
	if spec.Size == 0 {
		spec.Size = 50
	}
	spec.Filter = node

	return repository.ItemSearch{
		Anchor: repository.Anchor{
			Kind: repository.AnchorCollection, CollectionID: collection, IncludeDescendants: true,
		},
		Spec: spec,
	}
}

func titlesOf(items []work.WorkItem) []string {
	titles := make([]string, 0, len(items))
	for _, item := range items {
		titles = append(titles, item.Title)
	}
	return titles
}

func idsOfItems(items []work.WorkItem) []shared.ID {
	ids := make([]shared.ID, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

// queryFixture writes a collection with four tasks whose fields differ in every way the operators
// can ask about, and one work package under the first of them.
type fixture struct {
	collection shared.ID
	tasks      []work.WorkItem
	child      work.WorkItem
	bucket     work.Bucket
	label      work.Label
	// lastKey is the highest rank in the collection, so that a test adding an entry can rank it
	// after everything the fixture wrote rather than inventing a key the domain would refuse.
	lastKey string
}

func queryFixture(ctx context.Context, t *testing.T) fixture {
	t.Helper()

	collection := collectionFor(ctx, t, tenantA, authorA)
	built := fixture{
		collection: collection,
		bucket:     seedBucket(ctx, t, tenantA, collection, "a0"),
		label:      seedLabel(ctx, t, tenantA, collection),
	}

	titles := []string{"Alpha milk", "Beta bread", "Gamma milk", "Delta cheese"}
	items := itemRepo()
	previous := ""

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for index, title := range titles {
			key, err := service.OrderKeyAfter(previous)
			if err != nil {
				return err
			}
			previous = key

			task := taskIn(tenantA, authorA, collection, freshID(t), title, key)
			if index == 0 {
				task.Notes = "Semi-skimmed"
			}
			if index <= 1 {
				task.BucketID = built.bucket.ID
			}
			if err := items.Insert(ctx, task); err != nil {
				return err
			}
			built.tasks = append(built.tasks, task)
		}

		// One level down, and ranked after its aunts and uncles: a query over a whole collection
		// mixes the levels, and a child that ranked first would make every ordering assertion below
		// a statement about the fixture rather than about the ordering.
		key, err := service.OrderKeyAfter(previous)
		if err != nil {
			return err
		}
		previous = key

		child := taskIn(tenantA, authorA, collection, freshID(t), "Epsilon detail", key)
		child.Type, child.ParentID = work.ItemWorkPackage, built.tasks[0].ID
		child.Path, child.Depth = built.tasks[0].ChildPath(child.ID), 2
		if err := items.Insert(ctx, child); err != nil {
			return err
		}
		built.child, built.lastKey = child, previous

		// The completion and the archive stamp are written afterwards rather than in the insert,
		// because that is how they are written in production: the insert takes the fields
		// CreateWorkItem owns, and the lifecycle is a transition of its own (B-07, B-10).
		completed := built.tasks[1].Completed(authorA, created)
		if err := items.SetCompletion(ctx, completed, built.tasks[1].Version); err != nil {
			return err
		}
		built.tasks[1] = completed

		archived, _, err := built.tasks[2].Archived(created.Add(time.Hour))
		if err != nil {
			return err
		}
		if err := items.SetArchived(ctx, archived, built.tasks[2].Version); err != nil {
			return err
		}
		built.tasks[2] = archived

		return itemLabelRepo().Add(ctx, built.tasks[0].ID, built.label.ID, shared.HLC{})
	}); err != nil {
		t.Fatalf("seeding the query fixture: %v", err)
	}
	return built
}

// Every operator the catalogue offers, asked of a real column. A filter that compiles and then
// fails to plan - a text parameter against a uuid column, an enum compared without its cast - shows
// up here and nowhere earlier.
func TestEveryOperatorAnswersAgainstTheDatabase(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)

	tests := []struct {
		name   string
		filter any
		want   []string
	}{
		{
			"equality on an enum",
			map[string]any{"field": "type", "op": "EQ", "value": "WORK_PACKAGE"},
			[]string{"Epsilon detail"},
		},
		{
			"a list of enums",
			map[string]any{"field": "type", "op": "IN", "value": []any{"TASK"}},
			[]string{"Alpha milk", "Beta bread", "Delta cheese"},
		},
		{
			"a boolean",
			map[string]any{"field": "is_completed", "op": "EQ", "value": true},
			[]string{"Beta bread"},
		},
		{
			"a substring, case-insensitively",
			map[string]any{"field": "title", "op": "CONTAINS", "value": "MILK"},
			[]string{"Alpha milk"},
		},
		{
			"a prefix",
			map[string]any{"field": "title", "op": "STARTS_WITH", "value": "delta"},
			[]string{"Delta cheese"},
		},
		{
			"full text over the title and the notes",
			map[string]any{"field": "text", "op": "MATCHES", "value": "semi-skimmed"},
			[]string{"Alpha milk"},
		},
		{
			"absence",
			map[string]any{"field": "notes", "op": "IS_NULL"},
			[]string{"Beta bread", "Delta cheese", "Epsilon detail"},
		},
		{
			"a range on a whole number",
			map[string]any{"field": "depth", "op": "BETWEEN", "value": []any{2.0, 9.0}},
			[]string{"Epsilon detail"},
		},
		{
			"a comparison on a timestamp",
			map[string]any{"field": "created_at", "op": "GTE", "value": created.Format(time.RFC3339)},
			[]string{"Alpha milk", "Beta bread", "Delta cheese", "Epsilon detail"},
		},
		{
			"an identifier",
			map[string]any{"field": "parent_id", "op": "EQ", "value": ""},
			[]string{"Epsilon detail"},
		},
		{
			// The null-safe half of NEQ: the entries on no board are "not in that column" too.
			"inequality keeps the entries with no value",
			map[string]any{"field": "bucket_id", "op": "NEQ", "value": ""},
			[]string{"Delta cheese", "Epsilon detail"},
		},
		{
			"a list of identifiers, negated",
			map[string]any{"field": "bucket_id", "op": "NOT_IN", "value": []any{""}},
			[]string{"Delta cheese", "Epsilon detail"},
		},
		{
			"one label",
			map[string]any{"field": "labels", "op": "CONTAINS", "value": ""},
			[]string{"Alpha milk"},
		},
		{
			"all of a set of labels",
			map[string]any{"field": "labels", "op": "CONTAINS_ALL", "value": []any{""}},
			[]string{"Alpha milk"},
		},
		{
			"a combination one level deep",
			map[string]any{"op": "AND", "nodes": []any{
				map[string]any{"field": "is_completed", "op": "EQ", "value": false},
				map[string]any{"op": "OR", "nodes": []any{
					map[string]any{"field": "title", "op": "CONTAINS", "value": "milk"},
					map[string]any{"field": "title", "op": "CONTAINS", "value": "cheese"},
				}},
			}},
			[]string{"Alpha milk", "Delta cheese"},
		},
		{
			"a negation",
			map[string]any{"op": "NOT", "nodes": []any{
				map[string]any{"field": "type", "op": "EQ", "value": "TASK"},
			}},
			[]string{"Epsilon detail"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// The fixture's identifiers are only known at run time, so the placeholders written as
			// empty strings above are filled in here.
			filter := withFixtureIDs(test.filter, f)

			result := queried(ctx, t, tenantA, searchIn(t, f.collection, filter, view.Spec{}))
			assertTitles(t, titlesOf(result.Items), test.want)
		})
	}
}

// withFixtureIDs replaces the empty identifier placeholders in a filter document with the fixture's
// own. Written this way so that the table above reads as a list of filters rather than as a list of
// closures.
// The two fields C-01 gave use cases, asked of a real column and a real join table. The assignee is
// a scalar and the members are a relation, which is the whole difference between them - and a
// filter that compiles and then fails to plan shows up here and nowhere earlier.
func TestTheAssigneeAndTheMembersAnswerAgainstTheDatabase(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)
	account := seedAccount(ctx, t, tenantA)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		assigned := f.tasks[0].Assigned(account, changedAt)
		if err := itemRepo().SetAssignee(ctx, assigned, f.tasks[0].Version); err != nil {
			return err
		}
		// A different entry carries the member, so that a filter answering both would be visible as
		// one answering neither.
		return itemMemberRepo().Add(ctx, f.tasks[3].ID, account, shared.HLC{})
	}); err != nil {
		t.Fatalf("seeding the assignment: %v", err)
	}

	tests := []struct {
		name   string
		filter any
		want   []string
	}{
		{
			"the entry one person is on",
			map[string]any{"field": "assignee_id", "op": "EQ", "value": account.String()},
			[]string{"Alpha milk"},
		},
		{
			"the entries nobody is on",
			map[string]any{"field": "assignee_id", "op": "IS_NULL"},
			[]string{"Beta bread", "Delta cheese", "Epsilon detail"},
		},
		{
			"the entries somebody is a member of",
			map[string]any{"field": "members", "op": "CONTAINS", "value": account.String()},
			[]string{"Delta cheese"},
		},
		{
			"all of a set of members",
			map[string]any{"field": "members", "op": "CONTAINS_ALL", "value": []any{account.String()}},
			[]string{"Delta cheese"},
		},
		{
			// "my items" as api-guidelines.md §3 writes it: the scalar and the set, joined by OR.
			"assigned to me or a member of it",
			map[string]any{"op": "OR", "nodes": []any{
				map[string]any{"field": "assignee_id", "op": "EQ", "value": account.String()},
				map[string]any{"field": "members", "op": "CONTAINS", "value": account.String()},
			}},
			[]string{"Alpha milk", "Delta cheese"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := queried(ctx, t, tenantA, searchIn(t, f.collection, test.filter, view.Spec{}))
			assertTitles(t, titlesOf(result.Items), test.want)
		})
	}
}

func withFixtureIDs(filter any, f fixture) any {
	document, ok := filter.(map[string]any)
	if !ok {
		return filter
	}

	replacement := ""
	switch document["field"] {
	case view.FieldBucketID:
		replacement = f.bucket.ID.String()
	case view.FieldLabels:
		replacement = f.label.ID.String()
	case view.FieldParentID:
		// An entry that has a parent, which is the work package.
		replacement = f.tasks[0].ID.String()
	}

	if replacement != "" {
		switch value := document["value"].(type) {
		case string:
			document["value"] = replacement
		case []any:
			for index := range value {
				value[index] = replacement
			}
		}
	}
	if nodes, nested := document["nodes"].([]any); nested {
		for index := range nodes {
			nodes[index] = withFixtureIDs(nodes[index], f)
		}
	}
	return document
}

func assertTitles(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("answered %v, want %v", got, want)
	}
	seen := map[string]bool{}
	for _, title := range got {
		seen[title] = true
	}
	for _, title := range want {
		if !seen[title] {
			t.Errorf("answered %v, want %v", got, want)
			return
		}
	}
}

// The archived and trashed entries: absent unless asked for, which is what makes the plain query
// mean "the work that is live".
func TestTheLifecycleFlagsWidenTheQuery(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)

	plain := queried(ctx, t, tenantA, searchIn(t, f.collection, nil, view.Spec{}))
	if len(plain.Items) != 4 {
		t.Errorf("the live entries are %v", titlesOf(plain.Items))
	}

	archived := queried(ctx, t, tenantA,
		searchIn(t, f.collection, nil, view.Spec{IncludeArchived: true}))
	if len(archived.Items) != 5 {
		t.Errorf("with the archived ones the answer is %v", titlesOf(archived.Items))
	}

	// A deletion, and then the same query twice.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		trashed := f.tasks[3]
		stamp := created.Add(2 * time.Hour)
		trashed.DeletedAt, trashed.TrashBatchID = &stamp, freshID(t)
		_, err := itemRepo().TrashSubtree(ctx, repository.ItemTrash{
			Item: trashed, Prefix: trashed.Path, BatchID: trashed.TrashBatchID,
			ExpectedVersion: trashed.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("trashing an entry: %v", err)
	}

	live := queried(ctx, t, tenantA, searchIn(t, f.collection, nil, view.Spec{}))
	if len(live.Items) != 3 {
		t.Errorf("after the deletion the live entries are %v", titlesOf(live.Items))
	}
	withTrash := queried(ctx, t, tenantA,
		searchIn(t, f.collection, nil, view.Spec{IncludeTrashed: true}))
	if len(withTrash.Items) != 4 {
		t.Errorf("with the trash the answer is %v", titlesOf(withTrash.Items))
	}
}

// The three anchors, against the tree the fixture built.
func TestTheScopeDecidesWhatIsSearched(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)
	hub := collectionParent(ctx, t, f.collection)

	sort, err := view.ParseSort(nil, "/sort")
	if err != nil {
		t.Fatalf("the default sort does not parse: %v", err)
	}

	tests := []struct {
		name   string
		anchor repository.Anchor
		want   int
	}{
		{
			"a collection, whole",
			repository.Anchor{
				Kind: repository.AnchorCollection, CollectionID: f.collection, IncludeDescendants: true,
			},
			4,
		},
		{
			"a collection, one level",
			repository.Anchor{Kind: repository.AnchorCollection, CollectionID: f.collection},
			3,
		},
		{
			"the hub above it",
			repository.Anchor{Kind: repository.AnchorHub, HubID: hub, IncludeDescendants: true},
			4,
		},
		{
			// The anchor is not in its own subtree: "what is inside this entry" is not the entry.
			"an entry's subtree",
			repository.Anchor{
				Kind: repository.AnchorItem, CollectionID: f.collection, ItemID: f.tasks[0].ID,
				PathPrefix: f.tasks[0].Path, IncludeDescendants: true,
			},
			1,
		},
		{
			"an entry's children",
			repository.Anchor{
				Kind: repository.AnchorItem, CollectionID: f.collection, ItemID: f.tasks[0].ID,
				PathPrefix: f.tasks[0].Path,
			},
			1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := queried(ctx, t, tenantA, repository.ItemSearch{
				Anchor: test.anchor, Spec: view.Spec{Sort: sort, Size: 50},
			})
			if len(result.Items) != test.want {
				t.Errorf("the scope answered %v, want %d entries", titlesOf(result.Items), test.want)
			}
		})
	}
}

func collectionParent(ctx context.Context, t *testing.T, collection shared.ID) shared.ID {
	t.Helper()

	var container work.Container
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		container, err = containerRepo().Find(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("reading the collection: %v", err)
	}
	return container.ParentID
}

// The orderings, including the two shapes of keyset the compiler has: the row comparison an index
// can seek with, and the expansion that places the nulls.
func TestTheOrderingsAnswerInOrder(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)

	tests := []struct {
		name string
		sort any
		want []string
	}{
		{"the manual order by default", nil, []string{
			"Alpha milk", "Beta bread", "Delta cheese", "Epsilon detail",
		}},
		{
			"by title, descending",
			[]any{map[string]any{"field": "title", "dir": "DESC"}},
			[]string{"Epsilon detail", "Delta cheese", "Beta bread", "Alpha milk"},
		},
		{
			// completed_at is null for three of the four, so where the nulls go decides the answer.
			"by a nullable field with the nulls first",
			[]any{
				map[string]any{"field": "completed_at", "nulls": "FIRST"},
				map[string]any{"field": "title"},
			},
			[]string{"Alpha milk", "Delta cheese", "Epsilon detail", "Beta bread"},
		},
		{
			"by a nullable field with the nulls last",
			[]any{
				map[string]any{"field": "completed_at", "nulls": "LAST"},
				map[string]any{"field": "title"},
			},
			[]string{"Beta bread", "Alpha milk", "Delta cheese", "Epsilon detail"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sort, err := view.ParseSort(test.sort, "/sort")
			if err != nil {
				t.Fatalf("the grammar refused the sort: %v", err)
			}

			result := queried(ctx, t, tenantA, searchIn(t, f.collection, nil, view.Spec{Sort: sort}))
			got := titlesOf(result.Items)
			for index, want := range test.want {
				if index >= len(got) || got[index] != want {
					t.Fatalf("answered %v, want %v", got, test.want)
				}
			}
		})
	}
}

// A keyset walk is correct or it is not, and the way it is wrong - a row seen twice, a row never
// seen - is invisible against a fake. Every ordering is walked a page at a time, because each one
// takes a different branch of the comparison.
func TestAWalkVisitsEveryEntryExactlyOnce(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)

	for name, sort := range map[string]any{
		"the manual order": nil,
		"descending":       []any{map[string]any{"field": "title", "dir": "DESC"}},
		"nullable, nulls last": []any{
			map[string]any{"field": "completed_at"}, map[string]any{"field": "title"},
		},
		"nullable, nulls first": []any{
			map[string]any{"field": "completed_at", "nulls": "FIRST"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			parsed, err := view.ParseSort(sort, "/sort")
			if err != nil {
				t.Fatalf("the grammar refused the sort: %v", err)
			}

			search := searchIn(t, f.collection, nil, view.Spec{Sort: parsed, Size: 1})
			var seen []shared.ID

			for pages := 1; ; pages++ {
				result := queried(ctx, t, tenantA, search)
				seen = append(seen, idsOfItems(result.Items)...)

				if !result.Info.HasMore {
					break
				}
				if result.Info.NextCursor == "" {
					t.Fatal("a page reported more rows and carried no cursor to reach them")
				}
				search.Spec.Cursor = result.Info.NextCursor
				if pages > 20 {
					t.Fatal("the walk did not terminate")
				}
			}

			if len(seen) != 4 {
				t.Fatalf("a walk of pages of one saw %d entries, want 4", len(seen))
			}
			unique := map[shared.ID]bool{}
			for _, id := range seen {
				if unique[id] {
					t.Fatalf("the walk saw %s twice", id)
				}
				unique[id] = true
			}
		})
	}
}

func TestGroupingAnswersOneColumnPerValue(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)

	group, err := view.ParseGroupBy(
		map[string]any{"field": "bucket_id", "limit_per_group": 1.0}, "/group_by")
	if err != nil {
		t.Fatalf("the grammar refused the grouping: %v", err)
	}

	result := queried(ctx, t, tenantA, searchIn(t, f.collection, nil, view.Spec{
		GroupBy: group, Count: view.CountExact,
	}))
	if len(result.Items) != 0 {
		t.Errorf("a grouped query also answered %d ungrouped rows", len(result.Items))
	}
	if len(result.Groups) != 2 {
		t.Fatalf("%d groups, want one per bucket and one for the entries on no board", len(result.Groups))
	}

	// The column with a value comes first; the entries that have none are the group at the end.
	column, none := result.Groups[0], result.Groups[1]
	if column.Absent || column.Key != f.bucket.ID.String() {
		t.Errorf("the first group is %+v", column)
	}
	if !none.Absent || none.Key != "" {
		t.Errorf("the last group is %+v", none)
	}

	// A column of one, with more behind it: the group's own cursor is what continues it.
	if len(column.Items) != 1 || !column.Info.HasMore || column.Info.NextCursor == "" {
		t.Errorf("the first column reads %d entries, more=%v", len(column.Items), column.Info.HasMore)
	}
	if column.Total != 2 || none.Total != 2 {
		t.Errorf("the columns count %d and %d, want 2 and 2", column.Total, none.Total)
	}
	if result.Total != 4 {
		t.Errorf("the whole result counts %d, want 4", result.Total)
	}
}

// Every field that may be grouped by, with a count: the key comes back from a different column type
// each time - a uuid, an enum, a boolean - and the counted query has to render all three the same
// way the rows are keyed, or a column's total lands on the wrong column or on none.
func TestEveryGroupableFieldAnswersItsColumnsAndCounts(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)

	tests := map[string]struct {
		field  string
		groups int
		keys   []string
	}{
		"a board column": {view.FieldBucketID, 2, []string{f.bucket.ID.String(), ""}},
		"an item type":   {view.FieldType, 2, []string{"TASK", "WORK_PACKAGE"}},
		"done and open":  {view.FieldIsCompleted, 2, []string{"false", "true"}},
		"the author":     {view.FieldCreatedBy, 1, []string{authorA.String()}},
		"the collection": {view.FieldCollection, 1, []string{f.collection.String()}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			group, err := view.ParseGroupBy(map[string]any{"field": test.field}, "/group_by")
			if err != nil {
				t.Fatalf("the grammar refused the grouping: %v", err)
			}

			result := queried(ctx, t, tenantA, searchIn(t, f.collection, nil, view.Spec{
				GroupBy: group, Count: view.CountExact,
			}))
			if len(result.Groups) != test.groups {
				t.Fatalf("%s answered %d groups, want %d", test.field, len(result.Groups), test.groups)
			}

			counted := 0
			for index, answered := range result.Groups {
				if index < len(test.keys) && answered.Key != test.keys[index] {
					t.Errorf("group %d is keyed %q, want %q", index, answered.Key, test.keys[index])
				}
				if answered.Total == 0 {
					t.Errorf("group %q was counted as none - the count did not reach it", answered.Key)
				}
				counted += answered.Total
			}
			if counted != 4 || result.Total != 4 {
				t.Errorf("the columns count %d and the result %d, want 4 and 4", counted, result.Total)
			}
		})
	}
}

// A group's cursor continues that group: the client sends it back with the group's key as a filter,
// which is what the contract says and what has to actually work.
func TestAGroupsCursorContinuesThatGroup(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)

	group, err := view.ParseGroupBy(
		map[string]any{"field": "bucket_id", "limit_per_group": 1.0}, "/group_by")
	if err != nil {
		t.Fatalf("the grammar refused the grouping: %v", err)
	}

	grouped := queried(ctx, t, tenantA, searchIn(t, f.collection, nil, view.Spec{GroupBy: group}))
	column := grouped.Groups[0]

	continued := queried(ctx, t, tenantA, searchIn(t,
		f.collection,
		map[string]any{"field": "bucket_id", "op": "EQ", "value": f.bucket.ID.String()},
		view.Spec{Cursor: column.Info.NextCursor, Size: 50},
	))
	if len(continued.Items) != 1 {
		t.Fatalf("continuing the column answered %v", titlesOf(continued.Items))
	}
	if continued.Items[0].ID == column.Items[0].ID {
		t.Error("continuing the column answered the entry it started from")
	}
}

func TestAnExactCountCountsTheWholeResult(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)

	uncounted := queried(ctx, t, tenantA, searchIn(t, f.collection, nil, view.Spec{Size: 2}))
	if uncounted.Total != 0 {
		t.Errorf("a query that asked for no count answered a total of %d", uncounted.Total)
	}

	counted := queried(ctx, t, tenantA,
		searchIn(t, f.collection, nil, view.Spec{Size: 2, Count: view.CountExact}))
	if len(counted.Items) != 2 {
		t.Errorf("the page holds %d entries, want 2", len(counted.Items))
	}
	if counted.Total != 4 {
		t.Errorf("the total is %d, want 4 - the whole result, not the page", counted.Total)
	}
}

// The cross-tenant negative test every new repository method owes (gate SG-3, security.md §6).
//
// The query path is the one place where the statement is assembled at run time, so "row level
// security still applies" is a claim worth checking rather than assuming: nothing in the compiled
// text names a tenant, and the transaction the caller opened is what decides what it can see.
func TestAQueryNeverCrossesTheTenantBoundary(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)

	// Tenant B asks the same question about tenant A's collection, by identifier.
	result := queried(ctx, t, tenantB, searchIn(t, f.collection, nil, view.Spec{}))
	if len(result.Items) != 0 {
		t.Errorf("tenant B read %v out of tenant A's collection", titlesOf(result.Items))
	}

	// And with every widening the grammar offers, in case one of them is what the policy misses.
	widened := searchIn(t, f.collection, nil, view.Spec{
		IncludeArchived: true, IncludeTrashed: true, Count: view.CountExact,
	})
	if wide := queried(ctx, t, tenantB, widened); len(wide.Items) != 0 || wide.Total != 0 {
		t.Errorf("tenant B read %d entries and a total of %d", len(wide.Items), wide.Total)
	}

	// A hub scope, which reaches the collections through a subquery of its own.
	hub := collectionParent(ctx, t, f.collection)
	sort, err := view.ParseSort(nil, "/sort")
	if err != nil {
		t.Fatalf("the default sort does not parse: %v", err)
	}
	fromHub := queried(ctx, t, tenantB, repository.ItemSearch{
		Anchor: repository.Anchor{Kind: repository.AnchorHub, HubID: hub, IncludeDescendants: true},
		Spec:   view.Spec{Sort: sort, Size: 50},
	})
	if len(fromHub.Items) != 0 {
		t.Errorf("tenant B read %d entries through tenant A's hub", len(fromHub.Items))
	}

	// And the subtree scope, whose predicate is a path prefix rather than an identifier.
	fromSubtree := queried(ctx, t, tenantB, repository.ItemSearch{
		Anchor: repository.Anchor{
			Kind: repository.AnchorItem, CollectionID: f.collection, ItemID: f.tasks[0].ID,
			PathPrefix: f.tasks[0].Path, IncludeDescendants: true,
		},
		Spec: view.Spec{Sort: sort, Size: 50},
	})
	if len(fromSubtree.Items) != 0 {
		t.Errorf("tenant B read %d entries through tenant A's subtree", len(fromSubtree.Items))
	}
}

// A value that looks like SQL is a value: the fuzz test proves it never reaches the statement's
// text, and this proves the database agrees - the query runs, matches the entry whose title
// actually contains those characters, and leaves the table standing.
func TestAValueThatLooksLikeSQLIsAValue(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)

	hostile := "'; DROP TABLE work_item; --"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		key, err := service.OrderKeyAfter(f.lastKey)
		if err != nil {
			return err
		}
		return itemRepo().Insert(ctx,
			taskIn(tenantA, authorA, f.collection, freshID(t), hostile, key))
	}); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}

	result := queried(ctx, t, tenantA, searchIn(t, f.collection,
		map[string]any{"field": "title", "op": "EQ", "value": hostile}, view.Spec{}))
	if len(result.Items) != 1 || result.Items[0].Title != hostile {
		t.Fatalf("the query answered %v", titlesOf(result.Items))
	}

	// The table is still there, which the next query would fail on if it were not.
	if all := queried(ctx, t, tenantA, searchIn(t, f.collection, nil, view.Spec{})); len(all.Items) != 5 {
		t.Errorf("after the hostile value the collection holds %d entries", len(all.Items))
	}
}

// The bound the grammar puts on a list operator, exercised at its limit: a hundred identifiers in
// one array parameter is one statement, not a hundred placeholders.
func TestAListOperatorTakesItsWholeArrayAsOneParameter(t *testing.T) {
	ctx := context.Background()
	f := queryFixture(ctx, t)

	values := make([]any, 0, view.MaxValues)
	values = append(values, f.bucket.ID.String())
	for len(values) < view.MaxValues {
		// Identifiers that exist nowhere, in the generated namespace so that they cannot collide
		// with a fixture constant.
		values = append(values, fmt.Sprintf("01936f2a-7c1e-7000-8f00-%012x", 900000+len(values)))
	}

	result := queried(ctx, t, tenantA, searchIn(t, f.collection,
		map[string]any{"field": "bucket_id", "op": "IN", "value": values}, view.Spec{}))
	if len(result.Items) != 2 {
		t.Errorf("the query answered %v, want the two entries on the board", titlesOf(result.Items))
	}
}
