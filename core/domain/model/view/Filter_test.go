// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The grammar is the whole security boundary of the query path: what reaches the compiler is what
// passed through here, so a case this file does not cover is a case the compiler has to be trusted
// about (ADR-0026). The table is therefore about refusals first and acceptance second.

const filterPath = "/filter"

// detailOf reads the detail code a refusal carries, which is what a client acts on.
func detailOf(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected a refusal, got none")
	}
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected a domain error, got %T: %v", err, err)
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a malformed query is the client's mistake; got the category %q", domainErr.Category)
	}
	return domainErr.DetailCode
}

// pathOf reads the JSON pointer a refusal points at. A refusal that names no field is one a form
// cannot mark, which is the whole reason the parser threads a path through itself.
func pathOf(t *testing.T, err error) string {
	t.Helper()
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || len(domainErr.Fields) == 0 {
		t.Fatalf("expected a field error, got %v", err)
	}
	return domainErr.Fields[0].Path
}

func leaf(field, op string, value any) map[string]any {
	node := map[string]any{"field": field, "op": op}
	if value != nil {
		node["value"] = value
	}
	return node
}

func TestParseFilterRefuses(t *testing.T) {
	deep := any(leaf("title", "EQ", "x"))
	for range MaxFilterDepth {
		deep = map[string]any{"op": "AND", "nodes": []any{deep}}
	}

	wide := make([]any, 0, MaxFilterNodes+1)
	for range MaxFilterNodes + 1 {
		wide = append(wide, leaf("title", "EQ", "x"))
	}

	tests := []struct {
		name     string
		document any
		code     string
		path     string
	}{
		{"a node that is not an object", "type EQ TASK", "query.node_malformed", filterPath},
		{"no operator", map[string]any{"field": "type"}, "query.operator_required", filterPath + "/op"},
		{
			"an operator that does not exist",
			leaf("title", "SOUNDS_LIKE", "x"), "query.operator_unknown", filterPath + "/op",
		},
		{"no field", map[string]any{"op": "EQ", "value": "x"}, "query.field_required", filterPath + "/field"},
		{
			"a field this installation does not serve",
			leaf("recurrence_rule_id", "EQ", "0192f000-0000-7000-8000-000000000001"),
			"query.field_unknown", filterPath + "/field",
		},
		{
			"an operator the field does not permit",
			leaf("title", "GT", "x"), "query.operator_unsupported", filterPath + "/op",
		},
		{
			"IS_NULL on a field that is never null",
			leaf("title", "IS_NULL", nil), "query.operator_unsupported", filterPath + "/op",
		},
		{
			"IS_NULL with a value",
			leaf("notes", "IS_NULL", false), "query.value_not_allowed", filterPath + "/value",
		},
		{"a leaf with no value", leaf("title", "EQ", nil), "query.value_required", filterPath + "/value"},
		{"an empty value", leaf("title", "EQ", "   "), "query.value_required", filterPath + "/value"},
		{
			"a value longer than anything stored",
			leaf("title", "CONTAINS", strings.Repeat("a", MaxValueLength+1)),
			"query.value_too_long", filterPath + "/value",
		},
		{
			"a number where a string belongs",
			leaf("title", "EQ", 7.0), "query.value_type_invalid", filterPath + "/value",
		},
		{
			"a string where a boolean belongs",
			leaf("is_completed", "EQ", "true"), "query.value_type_invalid", filterPath + "/value",
		},
		{
			"a fraction where a whole number belongs",
			leaf("depth", "LT", 2.5), "query.value_type_invalid", filterPath + "/value",
		},
		{
			"a type that is not one",
			leaf("type", "EQ", "EPIC"), "query.value_not_in_enum", filterPath + "/value",
		},
		{
			"an identifier that is not one",
			leaf("bucket_id", "EQ", "not-a-uuid"), "shared.id_malformed", filterPath + "/value",
		},
		{
			"a timestamp that is not one",
			leaf("created_at", "GT", "yesterday"), "query.timestamp_malformed", filterPath + "/value",
		},
		{
			"BETWEEN with one bound",
			leaf("depth", "BETWEEN", []any{1.0}), "query.values_arity_invalid", filterPath + "/value",
		},
		{
			"BETWEEN with a scalar",
			leaf("depth", "BETWEEN", 1.0), "query.values_required", filterPath + "/value",
		},
		{
			"IN with an empty list",
			leaf("type", "IN", []any{}), "query.values_arity_invalid", filterPath + "/value",
		},
		{
			"IN with more values than a selection can be",
			leaf("depth", "IN", tooManyValues()), "query.values_arity_invalid", filterPath + "/value",
		},
		{
			"a combination with no nodes",
			map[string]any{"op": "AND"}, "query.nodes_required", filterPath + "/nodes",
		},
		{
			"a combination with an empty node list",
			map[string]any{"op": "OR", "nodes": []any{}}, "query.nodes_required", filterPath + "/nodes",
		},
		{
			"NOT with two nodes",
			map[string]any{"op": "NOT", "nodes": []any{
				leaf("title", "EQ", "a"), leaf("title", "EQ", "b"),
			}},
			"query.not_takes_one", filterPath + "/nodes",
		},
		{
			"a combination that also names a field",
			map[string]any{"op": "AND", "field": "type", "nodes": []any{leaf("title", "EQ", "a")}},
			"query.node_ambiguous", filterPath,
		},
		{
			"a leaf that also carries nodes",
			map[string]any{"op": "EQ", "field": "title", "value": "a", "nodes": []any{}},
			"query.node_ambiguous", filterPath,
		},
		{"nesting past the limit", deep, "query.filter_too_deep", ""},
		{
			"more nodes than the bound",
			map[string]any{"op": "AND", "nodes": wide}, "query.filter_too_large", "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node, err := ParseFilter(test.document, filterPath)
			if node != nil {
				t.Errorf("a refused filter must produce no node, got %+v", node)
			}
			if code := detailOf(t, err); code != test.code {
				t.Errorf("detail code = %q, want %q", code, test.code)
			}
			if test.path != "" && pathOf(t, err) != test.path {
				t.Errorf("field path = %q, want %q", pathOf(t, err), test.path)
			}
		})
	}
}

