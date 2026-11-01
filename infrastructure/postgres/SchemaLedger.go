// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// undefinedFunction is PostgreSQL's SQLSTATE for a function that does not exist.
const undefinedFunction = "42883"

// SchemaLedger reads which migrations the database has applied, for the readiness probe
// (deployment.md §5). The ledger itself is closed to the application role, so it is read through
// applied_schema_versions() (migration 0150), which answers the version numbers and nothing else.
type SchemaLedger struct {
	UnitOfWork persistence.UnitOfWork
}

// Applied answers the applied versions. older is true when the database predates the reader - the
// function is not there - which means it lacks at least the migration that brought it.
func (l SchemaLedger) Applied(ctx context.Context) (applied []int64, older bool, err error) {
	err = l.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		queries, err := queriesFrom(ctx)
		if err != nil {
			return err
		}
		versions, err := queries.AppliedSchemaVersions(ctx)
		applied = versions
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == undefinedFunction {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the applied migrations: %w", err))
	}
	if applied == nil {
		// NULL: a database without goose's ledger, which no migrated installation is.
		return nil, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(errors.New("the database has no migration ledger"))
	}
	return applied, false, nil
}

// Floor answers the schema's compatibility floor (migration 0337): the oldest binary, by its newest
// embedded migration, that still works on it. A database from before 0337 has no floor, which is 0.
func (l SchemaLedger) Floor(ctx context.Context) (int64, error) {
	var floor int64
	err := l.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		queries, err := queriesFrom(ctx)
		if err != nil {
			return err
		}
		floor, err = queries.SchemaCompatibilityFloor(ctx)
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == undefinedFunction {
		return 0, nil
	}
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the compatibility floor: %w", err))
	}
	return floor, nil
}
