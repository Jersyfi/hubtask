// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/query"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The query language's half of the item repository (B-12).
//
// It sits beside the generated queries rather than among them because it is the one read whose
// statement is not written in advance: the compiler next door produces it from a validated
// specification, and this file runs it on the transaction the unit of work opened. Which is the
// whole of what changes - the tenant still comes from `SET LOCAL app.tenant_id` and row level
// security applies to a compiled statement exactly as it does to a generated one (ADR-0010,
// ADR-0026).

// Query answers the query language: a filter over an anchored scope, sorted, paged, grouped.
func (r ItemRepository) Query(
	ctx context.Context, search repository.ItemSearch,
) (repository.ItemQueryResult, error) {
	tx, err := FromContext(ctx)
	if err != nil {
		return repository.ItemQueryResult{}, err
	}

	result, err := r.answer(ctx, tx, search)
	if err != nil {
		return repository.ItemQueryResult{}, err
	}
	if search.Spec.Count != view.CountExact {
		return result, nil
	}
	return r.counted(ctx, tx, search, result)
}

// answer reads the rows: one page, or one page per group.
func (r ItemRepository) answer(
	ctx context.Context, tx pgx.Tx, search repository.ItemSearch,
) (repository.ItemQueryResult, error) {
	if !search.Spec.GroupBy.IsZero() {
		return r.groups(ctx, tx, search)
	}
	return r.rows(ctx, tx, search)
}

func (r ItemRepository) rows(
	ctx context.Context, tx pgx.Tx, search repository.ItemSearch,
) (repository.ItemQueryResult, error) {
	boundary, err := r.boundary(search.Spec.Cursor)
	if err != nil {
		return repository.ItemQueryResult{}, err
	}

	size := search.Spec.Size
	statement, err := query.Rows(search, boundary, int(pageProbe(size)))
	if err != nil {
		return repository.ItemQueryResult{}, err
	}

	items, err := readItems(ctx, tx, statement)
	if err != nil {
		return repository.ItemQueryResult{}, err
	}

	result := repository.ItemQueryResult{Groups: []repository.ItemGroup{}}
	result.Items, result.Info = pageOf(items, size, r.cursors, r.positionIn(search.Spec.Sort))
	return result, nil
}

// groups reads the board projection and cuts the flat result into its columns.
//
// The rows arrive ordered by the grouping value and then by the sort, so a change of value is a
// change of group: one pass, no map, and the order the database produced is the order the client
// sees.
func (r ItemRepository) groups(
	ctx context.Context, tx pgx.Tx, search repository.ItemSearch,
) (repository.ItemQueryResult, error) {
	size := search.Spec.GroupBy.LimitPerGroup
	statement, err := query.Groups(search, int(pageProbe(size)))
	if err != nil {
		return repository.ItemQueryResult{}, err
	}

	items, err := readItems(ctx, tx, statement)
	if err != nil {
		return repository.ItemQueryResult{}, err
	}

	result := repository.ItemQueryResult{Items: []work.WorkItem{}, Groups: []repository.ItemGroup{}}
	boundary := r.positionIn(search.Spec.Sort)

	for _, item := range items {
		key, present := groupKeyOf(search.Spec.GroupBy.Field, item)
		absent := !present

		last := len(result.Groups) - 1
		if last < 0 || result.Groups[last].Key != key || result.Groups[last].Absent != absent {
			result.Groups = append(result.Groups, repository.ItemGroup{Key: key, Absent: absent})
			last++
		}
		result.Groups[last].Items = append(result.Groups[last].Items, item)
	}

	for index, group := range result.Groups {
		result.Groups[index].Items, result.Groups[index].Info = pageOf(
			group.Items, size, r.cursors, boundary)
	}
	return result, nil
}

