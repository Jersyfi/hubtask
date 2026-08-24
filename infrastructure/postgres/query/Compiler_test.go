// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

var (
	collection = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	hub        = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	task       = shared.MustParseID("0192f000-0000-7000-8000-00000000000e")
	label      = shared.MustParseID("0192f000-0000-7000-8000-00000000000f")
)

// searchOf builds what the adapter would hand the compiler: a resolved anchor and a query that has
// been through the grammar. Every test goes through the real parser rather than constructing a Spec
// by hand, because a Spec nobody validated is not what this package is ever given.
func searchOf(t *testing.T, filter any, spec view.Spec) repository.ItemSearch {
	t.Helper()

	node, err := view.ParseFilter(filter, "/filter")
	if err != nil {
		t.Fatalf("the grammar refused the filter: %v", err)
	}
	if spec.Sort == nil {
		spec.Sort, _ = view.ParseSort(nil, "/sort")
	}
	spec.Filter = node
	return repository.ItemSearch{
		Anchor: repository.Anchor{
			Kind: repository.AnchorCollection, CollectionID: collection, IncludeDescendants: true,
		},
		Spec: spec,
	}
}

func compile(t *testing.T, filter any, spec view.Spec) Statement {
	t.Helper()

	statement, err := Rows(searchOf(t, filter, spec), Boundary{}, 51)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	return statement
}

func TestTheScopeIsTheAnchorTheUseCaseResolved(t *testing.T) {
	sort, _ := view.ParseSort(nil, "/sort")

	tests := []struct {
		name   string
		anchor repository.Anchor
		want   string
	}{
		{
			"one collection, whole",
			repository.Anchor{
				Kind: repository.AnchorCollection, CollectionID: collection, IncludeDescendants: true,
			},
			`wi.collection_id = $1::uuid AND wi.deleted_at IS NULL AND wi.archived_at IS NULL`,
		},
		{
			"one collection, one level",
			repository.Anchor{Kind: repository.AnchorCollection, CollectionID: collection},
			`wi.collection_id = $1::uuid AND wi.parent_id IS NULL`,
		},
		{
			"a hub",
			repository.Anchor{Kind: repository.AnchorHub, HubID: hub, IncludeDescendants: true},
			`wi.collection_id IN (SELECT c.id FROM container c WHERE c.parent_id = $1::uuid ` +
				`AND c.deleted_at IS NULL)`,
		},
		{
			"an entry's subtree",
			repository.Anchor{
				Kind: repository.AnchorItem, CollectionID: collection, ItemID: task,
				PathPrefix: "/" + task.String() + "/", IncludeDescendants: true,
			},
			`wi.collection_id = $1::uuid AND wi.path LIKE $2 AND wi.id <> $3::uuid`,
		},
		{
			"an entry's children",
			repository.Anchor{
				Kind: repository.AnchorItem, CollectionID: collection, ItemID: task,
				PathPrefix: "/" + task.String() + "/",
			},
			`wi.collection_id = $1::uuid AND wi.parent_id = $2::uuid`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statement, err := Rows(
				repository.ItemSearch{Anchor: test.anchor, Spec: view.Spec{Sort: sort}},
				Boundary{}, 51)
			if err != nil {
				t.Fatalf("compilation failed: %v", err)
			}
			if !strings.Contains(statement.SQL, test.want) {
				t.Errorf("the scope reads\n  %s\nand should contain\n  %s", statement.SQL, test.want)
			}
		})
	}
}

func TestTheLifecycleStampsFollowTheRequest(t *testing.T) {
	both := compile(t, nil, view.Spec{IncludeArchived: true, IncludeTrashed: true})
	// The prefixed spellings, deliberately: the visible-fields projection checks the *definition*
	// table's deleted_at, which is not the entry's lifecycle and is there on every query.
	if strings.Contains(both.SQL, "wi.deleted_at IS NULL") || strings.Contains(both.SQL, "wi.archived_at IS NULL") {
		t.Errorf("a query that asked for everything still excludes: %s", both.SQL)
	}

	plain := compile(t, nil, view.Spec{})
	for _, want := range []string{"wi.deleted_at IS NULL", "wi.archived_at IS NULL"} {
		if !strings.Contains(plain.SQL, want) {
			t.Errorf("the plain query is missing %q: %s", want, plain.SQL)
		}
	}
}

