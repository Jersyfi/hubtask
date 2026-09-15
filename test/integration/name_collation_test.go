// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Names sort the same on every installation (M-08, i18n-l10n.md §5): migration 0080 defines
// hubtask_name from the ICU root collation where PostgreSQL has it, and every image the support
// matrix names has it - so on the integration database the object is ICU's, "Ä" sits beside "A",
// and the manifest says so.
func TestNamesSortUnderTheICURootCollation(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)

	var provider string
	if err := admin.QueryRow(ctx,
		`SELECT collprovider::text FROM pg_collation WHERE collname = 'hubtask_name'`).Scan(&provider); err != nil {
		t.Fatalf("the collation migration 0080 defines is not there: %v", err)
	}
	if provider != "i" {
		t.Fatalf("hubtask_name is a %q collation on an image built with ICU, want i", provider)
	}

	var order string
	if err := admin.QueryRow(ctx,
		`SELECT string_agg(n, ',' ORDER BY n COLLATE hubtask_name)
		   FROM unnest(ARRAY['Zebra', 'Ärger', 'Apfel', 'apfel', 'Äpfel', 'zebra']) AS n`).Scan(&order); err != nil {
		t.Fatalf("ordering: %v", err)
	}
	if order != "apfel,Apfel,Äpfel,Ärger,zebra,Zebra" {
		t.Errorf("names sort as %s - not the ICU root order", order)
	}

	var available bool
	if err := postgres.NewUnitOfWork(appPool(ctx, t)).WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			var err error
			available, err = postgres.NewNaturalOrderingRepository().Available(ctx)
			return err
		}); err != nil {
		t.Fatalf("reading the capability: %v", err)
	}
	if !available {
		t.Error("the manifest would say natural_ordering: false on a database whose collation is ICU's")
	}
}

// The other branch of migration 0080, which no image in the matrix reaches: a PostgreSQL built
// without ICU gets the database's own locale as a named collation. The statement is run here
// under another name, in a transaction that is rolled back, so that the branch is proved on the
// same database rather than on one nobody has.
func TestTheFallbackCollationIsTheDatabasesOwnLocale(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)

	tx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DO $$
		BEGIN
		  EXECUTE format('CREATE COLLATION hubtask_name_fallback (provider = libc, locale = %L)',
		                 (SELECT datcollate FROM pg_database WHERE datname = current_database()));
		END $$`); err != nil {
		t.Fatalf("the fallback statement does not run: %v", err)
	}

	var provider, locale, own string
	if err := tx.QueryRow(ctx,
		`SELECT c.collprovider::text, c.collcollate, d.datcollate
		   FROM pg_collation c, pg_database d
		  WHERE c.collname = 'hubtask_name_fallback' AND d.datname = current_database()`).
		Scan(&provider, &locale, &own); err != nil {
		t.Fatalf("reading the fallback: %v", err)
	}
	if provider != "c" || locale != own {
		t.Errorf("the fallback is a %q collation over %q, want libc over the database's own %q", provider, locale, own)
	}

	var order string
	if err := tx.QueryRow(ctx,
		`SELECT string_agg(n, ',' ORDER BY n COLLATE hubtask_name_fallback)
		   FROM unnest(ARRAY['Zebra', 'Apfel']) AS n`).Scan(&order); err != nil {
		t.Fatalf("ordering under the fallback: %v", err)
	}
	if !strings.HasPrefix(order, "Apfel") {
		t.Errorf("the fallback collation does not sort: %s", order)
	}
}
