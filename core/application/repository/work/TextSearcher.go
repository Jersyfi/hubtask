// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "context"

// TextSearcher is the search port (F-9): what answers "where is this" with ranked entries.
//
// Its own port rather than a method of Items, so an external index can answer a search without
// becoming the store of entries. Whatever answers, it answers candidates only: the scope and the
// share rule arrive resolved in the TextSearch, and narrowing the hits to what the actor may read
// stays with the use case (rule 2, SearchItems.visible). The PostgreSQL adapter reads inside the
// transaction the use case opened, bound to one workspace by row level security; an adapter that
// does not must still answer for that workspace alone.
type TextSearcher interface {
	// Search answers one page of entries in the order they were ranked, with the hub each one
	// sits under. Ranked rather than sorted, so the cursor is a keyset over the rank, the
	// identifier descending after it: entries matching equally well come newest first - a UUIDv7
	// carries its own time (ADR-0034). A search without words is ordered by its sort instead.
	Search(ctx context.Context, search TextSearch) (ItemHitPage, error)
}
