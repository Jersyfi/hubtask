// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The statements the saved views run on, against a real database (D-07): the round trip, the
// tenant boundary per method (gate SG-3), and the one deletion semantics D-08 will lean on.

func viewRepo() postgres.SavedViewRepository { return postgres.NewSavedViewRepository() }

func savedViewIn(tenant, owner, collection shared.ID, id shared.ID, name string) view.SavedView {
	saved, err := view.NewSavedView(view.NewSavedViewInput{
		ID: id, TenantID: tenant, OwnerID: owner,
		ScopeType: view.ViewScopeCollection, ScopeID: collection,
		Name: name, Layout: "KANBAN",
		Query: map[string]any{
			"filter": map[string]any{"field": "due_at", "op": "LTE", "value": "@today+P7D"},
		},
		Sharing: view.SharingPrivate,
		Now:     created,
	})
	if err != nil {
		panic(err)
	}
	return saved
}

func TestASavedViewRoundTripsWhole(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	id := freshID(t)
	saved := savedViewIn(tenantA, authorA, collection, id, freshName(t))
	saved.VisibleFields = []string{"title", "due_at", "custom_fields.priority"}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return viewRepo().Insert(ctx, saved)
	}); err != nil {
		t.Fatalf("writing the view: %v", err)
	}

	var stored view.SavedView
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = viewRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the view: %v", err)
	}

	if stored.Name != saved.Name || stored.Layout != view.LayoutKanban ||
		stored.Sharing != view.SharingPrivate || stored.Version != 1 {
		t.Errorf("the view came back as %+v", stored)
	}
	filter, _ := stored.Query["filter"].(map[string]any)
	if filter["value"] != "@today+P7D" {
		t.Errorf("the stored query came back as %v", stored.Query)
	}
	if len(stored.VisibleFields) != 3 || stored.VisibleFields[2] != "custom_fields.priority" {
		t.Errorf("the visible fields came back as %v", stored.VisibleFields)
	}

	// The attributes write whole, under the lock; the sharing has its own statement.
	renamed, _, err := stored.Updated(view.ViewAttributes{Name: strPtr("Overdue board")})
	if err != nil {
		t.Fatalf("the rename was refused: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return viewRepo().SetAttributes(ctx, renamed, stored.Version)
	}); err != nil {
		t.Fatalf("writing the attributes: %v", err)
	}
	published, _, err := renamed.Shared(view.SharingScope)
	if err != nil {
		t.Fatalf("sharing was refused: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return viewRepo().SetSharing(ctx, published, stored.Version+1)
	}); err != nil {
		t.Fatalf("writing the sharing: %v", err)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = viewRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("re-reading the view: %v", err)
	}
	if stored.Name != "Overdue board" || stored.Sharing != view.SharingScope || stored.Version != 3 {
		t.Errorf("the writes left the view as %+v", stored)
	}
}

func TestTheReachableListAnswersOwnAndSharedAlongThePath(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	reader := seedAccount(ctx, t, tenantA)

	own := savedViewIn(tenantA, reader, collection, freshID(t), freshName(t))
	privateElse := savedViewIn(tenantA, authorA, collection, freshID(t), freshName(t))
	sharedHere := savedViewIn(tenantA, authorA, collection, freshID(t), freshName(t))
	sharedHere.Sharing = view.SharingScope
	elsewhere := savedViewIn(tenantA, authorA, collectionFor(ctx, t, tenantA, authorA),
		freshID(t), freshName(t))
	elsewhere.Sharing = view.SharingScope

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, seed := range []view.SavedView{own, privateElse, sharedHere, elsewhere} {
			if err := viewRepo().Insert(ctx, seed); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the views: %v", err)
	}

	var reachable []view.SavedView
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		reachable, err = viewRepo().ListReachable(ctx, reader, []shared.ID{collection})
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}

	found := map[shared.ID]bool{}
	for _, entry := range reachable {
		found[entry.ID] = true
	}
	if !found[own.ID] || !found[sharedHere.ID] {
		t.Errorf("the list misses what it owes: %v", found)
	}
	if found[privateElse.ID] {
		t.Error("another account's private view is in the list")
	}
	if found[elsewhere.ID] {
		t.Error("a view shared into another collection is in the list")
	}
}

