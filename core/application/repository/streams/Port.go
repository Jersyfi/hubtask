// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package streams holds the outbound port of the partitioned streams' maintenance (H-09):
// activity entries, outbox events and rule runs partition by month, the leader keeps the
// coming months existing, and the retention engine drops what has wholly aged out.
package streams

import (
	"context"
	"time"

	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
)

// Tables is the closed set of partitioned streams, the vocabulary the two functions validate
// against (migration 0068).
func Tables() []string { return []string{"activity_entry", "outbox_event", "rule_run"} }

// DefaultDays answers the stream's catalogue default - the floor of the drop cutoff. 0 means
// the stream has no bound and no month of it ever falls (activity entries, until the catalogue
// gives them a period).
func DefaultDays(table string) int {
	kinds := map[string]lifecycle.DataKind{
		"activity_entry": lifecycle.KindActivityEntry,
		"outbox_event":   lifecycle.KindOutboxEvent,
		"rule_run":       lifecycle.KindRuleRun,
	}
	kind, held := kinds[table]
	if !held {
		return 0
	}
	entry, found := lifecycle.FindKind(kind)
	if !found {
		return 0
	}
	return entry.DefaultDays
}

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

	// DropAged detaches and drops every partition of the stream that has wholly aged out - the
	// default partition never among them - and reports each with its row count. The cutoff is
	// the function's own: it holds a month back until every tenant's configured retention for
	// the stream's kind has passed, with defaultDays as the floor. 0 means the stream has no
	// bound and nothing falls.
	DropAged(ctx context.Context, table string, defaultDays int) ([]Dropped, error)
}
