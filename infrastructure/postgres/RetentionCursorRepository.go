// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// RetentionCursorRepository keeps where each read of the rule sweep stopped (data-retention.md §5),
// under the tenant's row security like every tenant table.
type RetentionCursorRepository struct{}

func NewRetentionCursorRepository() RetentionCursorRepository {
	return RetentionCursorRepository{}
}

var _ repository.Cursors = RetentionCursorRepository{}

// Find answers the reader's place, and false when it has none.
func (r RetentionCursorRepository) Find(
	ctx context.Context, reader repository.Reader,
) (repository.Position, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Position{}, false, err
	}
	key, err := readerKey(reader)
	if err != nil {
		return repository.Position{}, false, err
	}
	row, err := queries.FindRetentionCursor(ctx, sqlc.FindRetentionCursorParams{
		Reader: key.reader, DataKind: key.dataKind, RuleID: key.ruleID,
	})
	if err != nil {
		if IsNoRows(err) {
			return repository.Position{}, false, nil
		}
		return repository.Position{}, false, cursorFailed("reading", err)
	}
	id, err := idFrom(row.AfterID)
	if err != nil {
		return repository.Position{}, false, err
	}
	return repository.Position{At: row.AfterAt.Time, ID: id}, true, nil
}

// Set records the reader's place, replacing the one it had.
func (r RetentionCursorRepository) Set(
	ctx context.Context, reader repository.Reader, at repository.Position,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := readerKey(reader)
	if err != nil {
		return err
	}
	after, err := uuidOf(at.ID)
	if err != nil {
		return err
	}
	if err := queries.SetRetentionCursor(ctx, sqlc.SetRetentionCursorParams{
		Reader: key.reader, DataKind: key.dataKind, RuleID: key.ruleID,
		AfterAt: timestampOf(at.At), AfterID: after,
	}); err != nil {
		return cursorFailed("writing", err)
	}
	return nil
}

// Clear forgets the reader's place.
func (r RetentionCursorRepository) Clear(ctx context.Context, reader repository.Reader) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := readerKey(reader)
	if err != nil {
		return err
	}
	if err := queries.ClearRetentionCursor(ctx, sqlc.ClearRetentionCursorParams{
		Reader: key.reader, DataKind: key.dataKind, RuleID: key.ruleID,
	}); err != nil {
		return cursorFailed("clearing", err)
	}
	return nil
}

type cursorKey struct {
	reader   string
	dataKind *string
	ruleID   pgtype.UUID
}

// readerKey is a reader as the table stores it: the field its phase does not use is NULL. A reader
// of a shape the table refuses is refused here first, with a code rather than a constraint error.
func readerKey(reader repository.Reader) (cursorKey, error) {
	key := cursorKey{reader: string(reader.Phase)}
	switch reader.Phase {
	case repository.ReaderAnnounce:
		if reader.DataKind == "" || !reader.RuleID.IsZero() {
			return cursorKey{}, shared.ErrInternal.WithDetail("retention.cursor_reader_invalid")
		}
		kind := string(reader.DataKind)
		key.dataKind = &kind
	case repository.ReaderChain:
		if reader.DataKind != "" || reader.RuleID.IsZero() {
			return cursorKey{}, shared.ErrInternal.WithDetail("retention.cursor_reader_invalid")
		}
		rule, err := uuidOf(reader.RuleID)
		if err != nil {
			return cursorKey{}, err
		}
		key.ruleID = rule
	case repository.ReaderAct:
		if reader.DataKind != "" || !reader.RuleID.IsZero() {
			return cursorKey{}, shared.ErrInternal.WithDetail("retention.cursor_reader_invalid")
		}
	default:
		return cursorKey{}, shared.ErrInternal.WithDetail("retention.cursor_reader_invalid")
	}
	return key, nil
}

func cursorFailed(doing string, err error) error {
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("%s the retention cursor: %w", doing, err))
}
