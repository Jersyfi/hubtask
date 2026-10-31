// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-WRK-17 check 12: a line is another spelling of the filter tree and the sort, read into the
// very document the structured query sends.
func TestALineIsReadIntoTheFilterDocument(t *testing.T) {
	cases := []struct {
		line   string
		filter any
		sort   []any
	}{
		{line: "", filter: nil},
		{line: "status = open", filter: map[string]any{"field": "status", "op": "EQ", "value": "open"}},
		{
			line: "status = open AND custom_fields.priority >= 3 ORDER BY due_at",
			filter: map[string]any{"op": "AND", "nodes": []any{
				map[string]any{"field": "status", "op": "EQ", "value": "open"},
				map[string]any{"field": "custom_fields.priority", "op": "GTE", "value": float64(3)},
			}},
			sort: []any{map[string]any{"field": "due_at"}},
		},
		{
			line: "(status != done OR is_completed = false) and not title ~ 'ord\\'er'",
			filter: map[string]any{"op": "AND", "nodes": []any{
				map[string]any{"op": "OR", "nodes": []any{
					map[string]any{"field": "status", "op": "NEQ", "value": "done"},
					map[string]any{"field": "is_completed", "op": "EQ", "value": false},
				}},
				map[string]any{"op": "NOT", "nodes": []any{map[string]any{"field": "title", "op": "CONTAINS", "value": "ord'er"}}},
			}},
		},
		{
			line:   "assignee_id IN (@me, '0190a8f0-0000-7000-8000-000000000001')",
			filter: map[string]any{"field": "assignee_id", "op": "IN", "value": []any{"@me", "0190a8f0-0000-7000-8000-000000000001"}},
		},
		{line: "due_at IS EMPTY", filter: map[string]any{"field": "due_at", "op": "IS_NULL"}},
		{line: "due_at IS NOT EMPTY", filter: map[string]any{"op": "NOT", "nodes": []any{map[string]any{"field": "due_at", "op": "IS_NULL"}}}},
		{line: "depth BETWEEN 1 AND 2", filter: map[string]any{"field": "depth", "op": "BETWEEN", "value": []any{float64(1), float64(2)}}},
		{line: "labels CONTAINS ANY (a, b)", filter: map[string]any{"field": "labels", "op": "CONTAINS_ANY", "value": []any{"a", "b"}}},
		{line: "title STARTS WITH 'Q4'", filter: map[string]any{"field": "title", "op": "STARTS_WITH", "value": "Q4"}},
		{line: "status NOT IN (done)", filter: map[string]any{"field": "status", "op": "NOT_IN", "value": []any{"done"}}},
		{line: "due_at < @today+P3D", filter: map[string]any{"field": "due_at", "op": "LT", "value": "@today+P3D"}},
		{
			line: "ORDER BY due_at DESC NULLS FIRST, title",
			sort: []any{map[string]any{"field": "due_at", "dir": "DESC", "nulls": "FIRST"}, map[string]any{"field": "title"}},
		},
	}
	for _, c := range cases {
		filter, sort, err := ParseLine(c.line, "/q")
		if err != nil {
			t.Errorf("%q: %v", c.line, err)
			continue
		}
		if !reflect.DeepEqual(filter, c.filter) || !reflect.DeepEqual(sort, c.sort) {
			t.Errorf("%q read as %#v %#v", c.line, filter, sort)
		}
	}
}

// UC-WRK-17 check 12: what the line cannot say is refused with where it stopped, and what the tree
// cannot hold is refused by the tree's own rules, not by a second copy of them.
func TestALineThatIsNotOneIsRefusedWhereItStops(t *testing.T) {
	cases := map[string]string{
		"status =":                           "query.line_malformed",
		"status = 'open":                     "query.line_malformed",
		"(status = open":                     "query.line_malformed",
		"status open":                        "query.line_malformed",
		"AND = open":                         "query.line_malformed",
		"status = open ORDER due_at":         "query.line_malformed",
		"status ! open":                      "query.line_malformed",
		strings.Repeat("(", 40) + "a = b":    "query.line_malformed",
		strings.Repeat("x", MaxLineLength+1): "query.line_too_long",
	}
	for line, code := range cases {
		_, _, err := ParseLine(line, "/q")
		if !errors.Is(err, shared.ErrValidation) || !strings.Contains(err.Error(), code) {
			t.Errorf("%.40q answered %v, want %s", line, err, code)
		}
	}
	// The tree's own refusals: an unknown field, an operator the field does not take, the depth.
	for line, code := range map[string]string{
		"colour = red":                  "query.field_unknown",
		"is_completed ~ yes":            "query.operator_unsupported",
		"((((((a = b))))))":             "query.field_unknown",
		"NOT NOT NOT NOT NOT title = a": "query.filter_too_deep",
	} {
		filter, _, err := ParseLine(line, "/q")
		if err == nil {
			_, err = ParseFilter(filter, "/q")
		}
		if err == nil || !strings.Contains(err.Error(), code) {
			t.Errorf("%q answered %v, want %s", line, err, code)
		}
	}
}

// A saved filter shared as one line reads back to the same document.
func TestALineIsWrittenBackToTheSameDocument(t *testing.T) {
	for _, line := range []string{
		"status = 'open' AND (custom_fields.priority >= 3 OR title CONTAINS 'it\\'s') ORDER BY due_at DESC",
		"NOT (status IN ('a', 'b') AND due_at IS EMPTY)",
		"depth BETWEEN 1 AND 2 AND labels CONTAINS ALL ('x')",
		"title ~ 'back\\\\slash \"and\" quote\\''",
	} {
		if _, _, err := ParseLine(line, "/q"); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		assertRoundTrip(t, line)
	}
}

func assertRoundTrip(t *testing.T, line string) {
	t.Helper()
	filter, sort, err := ParseLine(line, "/q")
	if err != nil {
		return
	}
	written, ok := FormatLine(filter, sort)
	if !ok {
		t.Fatalf("%q: read but not written back", line)
	}
	again, sortAgain, err := ParseLine(written, "/q")
	if err != nil || !reflect.DeepEqual(again, filter) || !reflect.DeepEqual(sortAgain, sort) {
		t.Fatalf("%q written as %q read back as %#v %#v (%v)", line, written, again, sortAgain, err)
	}
}

// FuzzParseLine: no line panics, every refusal is a validation error, and every line read is
// written back to the same document.
func FuzzParseLine(f *testing.F) {
	for _, seed := range []string{
		"status = open AND custom_fields.priority >= 3 ORDER BY due_at",
		"NOT (a IN (1, 2) OR b IS NOT EMPTY)", "title ~ 'x\\'y'", "((((", "a BETWEEN 1 AND", "ORDER BY",
		"x = \"é\"", "depth > -1", "a = @me", strings.Repeat("NOT ", 30) + "a = b",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, line string) {
		filter, _, err := ParseLine(line, "/q")
		if err != nil {
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("%q refused as %v", line, err)
			}
			return
		}
		if _, err := ParseFilter(filter, "/q"); err != nil && !errors.Is(err, shared.ErrValidation) {
			t.Fatalf("%q's tree refused as %v", line, err)
		}
		assertRoundTrip(t, line)
	})
}
