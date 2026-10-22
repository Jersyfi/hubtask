// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Where an arrival may become work, against a real database and the real authoriser: the shared
// set leaves out what is trashed, archived (by its own stamp or its hub's) and private; a person's
// picker keeps the private hubs they are in and what they may create in.

// destinationWorld is a workspace with an owner (also the private hub's owner), a member, a
// contributor and a viewer; a shared hub with an open and an archived collection, an archived hub
// with a collection, a trashed collection, and a private hub with one collection.
type destinationWorld struct {
	jumbleWorld
	member, contributor, viewer            shared.ID
	open, archived, underArchived, trashed shared.ID
	private                                shared.ID
}

func seedDestinationWorld(ctx context.Context, t *testing.T) destinationWorld {
	t.Helper()
	w := destinationWorld{
		jumbleWorld: seedJumbleWorld(ctx, t),
		member:      freshID(t), contributor: freshID(t), viewer: freshID(t),
		open: freshID(t), archived: freshID(t), underArchived: freshID(t), trashed: freshID(t),
		private: freshID(t),
	}
	shop, closedHub, privateHub := freshID(t), freshID(t), freshID(t)
	repo := containerRepo()
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		hidden := containerIn(w.tenant, w.owner, privateHub, freshName(t), "a2")
		hidden.Private = true
		for _, insert := range []func() error{
			func() error { return repo.Insert(ctx, containerIn(w.tenant, w.owner, shop, freshName(t), "a0")) },
			func() error { return repo.Insert(ctx, containerIn(w.tenant, w.owner, closedHub, freshName(t), "a1")) },
			func() error { return repo.Insert(ctx, hidden) },
			func() error {
				return repo.Insert(ctx, collectionIn(w.tenant, w.owner, w.open, shop, "Orders", "a0"))
			},
			func() error {
				return repo.Insert(ctx, collectionIn(w.tenant, w.owner, w.archived, shop, "Old orders", "a1"))
			},
			func() error {
				return repo.Insert(ctx, collectionIn(w.tenant, w.owner, w.trashed, shop, "Binned", "a2"))
			},
			func() error {
				return repo.Insert(ctx, collectionIn(w.tenant, w.owner, w.underArchived, closedHub, "Last year", "a0"))
			},
			func() error {
				return repo.Insert(ctx, collectionIn(w.tenant, w.owner, w.private, privateHub, "Diary", "a0"))
			},
		} {
			if err := insert(); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the containers: %v", err)
	}

	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE container SET archived_at = now() WHERE id IN ($1, $2)`, []any{w.archived.String(), closedHub.String()}},
		{`UPDATE container SET deleted_at = now() WHERE id = $1`, []any{w.trashed.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role) VALUES ($1, $2, $3, 'HUB', $4, 'OWNER')`,
			[]any{freshID(t).String(), w.tenant.String(), w.owner.String(), privateHub.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, statement.sql)
		}
	}
	for account, role := range map[shared.ID]string{w.member: "MEMBER", w.contributor: "CONTRIBUTOR", w.viewer: "VIEWER"} {
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, $3, 'Somebody')`,
				[]any{account.String(), w.tenant.String(), "p-" + account.String() + "@example.org"}},
			{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', $4)`,
				[]any{freshID(t).String(), w.tenant.String(), account.String(), role}},
		} {
			if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
				t.Fatalf("seeding: %v\n%s", err, statement.sql)
			}
		}
	}
	return w
}

func destinationsOf(ctx context.Context, t *testing.T, actor appshared.ActorContext) ([]string, error) {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	authoriser := realAuthoriser(ctx, t)
	registry, err := usecase.NewRegistry(nil, work.ListJumbleDestinations{
		Collections: containerRepo(), Authorizer: authoriser, Reader: authoriser, UnitOfWork: unitOfWork,
	}.Descriptor())
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	out, err := registry.Invoke(ctx, work.ListJumbleDestinationsName, actor, usecase.Input{})
	if err != nil {
		return nil, err
	}
	ids := []string{}
	data, _ := out["data"].([]usecase.Output)
	for _, row := range data {
		ids = append(ids, row.String("id"))
	}
	return ids, nil
}

