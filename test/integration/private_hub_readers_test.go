// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// PrivateHubReaders is UC-ID-16 check 2's table (D14): one row per reader, each run against
// PostgreSQL as the application role as a workspace owner holding nothing in a private hub (the
// hub absent, or not found), as the hub's member (present), and as another workspace's owner (not
// found). test/architecture's TestEveryReaderIsInThePrivateHubTable holds the catalogue to it.
var PrivateHubReaders = []string{
	work.GetContainerName, work.ListContainersName, work.GetWorkItemName, work.ListWorkItemsName,
	work.SearchItemsName, work.ListCommentsName, work.ListTrashName,
}

// readerRegistry is the readers the way main.go builds them.
func readerRegistry(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	authoriser := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork,
		Audit: postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed)), Clock: fixed,
	}
	containers, items := containerRepo(), itemRepo()
	labels := postgres.NewItemLabelRepository()
	registry, err := usecase.NewRegistry(nil,
		work.GetContainer{Containers: containers, Authorizer: authoriser, UnitOfWork: unitOfWork}.Descriptor(),
		work.ListContainers{
			Containers: containers, Authorizer: authoriser, Reader: authoriser, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.GetWorkItem{
			Items: items, ItemLabels: labels, Containers: containers, Authorizer: authoriser, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.ListWorkItems{
			Items: items, ItemLabels: labels, Containers: containers, Authorizer: authoriser, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.SearchItems{
			Items: items, Containers: containers, Authorizer: authoriser, Anchored: authoriser,
			Reader: authoriser, UnitOfWork: unitOfWork, Clock: fixed,
		}.Descriptor(),
		work.ListComments{
			Comments: postgres.NewCommentRepository(pageCursors()), Items: items, Containers: containers,
			Authorizer: authoriser, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.ListTrash{Trash: postgres.NewTrashRepository(pageCursors()), Reader: authoriser, UnitOfWork: unitOfWork}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the readers: %v", err)
	}
	return registry
}

// readerCall is one reader asked about the private world, and whether its answer shows the hub.
type readerCall struct {
	name  string
	input func(w privateWorld) usecase.Input
	// pages is set for a list that may need more than one page to reach the hub.
	pages bool
	// absent is what an outsider is answered: a refusal as not found, or a list without the hub.
	absent string
}

const (
	notFound = "not found"
	omitted  = "omitted"
)

func readerCalls() []readerCall {
	return []readerCall{
		{work.GetContainerName, func(w privateWorld) usecase.Input {
			return usecase.Input{"container_id": w.hub.String()}
		}, false, notFound},
		{work.ListContainersName, func(privateWorld) usecase.Input {
			return usecase.Input{"size": 100}
		}, true, omitted},
		{work.GetWorkItemName, func(w privateWorld) usecase.Input {
			return usecase.Input{"item_id": w.item.String()}
		}, false, notFound},
		{work.ListWorkItemsName, func(w privateWorld) usecase.Input {
			return usecase.Input{"collection_id": w.collection.String()}
		}, false, notFound},
		{work.SearchItemsName, func(w privateWorld) usecase.Input {
			return usecase.Input{"q": w.title}
		}, false, omitted},
		{work.ListCommentsName, func(w privateWorld) usecase.Input {
			return usecase.Input{"item_id": w.item.String()}
		}, false, notFound},
		{work.ListTrashName, func(privateWorld) usecase.Input {
			return usecase.Input{"size": 100}
		}, true, omitted},
	}
}

// answer runs a reader and reports whether its answer names the private hub or anything in it.
func answer(
	ctx context.Context, t *testing.T, registry *usecase.Registry, call readerCall,
	actor appshared.ActorContext, w privateWorld,
) (shows bool, err error) {
	t.Helper()
	input := call.input(w)
	for {
		out, err := registry.Invoke(ctx, call.name, actor, input)
		if err != nil {
			return false, err
		}
		encoded, _ := json.Marshal(out)
		for _, id := range []shared.ID{w.hub, w.collection, w.item, w.trashed} {
			if !id.IsZero() && strings.Contains(string(encoded), id.String()) {
				return true, nil
			}
		}
		page, _ := out["page"].(map[string]any)
		next, _ := page["next_cursor"].(string)
		if !call.pages || next == "" {
			return false, nil
		}
		input["cursor"] = next
	}
}

func TestEveryReaderKeepsAPrivateHubFromOutsiders(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	w.trashed = trashedInPrivateHub(ctx, t, w)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO comment (id, tenant_id, item_id, author_id, body) VALUES ($1, $2, $3, $4, 'Bike for Lena')`,
		freshID(t).String(), w.tenant.String(), w.item.String(), w.member.String()); err != nil {
		t.Fatalf("seeding the comment: %v", err)
	}
	registry := readerRegistry(ctx, t)

	scopes := []string{"items:read", "containers:read", "comments:read", "trash:read", "search:read"}
	as := func(tenant, account shared.ID) appshared.ActorContext {
		actor := reader(tenant, account)
		actor.Scopes = scopes
		return actor
	}

	for _, call := range readerCalls() {
		t.Run(call.name, func(t *testing.T) {
			shows, err := answer(ctx, t, registry, call, as(w.tenant, w.member), w)
			if err != nil || !shows {
				t.Errorf("the hub's member: shows %v, error %v; want it shown", shows, err)
			}

			for who, actor := range map[string]appshared.ActorContext{
				"the workspace's owner":     as(w.tenant, w.administrator),
				"another workspace's owner": as(tenantB, authorB),
			} {
				shows, err := answer(ctx, t, registry, call, actor, w)
				switch {
				case shows:
					t.Errorf("%s was shown the private hub", who)
				case call.absent == notFound && !errors.Is(err, shared.ErrNotFound):
					t.Errorf("%s: %v, want not found (T-04)", who, err)
				case call.absent == omitted && err != nil:
					t.Errorf("%s: %v, want the page without the hub", who, err)
				}
			}
		})
	}
}

// trashedInPrivateHub puts a second entry of the private hub into the trash, for the trash's row.
func trashedInPrivateHub(ctx context.Context, t *testing.T, w privateWorld) shared.ID {
	t.Helper()
	id := freshID(t)
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(w.tenant, authorA, w.collection, id, "Trashed", "a1"))
	}); err != nil {
		t.Fatalf("seeding the trashed entry: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE work_item SET deleted_at = now(), trash_batch_id = $2 WHERE id = $1`,
		id.String(), freshID(t).String()); err != nil {
		t.Fatalf("trashing: %v", err)
	}
	return id
}
