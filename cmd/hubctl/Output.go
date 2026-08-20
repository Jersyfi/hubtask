// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	openapitypes "github.com/oapi-codegen/runtime/types"
)

// Table is what a command shows a person: a header and its rows, already turned into text.
//
// Formatting decisions - which fields, how a timestamp reads - belong to the command, because
// only the command knows what its answer is about. This type only aligns columns.
type Table struct {
	Columns []string
	Rows    [][]string
}

// Emit prints one command's answer.
//
// Two shapes, and the flag decides which: the documented payload for a pipe, a table for a
// person. What makes the first one usable is not the shape of the JSON but the discipline around
// it - exactly one document on standard output, nothing else on standard output ever, and every
// diagnostic on standard error. A script can then run `hubctl --json item ls | jq` and be sure
// that what it parses is either a document or nothing at all.
func (cli *CLI) Emit(payload any, table Table) error {
	if cli.JSON {
		return cli.emitJSON(payload)
	}
	cli.emitTable(table)
	return nil
}

func (cli *CLI) emitJSON(payload any) error {
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("printing the answer: %w", err)
	}
	printf(cli.Out, "%s\n", encoded)
	return nil
}

// emitTable writes the header even when there are no rows. An empty table says "nothing here" in
// the same shape as a full one; a blank screen says the command may not have run.
func (cli *CLI) emitTable(table Table) {
	tabbed := tabwriter.NewWriter(cli.Out, 0, 0, 3, ' ', 0)
	printf(tabbed, "%s\n", strings.Join(upper(table.Columns), "\t"))
	for _, row := range table.Rows {
		printf(tabbed, "%s\n", strings.Join(row, "\t"))
	}
	_ = tabbed.Flush()
}

func upper(columns []string) []string {
	loud := make([]string, len(columns))
	for i, column := range columns {
		loud[i] = strings.ToUpper(column)
	}
	return loud
}

// shortTime is how an instant reads in a table: local, to the minute, because a person reading a
// list wants to know which day and roughly when, and the API's own RFC 3339 is there under --json
// for anything that needs the exact value.
func shortTime(instant *time.Time) string {
	if instant == nil || instant.IsZero() {
		return "-"
	}
	return instant.Local().Format("2006-01-02 15:04")
}

// id renders an identifier for a table, and a missing one as a dash rather than as a row of
// zeroes that reads like a real UUID.
func id(value *openapitypes.UUID) string {
	if value == nil {
		return "-"
	}
	return value.String()
}

// text renders an optional string, empty and absent alike as a dash: a column that is sometimes
// blank and sometimes missing looks like two different things when it is one.
func text(value *string) string {
	if value == nil || *value == "" {
		return "-"
	}
	return *value
}

// optional turns a flag that was not typed into a field that is not sent. The API distinguishes
// an absent field from an empty one - a merge patch is built on that distinction - so a CLI that
// sent "" for every flag nobody used would be asking for something else entirely.
func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// yesNo renders an optional flag. Absent means the server did not say, which is not the same as
// false.
func yesNo(value *bool) string {
	switch {
	case value == nil:
		return "-"
	case *value:
		return "yes"
	default:
		return "no"
	}
}
