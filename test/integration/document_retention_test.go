// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	lifecycle "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The pages' and boards' trash and the page versions' thinning against the real statements
// (accepted.md B): another workspace's pass reads and removes nothing of this one's; this one's
// takes a trashed leaf with its live document and media rows, never a trashed page with a page
// under it, and thins the versions past the period while a named one and each day's last stay.
func TestTheDocumentRetentionStaysInItsWorkspaceAndKeepsWhatItMust(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	pool := adminPool(ctx, t)
	var hub string
	if err := pool.QueryRow(ctx, `SELECT parent_id::text FROM container WHERE tenant_id = $1 AND id = $2`,
		tenantA.String(), collection.String()).Scan(&hub); err != nil {
		t.Fatalf("reading the hub: %v", err)
	}
	long := time.Now().UTC().AddDate(0, 0, -200)
	parent, leaf, live := freshID(t), freshID(t), freshID(t)
	pageAt := func(id shared.ID, parentID any, deleted any) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO page (id, tenant_id, hub_id, parent_id, title, order_key, created_by,
			created_at, updated_by, updated_at, deleted_at) VALUES ($1, $2, $3, $4, 'P', 'a', $5, $6, $5, $6, $7)`,
			id.String(), tenantA.String(), hub, parentID, authorA.String(), long, deleted); err != nil {
			t.Fatalf("writing a page: %v", err)
		}
	}
	pageAt(parent, nil, long)
	pageAt(leaf, parent.String(), long)
	pageAt(live, nil, nil)
	if _, err := pool.Exec(ctx, `INSERT INTO realtime_document (tenant_id, kind, document_id, created_at, updated_at)
		VALUES ($1, 'page', $2, now(), now())`, tenantA.String(), leaf.String()); err != nil {
		t.Fatalf("writing the live document: %v", err)
	}
	// Versions of the live page: three on one day long ago (two unnamed, the day's last), one named.
	day := time.Date(long.Year(), long.Month(), long.Day(), 9, 0, 0, 0, time.UTC)
	for seq, version := range []struct {
		at   time.Time
		name string
	}{{day, ""}, {day.Add(time.Hour), "Signed off"}, {day.Add(2 * time.Hour), ""}, {day.Add(3 * time.Hour), ""}} {
		if _, err := pool.Exec(ctx, `INSERT INTO page_version (tenant_id, page_id, seq, title, body, name, created_at)
			VALUES ($1, $2, $3, 'P', '', $4, $5)`, tenantA.String(), live.String(), seq+1, version.name, version.at); err != nil {
			t.Fatalf("writing a version: %v", err)
		}
	}

	trash, versions := postgres.DocumentTrash{}, postgres.PageVersionThinning{}
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		expired, err := trash.Expired(ctx, cutoff, nil, 100)
		if err != nil || len(expired) != 0 {
			t.Errorf("another workspace read %d trashed documents (%v)", len(expired), err)
		}
		n, err := versions.DeleteThinned(ctx, cutoff, nil, nil, 100)
		if err != nil || n != 0 {
			t.Errorf("another workspace thinned %d versions (%v)", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		expired, err := trash.Expired(ctx, cutoff, nil, 100)
		if err != nil {
			return err
		}
		if len(expired) != 1 || expired[0].ID != leaf {
			t.Errorf("read %+v, want only the leaf", expired)
		}
		// The parent named directly is refused while its child stands.
		if n, err := trash.Remove(ctx, append(expired, postgresDocument(parent)), cutoff); err != nil || n != 1 {
			t.Errorf("removed %d (%v), want the leaf alone", n, err)
		}
		n, err := versions.DeleteThinned(ctx, time.Now().UTC().AddDate(0, 0, -90), nil, nil, 100)
		if err != nil || n != 2 {
			t.Errorf("thinned %d (%v), want the two unnamed versions before the day's last", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM realtime_document WHERE document_id = $1`, leaf.String()); n != 0 {
		t.Errorf("the leaf's live document is still there")
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM page WHERE id = $1`, parent.String()); n != 1 {
		t.Errorf("the parent went before the pass judged it as a leaf")
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM page_version WHERE page_id = $1`, live.String()); n != 2 {
		t.Errorf("%d versions left, want the named one and the day's last", n)
	}
}

func postgresDocument(id shared.ID) lifecycle.TrashedDocument {
	return lifecycle.TrashedDocument{Entity: "page", ID: id}
}

