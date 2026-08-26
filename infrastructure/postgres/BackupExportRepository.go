// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// BackupExportRepository reads a tenant out, one page at a time, for the archive writer (E-05).
//
// Paged on each entity's own key rather than on OFFSET, and the difference is not performance. A
// backup runs inside a REPEATABLE READ snapshot for minutes; an OFFSET re-counts the rows it
// already skipped on every page, and an OFFSET over a set that another statement is free to reorder
// can repeat a row or drop one. A key cursor can do neither, and it is also what lets a run that
// died resume where it stopped.
type BackupExportRepository struct {
	// batch is how many rows one statement answers. Large enough that a tenant is not read in
	// thousands of round trips, small enough that a page is a few hundred kilobytes rather than a
	// share of the heap - the memory this whole design exists to bound (T-17).
	batch int32
}

// DefaultExportBatch is the page size a run uses unless it is told otherwise.
const DefaultExportBatch = 500

func NewBackupExportRepository(batch int) BackupExportRepository {
	// Bounded in both directions, which is also what keeps the conversion below honest: a caller
	// asking for a page of two billion rows is asking for the memory bound to be ignored.
	if batch <= 0 || batch > maxExportBatch {
		batch = DefaultExportBatch
	}
	return BackupExportRepository{batch: int32(batch)}
}

// maxExportBatch is the largest page anybody may ask for.
const maxExportBatch = 10_000

var _ repository.Export = BackupExportRepository{}

// exporter is one entity's statement and the shape of its key.
type exporter struct {
	// delta is whether the statement takes a period at all. A table that cannot date a change
	// answers everything, and asking it for a period would be asking a question its schema cannot
	// answer (archive.Entity.Whole).
	delta bool
	// keys is how many parts the primary key has, and therefore how many parts the cursor splits
	// into.
	keys int
	read func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error)
}

// exportPage is one request for a page: where to continue from, and how much.
type exportPage struct {
	Since   *time.Time
	AfterAt pgtype.Timestamptz
	// After is the previous page's last identity, already split into its parts. Empty on the
	// first page, where the sentinels below stand in.
	After []string
	Batch int32
}

// exportRow is one row as every statement answers it. The three columns are the same in all
// thirty-one, which is what lets one loop drive them.
type exportRow struct {
	RecordID  string
	ChangedAt pgtype.Timestamptz
	Payload   []byte
}

// convert maps one statement's rows onto the common shape. A function rather than an interface,
// because sqlc gives every statement a result type of its own and none of them share a method.
func convert[T any](rows []T, take func(T) exportRow) []exportRow {
	out := make([]exportRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, take(row))
	}
	return out
}

// The sentinels a first page starts from: the smallest value of each key type, so that the same
// comparison serves the first page and every one after it. A branch for "no cursor yet" would be a
// second comparison to keep in step with the first.
const (
	zeroUUID   = "00000000-0000-0000-0000-000000000000"
	zeroText   = ""
	zeroNumber = "0"
)

// beginning is the value before every instant, for the first page of a delta statement.
//
// PostgreSQL's own negative infinity rather than a very old date, and rather than Go's zero time:
// the zero time is what pgtype encodes as NULL, and a comparison against NULL is NULL - which is
// how a first page comes back empty and an incremental silently exports nothing.
func beginning() pgtype.Timestamptz {
	return pgtype.Timestamptz{InfinityModifier: pgtype.NegativeInfinity, Valid: true}
}

func cursorPart(after []string, index int, empty string) string {
	if index < len(after) && after[index] != "" {
		return after[index]
	}
	return empty
}

func cursorUUID(after []string, index int) pgtype.UUID {
	var id pgtype.UUID
	if err := id.Scan(cursorPart(after, index, zeroUUID)); err != nil {
		// A cursor part that is not a UUID can only come from a record identity this package
		// built, so it cannot happen without a defect here - and starting over is safer than
		// continuing from a value the database would refuse.
		_ = id.Scan(zeroUUID)
	}
	return id
}

