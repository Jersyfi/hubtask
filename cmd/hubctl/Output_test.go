// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	openapitypes "github.com/oapi-codegen/runtime/types"
)

func emitter(asJSON bool) (*CLI, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	return &CLI{Streams: Streams{Out: &out, Err: &errOut}, JSON: asJSON}, &out, &errOut
}

// The property a script depends on: one document on standard output and nothing else on it.
func TestJSONOutputIsOneDocumentAndNothingElse(t *testing.T) {
	cli, out, errOut := emitter(true)
	payload := map[string]any{"data": []any{map[string]any{"id": "01J8", "name": "Hub"}}}

	if err := cli.Emit(payload, Table{Columns: []string{"id"}, Rows: [][]string{{"01J8"}}}); err != nil {
		t.Fatalf("emitting: %v", err)
	}
	if errOut.Len() != 0 {
		t.Errorf("standard error carried %q", errOut.String())
	}

	decoder := json.NewDecoder(strings.NewReader(out.String()))
	var first any
	if err := decoder.Decode(&first); err != nil {
		t.Fatalf("the output is not a JSON document: %v", err)
	}
	var second any
	if err := decoder.Decode(&second); err == nil {
		t.Error("a second document followed the first")
	}
	if !strings.HasSuffix(out.String(), "\n") {
		t.Error("the document is not terminated by a newline")
	}
}

func TestTableOutputAlignsAndShoutsItsHeader(t *testing.T) {
	cli, out, _ := emitter(false)

	if err := cli.Emit(nil, Table{
		Columns: []string{"id", "name"},
		Rows:    [][]string{{"01J8", "Hub"}, {"01J9", "A much longer name"}},
	}); err != nil {
		t.Fatalf("emitting: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d lines, want a header and two rows: %q", len(lines), out.String())
	}
	if !strings.HasPrefix(lines[0], "ID") || !strings.Contains(lines[0], "NAME") {
		t.Errorf("header %q", lines[0])
	}
	if strings.Index(lines[1], "Hub") != strings.Index(lines[2], "A much longer name") {
		t.Errorf("the second column is not aligned:\n%s", out.String())
	}
}

// An empty table still says which columns it would have had. A blank screen reads like a command
// that did not run.
func TestAnEmptyTableStillHasItsHeader(t *testing.T) {
	cli, out, _ := emitter(false)
	if err := cli.Emit(nil, Table{Columns: []string{"id", "title"}}); err != nil {
		t.Fatalf("emitting: %v", err)
	}
	if !strings.Contains(out.String(), "TITLE") {
		t.Errorf("output %q", out.String())
	}
}

func TestTheTableHelpersRenderAbsenceAsADash(t *testing.T) {
	empty := ""
	filled := "value"
	yes := true
	no := false
	identifier := openapitypes.UUID{}

	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"an absent identifier", id(nil), "-"},
		{"an identifier", id(&identifier), identifier.String()},
		{"an absent string", text(nil), "-"},
		{"an empty string", text(&empty), "-"},
		{"a string", text(&filled), "value"},
		{"an absent flag", yesNo(nil), "-"},
		{"a set flag", yesNo(&yes), "yes"},
		{"an unset flag", yesNo(&no), "no"},
		{"an absent instant", shortTime(nil), "-"},
		{"the zero instant", shortTime(&time.Time{}), "-"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestAnInstantIsShownInTheReadersOwnTimeZone(t *testing.T) {
	instant := time.Date(2026, 8, 20, 14, 30, 0, 0, time.UTC)
	want := instant.Local().Format("2006-01-02 15:04")
	if got := shortTime(&instant); got != want {
		t.Errorf("%q, want %q", got, want)
	}
}