// D-08's question, answered where the schema answers it: a feed whose view is gone keeps its row
// and loses the reference (ON DELETE SET NULL, migration 0005).
func TestDeletingAViewNullsTheFeedThatServedIt(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	saved := savedViewIn(tenantA, authorA, collection, freshID(t), freshName(t))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return viewRepo().Insert(ctx, saved)
	}); err != nil {
		t.Fatalf("writing the view: %v", err)
	}

	feedID := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO calendar_feed (id, tenant_id, account_id, view_id, token_hash)
		 VALUES ($1, $2, $3, $4, $5)`,
		feedID.String(), tenantA.String(), authorA.String(), saved.ID.String(),
		[]byte("integration-test-token-hash")); err != nil {
		t.Fatalf("seeding the feed: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return viewRepo().Delete(ctx, saved, saved.Version)
	}); err != nil {
		t.Fatalf("deleting the view: %v", err)
	}

	var viewRef *string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT view_id::text FROM calendar_feed WHERE id = $1`, feedID.String()).
		Scan(&viewRef); err != nil {
		t.Fatalf("reading the feed: %v", err)
	}
	if viewRef != nil {
		t.Errorf("the feed still references the deleted view: %v", *viewRef)
	}
}

// Gate SG-3: one negative per port method.
func TestSavedViewsAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	saved := savedViewIn(tenantA, authorA, collection, freshID(t), freshName(t))
	saved.Sharing = view.SharingScope
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return viewRepo().Insert(ctx, saved)
	}); err != nil {
		t.Fatalf("seeding the view: %v", err)
	}

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := viewRepo().Find(ctx, saved.ID)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("tenant B's find answered %v", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		var listed []view.SavedView
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			listed, err = viewRepo().ListOwned(ctx, authorA)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if len(listed) != 0 {
			t.Errorf("tenant B listed tenant A's views: %v", listed)
		}
	})

	t.Run("reachable", func(t *testing.T) {
		var listed []view.SavedView
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			listed, err = viewRepo().ListReachable(ctx, authorA, []shared.ID{collection})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if len(listed) != 0 {
			t.Errorf("tenant B reached tenant A's shared views: %v", listed)
		}
	})

	t.Run("attributes", func(t *testing.T) {
		renamed, _, err := saved.Updated(view.ViewAttributes{Name: strPtr("Stolen")})
		if err != nil {
			t.Fatal(err)
		}
		writeErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			return viewRepo().SetAttributes(ctx, renamed, saved.Version)
		})
		if !errors.Is(writeErr, shared.ErrVersionConflict) {
			t.Fatalf("tenant B's write answered %v", writeErr)
		}
	})

	t.Run("sharing", func(t *testing.T) {
		writeErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			return viewRepo().SetSharing(ctx, saved, saved.Version)
		})
		if !errors.Is(writeErr, shared.ErrVersionConflict) {
			t.Fatalf("tenant B's sharing write answered %v", writeErr)
		}
	})

	t.Run("delete", func(t *testing.T) {
		writeErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			return viewRepo().Delete(ctx, saved, saved.Version)
		})
		if !errors.Is(writeErr, shared.ErrVersionConflict) {
			t.Fatalf("tenant B's delete answered %v", writeErr)
		}
	})

	t.Run("insert", func(t *testing.T) {
		// scope_id carries no foreign key - one column, three referents - so an insert naming
		// tenant A's collection does not fail: it lands in tenant B, because the statement writes
		// current_tenant_id() and never the caller's claim. What the boundary owes is that the
		// row is B's, invisible to A, referencing an identifier that resolves to nothing there.
		foreign := savedViewIn(tenantA, authorA, collection, freshID(t), freshName(t))
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return viewRepo().Insert(ctx, foreign)
		}); err != nil {
			t.Fatalf("the insert failed rather than landing in the caller's tenant: %v", err)
		}
		err := read(ctx, t, tenantA, func(ctx context.Context) error {
			_, err := viewRepo().Find(ctx, foreign.ID)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("tenant A sees tenant B's row: %v", err)
		}
	})

	// And nothing above changed the row.
	var untouched view.SavedView
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		untouched, err = viewRepo().Find(ctx, saved.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if untouched.Name != saved.Name || untouched.Version != 1 {
		t.Errorf("the boundary leaked a write: %+v", untouched)
	}
}

func strPtr(value string) *string { return &value }
