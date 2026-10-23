// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A person's picture against a real database: the account's reference written and read back, every
// member's read of it bounded to the workspace, the recount keeping it alive, and both ways it ends
// - the object removed under it, and the erasure (gate SG-3 for SetAvatar and ServesAvatar).

func findAccount(ctx context.Context, t *testing.T, tenant, id shared.ID) identity.Account {
	t.Helper()
	var found identity.Account
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		found, err = postgres.NewAccountRepository().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the account: %v", err)
	}
	return found
}

func servesAvatar(ctx context.Context, t *testing.T, tenant, id shared.ID) bool {
	t.Helper()
	var serves bool
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		serves, err = mediaRepo().ServesAvatar(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("asking about the picture: %v", err)
	}
	return serves
}

func TestAPictureIsSetReadAndCountedInItsOwnWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)
	picture := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageAvatar))

	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return postgres.NewAccountRepository().SetAvatar(ctx, account.ID, picture.ID, created)
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("from tenant B: error %v, want not found", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewAccountRepository().SetAvatar(ctx, account.ID, picture.ID, created)
	}); err != nil {
		t.Fatalf("setting: %v", err)
	}
	if got := findAccount(ctx, t, tenantA, account.ID); got.AvatarMediaID != picture.ID {
		t.Fatalf("read back %q, want %q", got.AvatarMediaID, picture.ID)
	}
	if !servesAvatar(ctx, t, tenantA, picture.ID) {
		t.Error("the picture is not known as one in its own workspace")
	}
	if servesAvatar(ctx, t, tenantB, picture.ID) {
		t.Error("another workspace learns that the picture is somebody's")
	}

	// The recount sees the account's reference: a picture somebody shows is never an orphan.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return mediaRepo().Recount(ctx, created)
	}); err != nil {
		t.Fatalf("recounting: %v", err)
	}
	if stored := findMedia(ctx, t, tenantA, picture.ID); stored.RefCount != 1 {
		t.Errorf("ref_count %d, want 1", stored.RefCount)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewAccountRepository().SetAvatar(ctx, account.ID, "", created)
	}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if got := findAccount(ctx, t, tenantA, account.ID); !got.AvatarMediaID.IsZero() {
		t.Errorf("the picture stayed: %q", got.AvatarMediaID)
	}
	if servesAvatar(ctx, t, tenantA, picture.ID) {
		t.Error("a picture nobody shows is still somebody's")
	}
}

// The object going takes the reference with it rather than being refused: a replace-restore empties
// the media before the accounts, and an erasure discards the person's uploads after their row.
func TestAPictureRemovedLeavesTheAccountWithTheInitials(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)
	picture := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageAvatar))

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := postgres.NewAccountRepository().SetAvatar(ctx, account.ID, picture.ID, created); err != nil {
			return err
		}
		_, err := mediaRepo().RemoveRows(ctx, []shared.ID{picture.ID})
		return err
	}); err != nil {
		t.Fatalf("removing the picture: %v", err)
	}
	if got := findAccount(ctx, t, tenantA, account.ID); !got.AvatarMediaID.IsZero() {
		t.Errorf("the account still points at %q", got.AvatarMediaID)
	}
}

func TestAnAnonymisationTakesThePicture(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)
	picture := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, account.ID, media.UsageAvatar))

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := postgres.NewAccountRepository().SetAvatar(ctx, account.ID, picture.ID, created); err != nil {
			return err
		}
		_, err := postgres.NewPrivacyRepository(pageCursors()).Anonymise(ctx, account.ID, "Former user", created)
		return err
	}); err != nil {
		t.Fatalf("anonymising: %v", err)
	}

	var orphaned []shared.ID
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		media, err := postgres.NewPrivacyRepository(pageCursors()).OrphanedMedia(ctx, account.ID)
		for _, medium := range media {
			orphaned = append(orphaned, medium.ID)
		}
		if err != nil {
			return err
		}
		// What the erasure does next with each: the row goes, and nothing holds it back.
		return postgres.NewPrivacyRepository(pageCursors()).DiscardMedium(ctx, picture.ID)
	}); err != nil {
		t.Fatalf("discarding the picture: %v", err)
	}
	if len(orphaned) != 1 || orphaned[0] != picture.ID {
		t.Errorf("the erasure would discard %v, want the picture", orphaned)
	}
	if servesAvatar(ctx, t, tenantA, picture.ID) {
		t.Error("an anonymised account still shows a picture")
	}
}
