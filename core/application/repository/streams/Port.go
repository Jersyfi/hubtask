// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package streams holds the outbound port of the partitioned streams' maintenance (H-09):
// activity entries, outbox events and rule runs partition by month, the leader keeps the
// coming months existing, and the retention engine drops what has wholly aged out.
package streams

import (
	"context"
	"time"
)

// Tables is the closed set of partitioned streams, the vocabulary the two functions validate
// against (migration 0068).
func Tables() []string { return []string{"activity_entry", "outbox_event", "rule_run"} }

// Dropped is one partition the retention let go: its name, and how many rows went with it -
// what the evidence records.
type Dropped struct {
	Name string
	Rows int64
}

// Partitions maintains the monthly partitions through the two SECURITY DEFINER acts.
type Partitions interface {
	// Ensure creates the month's partition for one stream if it is missing, and repairs its
	// policy and grants if they were tampered with (ensure_audit_partition's contract). The
	// empty name means the month's rows already live in the default partition - a state to
	// live with for a month, not an error.
	Ensure(ctx context.Context, table string, month time.Time) (string, error)

	// DropAged detaches and drops every partition of the stream whose upper bound lies at or
	// before the cutoff - the default partition never among them - and reports each with its
	// row count.
	DropAged(ctx context.Context, table string, cutoff time.Time) ([]Dropped, error)
}
