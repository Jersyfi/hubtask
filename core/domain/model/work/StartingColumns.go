// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "slices"

// StartingWipLimit is the limit a board-first collection starts with on the column of the work
// in progress: UC-WRK-31's story agrees on three, and anybody may change or clear it.
const StartingWipLimit = 3

// StartingColumn is a column a new collection's board starts with. Its name is a message code
// (rule 8): the creating use case renders it in the acting person's language, else the
// workspace's, and from then on the name is content like any other.
type StartingColumn struct {
	NameCode string
	// Name is the column's name as given - a status's own name -, used in place of NameCode.
	Name string
	// Status is the status the column shows under a workflow; empty without one.
	Status       string
	ColorToken   string
	IsDoneBucket bool
	// WipLimit is zero for no limit, as NewBucketInput reads it.
	WipLimit int
}

// StartingColumns answers the columns a new collection starts with under the structure in force
// (UC-WRK-31 check 1): where the board is the default view, as on *Flow*, the states to do, in
// progress and done - the last completing what enters it - and a limit on the work in progress.
// A board only offered beside a list starts as a list, without columns (L0, §3.5a).
func StartingColumns(values StructureValues) []StartingColumn {
	if values.Views == nil || values.Views.Default != ViewKanban || !slices.Contains(values.Views.Offered, ViewKanban) {
		return nil
	}
	limit := StartingWipLimit
	return []StartingColumn{
		{NameCode: "seed.bucket.todo", ColorToken: "slate"},
		{NameCode: "seed.bucket.doing", ColorToken: "blue", WipLimit: limit},
		{NameCode: "seed.bucket.done", ColorToken: "green", IsDoneBucket: true},
	}
}

// WorkflowStartingColumns answers the columns a collection created under a workflow starts with
// (§3.5a): one per status in the workflow's order, each showing its status, named as the status -
// or, for a status of the system workflow that has no name, by its category -, the first done
// status's column marked done.
func WorkflowStartingColumns(shape WorkflowShape) []StartingColumn {
	words := map[StatusCategory]StartingColumn{
		CategoryBacklog:    {NameCode: "seed.bucket.todo", ColorToken: "slate"},
		CategoryTodo:       {NameCode: "seed.bucket.todo", ColorToken: "slate"},
		CategoryInProgress: {NameCode: "seed.bucket.doing", ColorToken: "blue"},
		CategoryDone:       {NameCode: "seed.bucket.done", ColorToken: "green"},
	}
	columns := make([]StartingColumn, 0, len(shape.Statuses))
	marked := false
	for _, status := range shape.Statuses {
		column := words[status.Category]
		column.Status = status.Key
		if status.Name != "" {
			column.Name, column.NameCode = status.Name, ""
		}
		if status.ColorToken != "" {
			column.ColorToken = status.ColorToken
		}
		if status.Category.Completes() && !marked {
			column.IsDoneBucket, marked = true, true
		}
		columns = append(columns, column)
	}
	return columns
}
