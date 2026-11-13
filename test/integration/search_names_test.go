// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// seedEveryKind writes one object of every kind the search over names finds, each named with the
// word given, in a fresh hub and collection of the tenant; it answers the hub and the entry.
func seedEveryKind(ctx context.Context, t *testing.T, tenant shared.ID, word string) (hub, item shared.ID) {
	t.Helper()
	admin := adminPool(ctx, t)
	item = seedWorkItemForRules(ctx, t, tenant)
	if err := admin.QueryRow(ctx,
		`SELECT c.parent_id FROM work_item i JOIN container c ON c.id = i.collection_id WHERE i.id = $1`,
		item.String()).Scan(&hub); err != nil {
		t.Fatalf("reading the hub: %v", err)
	}
	var collection string
	if err := admin.QueryRow(ctx, `SELECT collection_id FROM work_item WHERE id = $1`, item.String()).Scan(&collection); err != nil {
		t.Fatalf("reading the collection: %v", err)
	}
	period, media := freshID(t), freshID(t)
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO whiteboard (id, tenant_id, hub_id, title, created_by, created_at, updated_by, updated_at)
		  VALUES ($1, $2, $3, $4, $5, now(), $5, now())`, []any{freshID(t).String(), tenant.String(), hub.String(), word + " board", authorA.String()}},
		{`INSERT INTO goal_period (id, tenant_id, name, start_on, end_on, state, created_at, updated_at)
		  VALUES ($1, $2, 'Q4', '2026-10-01', '2026-12-31', 'OPEN', now(), now())`, []any{period.String(), tenant.String()}},
		{`INSERT INTO goal (id, tenant_id, period_id, hub_id, title, status, created_at, updated_at)
		  VALUES ($1, $2, $3, $4, $5, 'ON_TRACK', now(), now())`, []any{freshID(t).String(), tenant.String(), period.String(), hub.String(), word + " goal"}},
		{`INSERT INTO dashboard (id, tenant_id, hub_id, owner_id, name, sharing, created_at, updated_at)
		  VALUES ($1, $2, $3, $4, $5, 'SCOPE', now(), now())`, []any{freshID(t).String(), tenant.String(), hub.String(), authorA.String(), word + " dashboard"}},
		{`INSERT INTO cmdb_ci (id, tenant_id, hub_id, class_key, name, stage, status, created_by, created_at, updated_by, updated_at)
		  VALUES ($1, $2, $3, 'server', $4, 'IN_USE', 'OK', $5, now(), $5, now())`, []any{freshID(t).String(), tenant.String(), hub.String(), word + " server", authorA.String()}},
		{`INSERT INTO portfolio (id, tenant_id, name, status, created_at, updated_at)
		  VALUES ($1, $2, $3, 'ON_TRACK', now(), now())`, []any{freshID(t).String(), tenant.String(), word + " portfolio"}},
		{`INSERT INTO release (id, tenant_id, hub_id, name, release_on, state, created_at, updated_at)
		  VALUES ($1, $2, $3, $4, '2026-12-01', 'PLANNED', now(), now())`, []any{freshID(t).String(), tenant.String(), hub.String(), word + " release"}},
		{`INSERT INTO sprint (id, tenant_id, collection_id, name, start_on, end_on, state, created_at, updated_at)
		  VALUES ($1, $2, $3, $4, '2026-11-01', '2026-11-14', 'PLANNED', now(), now())`, []any{freshID(t).String(), tenant.String(), collection, word + " sprint"}},
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, $3)`, []any{freshID(t).String(), tenant.String(), word + " Person"}},
		{`INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, usage, created_by, file_name, status)
		  VALUES ($1, $2, $5, 'application/pdf', 10, 'ATTACHMENT', $3, $4, 'READY')`, []any{media.String(), tenant.String(), authorA.String(), word + "-report.pdf", "key-" + media.String()}},
		{`INSERT INTO item_attachment (tenant_id, item_id, media_id) VALUES ($1, $2, $3)`, []any{tenant.String(), item.String(), media.String()}},
		{`INSERT INTO comment (id, tenant_id, item_id, author_id, body) VALUES ($1, $2, $3, $4, $5)`, []any{freshID(t).String(), tenant.String(), item.String(), authorA.String(), "see the " + word + " notes"}},
		{`INSERT INTO comment (id, tenant_id, item_id, author_id, body, kind, system_code) VALUES ($1, $2, $3, $4, $5, 'SYSTEM', 'x')`, []any{freshID(t).String(), tenant.String(), item.String(), authorA.String(), word}},
	}
	for _, s := range statements {
		if _, err := admin.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatalf("seeding %.40q: %v", s.sql, err)
		}
	}
	return hub, item
}

