// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// RecoveredHoldsTimeout bounds one read of a temporary cluster: connecting, and the statement.
const RecoveredHoldsTimeout = 2 * time.Minute

// recoveredHoldsSource is the configuration of the one connection a read of the temporary cluster
// opens. The session defaults to read-only, beneath the read-only transaction the unit of work
// opens anyway: the cluster is somebody's evidence of the moment before the loss, and a statement
// that wrote to it would change what the next reader finds.
func recoveredHoldsSource(dsn string, timeout time.Duration) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// The connection string may carry a password; whatever the driver says about it stays out
		// of the message (T-18).
		return nil, shared.ErrValidation.WithDetail("postgres.dsn_invalid").
			WithCause(fmt.Errorf("parsing the temporary cluster's connection string: %w", err))
	}
	cfg.MaxConns = 1
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = timeout
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(timeout.Milliseconds(), 10)
	cfg.ConnConfig.RuntimeParams["application_name"] = "hubtask/recovered-holds"
	return cfg, nil
}

// ReadRecoveredHolds answers one workspace's legal holds, released or not, as another database
// holds them: the temporary cluster the operator recovered to the end of the WAL archive
// (backup-restore.md §8.5 step 5).
//
// Through the transaction wrapper under the workspace's scope, like every read of the
// installation's own database (rule 3), and with the workspace named in the statement as well,
// because the role the operator connects with may bypass row level security.
func ReadRecoveredHolds(
	ctx context.Context, dsn string, tenantID shared.ID, timeout time.Duration,
) ([]domain.LegalHold, error) {
	if tenantID.IsZero() {
		return nil, shared.ErrInternal.WithDetail("postgres.scope_missing")
	}
	cfg, err := recoveredHoldsSource(dsn, timeout)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.pool_unavailable").
			WithCause(fmt.Errorf("opening the temporary cluster: %w", err))
	}
	defer pool.Close()

	var holds []domain.LegalHold
	err = NewUnitOfWork(pool).WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			queries, err := queriesFrom(ctx)
			if err != nil {
				return err
			}
			rows, err := queries.RecoveredLegalHolds(ctx)
			if err != nil {
				return shared.ErrUnavailable.WithDetail("postgres.query_failed").
					WithCause(fmt.Errorf("reading the temporary cluster's legal holds: %w", err))
			}
			holds = make([]domain.LegalHold, 0, len(rows))
			for _, row := range rows {
				hold, err := holdFrom(sqlc.ListLegalHoldsRow(row))
				if err != nil {
					return err
				}
				holds = append(holds, hold)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return holds, nil
}