func TestTheOperatorsCompile(t *testing.T) {
	tests := []struct {
		name   string
		filter any
		want   string
		args   []any
	}{
		{
			"equality on an enum",
			map[string]any{"field": "type", "op": "EQ", "value": "TASK"},
			`(wi.type = $2::item_type)`, []any{"TASK"},
		},
		{
			"inequality is null-safe",
			map[string]any{"field": "bucket_id", "op": "NEQ", "value": label.String()},
			`(wi.bucket_id IS DISTINCT FROM $2::uuid)`, []any{label.String()},
		},
		{
			"a list is one array parameter",
			map[string]any{"field": "type", "op": "IN", "value": []any{"TASK", "ACTIVITY"}},
			`(wi.type = ANY($2::item_type[]))`, []any{[]string{"TASK", "ACTIVITY"}},
		},
		{
			"NOT_IN keeps the entries that have no value",
			map[string]any{"field": "bucket_id", "op": "NOT_IN", "value": []any{label.String()}},
			`(wi.bucket_id IS NULL OR wi.bucket_id <> ALL($2::uuid[]))`, nil,
		},
		{
			"a range",
			map[string]any{"field": "depth", "op": "BETWEEN", "value": []any{0.0, 2.0}},
			`(wi.depth BETWEEN $2::bigint AND $3::bigint)`, []any{int64(0), int64(2)},
		},
		{
			"absence",
			map[string]any{"field": "notes", "op": "IS_NULL"},
			`(wi.notes IS NULL)`, nil,
		},
		{
			"a substring, without a wildcard to escape",
			map[string]any{"field": "title", "op": "CONTAINS", "value": "50%"},
			`(position(lower(normalize($2::text, NFC)) IN lower(coalesce(wi.title, ''))) > 0)`,
			[]any{"50%"},
		},
		{
			"a prefix",
			map[string]any{"field": "title", "op": "STARTS_WITH", "value": "draft"},
			`(starts_with(lower(wi.title), lower(normalize($2::text, NFC))))`, nil,
		},
		{
			// Two branches, and the argument order is the point: the configuration is a bound
			// parameter resolved in the database, so the tag never becomes text (C-08, ADR-0034).
			// The `simple` branch is what finds an entry that stated no language at all.
			"full text, under the searcher's configuration and under simple",
			map[string]any{"field": "text", "op": "MATCHES", "value": "quarterly report"},
			`(wi.search_document @@ websearch_to_tsquery(hubtask_text_config($2::text), ` +
				`normalize($3::text, NFC)) OR wi.search_document @@ ` +
				`websearch_to_tsquery('simple', normalize($4::text, NFC)))`,
			[]any{"", "quarterly report", "quarterly report"},
		},
		{
			"one label",
			map[string]any{"field": "labels", "op": "CONTAINS", "value": label.String()},
			`(EXISTS (SELECT 1 FROM item_label il JOIN label l ON l.id = il.label_id ` +
				`WHERE il.item_id = wi.id AND l.deleted_at IS NULL AND il.label_id = ANY($2::uuid[])))`,
			nil,
		},
		{
			"all of several labels",
			map[string]any{"field": "labels", "op": "CONTAINS_ALL", "value": []any{
				label.String(), task.String(),
			}},
			`AND il.label_id = ANY($2::uuid[])) = $3`, []any{[]string{label.String(), task.String()}, int64(2)},
		},
		{
			// The assignee is a scalar, so it is a column comparison rather than a relation - which
			// is the whole difference between it and the members beside it (C-01).
			"the assignee is a column",
			map[string]any{"field": "assignee_id", "op": "EQ", "value": task.String()},
			`(wi.assignee_id = $2::uuid)`, []any{task.String()},
		},
		{
			"nobody is on it",
			map[string]any{"field": "assignee_id", "op": "IS_NULL"},
			`(wi.assignee_id IS NULL)`, nil,
		},
		{
			// No join against `account`: an account has no deletion stamp a filter could read, so
			// there is no second table whose state could hide a row.
			"one member",
			map[string]any{"field": "members", "op": "CONTAINS", "value": task.String()},
			`(EXISTS (SELECT 1 FROM item_member im ` +
				`WHERE im.item_id = wi.id AND im.account_id = ANY($2::uuid[])))`,
			nil,
		},
		{
			"all of several members",
			map[string]any{"field": "members", "op": "CONTAINS_ALL", "value": []any{
				label.String(), task.String(),
			}},
			`AND im.account_id = ANY($2::uuid[])) = $3`,
			[]any{[]string{label.String(), task.String()}, int64(2)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statement := compile(t, test.filter, view.Spec{})
			if !strings.Contains(statement.SQL, test.want) {
				t.Errorf("compiled to\n  %s\nand should contain\n  %s", statement.SQL, test.want)
			}
			for index, want := range test.args {
				if index+1 >= len(statement.Args) {
					t.Fatalf("expected an argument %d, got %v", index+1, statement.Args)
				}
				// The first argument is the scope; the filter's own start after it.
				if !equalArgs(statement.Args[index+1], want) {
					t.Errorf("argument %d is %#v, want %#v", index+1, statement.Args[index+1], want)
				}
			}
		})
	}
}

