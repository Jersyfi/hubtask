// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// The gate T-06 asks for: no filter produces invalid SQL, and none escapes parameterisation.
//
// It runs as an ordinary test over its seed corpus in `make gate-unit`, and as a fuzz target for
// five minutes a night in `make gate-fuzz`. Both matter: the seeds are the cases somebody thought
// of, and the fuzzer is for the ones nobody did.
//
// Four properties, and the third is the one that carries ADR-0026:
//
//  1. Anything the grammar accepts, the compiler compiles. A filter that parses and then fails to
//     become a statement would be a 500 on a well-formed request.
//  2. Placeholders and arguments agree in number and are numbered $1..$n without a gap. A statement
//     whose parameters are off by one asks a different question and gets an answer.
//  3. **The text is a function of the query's shape and not of its values.** Compiled twice, once
//     with the values that arrived and once with the same tree carrying fixed values of the same
//     kinds, the two statements are identical - which is only possible if no value reached the
//     text.
//  4. The text stays inside the alphabet this package writes in. A semicolon, a backslash or a
//     comment marker in the statement would mean something got in that this package never wrote.
func FuzzCompile(f *testing.F) {
	for _, seed := range []string{
		`{"field":"title","op":"EQ","value":"hello"}`,
		`{"field":"title","op":"CONTAINS","value":"'; DROP TABLE work_item; --"}`,
		`{"field":"title","op":"STARTS_WITH","value":"\\"}`,
		`{"field":"text","op":"MATCHES","value":"a & b | !c <-> d"}`,
		`{"field":"type","op":"IN","value":["TASK","ACTIVITY"]}`,
		`{"field":"entry_type","op":"NOT_IN","value":["bug","'; DROP TABLE work_item; --"]}`,
		`{"field":"depth","op":"BETWEEN","value":[0,3]}`,
		`{"field":"notes","op":"IS_NULL"}`,
		`{"field":"blocked","op":"EQ","value":true}`,
		`{"field":"blocked","op":"NEQ","value":false}`,
		`{"field":"blocking","op":"EQ","value":true}`,
		`{"field":"linked_to","op":"IN","value":["0190a4c2-7b1e-7000-8000-000000000001"]}`,
		`{"field":"sla_due","op":"LTE","value":"2026-10-30T09:00:00Z"}`,
		`{"field":"sla_standing","op":"IN","value":["AT_RISK","BREACHED"]}`,
		`{"field":"bucket_id","op":"NOT_IN","value":["0192f000-0000-7000-8000-00000000000f"]}`,
		`{"field":"labels","op":"CONTAINS_ALL","value":["0192f000-0000-7000-8000-00000000000f"]}`,
		`{"field":"created_at","op":"GTE","value":"2026-08-19T00:00:00Z"}`,
		`{"op":"AND","nodes":[{"field":"is_completed","op":"EQ","value":false},` +
			`{"op":"OR","nodes":[{"field":"depth","op":"LT","value":2},` +
			`{"op":"NOT","nodes":[{"field":"title","op":"EQ","value":"x"}]}]}]}`,
		`{"op":"AND","nodes":[]}`,
		`{"field":"due_at","op":"EQ","value":"2026-08-19T00:00:00Z"}`,
		`{"field":"title","op":"EQ","value":"$1"}`,
		`{"field":"title","op":"EQ","value":"COLLATE \"C\""}`,
		// The custom field family. The key is the one part of a field *name* that is a
		// value, so these seeds are what says it never reaches the SQL text.
		`{"field":"custom_fields.priority","op":"EQ","value":"high"}`,
		`{"field":"custom_fields.budget","op":"EQ","value":1000}`,
		`{"field":"custom_fields.done_twice","op":"EQ","value":true}`,
		`{"field":"custom_fields.tags","op":"CONTAINS","value":"urgent"}`,
		`{"field":"custom_fields.priority","op":"IN","value":["high","low"]}`,
		`{"field":"custom_fields.priority","op":"IS_NULL"}`,
		`{"field":"custom_fields.'; DROP TABLE work_item; --","op":"EQ","value":"x"}`,
		`{"field":"custom_fields.a\" OR 1=1 --","op":"EQ","value":"x"}`,
		// The typed side (F-2): ranges over the value table, the column chosen by the shape.
		`{"field":"custom_fields.budget","op":"GT","value":1000}`,
		`{"field":"custom_fields.review","op":"BETWEEN","value":["2026-01-01","2026-12-31"]}`,
		`{"field":"custom_fields.code","op":"LTE","value":"'; DROP TABLE work_item; --"}`,
		`{"field":"custom_fields.x","op":"LT","value":"$1"}`,
		// An aggregation over a key that tries to close the statement (S-11).
		`{"field":"custom_fields.hours) FROM work_item; --","op":"GT","value":0}`,
		// The set and prefix operators on a custom field (S-11): a text set, a text, undefined.
		`{"field":"custom_fields.release_tags","op":"CONTAINS_ANY","value":["a","'; DROP TABLE work_item; --"]}`,
		`{"field":"custom_fields.release_tags","op":"CONTAINS_ALL","value":["a","b"]}`,
		`{"field":"custom_fields.colours","op":"STARTS_WITH","value":"%_\\"}`,
		`{"field":"custom_fields.tags","op":"CONTAINS_ALL","value":["x"]}`,
		`not json at all`,
		`{}`,
		`[]`,
		`null`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, document string) {
		var raw any
		if err := json.Unmarshal([]byte(document), &raw); err != nil {
			// Not a document at all. The transport refuses this long before the grammar sees it.
			return
		}

		node, err := view.ParseFilter(raw, "/filter")
		if err != nil {
			// Refused by the grammar, which is the answer for most inputs and the point of having
			// one. Nothing reaches the compiler.
			return
		}

		sort, err := view.ParseSort(nil, "/sort")
		if err != nil {
			t.Fatalf("the default sort does not parse: %v", err)
		}
		// The use case shapes every custom field before the compiler sees it; a filter the
		// shape refuses never reaches it.
		shaped, err := view.Spec{Filter: node, Sort: sort}.Shaped(fuzzShapes(node))
		if err != nil {
			return
		}
		node = shaped.Filter
		search := searchFor(node, sort)

		statement, err := Rows(search, Boundary{}, 51)
		if err != nil {
			// Property 1. The one legitimate exception is a placeholder nothing resolved, which is
			// a use case's mistake and cannot arrive from a client - the use case resolves before
			// it calls, and the compiler refuses on purpose.
			if hasPlaceholder(node) {
				return
			}
			t.Fatalf("the grammar accepted a filter the compiler cannot write:\n  %s\n  %v",
				document, err)
		}

		assertParameters(t, statement)
		assertShapeOnly(t, node, sort, statement)
		assertAlphabet(t, statement, document)

		// The same filter under an aggregation (S-11): a count per status and week, a lead time
		// per Sunday week, and across a set of collections.
		for _, raw := range []map[string]any{
			{"function": "COUNT", "group_by": "status",
				"bucket": map[string]any{"field": "created_at", "period": "week"}},
			{"function": "AVG", "field": "lead_time",
				"bucket": map[string]any{"field": "completed_at", "period": "week", "week_start": "sunday"}},
			{"function": "SUM", "field": "scheduled_span", "group_by": "assignee_id"},
			{"source": "links", "function": "COUNT", "group_by": "link_type",
				"bucket": map[string]any{"field": "created_at", "period": "month"}},
			{"source": "time", "function": "SUM", "field": "minutes", "group_by": "person",
				"bucket": map[string]any{"field": "day", "period": "week", "week_start": "sunday"}},
			{"source": "pages", "function": "COUNT", "group_by": "verification",
				"bucket": map[string]any{"field": "created_at", "period": "day"}},
		} {
			counted, err := view.ParseAggregation(raw, "/aggregate")
			if err != nil {
				t.Fatalf("the aggregation does not parse: %v", err)
			}
			for _, anchor := range []repository.Anchor{search.Anchor, {
				Kind: repository.AnchorCollections, CollectionIDs: []shared.ID{collection}, ItemIDs: []shared.ID{collection},
			}} {
				summary, err := Aggregate(repository.ItemSearch{Anchor: anchor, Spec: search.Spec},
					counted, document, 5001)
				if err != nil && !hasPlaceholder(node) {
					t.Fatalf("an aggregation did not compile: %v", err)
				}
				if err == nil {
					assertParameters(t, summary)
					assertAlphabet(t, summary, document)
				}
			}
		}

		// The same keys as a sort and a grouping, which write the typed value in two more places.
		for _, key := range (view.Spec{Filter: node}).CustomKeys() {
			raw := map[string]any{"field": view.CustomFieldPrefix + key, "dir": "DESC"}
			custom, err := view.ParseSort([]any{raw}, "/sort")
			if err != nil {
				t.Fatalf("a custom sort does not parse: %v", err)
			}
			group, err := view.ParseGroupBy(map[string]any{"field": view.CustomFieldPrefix + key}, "/group_by")
			if err != nil {
				t.Fatalf("a custom grouping does not parse: %v", err)
			}
			spec, err := view.Spec{Filter: node, Sort: custom, GroupBy: group}.Shaped(fuzzShapes(node))
			if err != nil {
				continue
			}
			sorted, err := Rows(repository.ItemSearch{Anchor: search.Anchor, Spec: spec},
				Boundary{Keys: []string{keyNull}, ID: collection}, 51)
			if err != nil {
				t.Fatalf("a custom sort did not compile: %v", err)
			}
			assertParameters(t, sorted)
			assertAlphabet(t, sorted, document)
			grouped, err := Groups(repository.ItemSearch{Anchor: search.Anchor, Spec: spec}, 51)
			if err != nil {
				t.Fatalf("a custom grouping did not compile: %v", err)
			}
			assertParameters(t, grouped)
			assertAlphabet(t, grouped, document)
			counted, err := Count(repository.ItemSearch{Anchor: search.Anchor, Spec: spec})
			if err != nil {
				t.Fatalf("a custom count did not compile: %v", err)
			}
			assertParameters(t, counted)
			assertAlphabet(t, counted, document)

			// The key as an aggregation's measure, grouping and bucket, each where its shape allows.
			for _, raw := range []map[string]any{
				{"function": "SUM", "field": view.CustomFieldPrefix + key},
				{"function": "COUNT", "group_by": view.CustomFieldPrefix + key},
				{"function": "COUNT", "bucket": map[string]any{"field": view.CustomFieldPrefix + key, "period": "day"}},
			} {
				aggregation, err := view.ParseAggregation(raw, "/aggregate")
				if err != nil {
					t.Fatalf("a custom aggregation does not parse: %v", err)
				}
				aggregation, err = aggregation.Shaped(fuzzShapes(node))
				if err != nil {
					continue
				}
				summary, err := Aggregate(repository.ItemSearch{Anchor: search.Anchor, Spec: spec},
					aggregation, document, 5001)
				if err != nil {
					t.Fatalf("a custom aggregation did not compile: %v", err)
				}
				assertParameters(t, summary)
				assertAlphabet(t, summary, document)
			}
		}
	})
}

