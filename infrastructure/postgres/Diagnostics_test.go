// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The diagnosis names the statement's failure and quotes nothing of the row: a constraint and a
// SQLSTATE reach the log, the message and the detail - which carry the values - do not, and an
// error that is not the driver's yields nothing.
func TestDiagnosticsNameTheConstraintAndQuoteNothing(t *testing.T) {
	cause := &pgconn.PgError{
		Code:           "23505",
		Message:        `duplicate key value violates unique constraint "work_item_pkey"`,
		Detail:         "Key (id)=(0192f000-0000-7000-8000-00000000000e) already exists. Title: Buy milk",
		ConstraintName: "work_item_pkey",
		TableName:      "work_item",
	}
	// Wrapped the way every repository wraps: a typed error over a %w chain.
	err := shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("writing the work item: %w", cause))

	attrs := Diagnostics(err)
	rendered := map[string]string{}
	for _, attr := range attrs {
		rendered[attr.Key] = attr.Value.String()
	}
	if rendered["sqlstate"] != "23505" || rendered["constraint"] != "work_item_pkey" || rendered["table"] != "work_item" {
		t.Errorf("attributes = %v", rendered)
	}
	if _, has := rendered["column"]; has {
		t.Error("an empty column name was rendered")
	}
	for _, value := range rendered {
		if strings.Contains(value, "Buy milk") || strings.Contains(value, "0192f000") || strings.Contains(value, "duplicate key") {
			t.Errorf("a value of the row reached the log: %q", value)
		}
	}
	if len(attrs) != 3 {
		t.Errorf("%d attributes, want three", len(attrs))
	}

	if got := Diagnostics(errors.New("not the driver's")); got != nil {
		t.Errorf("an error that is not the driver's yields %v", got)
	}
}
