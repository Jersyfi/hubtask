// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// QueryObserver receives one call per statement a pool ran (F-10, observability-reliability.md
// §4.2). name is the sqlc query name or `dynamic` for SQL the query compiler built - a closed set
// compiled into the binary, never the statement's text, which may hold nothing a request typed
// (rule 9) but would still be a label nobody bounded.
type QueryObserver interface {
	DBQuery(ctx context.Context, pool, name, outcome string, seconds float64)
	// DBError counts a statement that failed, by ErrorKind.
	DBError(ctx context.Context, pool, kind string)
}

// The names a statement without a `-- name:` header is filed under.
const (
	queryNameDynamic = "dynamic"
)

type queryStartKey struct{}

type queryStart struct {
	at   time.Time
	name string
}

// queryTracer is the pgx hook; it measures, it never decides anything about a statement.
type queryTracer struct {
	pool     string
	observer QueryObserver
}

func (t queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryStartKey{}, queryStart{at: time.Now(), name: QueryName(data.SQL)})
}

func (t queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(queryStartKey{}).(queryStart)
	if !ok {
		return
	}
	t.observer.DBQuery(ctx, t.pool, start.name, queryOutcome(data.Err), time.Since(start.at).Seconds())
	if data.Err != nil {
		t.observer.DBError(ctx, t.pool, ErrorKind(data.Err))
	}
}

// ErrorKind names why a statement failed, as the operator acts on it: `timeout` (a deadline, a
// cancel, Postgres's statement or lock timeout), `serialization` (a serialization failure or a
// deadlock - retryable), `connection` (the connection broke or could not be made), `constraint` (an
// integrity rule refused the write) and `other`. SQLSTATE classes, never the message.
func ErrorKind(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == "57014", pgErr.Code == "55P03":
			return "timeout"
		case pgErr.Code == "40001", pgErr.Code == "40P01":
			return "serialization"
		case strings.HasPrefix(pgErr.Code, "08"), pgErr.Code == "57P01", pgErr.Code == "57P02", pgErr.Code == "57P03":
			return "connection"
		case strings.HasPrefix(pgErr.Code, "23"):
			return "constraint"
		}
		return "other"
	}
	var connectErr *pgconn.ConnectError
	if errors.As(err, &connectErr) || pgconn.SafeToRetry(err) || pgconn.Timeout(err) {
		if pgconn.Timeout(err) {
			return "timeout"
		}
		return "connection"
	}
	return "other"
}

// QueryName is the sqlc name in a statement's first line (`-- name: ClaimJobs :many`), and
// `dynamic` for any other statement.
func QueryName(sql string) string {
	rest, ok := strings.CutPrefix(sql, "-- name: ")
	if !ok {
		return queryNameDynamic
	}
	name, _, _ := strings.Cut(rest, " ")
	for _, r := range name {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return queryNameDynamic
		}
	}
	if name == "" || len(name) > 80 {
		return queryNameDynamic
	}
	return name
}

// queryOutcome is ok, canceled (the caller's deadline or cancel, rule 7) or error.
func queryOutcome(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled"
	default:
		return "error"
	}
}
