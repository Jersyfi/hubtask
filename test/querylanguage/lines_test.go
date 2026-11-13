// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package querylanguage holds the Hubtask query's shared test lines (lines.json) and the test that
// holds the server's parser to them; the browser's twin in packages/query reads the same file.
package querylanguage

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// sharedLine is one row of test/querylanguage/lines.json, the file the browser's twin reads too.
type sharedLine struct {
	Line  string `json:"line"`
	Query *struct {
		Filter          any      `json:"filter"`
		Sort            []any    `json:"sort"`
		Words           string   `json:"words"`
		Kinds           []string `json:"kinds"`
		IncludeArchived bool     `json:"include_archived"`
		IncludeTrashed  bool     `json:"include_trashed"`
	} `json:"query"`
	Canonical string `json:"canonical"`
	Error     *struct {
		Code       string `json:"code"`
		At         int    `json:"at"`
		Suggestion string `json:"suggestion"`
	} `json:"error"`
}

func sharedLines(t *testing.T) []sharedLine {
	t.Helper()
	raw, err := os.ReadFile("lines.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Lines []sharedLine `json:"lines"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	return file.Lines
}

// normal passes a value through JSON, so a tree built here compares with one read from the file.
func normal(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// UC-WRK-43 check 1: both spellings read into the filter tree and the sort, free words apart, and
// the `field: value` text written back reads into the same query (Q-1).
func TestTheSharedQueryLinesReadAndWriteBack(t *testing.T) {
	for _, row := range sharedLines(t) {
		t.Run(row.Line, func(t *testing.T) {
			got, err := view.ParseQuery(row.Line, "/q")
			if row.Error != nil {
				var domainErr *shared.Error
				if !errors.As(err, &domainErr) || len(domainErr.Fields) != 1 {
					t.Fatalf("want %s, got %v", row.Error.Code, err)
				}
				f := domainErr.Fields[0]
				if f.Code != row.Error.Code || f.Params["at"] != itoa(row.Error.At) || f.Params["suggestion"] != row.Error.Suggestion {
					t.Fatalf("want %+v, got %s %v", *row.Error, f.Code, f.Params)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := row.Query
			if !reflect.DeepEqual(normal(t, got.Filter), want.Filter) || !reflect.DeepEqual(normal(t, got.Sort), normal(t, want.Sort)) ||
				got.Words != want.Words || !reflect.DeepEqual(got.Kinds, want.Kinds) ||
				got.IncludeArchived != want.IncludeArchived || got.IncludeTrashed != want.IncludeTrashed {
				t.Fatalf("read\n%+v\nwant\n%+v", got, *want)
			}
			// A key of one kind is held to that kind's search, every other tree to the entries'.
			if kind, err := got.KindFilter("/q"); err != nil {
				t.Fatalf("the kind's search refuses the tree: %v", err)
			} else if kind.Kind == view.ObjectEntry {
				if _, err := view.ParseFilter(normal(t, got.Filter), "/q"); err != nil {
					t.Fatalf("the grammar refuses the tree: %v", err)
				}
			}
			text, ok := view.FormatQuery(got)
			if !ok || text != row.Canonical {
				t.Fatalf("canonical %q, want %q", text, row.Canonical)
			}
			again, err := view.ParseQuery(text, "/q")
			if err != nil || !reflect.DeepEqual(normal(t, again), normal(t, got)) {
				t.Fatalf("the canonical text reads into another query: %+v %v", again, err)
			}
		})
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