func cursorText(after []string, index int) string { return cursorPart(after, index, zeroText) }

func cursorNumber(after []string, index int) int64 {
	value, err := strconv.ParseInt(cursorPart(after, index, zeroNumber), 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// Rows hands over one table's rows, oldest change first.
func (r BackupExportRepository) Rows(
	ctx context.Context, table string, since time.Time, yield func(repository.Row) error,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	statement, known := exporters[table]
	if !known {
		// A table nothing exports is a defect rather than an empty answer: an archive that
		// silently held nothing of an entity would restore a tenant with a feature missing.
		return shared.Internalf("postgres: no export statement for %s", table)
	}

	page := exportPage{AfterAt: beginning(), Batch: r.batch}
	if statement.delta && !since.IsZero() {
		period := since
		page.Since = &period
	}

	for {
		rows, err := statement.read(ctx, queries, page)
		if err != nil {
			return shared.ErrUnavailable.WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("exporting %s: %w", table, err))
		}
		for _, row := range rows {
			record, err := rowOf(table, row)
			if err != nil {
				return err
			}
			if err := yield(record); err != nil {
				return err
			}
		}
		if len(rows) < int(r.batch) {
			return nil
		}

		last := rows[len(rows)-1]
		page.After = strings.SplitN(last.RecordID, "/", statement.keys)
		if statement.delta {
			page.AfterAt = last.ChangedAt
		}
	}
}

func rowOf(table string, row exportRow) (repository.Row, error) {
	var data map[string]any
	if err := json.Unmarshal(row.Payload, &data); err != nil {
		return repository.Row{}, shared.Internalf("postgres: reading a %s row: %w", table, err)
	}
	// A row is never nil, even when the table has nothing but its key: the archive distinguishes
	// "an object with no fields" from "a line that lost its payload", and only one of them is
	// allowed to be written.
	if data == nil {
		data = map[string]any{}
	}
	return repository.Row{ID: row.RecordID, ChangedAt: row.ChangedAt.Time, Data: data}, nil
}

// Tombstones hands over one table's deletion markers after an instant.
func (r BackupExportRepository) Tombstones(
	ctx context.Context, table string, since time.Time, yield func(repository.Tombstone) error,
) error {
	// A full archive is the whole truth and has nothing to deny, so it carries no markers. Asking
	// for them from the beginning of time would also hand it every deletion the offline window
	// still remembers, as tombstones for rows that were never in it.
	if since.IsZero() {
		return nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	after := beginning()
	afterID := cursorUUID(nil, 0)
	for {
		rows, err := queries.ExportTombstones(ctx, sqlc.ExportTombstonesParams{
			Entity: table, Since: timestampOf(since), AfterAt: after, AfterID: afterID,
			Batch: r.batch,
		})
		if err != nil {
			return shared.ErrUnavailable.WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("exporting the deletions of %s: %w", table, err))
		}
		for _, row := range rows {
			marker := repository.Tombstone{ID: row.EntityID, DeletedAt: row.DeletedAt.Time}
			if err := yield(marker); err != nil {
				return err
			}
		}
		if len(rows) < int(r.batch) {
			return nil
		}

		last := rows[len(rows)-1]
		after = last.DeletedAt
		if err := afterID.Scan(last.EntityID); err != nil {
			return shared.Internalf("postgres: a deletion marker with no identifier: %w", err)
		}
	}
}

// MediaLocation answers where one medium's bytes lie, by the checksum the archive addresses it
// with.
func (r BackupExportRepository) MediaLocation(
	ctx context.Context, checksum string,
) (repository.MediaLocation, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.MediaLocation{}, err
	}

	row, err := queries.FindMediaStorageKey(ctx, checksum)
	if err != nil {
		if IsNoRows(err) {
			return repository.MediaLocation{}, shared.ErrNotFound.WithDetail("media.not_found")
		}
		return repository.MediaLocation{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("locating a medium: %w", err))
	}
	return repository.MediaLocation{StorageKey: row.StorageKey, Bytes: row.ByteSize}, nil
}
