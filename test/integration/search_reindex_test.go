// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The search remembers which configuration built each document (M-09, ADR-0034): the trigger
// fills it beside the document, a row written before the column existed is stale, and the
// rebuild rewrites exactly the stale rows of the workspace the transaction is bound to.
func TestTheSearchRemembersWhichConfigurationBuiltEachDocument(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	tenant, author := seedOwnTenant(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenant, author)
	admin := adminPool(ctx, t)

	// Three entries through the repository, so the trigger fires: English and German land on a
	// stock configuration, Japanese on simple.
	english := seedTask(ctx, t, tenant, author, collection)
	german := seedTask(ctx, t, tenant, author, collection)
	japanese := seedTask(ctx, t, tenant, author, collection)
	for id, language := range map[shared.ID]string{english: "en", german: "de-AT", japanese: "ja"} {
		if _, err := admin.Exec(ctx, `UPDATE work_item SET content_language = $2 WHERE id = $1`,
			id.String(), language); err != nil {
			t.Fatalf("setting the language: %v", err)
		}
	}

	configurations := map[string]string{}
	rows, err := admin.Query(ctx,
		`SELECT id::text, search_configuration FROM work_item WHERE collection_id = $1`, collection.String())
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		var configuration *string
		if err := rows.Scan(&id, &configuration); err != nil {
			t.Fatal(err)
		}
		if configuration != nil {
			configurations[id] = *configuration
		}
	}
	rows.Close()
	if configurations[english.String()] != "english" || configurations[german.String()] != "german" ||
		configurations[japanese.String()] != "simple" {
		t.Errorf("the trigger recorded %v", configurations)
	}

	// Two rows made stale by hand: one from before the column existed (NULL), one indexed under a
	// configuration the resolver would not choose today.
	if _, err := admin.Exec(ctx,
		`UPDATE work_item SET search_configuration = NULL WHERE id = $1`, english.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		`UPDATE work_item SET search_configuration = 'simple' WHERE id = $1`, german.String()); err != nil {
		t.Fatal(err)
	}

	index := postgres.NewSearchIndexRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	scope := persistence.Scope{TenantID: tenant}

	var stale int64
	if err := uow.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		stale, err = index.Stale(ctx)
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if stale != 2 {
		t.Fatalf("%d stale rows, want the two made stale by hand", stale)
	}

	// The walk, one row at a time so that the batch is seen to bound it.
	var rewritten []int64
	for range 3 {
		var count int64
		if err := uow.Within(ctx, scope, func(ctx context.Context) error {
			var err error
			count, err = index.Rebuild(ctx, 1)
			return err
		}); err != nil {
			t.Fatalf("rebuilding: %v", err)
		}
		rewritten = append(rewritten, count)
	}
	if rewritten[0] != 1 || rewritten[1] != 1 || rewritten[2] != 0 {
		t.Errorf("the batches rewrote %v, want 1, 1, then nothing", rewritten)
	}

	var after int64
	var germanNow string
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE search_configuration IS DISTINCT FROM hubtask_text_config(content_language)::text),
		        max(search_configuration) FILTER (WHERE id = $2)
		   FROM work_item WHERE collection_id = $1`, collection.String(), german.String()).
		Scan(&after, &germanNow); err != nil {
		t.Fatal(err)
	}
	if after != 0 || germanNow != "german" {
		t.Errorf("%d rows still stale after the walk, the German one under %q", after, germanNow)
	}
}

// The cross-tenant negative test for both methods (gate SG-3): a workspace's stale rows are not
// counted from another, and not rewritten from another.
func TestTheSearchIndexOfAnotherTenantIsOutOfReach(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	tenant, author := seedOwnTenant(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenant, author)
	admin := adminPool(ctx, t)

	item := seedTask(ctx, t, tenant, author, collection)
	if _, err := admin.Exec(ctx,
		`UPDATE work_item SET search_configuration = NULL WHERE id = $1`, item.String()); err != nil {
		t.Fatal(err)
	}

	index := postgres.NewSearchIndexRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	other := persistence.Scope{TenantID: tenantB}

	var stale, rewritten int64
	if err := uow.Within(ctx, other, func(ctx context.Context) error {
		var err error
		if stale, err = index.Stale(ctx); err != nil {
			return err
		}
		rewritten, err = index.Rebuild(ctx, 100)
		return err
	}); err != nil {
		t.Fatalf("from the other tenant: %v", err)
	}
	// tenantB's own rows may be stale from other tests; what must not be counted or rewritten is
	// this one. So the assertion is on the row rather than on the counts - and the counts are
	// read all the same, so that a method that errored is heard.
	t.Logf("from the other tenant: %d stale, %d rewritten (its own rows, if any)", stale, rewritten)
	var configuration *string
	if err := admin.QueryRow(ctx,
		`SELECT search_configuration FROM work_item WHERE id = $1`, item.String()).Scan(&configuration); err != nil {
		t.Fatal(err)
	}
	if configuration != nil {
		t.Errorf("another tenant's rebuild rewrote this workspace's row to %q", *configuration)
	}
}
