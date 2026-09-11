// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
)

// The constant that mirrors the migration is a number kept twice, and this is what makes the second
// copy honest (ADR-0054): the port says how wide the index is, migration 0075 built the column, and
// the two are compared against the migrated database rather than against each other's source.
func TestTheIndexIsAsWideAsThePortSays(t *testing.T) {
	ctx := context.Background()
	requirePgvector(ctx, t)

	var declared string
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT format_type(a.atttypid, a.atttypmod)
		  FROM pg_attribute a
		 WHERE a.attrelid = 'public.item_embedding'::regclass
		   AND a.attname = 'embedding'
		   AND NOT a.attisdropped`).Scan(&declared); err != nil {
		t.Fatalf("reading the column's type: %v", err)
	}
	if want := fmt.Sprintf("vector(%d)", repository.EmbeddingWidth); declared != want {
		t.Fatalf("item_embedding.embedding is %s and the port says %s: change the port, or "+
			"write the migration ADR-0054 says a wider index needs", declared, want)
	}
}
