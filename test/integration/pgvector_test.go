// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// pgvector is a capability, not a requirement (J-09, ADR-0050), and this is the half of that claim
// the ordinary gate cannot make: every gate runs a database that *has* the extension, which is what
// lets `support-matrix.md` §1 call semantic search supported at all - so the absence path needs a
// database of its own.
//
// It is in `migration_absent_extension_test.go`'s company rather than here for the migration half.
// What this file proves is the *presence* half, which the suite's own database can answer: the
// store exists, it is behind row level security like everything else, and the manifest says so.

func TestWhereTheExtensionIsThereTheStoreIsBehindTheBoundary(t *testing.T) {
	ctx := context.Background()

	var present bool
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT to_regclass('public.item_embedding') IS NOT NULL`).Scan(&present); err != nil {
		t.Fatalf("asking for the embedding store: %v", err)
	}
	if !present {
		t.Skip("this database has no pgvector, so there is no store to check (ADR-0050)")
	}

	// Row level security, forced, with the tenant policy - the same three statements every
	// tenant-scoped table in this schema carries. A store that held one workspace's vectors
	// readable by another would be T-04 with a different verb.
	var enabled, forced bool
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT relrowsecurity, relforcerowsecurity FROM pg_class
		  WHERE oid = 'public.item_embedding'::regclass`).Scan(&enabled, &forced); err != nil {
		t.Fatalf("reading the store's security: %v", err)
	}
	if !enabled || !forced {
		t.Errorf("row level security is enabled=%v forced=%v on the embedding store", enabled, forced)
	}

	var policies int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM pg_policies
		  WHERE tablename = 'item_embedding' AND policyname = 'tenant_isolation'`,
	).Scan(&policies); err != nil {
		t.Fatalf("reading the store's policy: %v", err)
	}
	if policies != 1 {
		t.Errorf("%d tenant_isolation policies on the embedding store, want one", policies)
	}

	// And the index the search will need, which is its own migration for the rolling update's sake.
	var indexes int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM pg_indexes
		  WHERE tablename = 'item_embedding' AND indexname = 'item_embedding_vector_idx'`,
	).Scan(&indexes); err != nil {
		t.Fatalf("reading the store's index: %v", err)
	}
	if indexes != 1 {
		t.Errorf("%d vector indexes on the embedding store, want one", indexes)
	}
}

// The manifest answers from the database rather than from configuration, which is the whole point
// of the capability being detected: an installation says what it can do rather than what the build
// can do.
func TestTheManifestAnswersWhatThisDatabaseCanDo(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	var (
		available bool
		present   bool
	)
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT to_regclass('public.item_embedding') IS NOT NULL`).Scan(&present); err != nil {
		t.Fatalf("asking for the embedding store: %v", err)
	}
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		var err error
		available, err = postgres.NewSemanticSearchRepository().Available(ctx)
		return err
	}); err != nil {
		t.Fatalf("asking the repository: %v", err)
	}

	if available != present {
		t.Errorf("the manifest says semantic_search=%v and the database says %v", available, present)
	}
}
