// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestQueryNameIsTheSqlcNameOrDynamic(t *testing.T) {
	for _, tc := range []struct{ sql, want string }{
		{"-- name: ClaimJobs :many\nWITH due AS (SELECT 1)", "ClaimJobs"},
		{"-- name: GetItem :one\nSELECT 1", "GetItem"},
		{"SELECT set_config('app.tenant_id', $1, true)", "dynamic"},
		{"-- name: Drop'; --\nSELECT 1", "dynamic"},
		{"-- name: \nSELECT 1", "dynamic"},
		{"  -- name: Indented :one", "dynamic"},
	} {
		if got := QueryName(tc.sql); got != tc.want {
			t.Errorf("QueryName(%q) = %q, want %q", tc.sql, got, tc.want)
		}
	}
}

func TestQueryOutcomeTellsACancelFromAFailure(t *testing.T) {
	if got := queryOutcome(nil); got != "ok" {
		t.Errorf("nil: %q", got)
	}
	if got := queryOutcome(context.DeadlineExceeded); got != "canceled" {
		t.Errorf("deadline: %q", got)
	}
	if got := queryOutcome(errors.New("23505")); got != "error" {
		t.Errorf("failure: %q", got)
	}
}

// hubtask_db_errors_total's kind (observability-reliability.md §4.2): read from the SQLSTATE class
// and the driver's own errors, never the message.
func TestErrorKindTellsWhyAStatementFailed(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "timeout"},
		{&pgconn.PgError{Code: "57014"}, "timeout"},
		{fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "40001"}), "serialization"},
		{&pgconn.PgError{Code: "40P01"}, "serialization"},
		{&pgconn.PgError{Code: "08006"}, "connection"},
		{&pgconn.PgError{Code: "57P01"}, "connection"},
		{&pgconn.PgError{Code: "23505"}, "constraint"},
		{&pgconn.PgError{Code: "42P01"}, "other"},
		{errors.New("unknown"), "other"},
	} {
		if got := ErrorKind(tc.err); got != tc.want {
			t.Errorf("ErrorKind(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