func TestTheCombinationsCompile(t *testing.T) {
	statement := compile(t, map[string]any{"op": "AND", "nodes": []any{
		map[string]any{"field": "is_completed", "op": "EQ", "value": false},
		map[string]any{"op": "OR", "nodes": []any{
			map[string]any{"field": "depth", "op": "EQ", "value": 0.0},
			map[string]any{"op": "NOT", "nodes": []any{
				map[string]any{"field": "title", "op": "EQ", "value": "x"},
			}},
		}},
	}}, view.Spec{})

	want := `((wi.is_completed = $2::boolean) AND ((wi.depth = $3::bigint) OR ` +
		`(NOT (wi.title = normalize($4::text, NFC)))))`
	if !strings.Contains(statement.SQL, want) {
		t.Errorf("compiled to\n  %s\nand should contain\n  %s", statement.SQL, want)
	}
}

func TestTheOrderingIsTheSortAndThenTheIdentifier(t *testing.T) {
	tests := []struct {
		name string
		sort any
		want string
	}{
		{
			"the manual order by default",
			nil, ` ORDER BY wi.order_key COLLATE "C" ASC, wi.id LIMIT `,
		},
		{
			"a descending key with the nulls placed",
			[]any{map[string]any{"field": "completed_at", "dir": "DESC", "nulls": "FIRST"}},
			` ORDER BY wi.completed_at DESC NULLS FIRST, wi.id LIMIT `,
		},
		{
			"several keys, most significant first",
			[]any{
				map[string]any{"field": "is_completed"},
				map[string]any{"field": "title", "dir": "DESC"},
			},
			` ORDER BY wi.is_completed ASC, wi.title DESC, wi.id LIMIT `,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sort, err := view.ParseSort(test.sort, "/sort")
			if err != nil {
				t.Fatalf("the grammar refused the sort: %v", err)
			}
			statement := compile(t, nil, view.Spec{Sort: sort})
			if !strings.Contains(statement.SQL, test.want) {
				t.Errorf("compiled to\n  %s\nand should contain\n  %s", statement.SQL, test.want)
			}
		})
	}
}

func TestTheKeysetContinuesTheWalk(t *testing.T) {
	tests := []struct {
		name string
		sort any
		keys []string
		want string
	}{
		{
			// The default order, and the one shape an index can seek with.
			"an ascending sort over columns that are never null is a row comparison",
			nil, []string{"vaaa"},
			`(wi.order_key COLLATE "C", wi.id) > ($2::text COLLATE "C", $3::uuid)`,
		},
		{
			"two ascending keys stay a row comparison",
			[]any{map[string]any{"field": "title"}, map[string]any{"field": "depth"}},
			[]string{"vAlpha", "v2"},
			`(wi.title, wi.depth, wi.id) > ($2::text, $3::bigint, $4::uuid)`,
		},
		{
			"a descending sort expands",
			[]any{map[string]any{"field": "title", "dir": "DESC"}},
			[]string{"vAlpha"},
			`(wi.title < $2::text OR (wi.title IS NOT DISTINCT FROM $3::text AND wi.id > $4::uuid))`,
		},
		{
			"a nullable key carries where its nulls were placed",
			[]any{map[string]any{"field": "completed_at", "nulls": "LAST"}},
			[]string{"n"},
			`((CASE WHEN NULL::timestamptz IS NULL THEN false WHEN wi.completed_at IS NULL THEN true ` +
				`ELSE wi.completed_at > NULL::timestamptz END) OR ` +
				`(wi.completed_at IS NOT DISTINCT FROM NULL::timestamptz AND wi.id > $2::uuid))`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sort, err := view.ParseSort(test.sort, "/sort")
			if err != nil {
				t.Fatalf("the grammar refused the sort: %v", err)
			}
			statement, err := Rows(
				searchOf(t, nil, view.Spec{Sort: sort}),
				Boundary{Keys: test.keys, ID: task}, 51)
			if err != nil {
				t.Fatalf("compilation failed: %v", err)
			}
			if !strings.Contains(statement.SQL, test.want) {
				t.Errorf("compiled to\n  %s\nand should contain\n  %s", statement.SQL, test.want)
			}
		})
	}
}