// UC-WRK-18 check 9. Every kind is found by its words, in its hub, and never by the workspace next door (portfolios
// and people standing in none); a word not
// held finds nothing; the server's own comment lines are not searched (UC-WRK-18 check 9).
func TestEveryKindIsFoundByItsNameInItsWorkspaceAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	word := "zq" + freshName(t)[:6]
	hub, item := seedEveryKind(ctx, t, tenantA, word)
	var collection string
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT collection_id FROM work_item WHERE id = $1`, item.String()).Scan(&collection); err != nil {
		t.Fatalf("reading the collection: %v", err)
	}
	repo := postgres.NewSearchNamesRepository()
	kinds := map[string]func(context.Context, repository.Ask) ([]repository.Row, error){
		"whiteboards": repo.Whiteboards, "goals": repo.Goals, "dashboards": repo.Dashboards,
		"cis": repo.Cis, "people": repo.People, "files": repo.EntryFiles, "comments": repo.EntryComments,
		"portfolios": repo.Portfolios, "releases": repo.Releases, "sprints": repo.Sprints,
	}
	ask := repository.Ask{Terms: []string{strings.ToLower(word)}, Batch: 10}
	for name, find := range kinds {
		t.Run(name, func(t *testing.T) {
			if err := read(ctx, t, tenantA, func(ctx context.Context) error {
				rows, err := find(ctx, ask)
				if err != nil {
					return err
				}
				if len(rows) != 1 {
					t.Fatalf("found %+v, want one", rows)
				}
				if name != "people" && name != "portfolios" && rows[0].HubID != hub {
					t.Errorf("the hit stands in %s, want %s", rows[0].HubID, hub)
				}
				if name == "sprints" && rows[0].CollectionID.String() != collection {
					t.Errorf("the sprint stands in %s, want the collection %s", rows[0].CollectionID, collection)
				}
				if (name == "files" || name == "comments") && rows[0].OnID != item {
					t.Errorf("the hit is on %s, want the entry %s", rows[0].OnID, item)
				}
				inHub, err := find(ctx, repository.Ask{Terms: ask.Terms, Batch: 10, HubID: hub})
				if err != nil || len(inHub) != 1 {
					t.Errorf("narrowed to its hub it found %+v, %v", inHub, err)
				}
				both, err := find(ctx, repository.Ask{Terms: []string{strings.ToLower(word), "nowhere-held"}, Batch: 10})
				if err != nil || len(both) != 0 {
					t.Errorf("a word not held still found %+v, %v", both, err)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := read(ctx, t, tenantB, func(ctx context.Context) error {
				rows, err := find(ctx, ask)
				if err != nil {
					return err
				}
				if len(rows) != 0 {
					t.Errorf("the workspace next door found %+v", rows)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// UC-WRK-18 check 11. A ticket key finds its entry by the collection's key prefix or its handle, in its hub, and
// never in the workspace next door, which may hold the same key.
func TestATicketKeyFindsItsEntryInItsWorkspaceAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	random := freshID(t).String()
	handle := "k" + random[len(random)-6:]
	keyed := func(tenant shared.ID, prefix string) (item, hub shared.ID) {
		item = seedWorkItemForRules(ctx, t, tenant)
		if err := admin.QueryRow(ctx,
			`UPDATE container c SET handle = $2, key_prefix = $3 FROM work_item i
			 WHERE i.id = $1 AND c.id = i.collection_id RETURNING c.parent_id`,
			item.String(), handle, prefix).Scan(&hub); err != nil {
			t.Fatalf("keying the collection: %v", err)
		}
		if _, err := admin.Exec(ctx, `UPDATE work_item SET number = 7 WHERE id = $1`, item.String()); err != nil {
			t.Fatalf("numbering the entry: %v", err)
		}
		return item, hub
	}
	prefix := "K" + strings.ToUpper(random[len(random)-9:len(random)-6])
	item, hub := keyed(tenantA, prefix)
	keyed(tenantB, prefix)
	repo := postgres.NewSearchNamesRepository()
	for _, word := range []string{strings.ToLower(prefix), handle} {
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			rows, err := repo.EntryByKey(ctx, repository.Key{Word: word, Number: 7}, "")
			if err != nil {
				return err
			}
			if len(rows) != 1 || rows[0].ID != item || rows[0].HubID != hub || rows[0].Key != prefix+"-7" {
				t.Fatalf("%s-7 found %+v, want the entry %s keyed %s-7", word, rows, item, prefix)
			}
			if rows, err := repo.EntryByKey(ctx, repository.Key{Word: word, Number: 8}, ""); err != nil || len(rows) != 0 {
				t.Errorf("a number not held found %+v, %v", rows, err)
			}
			if rows, err := repo.EntryByKey(ctx, repository.Key{Word: word, Number: 7}, freshID(t)); err != nil || len(rows) != 0 {
				t.Errorf("another hub found %+v, %v", rows, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		rows, err := repo.EntryByKey(ctx, repository.Key{Word: prefix, Number: 7}, "")
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.ID == item {
				t.Errorf("the workspace next door found the entry %s", item)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
