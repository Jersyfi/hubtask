// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// previousRecount is the reconciliation's recount of the release before migration 0449, word for
// word but its comments: it does not count `workspace_picture`. A process of that release runs it
// during a rolling update, and after a rollback for as long as it runs.
const previousRecount = `
UPDATE media_object m SET
  ref_count = c.total,
  unreferenced_since = CASE
    WHEN c.total = 0 AND m.status = 'READY' THEN COALESCE(m.unreferenced_since, $1)
    ELSE NULL
  END
FROM (
  SELECT o.id,
    (SELECT count(*) FROM item_attachment a WHERE a.media_id = o.id)
    + (SELECT count(*) FROM work_item w WHERE w.cover_media_id = o.id)
    + (SELECT count(*) FROM jumble_entry j WHERE o.id = ANY(j.attachments))
    + (SELECT count(*) FROM account p WHERE p.avatar_media_id = o.id)
    + (SELECT count(*) FROM container d WHERE d.logo_media_id = o.id)
    + (SELECT count(*) FROM whiteboard_media b WHERE b.media_id = o.id)
    + (SELECT count(*) FROM page_media g WHERE g.media_id = o.id)
    + (SELECT count(*) FROM cmdb_ci_media c WHERE c.media_id = o.id)
    + (SELECT count(*) FROM media_version v WHERE v.earlier_media_id = o.id) AS total
  FROM media_object o
  WHERE o.deleted_at IS NULL
) c
WHERE m.id = c.id AND m.deleted_at IS NULL`

func mediaState(ctx context.Context, t *testing.T, id shared.ID) (refCount int, stamp, deleted *time.Time, found bool) {
	t.Helper()
	rows, err := adminPool(ctx, t).Query(ctx,
		`SELECT ref_count, unreferenced_since, deleted_at FROM media_object WHERE id = $1`, id.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&refCount, &stamp, &deleted); err != nil {
			t.Fatal(err)
		}
		found = true
	}
	return refCount, stamp, deleted, found
}

// A pass of the previous release - its recount, then its marking, a hand deletion and the purge,
// whose statements this release kept as they were - against a picture the workspace shows
// (migration 0449, rule 12). The picture was confirmed long before it was set, so it carried a
// stamp from before. No step of that pass marks it, so the purge never takes it and the media's
// cascade never takes the picture's row; a picture the workspace no longer shows is reclaimed as
// before.
func TestAPreviousReleasesSweepLeavesAWorkspacePictureInPlace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	t.Cleanup(func() { clearBranding(ctx, t, tenantA) })
	logo := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageWorkspaceLogo))
	taken := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageWorkspaceLogo))
	start := created.Add(10 * time.Hour)

	// Confirmed and unused at a pass: both are stamped then.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return mediaRepo().Recount(ctx, start) }); err != nil {
		t.Fatal(err)
	}
	if _, stamp, _, _ := mediaState(ctx, t, logo.ID); stamp == nil {
		t.Fatal("the unused picture was not stamped: the case below would not be the one it claims")
	}

	// Set as SetWorkspacePicture sets it: the rows and the counts in one transaction.
	set := func(branding identity.Branding, deltas map[shared.ID]int) {
		t.Helper()
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			if err := (postgres.WorkspaceBrandingRepository{}).Set(ctx, branding, start); err != nil {
				return err
			}
			for id, delta := range deltas {
				if err := mediaRepo().AdjustRefCount(ctx, id, delta); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	set(identity.Branding{Header: identity.HeaderLogo, LogoLight: logo.ID, LogoDark: taken.ID}, map[shared.ID]int{logo.ID: 1, taken.ID: 1})
	if _, stamp, _, _ := mediaState(ctx, t, logo.ID); stamp != nil {
		t.Errorf("a picture the workspace shows keeps the stamp %s from before it was set", stamp)
	}
	// The dark logo is taken out again: from here nothing references it.
	set(identity.Branding{Header: identity.HeaderLogo, LogoLight: logo.ID}, map[shared.ID]int{taken.ID: -1})

	// The previous release's pass, three times, each past every grace.
	for pass, at := range []time.Time{start.Add(3 * time.Hour), start.Add(6 * time.Hour), start.Add(9 * time.Hour)} {
		asWorkspace(ctx, t, tenantA, previousRecount, at)
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			if _, err := mediaRepo().MarkOrphans(ctx, at, mediarepo.Thresholds{
				Unreferenced: at.Add(-time.Hour), Pending: at.Add(-24 * time.Hour),
			}); err != nil {
				return err
			}
			marked, err := mediaRepo().MarkDeleted(ctx, logo.ID, at)
			if err != nil {
				return err
			}
			if marked {
				t.Errorf("pass %d: the hand deletion marked the workspace's logo", pass)
			}
			orphans, err := mediaRepo().TakeOrphans(ctx, at.Add(-time.Hour), 1000)
			if err != nil {
				return err
			}
			ids := make([]shared.ID, 0, len(orphans))
			for _, orphan := range orphans {
				if orphan.ID == logo.ID {
					t.Errorf("pass %d: the purge was handed the workspace's logo", pass)
				}
				ids = append(ids, orphan.ID)
			}
			_, err = mediaRepo().RemoveRows(ctx, ids)
			return err
		}); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		count, stamp, deleted, found := mediaState(ctx, t, logo.ID)
		if !found || count < 1 || stamp != nil || deleted != nil {
			t.Errorf("pass %d: the logo reads found %v, ref_count %d, unreferenced since %v, marked %v",
				pass, found, count, stamp, deleted)
		}
	}
	if held, err := lockBranding(ctx, t, tenantA); err != nil || held.LogoLight != logo.ID {
		t.Errorf("after the previous release's passes the workspace holds %+v, %v", held, err)
	}

	// The picture taken out is reclaimed as the previous release always did.
	if _, _, _, found := mediaState(ctx, t, taken.ID); found {
		t.Error("a picture the workspace no longer shows was kept")
	}
}
