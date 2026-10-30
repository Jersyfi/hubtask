// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/realtime"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A document's updates take consecutive places, a snapshot marks what it covers without removing
// it, and retention removes only what is compacted and old enough (migration 0230).
func TestADocumentLogIsOrderedCompactedAndPrunedByRetention(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewRealtimeRepository()
	ref := realtime.Ref{Kind: "page", ID: freshID(t)}
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for i := 1; i <= 3; i++ {
			seq, err := repo.Append(ctx, ref, []byte{byte(i)}, authorA, at)
			if err != nil {
				return err
			}
			if seq != int64(i) {
				t.Errorf("update %d took place %d", i, seq)
			}
		}
		stored, err := repo.StoreSnapshot(ctx, ref, realtime.Snapshot{UpToSeq: 2, State: []byte("ab"), At: at})
		if err != nil || !stored {
			t.Errorf("the snapshot was not stored: %v", err)
		}
		again, err := repo.StoreSnapshot(ctx, ref, realtime.Snapshot{UpToSeq: 2, State: []byte("ab"), At: at})
		if err != nil || again {
			t.Errorf("a second snapshot at the same place was stored: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		head, err := repo.Head(ctx, ref)
		if err != nil || head.Seq != 3 || head.SnapshotSeq != 2 {
			t.Errorf("head %+v, %v; want 3 with the snapshot at 2", head, err)
		}
		updates, err := repo.Since(ctx, ref, 0, 10)
		if err != nil || len(updates) != 3 || updates[0].AuthorID != authorA {
			t.Errorf("the log read back %+v, %v; want all three until retention", updates, err)
		}
		if n, err := repo.CountExpired(ctx, at.Add(time.Hour), 100); err != nil || n != 2 {
			t.Errorf("%d expired, %v; want the two compacted updates", n, err)
		}
		if n, err := repo.CountExpired(ctx, at, 100); err != nil || n != 0 {
			t.Errorf("%d expired at the moment of compaction, %v; want none", n, err)
		}
		removed, err := repo.DeleteExpired(ctx, at.Add(time.Hour), 100)
		if err != nil || removed != 2 {
			t.Errorf("removed %d, %v; want 2", removed, err)
		}
		snapshot, found, err := repo.Snapshot(ctx, ref)
		if err != nil || !found || string(snapshot.State) != "ab" {
			t.Errorf("the snapshot went with the history: %+v, %v", snapshot, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Another workspace sees no document, no head and no snapshot, cannot append to it, and erases
// nothing of it.
func TestADocumentIsInvisibleToTheWorkspaceNextDoor(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewRealtimeRepository()
	ref := realtime.Ref{Kind: "page", ID: freshID(t)}
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := repo.Append(ctx, ref, []byte{1}, authorA, at); err != nil {
			return err
		}
		_, err := repo.StoreSnapshot(ctx, ref, realtime.Snapshot{UpToSeq: 1, State: []byte{1}, At: at})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		head, err := repo.Head(ctx, ref)
		if err != nil || head.Seq != 0 {
			t.Errorf("the workspace next door read head %+v, %v", head, err)
		}
		if _, found, _ := repo.Snapshot(ctx, ref); found {
			t.Error("the workspace next door read the snapshot")
		}
		if updates, _ := repo.Since(ctx, ref, 0, 10); len(updates) != 0 {
			t.Errorf("the workspace next door read %d updates", len(updates))
		}
		if n, _ := repo.CountExpired(ctx, at.Add(time.Hour), 100); n != 0 {
			t.Errorf("the workspace next door counted %d of the history", n)
		}
		if n, _ := repo.DeleteExpired(ctx, at.Add(time.Hour), 100); n != 0 {
			t.Errorf("the workspace next door removed %d of the history", n)
		}
		if err := repo.Erase(ctx, ref); err != nil {
			return err
		}
		// An append under the same reference opens a document of its own workspace.
		seq, err := repo.Append(ctx, ref, []byte{2}, authorB, at)
		if err != nil || seq != 1 {
			t.Errorf("the workspace next door appended at %d, %v; want its own first place", seq, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		head, err := repo.Head(ctx, ref)
		if err != nil || head.Seq != 1 || head.SnapshotSeq != 1 {
			t.Errorf("the owner's document moved: %+v, %v", head, err)
		}
		if err := repo.Erase(ctx, ref); err != nil {
			return err
		}
		head, err = repo.Head(ctx, ref)
		if err != nil || head.Seq != 0 {
			t.Errorf("an erased document still stands at %+v", head)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A hub purged takes its pages' and boards' live documents with them, history and snapshots
// included; a document of a page in another hub stays (migration 0260).
func TestAHubPurgedTakesItsLiveDocuments(t *testing.T) {
	ctx := context.Background()
	pageHub, page, _ := seedSpace(ctx, t)
	boardHub, board, _ := seedBoard(ctx, t)
	_, otherPage, _ := seedSpace(ctx, t)
	repo := postgres.NewRealtimeRepository()
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	refs := []realtime.Ref{{Kind: "page", ID: page}, {Kind: "whiteboard", ID: board}, {Kind: "page", ID: otherPage}}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, ref := range refs {
			if _, err := repo.Append(ctx, ref, []byte{1}, authorA, at); err != nil {
				return err
			}
			if _, err := repo.StoreSnapshot(ctx, ref, realtime.Snapshot{UpToSeq: 1, State: []byte{1}, At: at}); err != nil {
				return err
			}
		}
		_, err := trashRepo().PurgeContainers(ctx, []shared.ID{pageHub, boardHub})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	for _, gone := range refs[:2] {
		if rows := countIn(ctx, t, `SELECT count(*) FROM realtime_update WHERE document_id = $1`, gone.ID.String()); rows != 0 {
			t.Errorf("the %s's history outlived its hub: %d updates", gone.Kind, rows)
		}
		if rows := countIn(ctx, t, `SELECT count(*) FROM realtime_document WHERE document_id = $1`, gone.ID.String()); rows != 0 {
			t.Errorf("the %s's live document outlived its hub", gone.Kind)
		}
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM realtime_snapshot WHERE document_id = $1`, otherPage.String()); rows != 1 {
		t.Errorf("a page in another hub lost its live document: %d snapshots", rows)
	}
}