// fuzzShapes gives every custom key a shape chosen by its length, so the corpus walks every typed
// column and the undefined key without the shape depending on anything but the name.
func fuzzShapes(node *view.Node) map[string]view.CustomShape {
	shapes := map[string]view.CustomShape{}
	if node == nil {
		return shapes
	}
	columns := []view.CustomColumn{
		view.CustomNumber, view.CustomInstant, view.CustomText, view.CustomRef, "",
	}
	for _, key := range (view.Spec{Filter: node}).CustomKeys() {
		column := columns[len(key)%len(columns)]
		if column == "" {
			shapes[key] = view.CustomShape{Resolved: true}
			continue
		}
		// A text key of even length holds several values, so the set operators meet a set.
		set := column == view.CustomText && len(key)%2 == 0
		shapes[key] = view.CustomShape{Resolved: true, Defined: true, Column: column, Set: set}
	}
	return shapes
}

func searchFor(node *view.Node, sort []view.SortTerm) repository.ItemSearch {
	return repository.ItemSearch{
		Anchor: repository.Anchor{
			Kind: repository.AnchorCollection, CollectionID: collection, IncludeDescendants: true,
		},
		Spec: view.Spec{Filter: node, Sort: sort},
	}
}

// fixedMoment is the timestamp every scrubbed value takes. Any constant does, as long as it is one.
var fixedMoment = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// assertParameters is property 2: as many placeholders as arguments, numbered without a gap.
func assertParameters(t *testing.T, statement Statement) {
	t.Helper()

	if strings.Count(statement.SQL, "$") != len(statement.Args) {
		t.Fatalf("%d placeholders, %d arguments:\n  %s",
			strings.Count(statement.SQL, "$"), len(statement.Args), statement.SQL)
	}
	for index := range statement.Args {
		placeholder := "$" + strconv.Itoa(index+1)
		if !strings.Contains(statement.SQL, placeholder) {
			t.Fatalf("%s is bound and never referred to:\n  %s", placeholder, statement.SQL)
		}
	}
}

