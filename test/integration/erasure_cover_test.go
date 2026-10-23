// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// An erasure discards the person's uploads that nothing points at - and a cover points at its
// object as surely as an attachment does. Handing a covered object to the discard removed its bytes
// and then failed on the cover's foreign key, leaving the entry a cover with nothing behind it.
func TestAnErasureLeavesACoverItsPicture(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	object := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageCover))

	item := findItem(ctx, t, tenantA, task)
	cover, err := work.NewCover(work.CoverImage, "", object.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCover(ctx, item.Covered(cover, changedAt), item.Version)
	}); err != nil {
		t.Fatalf("setting the cover: %v", err)
	}

	var orphaned []shared.ID
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		media, err := postgres.NewPrivacyRepository(pageCursors()).OrphanedMedia(ctx, authorA)
		for _, medium := range media {
			orphaned = append(orphaned, medium.ID)
		}
		return err
	}); err != nil {
		t.Fatalf("reading the uploads: %v", err)
	}
	for _, id := range orphaned {
		if id == object.ID {
			t.Fatal("the erasure would discard an object an entry's cover shows")
		}
	}
}
