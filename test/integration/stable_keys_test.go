// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// A view's key (migration 0286): drawn from the name and numbered on a clash in the same scope,
// the writer's own kept, a taken one refused rather than renamed, and another workspace's views
// never counted - a key is unique where the view lives, not across workspaces.
func TestAViewsKeyIsUniqueWhereItLives(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	other := collectionFor(ctx, t, tenantB, authorB)

	keyOf := func(tenant shared.ID, saved view.SavedView) (string, error) {
		var key string
		err := write(ctx, t, tenant, func(ctx context.Context) error {
			if err := viewRepo().Insert(ctx, saved); err != nil {
				return err
			}
			stored, err := viewRepo().Find(ctx, saved.ID)
			key = stored.Key
			return err
		})
		return key, err
	}

	first, err := keyOf(tenantA, savedViewIn(tenantA, authorA, collection, freshID(t), "About to breach!"))
	if err != nil || first != "about_to_breach" {
		t.Fatalf("the first view's key: %q, %v", first, err)
	}
	second, err := keyOf(tenantA, savedViewIn(tenantA, authorA, collection, freshID(t), "About to breach"))
	if err != nil || second != "about_to_breach_2" {
		t.Fatalf("a clash in the same scope is numbered: %q, %v", second, err)
	}
	foreign, err := keyOf(tenantB, savedViewIn(tenantB, authorB, other, freshID(t), "About to breach"))
	if err != nil || foreign != "about_to_breach" {
		t.Fatalf("another workspace's views are not counted: %q, %v", foreign, err)
	}

	own := savedViewIn(tenantA, authorA, collection, freshID(t), "Mine")
	own.Key = "about_to_breach"
	if _, err := keyOf(tenantA, own); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a taken key is refused, not renamed: %v", err)
	}
	own.ID, own.Key = freshID(t), "mine_open"
	if key, err := keyOf(tenantA, own); err != nil || key != "mine_open" {
		t.Fatalf("the writer's key is kept: %q, %v", key, err)
	}
}

// A hub is given a handle as a collection is, so the configuration document names it by a word.
func TestAHubHasAHandle(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	var handle *string
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT h.handle FROM container c JOIN container h ON h.id = c.parent_id WHERE c.id = $1`,
		collection.String()).Scan(&handle); err != nil {
		t.Fatalf("reading the hub's handle: %v", err)
	}
	if handle == nil || !shared.ValidKey(*handle) {
		t.Fatalf("the hub's handle: %v", handle)
	}
}
