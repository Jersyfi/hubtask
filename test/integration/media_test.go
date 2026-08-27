// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The media records against a real database (C-06): the upload life, the reference counting the
// reconciliation keeps honest, the cover under its constraints, and a cross-tenant negative for
// every method (gate SG-3).

func mediaRepo() postgres.MediaRepository {
	return postgres.NewMediaRepository(pageCursors())
}

// stagedMedia stages one object for the tenant and returns it as stored.
func stagedMedia(
	ctx context.Context, t *testing.T, tenant, author shared.ID, usage media.Usage,
) media.Object {
	t.Helper()

	object, err := media.NewPendingObject(media.NewObjectInput{
		ID: freshID(t), TenantID: tenant, FileName: "report.pdf",
		ClaimedType: "application/pdf", DeclaredSize: 4096, SizeLimit: 1 << 20,
		Usage: usage, CreatedBy: author, Now: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return mediaRepo().Insert(ctx, object)
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
	return object
}

func findMedia(ctx context.Context, t *testing.T, tenant, id shared.ID) media.Object {
	t.Helper()
	var stored media.Object
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		stored, err = mediaRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the object: %v", err)
	}
	return stored
}

func sealMedia(ctx context.Context, t *testing.T, tenant shared.ID, object media.Object) media.Object {
	t.Helper()
	sealed, err := object.Sealed("application/pdf", 4090)
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return mediaRepo().Seal(ctx, sealed)
	}); err != nil {
		t.Fatalf("sealing: %v", err)
	}
	return findMedia(ctx, t, tenant, object.ID)
}

func TestTheUploadLifeIsWrittenAndSealedOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	staged := stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment)

	stored := findMedia(ctx, t, tenantA, staged.ID)
	if stored.Status != media.StatusPending || stored.FileName != "report.pdf" ||
		stored.TenantID != tenantA {
		t.Fatalf("staged %+v", stored)
	}

	sealed := sealMedia(ctx, t, tenantA, staged)
	if sealed.Status != media.StatusReady || sealed.ByteSize != 4090 {
		t.Fatalf("sealed %+v", sealed)
	}

	// Sealing again matches nothing: the caller learns and answers from the row.
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return mediaRepo().Seal(ctx, sealed)
	})
	if got := shared.AsError(err).DetailCode; got != "media.already_confirmed" {
		t.Fatalf("the second seal answered %q", got)
	}
}

func TestReferencesAreCountedAndTheRecountKeepsThemHonest(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	object := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment))

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		linked, err := mediaRepo().Add(ctx, task, object.ID, shared.HLC{})
		if err != nil {
			return err
		}
		if !linked {
			t.Error("a fresh attachment reported itself known")
		}
		return mediaRepo().AdjustRefCount(ctx, object.ID, 1)
	}); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	if stored := findMedia(ctx, t, tenantA, object.ID); stored.RefCount != 1 {
		t.Fatalf("ref_count %d, want 1", stored.RefCount)
	}

	// Attaching what is attached changes nothing - and reports so.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		linked, err := mediaRepo().Add(ctx, task, object.ID, shared.HLC{})
		if linked {
			t.Error("a repeated attachment reported itself new")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// A marked deletion is refused while referenced.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		marked, err := mediaRepo().MarkDeleted(ctx, object.ID, changedAt)
		if marked {
			t.Error("a referenced object was marked for deletion")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// The purge path drops attachment rows through the cascade, never through AdjustRefCount:
	// the counter drifts, and the recount is what makes it honest.
	if _, err := adminPool(ctx, t).Exec(ctx,
		"DELETE FROM item_attachment WHERE media_id = $1", object.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return mediaRepo().Recount(ctx, changedAt)
	}); err != nil {
		t.Fatalf("recounting: %v", err)
	}
	if stored := findMedia(ctx, t, tenantA, object.ID); stored.RefCount != 0 {
		t.Fatalf("ref_count %d after the recount, want 0", stored.RefCount)
	}
	// The counter says nothing points at it; the stamp says since when, which is what the sweep
	// waits out. Read through SQL: it is bookkeeping the domain has no field for.
	if stamp := unreferencedSince(ctx, t, object.ID); stamp == nil {
		t.Fatal("the recount left an unreferenced object without a stamp")
	} else if !stamp.Equal(changedAt) {
		t.Errorf("stamped %s, want %s", stamp, changedAt)
	}

	// A reference appearing again clears it, so that the grace starts over the next time the
	// object loses one rather than measuring from the first time it ever did.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := mediaRepo().Add(ctx, task, object.ID, shared.HLC{}); err != nil {
			return err
		}
		return mediaRepo().Recount(ctx, changedAt.Add(time.Minute))
	}); err != nil {
		t.Fatalf("re-attaching: %v", err)
	}
	if stamp := unreferencedSince(ctx, t, object.ID); stamp != nil {
		t.Errorf("a referenced object still carries a stamp: %s", stamp)
	}
}

