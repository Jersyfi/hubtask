// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"math"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The keyset pagination every list in this adapter shares.
//
// It lives here rather than once per repository because the two halves have to agree with each
// other and with the SQL: the page reads one row beyond what the caller asked for, and the cursor it
// reports is built from the last row it *keeps*. Writing that twice is how the two lists come to
// disagree about whether a page of exactly `size` rows has a successor.

// maxPageProbe bounds what can be asked of the database, whatever arrives from above.
//
// The use case clamps the size to the contract's maximum (api-guidelines.md §4), so this is the
// second bound rather than the first: an adapter that trusted its caller's arithmetic would turn a
// defect one layer up into an unbounded query. The probe is size+1, so the ceiling leaves room for
// the extra row without overflowing int32.
const maxPageProbe = 1000

// pageProbe is how many rows to ask for: one more than the page, which is what answers "is there
// another page" without a second query or a COUNT.
func pageProbe(size int) int32 {
	if size < 1 {
		// Not an error to correct here. The use case decides the default, and a zero reaching this
		// far means it did not - one row is then the smallest honest answer, and the empty page it
		// produces is easier to diagnose than an unbounded read.
		size = 1
	}
	if size > maxPageProbe {
		size = maxPageProbe
	}
	return int32(math.Min(float64(size+1), float64(maxPageProbe+1)))
}

// after is a decoded cursor in the form the generated parameters take: both halves absent for the
// first page, which is what makes the SQL's `cursor_order_key IS NULL` mean "start at the beginning".
type after struct {
	sortKey *string
	id      pgtype.UUID
}

// cursorAfter decodes the boundary to continue from. An empty cursor is the first page rather than
// an error: a client walking a list starts without one.
func cursorAfter(cursors security.CursorCodec, cursor string) (after, error) {
	if cursor == "" {
		return after{}, nil
	}

	position, err := cursors.Decode(cursor)
	if err != nil {
		return after{}, err
	}
	id, err := uuidOf(position.ID)
	if err != nil {
		return after{}, err
	}
	// Taken by address because the column is nullable in the query and the generated parameter is
	// therefore a pointer. The value is a copy, so the loop variable of no caller can reach it.
	sortKey := position.SortKey
	return after{sortKey: &sortKey, id: id}, nil
}

// pageOf cuts the probe row off and reports the walk's state.
//
// The cursor is built from the last row *kept*, not from the row that was read beyond the page.
// Building it from the probe row would skip that row on the next request - the classic off-by-one of
// keyset pagination, and one that shows up as a single missing entry in a long list rather than as a
// failure.
//
// A page that is not full has no successor and therefore no cursor. That is what lets a client stop
// on `has_more` alone instead of paging once more to discover the end.
func pageOf[T any](
	rows []T, size int, cursors security.CursorCodec, boundary func(T) security.Position,
) ([]T, repository.PageInfo) {
	if size < 1 || len(rows) <= size {
		return rows, repository.PageInfo{}
	}

	kept := rows[:size]
	return kept, repository.PageInfo{
		NextCursor: cursors.Encode(boundary(kept[len(kept)-1])),
		HasMore:    true,
	}
}
