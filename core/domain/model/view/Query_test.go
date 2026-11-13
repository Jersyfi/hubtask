// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"reflect"
	"strings"
	"testing"
)

// The formatter refuses a tree no text spells rather than writing one that reads otherwise.
func TestATreeWithoutATextSpellingIsNotWritten(t *testing.T) {
	for _, filter := range []any{
		map[string]any{"field": "custom_fields.x", "op": "CONTAINS_ANY", "value": []any{"a", "b"}},
		map[string]any{"op": "AND", "nodes": []any{}},
		map[string]any{"field": "nowhere", "op": "EQ", "value": "x"},
	} {
		if text, ok := FormatQuery(Query{Filter: filter}); ok {
			t.Fatalf("%v written as %q", filter, text)
		}
	}
}

// A line longer than the bound or not UTF-8 is refused before it is read.
func TestAQueryOutsideItsBoundsIsRefused(t *testing.T) {
	if _, err := ParseQuery(strings.Repeat("a ", MaxLineLength), "/q"); err == nil {
		t.Fatal("a long query was read")
	}
	if _, err := ParseQuery("who: \xff", "/q"); err == nil {
		t.Fatal("invalid UTF-8 was read")
	}
	if _, err := ParseQuery(strings.Repeat("(", 30)+"a"+strings.Repeat(")", 30), "/q"); err == nil {
		t.Fatal("deep brackets were read")
	}
}

func FuzzParseQuery(f *testing.F) {
	for _, seed := range []string{"who: me, none", "(a OR b) -c", "due: 2026-01-01..+2w sort: -due", "NOT -(x"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		q, err := ParseQuery(text, "/q")
		if err != nil {
			return
		}
		if formatted, ok := FormatQuery(q); ok {
			again, err := ParseQuery(formatted, "/q")
			if err != nil || !reflect.DeepEqual(again, q) {
				t.Fatalf("%q -> %q reads otherwise: %v", text, formatted, err)
			}
		}
	})
}
