// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/activity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The item history, written and read (B-11).
//
// Nothing here names a tenant. The transaction the caller opened decided that, and row level
// security applies it to every statement below (ADR-0010) - which is why the cross-tenant tests in
// test/integration are what prove this file correct rather than a reading of it.
//
// One type for both halves of the port, unlike the trash, because there is nothing to keep apart:
// the write is one insert and the read is one select over the same table, and a second type would
// only mean a second thing for the composition root to build.
type ActivityRepository struct {
	cursors security.CursorCodec
}

func NewActivityRepository(cursors security.CursorCodec) ActivityRepository {
	return ActivityRepository{cursors: cursors}
}

var (
	_ repository.Journal = ActivityRepository{}
	_ repository.History = ActivityRepository{}
)

// Record appends one entry to an item's history, inside the caller's transaction.
func (r ActivityRepository) Record(ctx context.Context, entry domain.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	// The entry has to belong to the tenant the transaction runs as. The insert takes the tenant
	// from current_tenant_id() either way, so this refusal is about the caller having built the
	// entry from something other than what it is writing - which would put a correct row in the
	// database for the wrong reason.
	if scope, ok := scopeFromContext(ctx); ok && scope.TenantID != entry.TenantID {
		return shared.ErrInternal.
			WithDetail("activity.tenant_mismatch").
			WithParams(map[string]string{
				"entry": entry.TenantID.String(), "transaction": scope.TenantID.String(),
			})
	}

	params, err := activityInsertParams(entry)
	if err != nil {
		return err
	}
	if err := queries.RecordActivity(ctx, params); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the history of %s: %w", entry.ItemID, err))
	}
	return nil
}

func activityInsertParams(entry domain.Entry) (sqlc.RecordActivityParams, error) {
	var params sqlc.RecordActivityParams

	id, err := uuidOf(entry.ID)
	if err != nil {
		return params, err
	}
	itemID, err := uuidOf(entry.ItemID)
	if err != nil {
		return params, err
	}
	// Legitimately absent for a system actor, and for the collection where a caller has none to
	// hand. optionalUUID is what keeps those a NULL rather than an error about an empty identifier.
	collectionID, err := optionalUUID(entry.CollectionID)
	if err != nil {
		return params, err
	}
	actorID, err := optionalUUID(entry.Actor.ID)
	if err != nil {
		return params, err
	}

	set := entry.ChangeSet
	if set == nil {
		// The column is NOT NULL with an empty object as its default, and a nil map marshals to
		// `null` - which the column would refuse.
		set = map[string]any{}
	}
	changeSet, err := json.Marshal(set)
	if err != nil {
		return params, shared.ErrInternal.
			WithDetail("activity.change_set_unserialisable").
			WithCause(fmt.Errorf("serialising the change set of %s: %w", entry.ItemID, err))
	}

	return sqlc.RecordActivityParams{
		ID:          id,
		ItemID:      itemID,
		ContainerID: collectionID,
		ActorType:   string(entry.Actor.Kind),
		ActorID:     actorID,
		Verb:        string(entry.Verb),
		ChangeSet:   changeSet,
		OccurredAt:  timestampOf(entry.OccurredAt),
	}, nil
}

// List returns one page of one item's history, newest first.
func (r ActivityRepository) List(
	ctx context.Context, itemID shared.ID, page repository.Page,
) (repository.EntryPage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.EntryPage{}, err
	}
	id, err := uuidOf(itemID)
	if err != nil {
		return repository.EntryPage{}, err
	}
	boundary, err := activityCursor(r.cursors, page.Cursor)
	if err != nil {
		return repository.EntryPage{}, err
	}

	rows, err := queries.ListActivity(ctx, sqlc.ListActivityParams{
		ItemID:           id,
		CursorOccurredAt: boundary.occurredAt,
		CursorID:         boundary.id,
		PageSize:         pageProbe(page.Size),
	})
	if err != nil {
		return repository.EntryPage{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the history of %s: %w", itemID, err))
	}

	entries := make([]domain.Entry, 0, len(rows))
	for _, row := range rows {
		entry, err := activityEntryFrom(row)
		if err != nil {
			return repository.EntryPage{}, err
		}
		entries = append(entries, entry)
	}

	kept, info := pageOf(entries, page.Size, r.cursors, func(entry domain.Entry) security.Position {
		return security.Position{
			SortKey: entry.OccurredAt.UTC().Format(time.RFC3339Nano), ID: entry.ID,
		}
	})
	return repository.EntryPage{Entries: kept, Info: repository.PageInfo(info)}, nil
}

// activityBoundary is a decoded history cursor: the moment and the identifier the page continues
// after. Both absent for the first page, which is what makes the statement's
// `cursor_occurred_at IS NULL` mean "start at the newest".
type activityBoundary struct {
	occurredAt pgtype.Timestamptz
	id         pgtype.UUID
}

// activityCursor decodes the boundary. The sort key is a timestamp rather than a text column, so it
// is parsed rather than passed through - a malformed one is a cursor this server did not produce,
// which the signature should already have caught, and it is refused the same way.
func activityCursor(cursors security.CursorCodec, cursor string) (activityBoundary, error) {
	if cursor == "" {
		return activityBoundary{}, nil
	}

	position, err := cursors.Decode(cursor)
	if err != nil {
		return activityBoundary{}, err
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, position.SortKey)
	if err != nil {
		return activityBoundary{}, shared.ErrValidation.
			WithDetail("shared.cursor_invalid").WithCause(err)
	}
	id, err := uuidOf(position.ID)
	if err != nil {
		return activityBoundary{}, err
	}
	return activityBoundary{occurredAt: timestampOf(occurredAt), id: id}, nil
}

// activityEntryFrom maps one row onto the entry.
//
// The tenant is not read back. The row was found through row level security, so it is this tenant's
// by construction, and a column selected only to be compared with what the transaction already
// knows would be a check of the database against itself.
func activityEntryFrom(row sqlc.ListActivityRow) (domain.Entry, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Entry{}, err
	}
	itemID, err := idFrom(row.ItemID)
	if err != nil {
		return domain.Entry{}, err
	}
	collectionID, err := optionalID(row.ContainerID)
	if err != nil {
		return domain.Entry{}, err
	}
	actorID, err := optionalID(row.ActorID)
	if err != nil {
		return domain.Entry{}, err
	}
	if !row.OccurredAt.Valid {
		// The column is NOT NULL, so this is unreachable by construction - and unreachable is
		// exactly the state worth refusing rather than defaulting, because the zero time it would
		// default to is a moment forty-six years in the past.
		return domain.Entry{}, shared.ErrInternal.WithDetail("postgres.row_incoherent")
	}

	var changeSet map[string]any
	if len(row.ChangeSet) > 0 {
		if err := json.Unmarshal(row.ChangeSet, &changeSet); err != nil {
			return domain.Entry{}, shared.ErrInternal.
				WithDetail("activity.change_set_unreadable").
				WithCause(fmt.Errorf("reading the change set of %s: %w", id, err))
		}
	}
	if changeSet == nil {
		// An empty history entry says "the verb is the whole of it", which is a different answer
		// from "there was a change set and it could not be read".
		changeSet = map[string]any{}
	}

	return domain.Entry{
		ID:           id,
		ItemID:       itemID,
		CollectionID: collectionID,
		Actor:        domain.Actor{Kind: shared.ActorKind(row.ActorType), ID: actorID},
		Verb:         domain.Verb(row.Verb),
		ChangeSet:    changeSet,
		OccurredAt:   row.OccurredAt.Time,
	}, nil
}
