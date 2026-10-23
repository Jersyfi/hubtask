// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "slices"

// StartingWipLimit is the limit a board-first collection starts with on the column of the work
// in progress: UC-WRK-31's story agrees on three, and anybody may change or clear it.
const StartingWipLimit = 3

// StartingColumn is a column a new collection's board starts with. Its name is a message code
// (rule 8): the creating use case renders it in the workspace's language, and from then on the
// name is content like any other.
type StartingColumn struct {
	NameCode     string
	ColorToken   string
	IsDoneBucket bool
	// WipLimit is zero for no limit, as NewBucketInput reads it.
	WipLimit int
}

// StartingColumns answers the columns a new collection starts with under the structure in force
// (UC-WRK-31 check 1): where the board is offered, the states to do, in progress and done - the
// last completing what enters it -, and where the board is the default view, as on *Flow*, a
// limit on the work in progress. A collection whose structure offers no board starts with none.
func StartingColumns(values StructureValues) []StartingColumn {
	if values.Views == nil || !slices.Contains(values.Views.Offered, ViewKanban) {
		return nil
	}
	limit := 0
	if values.Views.Default == ViewKanban {
		limit = StartingWipLimit
	}
	return []StartingColumn{
		{NameCode: "seed.bucket.todo", ColorToken: "slate"},
		{NameCode: "seed.bucket.doing", ColorToken: "blue", WipLimit: limit},
		{NameCode: "seed.bucket.done", ColorToken: "green", IsDoneBucket: true},
	}
}