// A cursor is signed, so a boundary this package cannot read did not come from a client - but it
// must still be an answer rather than a statement the database refuses.
func TestAnUnreadableBoundaryIsRefused(t *testing.T) {
	sort, _ := view.ParseSort([]any{map[string]any{"field": "created_at"}}, "/sort")

	for _, keys := range [][]string{{"vnot-a-time"}, {"neither"}, {}} {
		if _, err := Rows(
			searchOf(t, nil, view.Spec{Sort: sort}), Boundary{Keys: keys, ID: task}, 51,
		); err == nil {
			t.Errorf("the boundary %v compiled", keys)
		}
	}
}

func TestGroupsAreOneWindowedQuery(t *testing.T) {
	group, err := view.ParseGroupBy(map[string]any{"field": "bucket_id"}, "/group_by")
	if err != nil {
		t.Fatalf("the grammar refused the grouping: %v", err)
	}

	statement, err := Groups(searchOf(t, nil, view.Spec{GroupBy: group}), 51)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	for _, want := range []string{
		`row_number() OVER (PARTITION BY wi.bucket_id ORDER BY wi.order_key COLLATE "C" ASC, wi.id) AS rn`,
		`) g WHERE g.rn <= $2`,
		` ORDER BY bucket_id ASC NULLS LAST, order_key COLLATE "C" ASC, id`,
	} {
		if !strings.Contains(statement.SQL, want) {
			t.Errorf("compiled to\n  %s\nand should contain\n  %s", statement.SQL, want)
		}
	}
}

func TestTheCountIsTheSameFilterWithoutThePage(t *testing.T) {
	plain, err := Count(searchOf(t, map[string]any{
		"field": "is_completed", "op": "EQ", "value": true,
	}, view.Spec{}))
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	if !strings.HasPrefix(plain.SQL, `SELECT count(*)::bigint FROM work_item wi WHERE `) {
		t.Errorf("the count reads %s", plain.SQL)
	}
	if strings.Contains(plain.SQL, "ORDER BY") || strings.Contains(plain.SQL, "LIMIT") {
		t.Errorf("a count has no page: %s", plain.SQL)
	}

	group, _ := view.ParseGroupBy(map[string]any{"field": "bucket_id"}, "/group_by")
	grouped, err := Count(searchOf(t, nil, view.Spec{GroupBy: group}))
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	if !strings.HasPrefix(grouped.SQL, `SELECT wi.bucket_id::text, count(*)::bigint `) ||
		!strings.HasSuffix(grouped.SQL, ` GROUP BY 1`) {
		t.Errorf("the grouped count reads %s", grouped.SQL)
	}
}

// A placeholder that was never resolved is a defect in the use case, and one the compiler must
// notice: `@me` has no comparable value, and binding it as text would compare an account against
// the literal string.
func TestAnUnresolvedPlaceholderIsADefect(t *testing.T) {
	search := searchOf(t, map[string]any{"field": "created_by", "op": "EQ", "value": "@me"}, view.Spec{})

	if _, err := Rows(search, Boundary{}, 51); err == nil {
		t.Error("an unresolved placeholder compiled into a statement")
	}
}

// equalArgs compares a bound value against what the test expects, arrays included: a list operator
// binds one array parameter rather than one per element, and that is part of what is being checked.
func equalArgs(got, want any) bool {
	if texts, ok := want.([]string); ok {
		bound, isSlice := got.([]string)
		return isSlice && slices.Equal(bound, texts)
	}
	return got == want
}

