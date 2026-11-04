// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A row the database refuses for what it holds - a check it fails, a column it leaves empty, a
// value no column takes - is refused the same way on every attempt: the restore and the import
// stop with it named, rather than answering a database error the queue retries until it gives up
// and leaves the run unfinished (backup-restore.md §9).
func TestARowRefusedForWhatItHoldsIsAConflictNotARetry(t *testing.T) {
	for _, tc := range []struct {
		name, code, constraint, want string
	}{
		{"a check", "23514", "work_item_title_check", "work_item_title_check"},
		{"an empty column", "23502", "", "work_item"},
		{"a zero byte in JSON", "22P05", "", "work_item"},
		{"a zero byte in text", "22021", "", "work_item"},
		{"a value too long", "22001", "", "work_item"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := fmt.Errorf("writing: %w", &pgconn.PgError{Code: tc.code, ConstraintName: tc.constraint})
			err := rowConflict(cause, "work_item", map[string]any{})
			if !errors.Is(err, shared.ErrConflict) || shared.AsError(err).DetailCode != "backup.row_refused" {
				t.Fatalf("answered %v, want a conflict backup.row_refused", err)
			}
			if got := shared.AsError(err).Params["constraint"]; got != tc.want {
				t.Errorf("names %q, want %q", got, tc.want)
			}
		})
	}
	if rowConflict(&pgconn.PgError{Code: "40001"}, "work_item", nil) != nil {
		t.Error("a serialisation failure is worth a retry")
	}
}
