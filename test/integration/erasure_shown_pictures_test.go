// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// shownPicture seeds a confirmed PNG the subject uploaded for a usage, to be shown by something
// that is the workspace's, not theirs.
func shownPicture(ctx context.Context, t *testing.T, subject person, usage string) shared.ID {
	t.Helper()
	id := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, checksum, usage,
		  status, created_by)
		VALUES ($1, $2, $3, 'image/png', 64, $4, $5, 'READY', $6)`,
		id.String(), subject.tenant.String(), "media/"+id.String(), "sha-"+id.String(), usage,
		subject.account.String()); err != nil {
		t.Fatalf("seeding the picture: %v", err)
	}
	return id
}

// fullyErase runs the whole erasure against PostgreSQL, and checks it discarded the subject's
// upload nothing points at, so a spared picture is spared by the rule and not by an erasure that
// discarded nothing.
func fullyErase(ctx context.Context, t *testing.T, subject person) {
	t.Helper()
	erased, err := eraserFor(t).Erase(ctx, subjectActor(subject),
		erasureRequest(ctx, t, subject, domain.ModeFullDelete))
	if err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if !erased.AccountRemoved {
		t.Fatalf("the account was not removed: %+v", erased)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM media_object WHERE id = $1`, subject.media.String()); rows != 0 {
		t.Fatalf("the subject's unattached upload survived the erasure")
	}
}

// UC-INS-22 check 1, the workspace keeps each picture: an erasure removes the uploads nothing else
// points at (data-catalog.md); a workspace's logo is the
// workspace's, as an attachment is, so the erasure of the person who uploaded it leaves the picture
// and the workspace's row naming it. The uploader's identifier stays on the object, pointing at
// nobody, as on a spared attachment; the pseudonym answers who it was.
func TestAnErasureLeavesTheWorkspaceItsPictures(t *testing.T) {
	ctx := context.Background()
	subject := seedSubject(ctx, t, "carla-"+freshID(t).String()+"@example.org")
	logo := shownPicture(ctx, t, subject, "WORKSPACE_LOGO")
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO workspace_picture (tenant_id, picture, media_id) VALUES ($1, 'LOGO_LIGHT', $2)`,
		subject.tenant.String(), logo.String()); err != nil {
		t.Fatalf("showing the logo: %v", err)
	}

	fullyErase(ctx, t, subject)

	if rows := countIn(ctx, t, `SELECT count(*) FROM media_object WHERE id = $1 AND created_by = $2`,
		logo.String(), subject.account.String()); rows != 1 {
		t.Error("the erasure discarded the workspace's logo")
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM workspace_picture WHERE tenant_id = $1 AND media_id = $2`,
		subject.tenant.String(), logo.String()); rows != 1 {
		t.Error("the workspace no longer shows its logo after its uploader's erasure")
	}
}

// UC-SVC-05 check 9, the same for a desk's logo: the collection's portal shows it to every requester, so it is the
// workspace's, and the erasure of its uploader leaves it and the collection's reference to it.
func TestAnErasureLeavesADeskItsLogo(t *testing.T) {
	ctx := context.Background()
	subject := seedSubject(ctx, t, "dora-"+freshID(t).String()+"@example.org")
	logo := shownPicture(ctx, t, subject, "DESK_LOGO")
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE container SET logo_media_id = $2 WHERE id = $1`,
		subject.collection.String(), logo.String()); err != nil {
		t.Fatalf("showing the desk's logo: %v", err)
	}

	fullyErase(ctx, t, subject)

	if rows := countIn(ctx, t, `SELECT count(*) FROM media_object WHERE id = $1`, logo.String()); rows != 1 {
		t.Error("the erasure discarded the desk's logo")
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM container WHERE id = $1 AND logo_media_id = $2`,
		subject.collection.String(), logo.String()); rows != 1 {
		t.Error("the desk no longer shows its logo after its uploader's erasure")
	}
}