// WHITEBOARD_HISTORY against the real statements (UC-CNV-06 check 5): another workspace's pass
// removes nothing of this one's; this one's takes an unnamed version past the cutoff and a thread
// resolved before it with its reply, while a named version, the board's newest, an open thread and
// a thread a held author wrote in stay.
func TestABoardsHistoryAgesOutInItsWorkspaceAndKeepsWhatItMust(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	pool := adminPool(ctx, t)
	var hub string
	if err := pool.QueryRow(ctx, `SELECT parent_id::text FROM container WHERE tenant_id = $1 AND id = $2`,
		tenantA.String(), collection.String()).Scan(&hub); err != nil {
		t.Fatalf("reading the hub: %v", err)
	}
	long := time.Now().UTC().AddDate(0, 0, -200)
	board := freshID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO whiteboard (id, tenant_id, hub_id, title, created_by, created_at,
		updated_by, updated_at) VALUES ($1, $2, $3, 'B', $4, $5, $4, $5)`,
		board.String(), tenantA.String(), hub, authorA.String(), long); err != nil {
		t.Fatalf("writing a board: %v", err)
	}
	for seq, name := range []string{"", "Agreed", "", ""} {
		if _, err := pool.Exec(ctx, `INSERT INTO whiteboard_version (tenant_id, whiteboard_id, seq, title, scene, name, created_at)
			VALUES ($1, $2, $3, 'B', '[]', $4, $5)`, tenantA.String(), board.String(), seq+1, name, long.Add(time.Duration(seq)*time.Hour)); err != nil {
			t.Fatalf("writing a version: %v", err)
		}
	}
	comment := func(parent any, author string, resolved any) shared.ID {
		t.Helper()
		id := freshID(t)
		if _, err := pool.Exec(ctx, `INSERT INTO whiteboard_comment (id, tenant_id, whiteboard_id, parent_id, author_id, body,
			resolved_at, created_at) VALUES ($1, $2, $3, $4, $5, 'c', $6, $7)`,
			id.String(), tenantA.String(), board.String(), parent, author, resolved, long); err != nil {
			t.Fatalf("writing a comment: %v", err)
		}
		return id
	}
	gone := comment(nil, authorA.String(), long)
	comment(gone.String(), authorA.String(), nil)
	open := comment(nil, authorA.String(), nil)
	held := comment(nil, authorA.String(), long)
	heldAuthor := freshID(t).String()
	if _, err := pool.Exec(ctx, `INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Held')`,
		heldAuthor, tenantA.String()); err != nil {
		t.Fatalf("writing the held author: %v", err)
	}
	comment(held.String(), heldAuthor, nil)

	history := postgres.WhiteboardHistoryAging{}
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		n, err := history.DeleteAged(ctx, cutoff, nil, nil, 100)
		if err != nil || n != 0 {
			t.Errorf("another workspace aged out %d rows (%v)", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	heldAccounts := []shared.ID{shared.MustParseID(heldAuthor)}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		due, err := history.CountAged(ctx, cutoff, nil, heldAccounts, 100)
		if err != nil {
			return err
		}
		n, err := history.DeleteAged(ctx, cutoff, nil, heldAccounts, 100)
		if err != nil {
			return err
		}
		if n != due {
			t.Errorf("counted %d due and removed %d", due, n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	var seqs []int32
	rows, err := pool.Query(ctx, `SELECT seq FROM whiteboard_version WHERE tenant_id = $1 AND whiteboard_id = $2 ORDER BY seq`,
		tenantA.String(), board.String())
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var seq int32
		if err := rows.Scan(&seq); err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, seq)
	}
	rows.Close()
	if len(seqs) != 2 || seqs[0] != 2 || seqs[1] != 4 {
		t.Errorf("versions left %v, want the named 2 and the newest 4", seqs)
	}
	var left []string
	rows, err = pool.Query(ctx, `SELECT id::text FROM whiteboard_comment WHERE tenant_id = $1 AND whiteboard_id = $2 AND parent_id IS NULL`,
		tenantA.String(), board.String())
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		left = append(left, id)
	}
	rows.Close()
	if len(left) != 2 || !containsText(left, open.String()) || !containsText(left, held.String()) {
		t.Errorf("threads left %v, want the open one and the held one", left)
	}
	var replies int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM whiteboard_comment WHERE tenant_id = $1 AND parent_id = $2`,
		tenantA.String(), gone.String()).Scan(&replies); err != nil || replies != 0 {
		t.Errorf("%d replies of the removed thread stayed (%v)", replies, err)
	}
}

func containsText(list []string, want string) bool {
	for _, each := range list {
		if each == want {
			return true
		}
	}
	return false
}
