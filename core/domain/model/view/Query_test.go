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

// UC-WRK-43 check 2: every key the catalogue serves is one the parser reads, with examples that
// read, and its field is one the grammar knows.
func TestEveryCatalogueKeyIsRead(t *testing.T) {
	for _, key := range QueryKeys() {
		if key.Field != "" && key.Field != CustomFieldPrefix {
			_, ofAKind := kindField(key.Field)
			if _, known := FieldByName(key.Field); !known && !ofAKind {
				t.Fatalf("%s names the unknown field %s", key.Key, key.Field)
			}
		}
		for _, example := range key.Examples {
			if strings.Contains(example, "here") {
				continue
			}
			if _, err := ParseQuery(example, "/q"); err != nil {
				t.Fatalf("%s: the example %q is refused: %v", key.Key, example, err)
			}
		}
	}
}

// Free words join the tree as a MATCHES leaf for a query of entries; another kind is refused.
func TestAQueryOfEntriesTakesItsWordsIntoTheTree(t *testing.T) {
	q, err := ParseQuery("passkey who: me", "/q")
	if err != nil {
		t.Fatal(err)
	}
	filter, err := q.EntryFilter("/q")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"op": "AND", "nodes": []any{
		map[string]any{"field": "assignee_id", "op": "EQ", "value": "@me"},
		map[string]any{"field": "text", "op": "MATCHES", "value": "passkey"},
	}}
	if !reflect.DeepEqual(filter, want) {
		t.Fatalf("got %v", filter)
	}
	q, _ = ParseQuery("type: page", "/q")
	if _, err := q.EntryFilter("/q"); err == nil {
		t.Fatal("a page was asked of entries")
	}
}