func TestThePickerOffersWhereEachPersonMayCreate(t *testing.T) {
	ctx := context.Background()
	w := seedDestinationWorld(ctx, t)

	for name, tc := range map[string]struct {
		account shared.ID
		want    []shared.ID
	}{
		"the owner, also in the private hub": {w.owner, []shared.ID{w.open, w.private}},
		"a member":                           {w.member, []shared.ID{w.open}},
		"a contributor":                      {w.contributor, []shared.ID{w.open}},
	} {
		got, err := destinationsOf(ctx, t, w.as(tc.account))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s was offered %v, want %v", name, got, tc.want)
			continue
		}
		for i, id := range tc.want {
			if got[i] != id.String() {
				t.Errorf("%s was offered %v, want %v in that order", name, got, tc.want)
			}
		}
	}

	if _, err := destinationsOf(ctx, t, w.as(w.viewer)); err == nil {
		t.Error("a viewer, who may convert nothing, was answered")
	}
	if _, err := destinationsOf(ctx, t, seedJumbleWorld(ctx, t).as(w.member)); err == nil {
		t.Error("an account of another workspace was answered")
	}
}

// The shared set: what everybody who may convert may write in - no private hub, nothing archived or
// trashed - and nothing of another workspace.
func TestTheSharedSetLeavesOutPrivateArchivedAndTrashed(t *testing.T) {
	ctx := context.Background()
	w := seedDestinationWorld(ctx, t)
	other := seedDestinationWorld(ctx, t)

	var page repository.CollectionPage
	if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
		var err error
		page, err = containerRepo().InOrder(ctx, repository.CollectionQuery{Page: repository.Page{Size: 100}})
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(page.Collections) != 1 || page.Collections[0].Collection.ID != w.open {
		t.Errorf("the shared set is %+v, want only %s", page.Collections, w.open)
	}
	for _, row := range page.Collections {
		if row.Collection.ID == other.open {
			t.Error("another workspace's collection is in the shared set")
		}
	}
}

// Check 11 through the real readers: a person's ask offers the model the shared set - the owner,
// who is in the private hub, is offered no collection of it, and nothing archived.
func TestAJumbleAskOffersTheSharedSetWhoeverAsks(t *testing.T) {
	ctx := context.Background()
	w := seedDestinationWorld(ctx, t)
	entry := seedJumbleEntry(ctx, t, w.tenant, nil)
	sources := suggestion.CatalogueSources{Catalogue: jumbleReaders(ctx, t), Collections: containerRepo()}

	for _, asker := range []shared.ID{w.owner, w.member} {
		var material suggestion.Material
		if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
			var err error
			material, err = sources.Material(ctx, w.as(asker), suggestion.Question{TargetType: suggestiondomain.TargetJumbleEntry, TargetID: entry.ID, PromptID: "suggest-fields"})
			return err
		}); err != nil {
			t.Fatalf("reading the material: %v", err)
		}
		var offered []string
		for _, set := range material.Choices {
			for _, option := range set.Options {
				offered = append(offered, option.ID)
			}
		}
		if len(offered) != 1 || offered[0] != w.open.String() {
			t.Errorf("%s was offered %v, want only the shared %s", asker, offered, w.open)
		}
	}
}

// The walk pages across the hubs without losing or repeating a collection.
func TestTheOrderedListingPagesAcrossTheHubs(t *testing.T) {
	ctx := context.Background()
	w := seedJumbleWorld(ctx, t)
	repo := containerRepo()
	var want []shared.ID
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		for _, hubKey := range []string{"a0", "a1", "a2"} {
			hub := freshID(t)
			if err := repo.Insert(ctx, containerIn(w.tenant, w.owner, hub, freshName(t), hubKey)); err != nil {
				return err
			}
			for _, key := range []string{"a0", "a1"} {
				id := freshID(t)
				want = append(want, id)
				if err := repo.Insert(ctx, collectionIn(w.tenant, w.owner, id, hub, freshName(t), key)); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	var got []shared.ID
	cursor := ""
	for range 10 {
		var page repository.CollectionPage
		if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
			var err error
			page, err = repo.InOrder(ctx, repository.CollectionQuery{Page: repository.Page{Size: 4, Cursor: cursor}})
			return err
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		for _, row := range page.Collections {
			got = append(got, row.Collection.ID)
		}
		if !page.Info.HasMore {
			break
		}
		cursor = page.Info.NextCursor
	}
	if len(got) != len(want) {
		t.Fatalf("walked %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("walked %v, want %v", got, want)
		}
	}
}
