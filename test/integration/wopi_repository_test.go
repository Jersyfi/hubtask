// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The WOPI host's rows against a real database (UC-MED-06): the document server, the lock and a
// save that keeps the earlier bytes as a version - and none of them reachable from another
// workspace (gate SG-3).

func TestTheWopiHostsRowsRoundTrip(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.WopiRepository{}
	at := created.Add(time.Hour)

	choice, err := media.NewDocumentServerChoice(media.DocumentServerCollabora, "https://office.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetDocumentServer(ctx, choice, authorA, at)
	}); err != nil {
		t.Fatal(err)
	}
	var got media.DocumentServerChoice
	if err := read(ctx, t, tenantA, func(ctx context.Context) (err error) {
		got, err = repo.DocumentServer(ctx)
		return err
	}); err != nil || got != choice {
		t.Fatalf("read back %+v, %v", got, err)
	}

	file := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment))
	lock := media.Lock{ID: "lock-1", ExpiresAt: at.Add(media.LockDuration)}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return repo.PutLock(ctx, file.ID, lock) }); err != nil {
		t.Fatal(err)
	}
	var held *media.Lock
	if err := read(ctx, t, tenantA, func(ctx context.Context) (err error) {
		held, err = repo.Lock(ctx, file.ID)
		return err
	}); err != nil || held == nil || held.ID != "lock-1" || !held.ExpiresAt.Equal(lock.ExpiresAt) {
		t.Fatalf("lock %+v, %v", held, err)
	}

	earlier := file
	earlier.ID = freshID(t)
	var number int
	if err := write(ctx, t, tenantA, func(ctx context.Context) (err error) {
		number, err = repo.SaveVersion(ctx, file.ID, mediarepo.Save{
			Earlier: earlier, StorageKey: file.StorageKey + "-v2", ContentType: file.ContentType,
			ByteSize: 7, SavedBy: authorA, SavedAt: at,
		})
		return err
	}); err != nil || number != 1 {
		t.Fatalf("save %d, %v", number, err)
	}
	head := findMedia(ctx, t, tenantA, file.ID)
	kept := findMedia(ctx, t, tenantA, earlier.ID)
	if head.StorageKey != file.StorageKey+"-v2" || head.ByteSize != 7 {
		t.Errorf("the file did not take the new bytes: %+v", head)
	}
	if kept.StorageKey != file.StorageKey || kept.Status != media.StatusReady {
		t.Errorf("the earlier version is not the old bytes: %+v", kept)
	}
	var versions []mediarepo.Version
	if err := read(ctx, t, tenantA, func(ctx context.Context) (err error) {
		versions, err = repo.Versions(ctx, file.ID)
		return err
	}); err != nil || len(versions) != 1 || versions[0].EarlierMediaID != earlier.ID || versions[0].SavedBy != authorA {
		t.Fatalf("versions %+v, %v", versions, err)
	}
	// The earlier version is referenced while the file lives: the recount keeps it.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return mediaRepo().Recount(ctx, at) }); err != nil {
		t.Fatal(err)
	}
	if kept := findMedia(ctx, t, tenantA, earlier.ID); kept.RefCount != 1 {
		t.Errorf("the earlier version counts %d references", kept.RefCount)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return repo.DropLock(ctx, file.ID) }); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) (err error) {
		held, err = repo.Lock(ctx, file.ID)
		return err
	}); err != nil || held != nil {
		t.Fatalf("the dropped lock reads %+v, %v", held, err)
	}

	// NONE removes the choice.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetDocumentServer(ctx, media.DocumentServerChoice{Kind: media.DocumentServerNone}, authorA, at)
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) (err error) {
		got, err = repo.DocumentServer(ctx)
		return err
	}); err != nil || got.IsOn() {
		t.Fatalf("after NONE %+v, %v", got, err)
	}
}

func TestTheWopiHostsRowsAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.WopiRepository{}
	at := created.Add(time.Hour)
	choice, _ := media.NewDocumentServerChoice(media.DocumentServerOnlyOffice, "https://docs.example")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetDocumentServer(ctx, choice, authorA, at)
	}); err != nil {
		t.Fatal(err)
	}
	file := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.PutLock(ctx, file.ID, media.Lock{ID: "a", ExpiresAt: at.Add(time.Hour)})
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("document server", func(t *testing.T) {
		var got media.DocumentServerChoice
		if err := read(ctx, t, tenantB, func(ctx context.Context) (err error) {
			got, err = repo.DocumentServer(ctx)
			return err
		}); err != nil || got.IsOn() {
			t.Errorf("tenant B read tenant A's document server: %+v %v", got, err)
		}
	})
	t.Run("lock", func(t *testing.T) {
		var held *media.Lock
		if err := read(ctx, t, tenantB, func(ctx context.Context) (err error) {
			held, err = repo.Lock(ctx, file.ID)
			return err
		}); err != nil || held != nil {
			t.Errorf("tenant B read tenant A's lock: %+v %v", held, err)
		}
		// A put from B cannot reach A's file: the tenant-scoped key refuses the row.
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return repo.PutLock(ctx, file.ID, media.Lock{ID: "b", ExpiresAt: at.Add(time.Hour)})
		}); err == nil {
			t.Error("tenant B locked tenant A's file")
		}
		if err := write(ctx, t, tenantB, func(ctx context.Context) error { return repo.DropLock(ctx, file.ID) }); err != nil {
			t.Fatal(err)
		}
		var still *media.Lock
		_ = read(ctx, t, tenantA, func(ctx context.Context) (err error) {
			still, err = repo.Lock(ctx, file.ID)
			return err
		})
		if still == nil || still.ID != "a" {
			t.Errorf("tenant B moved tenant A's lock: %+v", still)
		}
	})
	t.Run("save and versions", func(t *testing.T) {
		earlier := file
		earlier.ID = freshID(t)
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := repo.SaveVersion(ctx, file.ID, mediarepo.Save{
				Earlier: earlier, StorageKey: "x", ContentType: file.ContentType, ByteSize: 1, SavedAt: at,
			})
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B saved over tenant A's file: %v", err)
		}
		var versions []mediarepo.Version
		if err := read(ctx, t, tenantB, func(ctx context.Context) (err error) {
			versions, err = repo.Versions(ctx, file.ID)
			return err
		}); err != nil || len(versions) != 0 {
			t.Errorf("tenant B listed tenant A's versions: %+v %v", versions, err)
		}
	})
}