func tooManyValues() []any {
	values := make([]any, 0, MaxValues+1)
	for i := range MaxValues + 1 {
		values = append(values, float64(i))
	}
	return values
}

func TestParseFilterAccepts(t *testing.T) {
	t.Run("no filter at all", func(t *testing.T) {
		node, err := ParseFilter(nil, filterPath)
		if err != nil || node != nil {
			t.Fatalf("an absent filter is a query for everything in scope: %+v, %v", node, err)
		}
	})

	t.Run("a leaf", func(t *testing.T) {
		node, err := ParseFilter(leaf("type", "EQ", "TASK"), filterPath)
		if err != nil {
			t.Fatalf("unexpected refusal: %v", err)
		}
		if !node.IsLeaf() || node.Op != OpEq || node.Field.Name != FieldType {
			t.Fatalf("read as %+v", node)
		}
		if len(node.Values) != 1 || node.Values[0].Text != "TASK" || node.Values[0].Kind != KindEnum {
			t.Errorf("value read as %+v", node.Values)
		}
	})

	t.Run("a scalar where a list operator expects a list", func(t *testing.T) {
		node, err := ParseFilter(leaf("type", "IN", "TASK"), filterPath)
		if err != nil {
			t.Fatalf("unexpected refusal: %v", err)
		}
		if len(node.Values) != 1 {
			t.Errorf("one value expected, got %+v", node.Values)
		}
	})

	t.Run("the shape the guidelines document", func(t *testing.T) {
		node, err := ParseFilter(map[string]any{"op": "AND", "nodes": []any{
			leaf("type", "IN", []any{"TASK"}),
			leaf("is_completed", "EQ", false),
			leaf("created_at", "LTE", "2026-08-31T23:59:59Z"),
			leaf("labels", "CONTAINS_ANY", []any{
				"0192f000-0000-7000-8000-00000000000a",
				"0192f000-0000-7000-8000-00000000000b",
			}),
			map[string]any{"op": "OR", "nodes": []any{
				leaf("created_by", "EQ", "@me"),
				leaf("notes", "IS_NULL", nil),
			}},
			map[string]any{"op": "NOT", "nodes": []any{leaf("title", "STARTS_WITH", "draft")}},
		}}, filterPath)
		if err != nil {
			t.Fatalf("unexpected refusal: %v", err)
		}
		if node.Op != OpAnd || len(node.Nodes) != 6 {
			t.Fatalf("read as %+v", node)
		}
		if got := node.Nodes[2].Values[0].Time; !got.Equal(time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC)) {
			t.Errorf("timestamp read as %s", got)
		}
		if placeholder := node.Nodes[4].Nodes[0].Values[0]; !placeholder.IsPlaceholder() {
			t.Errorf("@me was read as a literal: %+v", placeholder)
		}
	})

	t.Run("nesting exactly at the limit", func(t *testing.T) {
		document := any(leaf("title", "EQ", "x"))
		for range MaxFilterDepth - 1 {
			document = map[string]any{"op": "AND", "nodes": []any{document}}
		}
		if _, err := ParseFilter(document, filterPath); err != nil {
			t.Errorf("the limit is inclusive: %v", err)
		}
	})

	t.Run("an integer that arrived as one", func(t *testing.T) {
		for _, raw := range []any{1, int64(1), 1.0} {
			node, err := ParseFilter(leaf("depth", "EQ", raw), filterPath)
			if err != nil {
				t.Fatalf("%T refused: %v", raw, err)
			}
			if node.Values[0].Int != 1 {
				t.Errorf("%T read as %d", raw, node.Values[0].Int)
			}
		}
	})
}

// The catalogue is the contract `/meta/capabilities` publishes, so its own coherence is worth a
// test: a field that permits an operator its kind cannot answer would be one a client is invited
// to send and the compiler then refuses.
func TestTheFieldCatalogueIsCoherent(t *testing.T) {
	seen := map[string]bool{}

	for _, field := range Fields() {
		if seen[field.Name] {
			t.Errorf("%s is in the catalogue twice", field.Name)
		}
		seen[field.Name] = true

		if (field.Kind == KindEnum) != (len(field.Values) > 0) {
			t.Errorf("%s: only an enum has values, and every enum has them", field.Name)
		}
		if !field.Nullable && field.Permits(OpIsNull) {
			t.Errorf("%s permits IS_NULL and is never null", field.Name)
		}
		if field.Permits(OpMatches) && field.Kind != KindText {
			t.Errorf("%s answers MATCHES without being full text", field.Name)
		}
		for _, op := range field.Operators {
			if op.Combines() {
				t.Errorf("%s permits %s, which joins nodes rather than comparing", field.Name, op)
			}
			if !operators[op] {
				t.Errorf("%s permits %s, which is not an operator", field.Name, op)
			}
		}
		if field.Kind == KindIDSet {
			for _, op := range []Operator{OpEq, OpNeq} {
				if field.Permits(op) {
					t.Errorf("%s is a set: %s would have two meanings", field.Name, op)
				}
			}
		}
	}

	if _, known := FieldByName("Title"); known {
		t.Error("field names are identifiers of the contract, not case-insensitive labels")
	}
}
