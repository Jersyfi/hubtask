// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"
)

// A hub's posts survive an archive: the post, who saw and confirmed it, its discussion with a
// reaction and who follows the hub's posts leave through the export and come back through the
// import with every column (backup-restore.md §3, tenant-export.md, migrations 0401 and 0404).
func TestAPostAndItsTracesSurviveTheRoundTripThroughTheArchive(t *testing.T) {
	ctx := context.Background()
	hub, page, _, at := seedPost(ctx, t)
	reply := freshID(t)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO post_read (tenant_id, page_id, account_id, read_at) VALUES ($1, $2, $3, $4)`, []any{tenantA.String(), page.String(), authorA.String(), at}},
		{`INSERT INTO post_confirmation (tenant_id, page_id, account_id, confirmed_at) VALUES ($1, $2, $3, $4)`, []any{tenantA.String(), page.String(), authorA.String(), at}},
		{`INSERT INTO post_reply (id, tenant_id, page_id, author_id, body, created_at, edited_at) VALUES ($1, $2, $3, $4, 'Thanks', $5, $5)`,
			[]any{reply.String(), tenantA.String(), page.String(), authorA.String(), at}},
		{`INSERT INTO post_reaction (tenant_id, reply_id, account_id, reaction) VALUES ($1, $2, $3, 'THANKS')`, []any{tenantA.String(), reply.String(), authorA.String()}},
		{`INSERT INTO post_follow (tenant_id, hub_id, account_id, followed_at) VALUES ($1, $2, $3, $4)`, []any{tenantA.String(), hub.String(), authorA.String(), at}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	// In restore order: the post before what hangs from it.
	tables := []string{"post", "post_read", "post_confirmation", "post_reply", "post_reaction", "post_follow"}
	ours := func(table string) map[string]map[string]any {
		rows := map[string]map[string]any{}
		for _, row := range exported(ctx, t, tenantA, 100, table, time.Time{}) {
			if row.Data["page_id"] == page.String() || row.Data["reply_id"] == reply.String() || row.Data["hub_id"] == hub.String() {
				rows[row.ID] = row.Data
			}
		}
		return rows
	}
	before := map[string]map[string]map[string]any{}
	for _, table := range tables {
		if before[table] = ours(table); len(before[table]) != 1 {
			t.Fatalf("%s exported %d rows of the post, want 1", table, len(before[table]))
		}
	}

	// The post goes with all that hangs from it; the follow goes on its own.
	if _, err := admin.Exec(ctx, `DELETE FROM post WHERE tenant_id = $1 AND page_id = $2`, tenantA.String(), page.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `DELETE FROM post_follow WHERE tenant_id = $1 AND hub_id = $2`, tenantA.String(), hub.String()); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, table := range tables {
			for _, row := range before[table] {
				if _, err := importRepo().Write(ctx, table, row, false); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("importing the post back: %v", err)
	}

	for _, table := range tables {
		after := ours(table)
		for id, row := range before[table] {
			for column, value := range row {
				if !sameJSON(value, after[id][column]) {
					t.Errorf("%s %s: %s came back as %#v, want %#v", table, id, column, after[id][column], value)
				}
			}
		}
	}
}