// The custom field family (C-07). The key is the one part of a field *name* that carries a value,
// so what these prove is that it is bound like any other value and never written into the text.
func TestACustomFieldFilterBindsItsKeyAsAParameter(t *testing.T) {
	cases := []struct {
		name     string
		document string
		fragment string
		args     []any
	}{
		{
			name:     "equality goes through containment, which the GIN index answers",
			document: `{"field":"custom_fields.priority","op":"EQ","value":"high"}`,
			fragment: `wi.custom_fields @> jsonb_build_object($2::text, to_jsonb(normalize($3::text, NFC)))`,
			args:     []any{"priority", "high"},
		},
		{
			name:     "a number stays a number, so it matches what a NUMBER field holds",
			document: `{"field":"custom_fields.budget","op":"EQ","value":1000}`,
			fragment: `to_jsonb($3::numeric)`,
			args:     []any{"budget", "1000"},
		},
		{
			name:     "a boolean stays a boolean",
			document: `{"field":"custom_fields.approved","op":"EQ","value":true}`,
			fragment: `to_jsonb($3::boolean)`,
			args:     []any{"approved", true},
		},
		{
			name:     "absence is the key missing from the document",
			document: `{"field":"custom_fields.priority","op":"IS_NULL"}`,
			fragment: `NOT jsonb_exists(wi.custom_fields, $2)`,
			args:     []any{"priority"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			statement := compile(t, documentOf(t, c.document), view.Spec{})

			if !strings.Contains(statement.SQL, c.fragment) {
				t.Fatalf("the statement is %s", statement.SQL)
			}
			for _, want := range c.args {
				if !containsArg(statement.Args, want) {
					t.Errorf("%#v is not among the arguments %#v", want, statement.Args)
				}
			}
			// The whole point: nothing of the key is in the text.
			if strings.Contains(statement.SQL, "priority") ||
				strings.Contains(statement.SQL, "budget") {
				t.Errorf("the key reached the SQL text: %s", statement.SQL)
			}
		})
	}
}

// A name that is not a key is not a field. The grammar refuses it before the compiler sees it,
// which is what keeps the family from being a hole in the closed vocabulary.
func TestACustomFieldNameThatIsNotAKeyIsRefused(t *testing.T) {
	for _, document := range []string{
		`{"field":"custom_fields.","op":"EQ","value":"x"}`,
		`{"field":"custom_fields.Priority","op":"EQ","value":"x"}`,
		`{"field":"custom_fields.a b","op":"EQ","value":"x"}`,
		`{"field":"custom_fields.'; DROP TABLE work_item; --","op":"EQ","value":"x"}`,
	} {
		t.Run(document, func(t *testing.T) {
			if _, err := view.ParseFilter(documentOf(t, document), "/filter"); err == nil {
				t.Error("the grammar accepted a name that is not a key")
			}
		})
	}
}

// containsArg reports whether the value is among the bound arguments.
func containsArg(args []any, want any) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

// documentOf decodes a filter the way a request body reaches the grammar: as a map, not as a Go
// literal, so the test exercises the same path a client does.
func documentOf(t *testing.T, document string) any {
	t.Helper()

	var raw any
	if err := json.Unmarshal([]byte(document), &raw); err != nil {
		t.Fatalf("the fixture is not JSON: %v", err)
	}
	return raw
}

// The searcher's language reaches the statement as a bound parameter and nothing else. It is the
// one value of a compiled query that comes from a preference rather than from the grammar, and the
// rule is the same for it as for every other: no byte of it becomes SQL text (rule 9, T-06).
func TestTheSearchersLanguageIsBoundRatherThanWritten(t *testing.T) {
	search := searchOf(t, map[string]any{
		"field": "text", "op": "MATCHES", "value": "quarterly report",
	}, view.Spec{})
	search.Language = "de-AT'; DROP TABLE work_item; --"

	statement, err := Rows(search, Boundary{}, 51)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	if strings.Contains(statement.SQL, "de-AT") || strings.Contains(statement.SQL, "DROP") {
		t.Fatalf("the language reached the statement's text:\n  %s", statement.SQL)
	}
	if !contains(statement.Args, search.Language) {
		t.Errorf("the language is not among the arguments: %v", statement.Args)
	}
}

func contains(args []any, want string) bool {
	for _, arg := range args {
		if value, ok := arg.(string); ok && value == want {
			return true
		}
	}
	return false
}
