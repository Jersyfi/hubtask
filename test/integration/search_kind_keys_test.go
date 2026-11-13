// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	knowledgerepo "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	repository "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func seedSQL(ctx context.Context, t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := adminPool(ctx, t).Exec(ctx, sql, args...); err != nil {
		t.Fatalf("seeding %.50q: %v", sql, err)
	}
}

func rowIDs(rows []repository.Row) []shared.ID {
	out := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	slices.Sort(out)
	return out
}

func sorted(ids ...shared.ID) []shared.ID {
	slices.Sort(ids)
	return ids
}

// UC-WRK-18 check 9, UC-WRK-43: each key of an inventory item is a bound filter of the item
// search - with words and without -, turned round by a minus, `none` the empty column; another
// workspace's items are never found by them.
func TestTheKeysOfAnInventoryItemNarrowItsSearch(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	word := strings.ToLower("zk" + freshName(t)[:6])
	hub, _ := seedEveryKind(ctx, t, tenantA, strings.ToLower("zs"+freshName(t)[:6]))
	server, laptop, room := freshID(t), freshID(t), freshID(t)
	tag := "INV-" + strings.ToUpper(word)
	ci := `INSERT INTO cmdb_ci (id, tenant_id, hub_id, class_key, name, stage, status, owner_account_id, location_ci_id,
	       created_by, created_at, updated_by, updated_at) VALUES ($1, $2, $3, $4, $5, $6, 'OK', $7, $8, $9, now(), $9, now())`
	seedSQL(ctx, t, ci, room.String(), tenantA.String(), hub.String(), "room", word+" room", "IN_USE", nil, nil, authorA.String())
	seedSQL(ctx, t, ci, server.String(), tenantA.String(), hub.String(), "server", word+" server", "IN_USE", authorA.String(), room.String(), authorA.String())
	seedSQL(ctx, t, ci, laptop.String(), tenantA.String(), hub.String(), "laptop", word+" laptop", "RETIRED", nil, nil, authorA.String())
	seedSQL(ctx, t, `INSERT INTO cmdb_asset (ci_id, tenant_id, hub_id, asset_tag, state, updated_by, updated_at)
	       VALUES ($1, $2, $3, $4, 'DEPLOYED', $5, now())`, server.String(), tenantA.String(), hub.String(), tag, authorA.String())

	repo := postgres.NewSearchNamesRepository()
	words := []string{word}
	cases := []struct {
		name  string
		terms []string
		key   view.KindCondition
		want  []shared.ID
	}{
		{"class", words, view.KindCondition{Field: view.FieldAssetClass, Values: []string{"Server"}}, sorted(server)},
		{"not class", words, view.KindCondition{Field: view.FieldAssetClass, Values: []string{"server"}, Negated: true}, sorted(laptop, room)},
		{"stage", words, view.KindCondition{Field: view.FieldAssetStage, Values: []string{"RETIRED"}}, sorted(laptop)},
		{"stages", words, view.KindCondition{Field: view.FieldAssetStage, Values: []string{"RETIRED", "IN_USE"}}, sorted(server, laptop, room)},
		{"owner", words, view.KindCondition{Field: view.FieldAssetOwner, Values: []string{authorA.String()}}, sorted(server)},
		{"owner or none", words, view.KindCondition{Field: view.FieldAssetOwner, Values: []string{authorA.String()}, None: true}, sorted(server, laptop, room)},
		{"no owner", words, view.KindCondition{Field: view.FieldAssetOwner, None: true}, sorted(laptop, room)},
		{"an owner", words, view.KindCondition{Field: view.FieldAssetOwner, None: true, Negated: true}, sorted(server)},
		{"location", words, view.KindCondition{Field: view.FieldAssetLocation, Values: []string{room.String()}}, sorted(server)},
		{"no location", words, view.KindCondition{Field: view.FieldAssetLocation, None: true}, sorted(laptop, room)},
		{"tag", words, view.KindCondition{Field: view.FieldAssetTag, Values: []string{strings.ToLower(tag)}}, sorted(server)},
		{"no tag", words, view.KindCondition{Field: view.FieldAssetTag, None: true}, sorted(laptop, room)},
		{"tag without words", nil, view.KindCondition{Field: view.FieldAssetTag, Values: []string{tag}}, sorted(server)},
		{"stage without words, in the hub", nil, view.KindCondition{Field: view.FieldAssetStage, Values: []string{"RETIRED"}}, sorted(laptop)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ask := repository.Ask{Terms: c.terms, Conditions: []view.KindCondition{c.key}, Batch: 50}
			if c.terms == nil {
				ask.HubID = hub
			}
			if err := read(ctx, t, tenantA, func(ctx context.Context) error {
				rows, err := repo.Cis(ctx, ask)
				if err == nil && !slices.Equal(rowIDs(rows), c.want) {
					t.Errorf("found %v, want %v", rowIDs(rows), c.want)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := read(ctx, t, tenantB, func(ctx context.Context) error {
				rows, err := repo.Cis(ctx, repository.Ask{Terms: c.terms, Conditions: ask.Conditions, Batch: 50})
				if err == nil && len(rows) != 0 && c.terms != nil {
					t.Errorf("the workspace next door found %v", rowIDs(rows))
				}
				for _, row := range rows {
					if slices.Contains(c.want, row.ID) {
						t.Errorf("the workspace next door found %s", row.ID)
					}
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// UC-WRK-18 check 9: goal.state is a bound filter of the goal search, with words and without;
// another workspace's goals are never found by it.
func TestTheStateOfAGoalNarrowsItsSearch(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	word := strings.ToLower("zg" + freshName(t)[:6])
	hub, _ := seedEveryKind(ctx, t, tenantA, strings.ToLower("zs"+freshName(t)[:6]))
	period, onTrack, atRisk := freshID(t), freshID(t), freshID(t)
	seedSQL(ctx, t, `INSERT INTO goal_period (id, tenant_id, name, start_on, end_on, state, created_at, updated_at)
	      VALUES ($1, $2, 'Q4', '2026-10-01', '2026-12-31', 'OPEN', now(), now())`, period.String(), tenantA.String())
	goal := `INSERT INTO goal (id, tenant_id, period_id, hub_id, title, status, created_at, updated_at)
	         VALUES ($1, $2, $3, $4, $5, $6, now(), now())`
	seedSQL(ctx, t, goal, onTrack.String(), tenantA.String(), period.String(), hub.String(), word+" growth", "ON_TRACK")
	seedSQL(ctx, t, goal, atRisk.String(), tenantA.String(), period.String(), hub.String(), word+" margin", "AT_RISK")

	repo := postgres.NewSearchNamesRepository()
	for _, c := range []struct {
		name string
		ask  repository.Ask
		want []shared.ID
	}{
		{"state", repository.Ask{Terms: []string{word}, Conditions: []view.KindCondition{{Field: view.FieldGoalState, Values: []string{"AT_RISK"}}}}, sorted(atRisk)},
		{"not the state", repository.Ask{Terms: []string{word}, Conditions: []view.KindCondition{{Field: view.FieldGoalState, Values: []string{"AT_RISK"}, Negated: true}}}, sorted(onTrack)},
		{"without words", repository.Ask{HubID: hub, Conditions: []view.KindCondition{{Field: view.FieldGoalState, Values: []string{"AT_RISK"}}}}, sorted(atRisk)},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.ask.Batch = 50
			if err := read(ctx, t, tenantA, func(ctx context.Context) error {
				rows, err := repo.Goals(ctx, c.ask)
				if err == nil && !slices.Equal(rowIDs(rows), c.want) {
					t.Errorf("found %v, want %v", rowIDs(rows), c.want)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := read(ctx, t, tenantB, func(ctx context.Context) error {
				rows, err := repo.Goals(ctx, repository.Ask{Conditions: c.ask.Conditions, Batch: 100})
				for _, row := range rows {
					if row.ID == onTrack || row.ID == atRisk {
						t.Errorf("the workspace next door found %s", row.ID)
					}
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// UC-DOC-01, UC-WRK-18 check 9: page.verified and page.owner narrow the page search, with words
// and without; another workspace's pages are never found by them.
func TestTheKeysOfAPageNarrowItsSearch(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	word := strings.ToLower("zp" + freshName(t)[:6])
	hub, _ := seedEveryKind(ctx, t, tenantA, strings.ToLower("zs"+freshName(t)[:6]))
	verified, lapsed, unowned := freshID(t), freshID(t), freshID(t)
	page := `INSERT INTO page (id, tenant_id, hub_id, title, body, order_key, owner_id, verified_until, created_by, created_at, updated_by, updated_at)
	         VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8, now(), $8, now())`
	seedSQL(ctx, t, page, verified.String(), tenantA.String(), hub.String(), word+" handbook", "a", authorA.String(), time.Now().Add(48*time.Hour), authorA.String())
	seedSQL(ctx, t, page, lapsed.String(), tenantA.String(), hub.String(), word+" runbook", "b", authorA.String(), time.Now().Add(-48*time.Hour), authorA.String())
	seedSQL(ctx, t, page, unowned.String(), tenantA.String(), hub.String(), word+" notes", "c", nil, nil, authorA.String())

	pages := postgres.NewPageRepository()
	yes, no := true, false
	for _, c := range []struct {
		name string
		q    knowledgerepo.PageQuery
		want []shared.ID
	}{
		{"verified", knowledgerepo.PageQuery{Text: word, Verified: &yes}, sorted(verified)},
		{"not verified", knowledgerepo.PageQuery{Text: word, Verified: &no}, sorted(lapsed, unowned)},
		{"owner", knowledgerepo.PageQuery{Text: word, Owner: &view.KindCondition{Field: view.FieldPageOwner, Values: []string{authorA.String()}}}, sorted(verified, lapsed)},
		{"no owner", knowledgerepo.PageQuery{Text: word, Owner: &view.KindCondition{Field: view.FieldPageOwner, None: true}}, sorted(unowned)},
		{"not this owner", knowledgerepo.PageQuery{Text: word, Owner: &view.KindCondition{Field: view.FieldPageOwner, Values: []string{authorA.String()}, Negated: true}}, sorted(unowned)},
		{"verified without words", knowledgerepo.PageQuery{Verified: &yes}, sorted(verified)},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.q.HubIDs, c.q.Limit, c.q.At = []shared.ID{hub}, 50, time.Now()
			if err := read(ctx, t, tenantA, func(ctx context.Context) error {
				hits, err := pages.Search(ctx, c.q)
				got := make([]shared.ID, 0, len(hits))
				for _, hit := range hits {
					got = append(got, hit.ID)
				}
				slices.Sort(got)
				if err == nil && !slices.Equal(got, c.want) {
					t.Errorf("found %v, want %v", got, c.want)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := read(ctx, t, tenantB, func(ctx context.Context) error {
				hits, err := pages.Search(ctx, c.q)
				if err == nil && len(hits) != 0 {
					t.Errorf("the workspace next door found %d pages", len(hits))
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