// counted runs the second pass an exact count costs, and hands the numbers to whichever shape the
// answer took.
func (r ItemRepository) counted(
	ctx context.Context, tx pgx.Tx, search repository.ItemSearch, result repository.ItemQueryResult,
) (repository.ItemQueryResult, error) {
	statement, err := query.Count(search)
	if err != nil {
		return repository.ItemQueryResult{}, err
	}

	rows, err := tx.Query(ctx, statement.SQL, statement.Args...)
	if err != nil {
		return repository.ItemQueryResult{}, queryFailed("counting the query's rows", err)
	}
	defer rows.Close()

	grouped := !search.Spec.GroupBy.IsZero()
	for rows.Next() {
		var count int64
		if !grouped {
			if err := rows.Scan(&count); err != nil {
				return repository.ItemQueryResult{}, queryFailed("reading the count", err)
			}
			result.Total = int(count)
			continue
		}

		key, err := scanGroupKey(rows, &count)
		if err != nil {
			return repository.ItemQueryResult{}, err
		}
		result.Total += int(count)
		for index := range result.Groups {
			if result.Groups[index].Key == key {
				result.Groups[index].Total = int(count)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return repository.ItemQueryResult{}, queryFailed("counting the query's rows", err)
	}
	return result, nil
}

// scanGroupKey reads one row of the grouped count: the value, rendered the way the rows themselves
// render it, and how many entries share it.
//
// The value is scanned as text whatever its column type, because the group key is text everywhere
// above this line - a uuid, an item type and a boolean all become the string a client compares
// against the key it was given.
func scanGroupKey(rows pgx.Rows, count *int64) (string, error) {
	var key *string
	if err := rows.Scan(&key, count); err != nil {
		return "", queryFailed("reading a group's count", err)
	}
	if key == nil {
		return "", nil
	}
	return *key, nil
}

// readItems runs a compiled statement and maps every row.
//
// The projection is fixed by the compiler and matches sqlc's FindWorkItemRow field for field, so
// the mapping is the same itemFrom every other read in this adapter uses. A column added to one
// and not the other fails on the first row rather than producing a wrong answer.
func readItems(ctx context.Context, tx pgx.Tx, statement query.Statement) ([]work.WorkItem, error) {
	rows, err := tx.Query(ctx, statement.SQL, statement.Args...)
	if err != nil {
		return nil, queryFailed("running the item query", err)
	}
	defer rows.Close()

	items := make([]work.WorkItem, 0, 64)
	for rows.Next() {
		var row sqlc.FindWorkItemRow
		if err := rows.Scan(
			&row.ID, &row.TenantID, &row.CollectionID, &row.Type, &row.ParentID, &row.Path,
			&row.Depth, &row.Title, &row.Notes, &row.IsCompleted, &row.CompletedAt, &row.CompletedBy,
			&row.BucketID, &row.OrderKey, &row.AssigneeID,
			&row.StartAt, &row.DueAt, &row.DueDateOnly, &row.DueTimeZone,
			&row.CoverKind, &row.CoverColorToken, &row.CoverMediaID, &row.CustomFields,
			&row.ContentLanguage,
			&row.ArchivedAt, &row.DeletedAt,
			&row.TrashBatchID, &row.CreatedBy, &row.CreatedAt, &row.UpdatedAt, &row.Version,
		); err != nil {
			return nil, queryFailed("reading a queried item", err)
		}
		item, err := itemFrom(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, queryFailed("running the item query", err)
	}
	return items, nil
}

// boundary decodes the cursor the caller is continuing from.
func (r ItemRepository) boundary(cursor string) (query.Boundary, error) {
	if cursor == "" {
		return query.Boundary{}, nil
	}
	position, err := r.cursors.Decode(cursor)
	if err != nil {
		return query.Boundary{}, err
	}
	return query.Boundary{Keys: position.Keys, ID: position.ID}, nil
}

// positionIn renders a row as the boundary of the walk it ends: one key per sort term, in the sort's
// own order, and the identifier that breaks the tie.
func (r ItemRepository) positionIn(sort []view.SortTerm) func(work.WorkItem) security.Position {
	return func(last work.WorkItem) security.Position {
		keys := make([]string, 0, len(sort))
		for _, term := range sort {
			keys = append(keys, query.Key(term, last))
		}
		return security.Position{Keys: keys, ID: last.ID}
	}
}

// groupKeyOf renders the grouping value of one entry, and says whether it has one at all.
//
// Text for every kind of field, because a group key travels to the client as text: the contract's
// group key is a nullable string, and a board column is looked up by the identifier it was drawn
// with (api-guidelines.md §3).
func groupKeyOf(field view.Field, item work.WorkItem) (string, bool) {
	switch field.Name {
	case view.FieldBucketID:
		return item.BucketID.String(), !item.BucketID.IsZero()
	case view.FieldCollection:
		return item.CollectionID.String(), true
	case view.FieldCreatedBy:
		return item.CreatedBy.String(), true
	case view.FieldType:
		return string(item.Type), true
	case view.FieldIsCompleted:
		if item.Completion.IsCompleted {
			return "true", true
		}
		return "false", true
	}
	return "", false
}

func queryFailed(what string, err error) error {
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("%s: %w", what, err))
}

// Search answers the full text search: one page of entries in the order the database ranked them
// (C-08).
//
// It sits beside the query language's half rather than among the generated statements for the same
// reason that one does: the statement is compiled from a validated request, and it runs on the
// transaction the unit of work opened. What is different is the walk - a search is ordered by a
// rank the query computes rather than by a column, so the cursor carries that rank.
func (r ItemRepository) Search(
	ctx context.Context, search repository.TextSearch,
) (repository.ItemHitPage, error) {
	tx, err := FromContext(ctx)
	if err != nil {
		return repository.ItemHitPage{}, err
	}

	boundary, err := r.searchBoundary(search.Request.Cursor)
	if err != nil {
		return repository.ItemHitPage{}, err
	}

	size := search.Request.Size
	statement, err := query.Search(search, boundary, int(pageProbe(size)))
	if err != nil {
		return repository.ItemHitPage{}, err
	}

	hits, err := readHits(ctx, tx, statement)
	if err != nil {
		return repository.ItemHitPage{}, err
	}

	var page repository.ItemHitPage
	page.Hits, page.Info = pageOf(hits, size, r.cursors, func(last repository.ItemHit) security.Position {
		return security.At(rankKey(last.Rank), last.Item.ID)
	})
	return page, nil
}

// readHits runs the compiled statement and maps every row: the item as every other read maps it,
// the hub beside it, and the rank the ordering was built from.
func readHits(
	ctx context.Context, tx pgx.Tx, statement query.Statement,
) ([]repository.ItemHit, error) {
	rows, err := tx.Query(ctx, statement.SQL, statement.Args...)
	if err != nil {
		return nil, queryFailed("running the search", err)
	}
	defer rows.Close()

	hits := make([]repository.ItemHit, 0, 64)
	for rows.Next() {
		var (
			row  sqlc.FindWorkItemRow
			hub  pgtype.UUID
			rank float32
		)
		if err := rows.Scan(
			&row.ID, &row.TenantID, &row.CollectionID, &row.Type, &row.ParentID, &row.Path,
			&row.Depth, &row.Title, &row.Notes, &row.IsCompleted, &row.CompletedAt, &row.CompletedBy,
			&row.BucketID, &row.OrderKey, &row.AssigneeID,
			&row.StartAt, &row.DueAt, &row.DueDateOnly, &row.DueTimeZone,
			&row.CoverKind, &row.CoverColorToken, &row.CoverMediaID, &row.CustomFields,
			&row.ContentLanguage,
			&row.ArchivedAt, &row.DeletedAt,
			&row.TrashBatchID, &row.CreatedBy, &row.CreatedAt, &row.UpdatedAt, &row.Version,
			&hub, &rank,
		); err != nil {
			return nil, queryFailed("reading a search hit", err)
		}

		item, err := itemFrom(row)
		if err != nil {
			return nil, err
		}
		hubID, err := optionalID(hub)
		if err != nil {
			return nil, err
		}
		hits = append(hits, repository.ItemHit{Item: item, HubID: hubID, Rank: rank})
	}
	if err := rows.Err(); err != nil {
		return nil, queryFailed("running the search", err)
	}
	return hits, nil
}

// searchBoundary decodes the cursor a search is continuing from.
func (r ItemRepository) searchBoundary(cursor string) (query.SearchBoundary, error) {
	if cursor == "" {
		return query.SearchBoundary{}, nil
	}
	position, err := r.cursors.Decode(cursor)
	if err != nil {
		return query.SearchBoundary{}, err
	}

	rank, err := strconv.ParseFloat(position.SortKey(), 32)
	if err != nil {
		// The cursor is this server's own, signed, and it decoded - so a key that is not a number
		// is a defect here rather than something a client sent (security.CursorCodec).
		return query.SearchBoundary{}, shared.ErrValidation.
			WithDetail("page.cursor_invalid").
			WithCause(fmt.Errorf("the rank in the cursor is not a number: %w", err))
	}
	return query.SearchBoundary{Rank: float32(rank), ID: position.ID}, nil
}

// rankKey renders a rank for the cursor. Thirty-two bits, formatted with the precision that reads
// back as the same float: the boundary is compared against `real` in the statement, and a value
// that lost a digit on the way out would skip a row or repeat one.
func rankKey(rank float32) string {
	return strconv.FormatFloat(float64(rank), 'g', -1, 32)
}
