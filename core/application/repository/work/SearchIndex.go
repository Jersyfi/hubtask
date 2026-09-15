// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "context"

// SearchIndex is the search's own bookkeeping (M-09, ADR-0034): the rows whose document was built
// under a configuration this installation has since replaced or gained.
//
// A row records which text search configuration built its document, beside the document, and one
// whose stored name differs from what the resolver answers today is stale - which is what happens
// when a PostgreSQL gains a configuration after the entries were written. Both methods read and
// write inside the transaction the caller opened, bound to one workspace by row level security,
// because the operation is the workspace's and nothing may enumerate tenants.
type SearchIndex interface {
	// Stale counts the rows whose document would be built differently today, including the ones
	// written before the configuration was recorded at all.
	Stale(ctx context.Context) (int64, error)
	// Rebuild rewrites the document and the recorded configuration of up to `batch` stale rows and
	// answers how many it rewrote. Zero means there are none left.
	Rebuild(ctx context.Context, batch int) (int64, error)
}
