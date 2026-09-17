// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"
)

// Diagnostics names a database failure in attributes a log line may carry: the SQLSTATE, and
// the constraint, table and column the server reported. Never the message or the detail - both
// quote the row, and a row is user content (rule 10, ADR-0017).
//
// It exists for the failure nobody could read: a job that failed with `postgres.query_failed`
// and nothing else, twice, in the e2e session (issue 692). The code said a statement failed and
// the log could not say which; the constraint name does.
func Diagnostics(err error) []slog.Attr {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}
	attrs := []slog.Attr{slog.String("sqlstate", pgErr.Code)}
	if pgErr.ConstraintName != "" {
		attrs = append(attrs, slog.String("constraint", pgErr.ConstraintName))
	}
	if pgErr.TableName != "" {
		attrs = append(attrs, slog.String("table", pgErr.TableName))
	}
	if pgErr.ColumnName != "" {
		attrs = append(attrs, slog.String("column", pgErr.ColumnName))
	}
	return attrs
}
