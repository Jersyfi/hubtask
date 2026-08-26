// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// BackupImportRepository writes a tenant back in, one row at a time (E-06, backup-restore.md §8).
//
// The mirror of BackupExportRepository, and deliberately its opposite in shape: the export pages
// because it reads a whole tenant, and the import does not because it decides each row - against
// the conflict rule, against the deletion journal - and a batch would lose which of thirty rows it
// had skipped.
//
// The row travels as one jsonb value and is unpacked by `jsonb_populate_record` against the
// table's own row type. That is what keeps rule 9 true here: no byte of an archive ever becomes
// SQL text, not even a column name. A field the archive carries and the table no longer has is
// dropped by the unpacking, and a column the archive predates arrives NULL - which is what makes
// an archive from an older schema restorable at all.
type BackupImportRepository struct{}

func NewBackupImportRepository() BackupImportRepository { return BackupImportRepository{} }

var _ repository.Import = BackupImportRepository{}

// importer is one entity's three statements. See BackupImportTables.go for the table of them.
type importer struct {
	write func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error)
	holds func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error)
	// clear is nil for the one entity that has none: the tenant row a restore is standing inside.
	clear func(ctx context.Context, queries *sqlc.Queries) (int64, error)
}

// Holds reports whether the tenant already has the row this data identifies.
func (r BackupImportRepository) Holds(
	ctx context.Context, table string, data map[string]any,
) (bool, error) {
	queries, entity, payload, err := r.prepare(ctx, table, data)
	if err != nil {
		return false, err
	}
	held, err := entity.holds(ctx, queries, payload)
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("asking whether %s holds a row: %w", table, err))
	}
	return held, nil
}

// Write inserts the row, or replaces it when overwrite is true.
func (r BackupImportRepository) Write(
	ctx context.Context, table string, data map[string]any, overwrite bool,
) (bool, error) {
	queries, entity, payload, err := r.prepare(ctx, table, data)
	if err != nil {
		return false, err
	}
	written, err := entity.write(ctx, queries, payload, overwrite)
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("importing a row into %s: %w", table, err))
	}
	return written > 0, nil
}

// Clear empties one table within the tenant.
func (r BackupImportRepository) Clear(ctx context.Context, table string) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	entity, known := importers[table]
	if !known || entity.clear == nil {
		return 0, shared.Internalf("postgres: %s cannot be cleared", table)
	}
	removed, err := entity.clear(ctx, queries)
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing %s: %w", table, err))
	}
	return int(removed), nil
}

// prepare is what all three share: the transaction, the entity's statements, and the row as jsonb.
func (r BackupImportRepository) prepare(
	ctx context.Context, table string, data map[string]any,
) (*sqlc.Queries, importer, []byte, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, importer{}, nil, err
	}
	entity, known := importers[table]
	if !known {
		// An entity this build has no statement for. A defect rather than input: the archive's
		// entity list and this table are kept in step by a test, so an unknown name here means one
		// of the two has moved without the other.
		return nil, importer{}, nil, shared.Internalf("postgres: no import statement for %s", table)
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, importer{}, nil, shared.Internalf("postgres: a row of %s could not be encoded: %w", table, err)
	}
	return queries, entity, payload, nil
}

// ImportableTables answers the tables this build can write a restore into. It is what the test
// beside the archive's entity list compares against, so that an entity added to the archive
// without a statement here fails the gate rather than a restore.
func ImportableTables() []string {
	tables := make([]string, 0, len(importers))
	for table := range importers {
		tables = append(tables, table)
	}
	return tables
}
