// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"testing"
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
