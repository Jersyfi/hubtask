// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UsageQuery is one read of the use: a range of days, optionally one metric and one workspace, a
// page of workspaces after the last one read.
type UsageQuery struct {
	From, To time.Time
	Metric   string
	Tenant   shared.ID
	// Cursor is where the last page stopped, as CursorAfter wrote it; empty for the first.
	Cursor string
	Size   int
}

// UsageRow is one workspace's count of one metric on one day, live or purged. Closing is the day's
// last count for the counts kept as the day's highest, nil for the sums. A row before the range is
// answered too, the last one per workspace and metric, so its closing value can be carried in.
type UsageRow struct {
	Tenant  shared.ID
	Day     time.Time
	Metric  string
	Value   int64
	Closing *int64
	Purged  bool
}

// UsageLedger reads every workspace's counts, under the installation's scope (multi-tenancy.md
// §4.3): the rows of at most Size workspaces, in the order of their ids.
type UsageLedger interface {
	Read(ctx context.Context, query UsageQuery) ([]UsageRow, error)
	// CursorAfter is the opaque, signed cursor of a page whose last workspace was last.
	CursorAfter(last shared.ID) string
}