// assertShapeOnly is property 3, and the whole of ADR-0026 in one comparison: the same tree with
// every value replaced compiles to the same text.
func assertShapeOnly(t *testing.T, node *view.Node, sort []view.SortTerm, statement Statement) {
	t.Helper()
	if node == nil {
		// No filter at all: there are no values for the text to depend on.
		return
	}

	scrubbed := scrub(*node)
	other, err := Rows(searchFor(&scrubbed, sort), Boundary{}, 51)
	if err != nil {
		t.Fatalf("the same filter with fixed values did not compile: %v", err)
	}
	if other.SQL != statement.SQL {
		t.Fatalf("the statement depends on the values in it:\n  %s\n  %s", statement.SQL, other.SQL)
	}
}

// assertAlphabet is property 4. The set is the characters this package's own fragments are written
// in; anything else in the output arrived from somewhere it should not have.
func assertAlphabet(t *testing.T, statement Statement, document string) {
	t.Helper()

	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" +
		` _,.()<>=$*'":[]@`
	for _, character := range statement.SQL {
		if !strings.ContainsRune(alphabet, character) {
			t.Fatalf("the statement contains %q, which this package never writes:\n  %s\n  from %s",
				character, statement.SQL, document)
		}
	}
}

// scrub replaces every value in a tree with a fixed one of the same kind, keeping the shape.
func scrub(node view.Node) view.Node {
	if !node.IsLeaf() {
		children := make([]view.Node, 0, len(node.Nodes))
		for _, child := range node.Nodes {
			children = append(children, scrub(child))
		}
		return view.Node{Op: node.Op, Nodes: children}
	}

	values := make([]view.Value, 0, len(node.Values))
	for _, value := range node.Values {
		values = append(values, fixedValue(value))
	}
	return view.Node{Op: node.Op, Field: node.Field, Values: values}
}

func fixedValue(value view.Value) view.Value {
	fixed := view.Value{Kind: value.Kind}
	switch value.Kind {
	case view.KindID, view.KindIDSet:
		fixed.ID = label
	case view.KindEnum:
		// An enum value has to stay one the column accepts, and there is only one enum field.
		fixed.Text = "TASK"
	case view.KindBool:
		fixed.Bool = true
	case view.KindInt:
		fixed.Int = 42
	case view.KindTimestamp:
		fixed.Time = fixedMoment
	default:
		fixed.Text = "fixed"
	}
	return fixed
}

func hasPlaceholder(node *view.Node) bool {
	if node == nil {
		return false
	}
	if !node.IsLeaf() {
		for _, child := range node.Nodes {
			if hasPlaceholder(&child) {
				return true
			}
		}
		return false
	}
	for _, value := range node.Values {
		if value.IsPlaceholder() {
			return true
		}
	}
	return false
}
