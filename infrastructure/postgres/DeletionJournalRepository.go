// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// DeletionJournalRepository reads the deletion journal (E-06, backup-restore.md §7).
//
// The first reader the table has ever had in production. It has been written since B-10 with a
// comment saying exactly that, and this is the other half of the promise it was making: objects
// deleted between the archive point and the restore do not come back.
type DeletionJournalRepository struct {
	// batch is how many entries one statement answers, for the reason the export has one: a
	// tenant that has been emptying its trash for two years has a journal larger than the thing
	// being restored (T-17).
	batch int32
}

// DefaultJournalBatch is the page size a restore reads the journal in.
const DefaultJournalBatch = 1000

func NewDeletionJournalRepository(batch int) DeletionJournalRepository {
	if batch <= 0 || batch > maxExportBatch {
		batch = DefaultJournalBatch
	}
	return DeletionJournalRepository{batch: int32(batch)}
}

var _ repository.Journal = DeletionJournalRepository{}

// DeletedSince hands over the deletions recorded after an instant, oldest first.
func (r DeletionJournalRepository) DeletedSince(
	ctx context.Context, since time.Time, yield func(repository.Deletion) error,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	// The window opens at `since` and the cursor starts before every possible entry, so that the
	// first page and every page after it are the same comparison. Negative infinity rather than a
	// very old date, for the reason the export gives: the zero time encodes as NULL, and a
	// comparison against NULL is NULL - which is how a first page comes back empty and a restore
	// silently brings back everything the journal was meant to keep out.
	after := beginning()
	afterEntity := ""
	afterID := cursorUUID(nil, 0)

	for {
		rows, err := queries.JournalledDeletions(ctx, sqlc.JournalledDeletionsParams{
			Since:       timestampOf(since),
			AfterAt:     after,
			AfterEntity: afterEntity,
			AfterID:     afterID,
			Batch:       r.batch,
		})
		if err != nil {
			return shared.ErrUnavailable.WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("reading the deletion journal: %w", err))
		}

		for _, row := range rows {
			id, err := idFrom(row.EntityID)
			if err != nil {
				return err
			}
			entry := repository.Deletion{
				Entity: row.Entity, EntityID: id,
				DeletedAt: row.DeletedAt.Time, Reason: row.Reason,
			}
			if err := yield(entry); err != nil {
				return err
			}
		}
		if len(rows) < int(r.batch) {
			return nil
		}

		last := rows[len(rows)-1]
		after, afterEntity = last.DeletedAt, last.Entity
		afterID = last.EntityID
	}
}