// unreferencedSince reads the sweep's stamp. Through the admin pool because it is the one column
// of media_object no port exposes: the domain has no use for it, and the reconciliation reads it
// only inside SQL.
func unreferencedSince(ctx context.Context, t *testing.T, id shared.ID) *time.Time {
	t.Helper()
	var stamp *time.Time
	if err := adminPool(ctx, t).QueryRow(ctx,
		"SELECT unreferenced_since FROM media_object WHERE id = $1", id.String()).
		Scan(&stamp); err != nil {
		t.Fatal(err)
	}
	return stamp
}

func TestTheOrphanSweepMarksTakesAndRemoves(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	orphan := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment))
	abandoned := stagedMedia(ctx, t, tenantA, authorA, media.UsageCover)
	fresh := stagedMedia(ctx, t, tenantA, authorA, media.UsageCover)

	now := changedAt
	// The abandoned staging is old; the fresh one is inside its upload window.
	if _, err := adminPool(ctx, t).Exec(ctx,
		"UPDATE media_object SET created_at = $2 WHERE id = $1",
		abandoned.ID.String(), now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	// The READY orphan lost its last reference two hours ago. The stamp is what says so, and the
	// recount is the only thing that writes it.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return mediaRepo().Recount(ctx, now.Add(-2*time.Hour))
	}); err != nil {
		t.Fatal(err)
	}

	// Confirmed just now and attached to nothing yet: the state every upload passes through
	// between its confirmation and the call that uses it. The pass below runs straight through
	// that window, which is what used to mark it.
	justConfirmed := sealMedia(
		ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment))

	// The suites share a tenant, so the sweep may find other tests' leftovers beside these -
	// the assertions are about *these* rows, never about totals.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := mediaRepo().Recount(ctx, now); err != nil {
			return err
		}
		marked, err := mediaRepo().MarkOrphans(ctx, now, mediarepo.Thresholds{
			Unreferenced: now.Add(-time.Hour), Pending: now.Add(-24 * time.Hour),
		})
		if err != nil {
			return err
		}
		if marked < 2 {
			t.Errorf("marked %d, want at least the READY orphan and the abandoned staging", marked)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if stored := findMedia(ctx, t, tenantA, fresh.ID); stored.DeletedAt != nil {
		t.Fatal("a staging inside its window was marked")
	}
	if stored := findMedia(ctx, t, tenantA, justConfirmed.ID); stored.DeletedAt != nil {
		t.Fatal("an object confirmed inside the grace was marked before anything could use it")
	}
	// The second recount did not restart the first one's clock: the orphan is marked on the stamp
	// it got two hours ago, not on the zero this pass saw.
	if stored := findMedia(ctx, t, tenantA, orphan.ID); stored.DeletedAt == nil {
		t.Fatal("an object unreferenced for two hours survived the sweep")
	}

	var taken []mediarepo.Orphan
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		taken, err = mediaRepo().TakeOrphans(ctx, now.Add(time.Second), 100)
		if err != nil {
			return err
		}
		mine := make([]shared.ID, 0, 2)
		for _, o := range taken {
			if o.StorageKey == "" {
				t.Errorf("an orphan travels without its key: %+v", o)
			}
			if o.ID == orphan.ID || o.ID == abandoned.ID {
				mine = append(mine, o.ID)
			}
		}
		if len(mine) != 2 {
			t.Errorf("the sweep took %d of these two rows, want both", len(mine))
		}
		removed, err := mediaRepo().RemoveRows(ctx, mine)
		if removed != len(mine) {
			t.Errorf("removed %d of %d", removed, len(mine))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := mediaRepo().Find(ctx, orphan.ID)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the removed orphan still answers: %v", err)
	}
}

func TestTheCoverRoundTripsUnderItsConstraints(t *testing.T) {
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

	stored := findItem(ctx, t, tenantA, task)
	if stored.Cover == nil || stored.Cover.Kind != work.CoverImage ||
		stored.Cover.MediaID != object.ID {
		t.Fatalf("the cover came back as %+v", stored.Cover)
	}

	// The FK holds: a covered object's row cannot be removed out from under the item.
	if _, err := adminPool(ctx, t).Exec(ctx,
		"DELETE FROM media_object WHERE id = $1", object.ID.String()); err == nil {
		t.Fatal("the database removed a media object a cover references")
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCover(ctx, stored.Uncovered(changedAt.Add(time.Minute)), stored.Version)
	}); err != nil {
		t.Fatalf("clearing the cover: %v", err)
	}
	if bare := findItem(ctx, t, tenantA, task); bare.Cover != nil {
		t.Fatalf("the cover survived its clearing: %+v", bare.Cover)
	}
}

func TestAttachmentsListAndPage(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	var objects []media.Object
	for i := 0; i < 3; i++ {
		object := sealMedia(ctx, t, tenantA,
			stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment))
		if _, err := adminPool(ctx, t).Exec(ctx,
			"UPDATE media_object SET created_at = $2 WHERE id = $1",
			object.ID.String(), created.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			if _, err := mediaRepo().Add(ctx, task, object.ID, shared.HLC{}); err != nil {
				return err
			}
			return mediaRepo().AdjustRefCount(ctx, object.ID, 1)
		}); err != nil {
			t.Fatal(err)
		}
		objects = append(objects, object)
	}

	var page mediarepo.ObjectPage
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		page, err = mediaRepo().ListForItem(ctx, task, repository.Page{Size: 2})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(page.Objects) != 2 || !page.Info.HasMore {
		t.Fatalf("first page: %d objects, more=%v", len(page.Objects), page.Info.HasMore)
	}

	var ids []shared.ID
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		ids, err = mediaRepo().MediaIDs(ctx, task)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 {
		t.Fatalf("the entry carries %d attachments, want 3", len(ids))
	}

	// Detaching reports the difference between a link and none.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		removed, err := mediaRepo().Remove(ctx, task, objects[0].ID, shared.HLC{})
		if err != nil || !removed {
			t.Errorf("detaching an attachment: removed=%v err=%v", removed, err)
		}
		again, err := mediaRepo().Remove(ctx, task, objects[0].ID, shared.HLC{})
		if again {
			t.Error("detaching nothing reported a link")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	var refs []mediarepo.ItemRef
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		refs, err = mediaRepo().ReferencingItems(ctx, objects[1].ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ItemID != task || refs[0].CollectionID != collection {
		t.Fatalf("the references are %+v", refs)
	}
}

func TestMediaIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	object := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := mediaRepo().Add(ctx, task, object.ID, shared.HLC{}); err != nil {
			return err
		}
		return mediaRepo().AdjustRefCount(ctx, object.ID, 1)
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := mediaRepo().Find(ctx, object.ID)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B read tenant A's object: %v", err)
		}
	})

	t.Run("seal and adjust and mark", func(t *testing.T) {
		pending := stagedMedia(ctx, t, tenantA, authorA, media.UsageCover)
		sealErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			sealed, err := pending.Sealed("application/pdf", 1)
			if err != nil {
				return err
			}
			return mediaRepo().Seal(ctx, sealed)
		})
		if shared.AsError(sealErr).DetailCode != "media.already_confirmed" {
			t.Errorf("tenant B sealed tenant A's staging: %v", sealErr)
		}
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return mediaRepo().AdjustRefCount(ctx, object.ID, 5)
		}); err != nil {
			t.Fatal(err)
		}
		if stored := findMedia(ctx, t, tenantA, object.ID); stored.RefCount != 1 {
			t.Errorf("tenant B moved tenant A's counter to %d", stored.RefCount)
		}
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			marked, err := mediaRepo().MarkDeleted(ctx, object.ID, changedAt)
			if marked {
				t.Error("tenant B marked tenant A's object")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("sweep stays home", func(t *testing.T) {
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			if err := mediaRepo().Recount(ctx, changedAt); err != nil {
				return err
			}
			marked, err := mediaRepo().MarkOrphans(ctx, changedAt, mediarepo.Thresholds{
				Unreferenced: changedAt, Pending: changedAt,
			})
			if err != nil {
				return err
			}
			if marked != 0 {
				t.Errorf("tenant B's sweep marked %d of tenant A's rows", marked)
			}
			orphans, err := mediaRepo().TakeOrphans(ctx, changedAt, 10)
			if err != nil {
				return err
			}
			if len(orphans) != 0 {
				t.Errorf("tenant B took %d of tenant A's orphans", len(orphans))
			}
			removed, err := mediaRepo().RemoveRows(ctx, []shared.ID{object.ID})
			if removed != 0 {
				t.Errorf("tenant B removed %d of tenant A's rows", removed)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("attachments and references", func(t *testing.T) {
		// Two ways in, both closed: the pair tenant A already linked collides on the primary key
		// and is swallowed as no rows; a pair of tenant A's rows that is not yet linked fails the
		// tenant-scoped foreign keys. Neither leaves a row of tenant B's.
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			linked, err := mediaRepo().Add(ctx, task, object.ID, shared.HLC{})
			if linked {
				t.Error("tenant B attached tenant A's object to tenant A's entry")
			}
			_ = err
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		fresh := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment))
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			linked, err := mediaRepo().Add(ctx, task, fresh.ID, shared.HLC{})
			if linked {
				t.Error("tenant B linked tenant A's rows across the boundary")
			}
			return err
		})
		if err == nil {
			t.Fatal("the tenant-scoped foreign keys let a cross-tenant link through")
		}
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			removed, err := mediaRepo().Remove(ctx, task, object.ID, shared.HLC{})
			if removed {
				t.Error("tenant B detached tenant A's attachment")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			ids, err := mediaRepo().MediaIDs(ctx, task)
			if len(ids) != 0 {
				t.Errorf("tenant B listed tenant A's attachments: %v", ids)
			}
			refs, refErr := mediaRepo().ReferencingItems(ctx, object.ID)
			if len(refs) != 0 {
				t.Errorf("tenant B read tenant A's references: %v", refs)
			}
			page, pageErr := mediaRepo().ListForItem(ctx, task, repository.Page{Size: 5})
			if len(page.Objects) != 0 {
				t.Errorf("tenant B paged tenant A's attachments")
			}
			if err != nil {
				return err
			}
			if refErr != nil {
				return refErr
			}
			return pageErr
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cover", func(t *testing.T) {
		item := findItem(ctx, t, tenantA, task)
		cover, err := work.NewCover(work.CoverColor, "blue", "")
		if err != nil {
			t.Fatal(err)
		}
		err = write(ctx, t, tenantB, func(ctx context.Context) error {
			return itemRepo().SetCover(ctx, item.Covered(cover, changedAt), item.Version)
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("tenant B's cover write answered %v", err)
		}
		if stored := findItem(ctx, t, tenantA, task); stored.Cover != nil {
			t.Error("tenant B covered tenant A's entry")
		}
	})
}
