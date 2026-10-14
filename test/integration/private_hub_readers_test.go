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
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	aiadapter "github.com/Jersyfi/hubtask/infrastructure/ai"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// PrivateHubReaders is UC-ID-16 check 2's table (D14): one row per reader, each run against
// PostgreSQL as the application role as a workspace owner holding nothing in a private hub (the
// hub absent, or not found), as the hub's member (present), and as another workspace's owner (not
// found). test/architecture's TestEveryReaderIsInThePrivateHubTable holds the catalogue to it.
var PrivateHubReaders = []string{
	work.GetContainerName, work.ListContainersName, work.GetWorkItemName, work.ListWorkItemsName,
	work.SearchItemsName, work.ListCommentsName, work.ListTrashName, work.QueryItemsName,
	work.GetSavedViewName, work.ListSavedViewsName, suggestion.ListSuggestionsName,
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
	views := postgres.NewSavedViewRepository()
	// ListSuggestions reads its target through the catalogue it is itself an entry of.
	catalogue := &catalogueLater{}
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
		work.ListTrash{
			Trash: postgres.NewTrashRepository(pageCursors()), Reader: authoriser, UnitOfWork: unitOfWork,
			Policies: postgres.NewLifecycleRepository(),
			Holds:    postgres.NewLegalHoldRepository(), Contributors: postgres.NewLifecycleRepository(),
		}.Descriptor(),
		work.QueryItems{
			Items: items, ItemLabels: labels, Containers: containers, Authorizer: authoriser,
			UnitOfWork: unitOfWork, Clock: fixed,
		}.Descriptor(),
		work.GetSavedView{
			Views: views, Containers: containers, Permits: authoriser, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.ListSavedViews{
			Views: views, Containers: containers, Authorizer: authoriser, UnitOfWork: unitOfWork,
		}.Descriptor(),
		suggestion.ListSuggestions{Cases: suggestion.Cases{
			Suggestions: postgres.NewSuggestionRepository(pageCursors()),
			Targets:     suggestion.EntryTargets{Catalogue: catalogue},
			Authorizer:  authoriser, UnitOfWork: unitOfWork, Clock: fixed,
		}}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the readers: %v", err)
	}
	catalogue.registry = registry
	return registry
}

// catalogueLater is the catalogue a reader inside it asks, set once the catalogue exists.
type catalogueLater struct{ registry *usecase.Registry }

func (c *catalogueLater) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return c.registry.Invoke(ctx, name, actor, in)
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
		// The saved-view query takes exactly one scope, so there is no unscoped query to leave the
		// hub out of: the hub and its collection are both refused as scopes.
		{work.QueryItemsName, func(w privateWorld) usecase.Input {
			return usecase.Input{"scope_container_id": w.hub.String()}
		}, false, notFound},
		{work.QueryItemsName, func(w privateWorld) usecase.Input {
			return usecase.Input{"scope_container_id": w.collection.String()}
		}, false, notFound},
		{work.GetSavedViewName, func(w privateWorld) usecase.Input {
			return usecase.Input{"view_id": w.view.String()}
		}, false, notFound},
		{work.ListSavedViewsName, func(w privateWorld) usecase.Input {
			return usecase.Input{"container_id": w.collection.String()}
		}, false, notFound},
		{suggestion.ListSuggestionsName, func(w privateWorld) usecase.Input {
			return usecase.Input{
				"target_type": string(suggestiondomain.TargetWorkItem), "target_id": w.item.String(),
			}
		}, false, notFound},
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
		for _, id := range []shared.ID{w.hub, w.collection, w.item, w.trashed, w.view} {
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
	w.view = viewSharedInPrivateHub(ctx, t, w)
	suggestionInPrivateHub(ctx, t, w)
	registry := readerRegistry(ctx, t)
	otherOwner := ownerElsewhere(ctx, t)

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
				"another workspace's owner": as(tenantB, otherOwner),
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

// ownerElsewhere is an owner of tenant B. An account there with no membership would be refused by
// a reader that asks about the workspace before the entry, which says nothing about the hub.
func ownerElsewhere(ctx context.Context, t *testing.T) shared.ID {
	t.Helper()
	owner := freshID(t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Olga Other')`,
		owner.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding the other workspace's owner: %v", err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		VALUES ($1, $2, $3, 'TENANT', 'OWNER')`, freshID(t).String(), tenantB.String(), owner.String()); err != nil {
		t.Fatalf("seeding the other workspace's owner: %v", err)
	}
	return owner
}

// viewSharedInPrivateHub is a saved view the hub's member shared into its collection.
func viewSharedInPrivateHub(ctx context.Context, t *testing.T, w privateWorld) shared.ID {
	t.Helper()
	saved, err := view.NewSavedView(view.NewSavedViewInput{
		ID: freshID(t), TenantID: w.tenant, OwnerID: w.member,
		ScopeType: view.ViewScopeCollection, ScopeID: w.collection,
		Name: "Presents", Layout: "LIST_EXPANDED",
		Query:   map[string]any{"scope_container_id": w.collection.String()},
		Sharing: view.SharingScope, Now: created,
	})
	if err != nil {
		t.Fatalf("the view was refused: %v", err)
	}
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		return viewRepo().Insert(ctx, saved)
	}); err != nil {
		t.Fatalf("seeding the view: %v", err)
	}
	return saved.ID
}

// suggestionInPrivateHub is what AI proposed about the hub's entry.
func suggestionInPrivateHub(ctx context.Context, t *testing.T, w privateWorld) {
	t.Helper()
	proposal, err := suggestiondomain.New(suggestiondomain.NewInput{
		ID: freshID(t), TenantID: w.tenant,
		TargetType: suggestiondomain.TargetWorkItem, TargetID: w.item,
		Kind:    suggestiondomain.KindFields,
		Payload: map[string]any{"title": "Bike for Lena"},
		Provenance: suggestiondomain.Provenance{
			Model: "a-model", PromptID: "suggest-fields", PromptVersion: "v1", ProducedAt: created,
		},
		InputDigest: suggestiondomain.Digest(w.title, ""),
		Now:         created,
	})
	if err != nil {
		t.Fatalf("the suggestion was refused: %v", err)
	}
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		return postgres.NewSuggestionRepository(pageCursors()).Record(ctx, proposal)
	}); err != nil {
		t.Fatalf("seeding the suggestion: %v", err)
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

// UC-ID-16 check 2, the calendar feed: it is not a catalogue reader - the route answers a token
// rather than an actor - so it is not a row of the table. The feed reads as its owner at fetch time,
// and a workspace owner's feed over the private hub's collection answers not found, as one saved
// before the hub was made private would; the member's feed over the same collection carries the
// dated entry. The owner cannot subscribe to the member's view shared into the hub either.
func TestTheCalendarFeedKeepsAPrivateHubFromOutsiders(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	harness := feedAcceptanceHarness(ctx, t)

	dated := findItem(ctx, t, w.tenant, w.item)
	dated.Due = &workmodel.DueDate{At: created.Add(24 * time.Hour)}
	dated.UpdatedAt = created.Add(time.Hour)
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, dated, dated.Version)
	}); err != nil {
		t.Fatalf("seeding the due date: %v", err)
	}

	feedOf := func(owner shared.ID) (work.ExportedView, error) {
		t.Helper()
		saved, err := view.NewSavedView(view.NewSavedViewInput{
			ID: freshID(t), TenantID: w.tenant, OwnerID: owner,
			ScopeType: view.ViewScopeCollection, ScopeID: w.collection,
			Name: "This week", Layout: "LIST_EXPANDED",
			Query:   map[string]any{"scope_container_id": w.collection.String()},
			Sharing: view.SharingPrivate, Now: created,
		})
		if err != nil {
			t.Fatalf("the view was refused: %v", err)
		}
		if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
			return viewRepo().Insert(ctx, saved)
		}); err != nil {
			t.Fatalf("writing the view: %v", err)
		}
		minted, err := harness.mint.Execute(ctx, subscriber(w.tenant, owner), saved.ID)
		if err != nil {
			t.Fatalf("minting the feed: %v", err)
		}
		return harness.read.Execute(ctx, minted.Token)
	}

	exported, err := feedOf(w.member)
	if err != nil {
		t.Fatalf("the member's feed: %v", err)
	}
	if !feedCarries(exported, w.item) {
		t.Error("the member's feed does not carry the dated entry of their own hub")
	}

	exported, err = feedOf(w.administrator)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the workspace owner's feed: %v, want not found", err)
	}
	if feedCarries(exported, w.item) {
		t.Error("the workspace owner's feed carries an entry of the private hub")
	}

	theirs := viewSharedInPrivateHub(ctx, t, w)
	if _, err := harness.mint.Execute(ctx, subscriber(w.tenant, w.administrator), theirs); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the workspace owner subscribing to the member's view: %v, want not found", err)
	}
}

func feedCarries(exported work.ExportedView, id shared.ID) bool {
	for _, item := range exported.Items {
		if item.ID == id {
			return true
		}
	}
	return false
}

// UC-ID-16 check 2, AI run by others: a translation is asked as the caller, through the entry's own
// read, so the hub's member reaches the provider and nobody outside the hub does - their request
// answers not found and the provider receives nothing. Not a row of the table, because its answer
// names no identifier for the table to look for. ListSuggestions, the other AI reader, is a row.
func TestAiRunByOthersKeepsAPrivateHubFromOutsiders(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	prompts, err := aiadapter.NewStore()
	if err != nil {
		t.Fatalf("prompts: %v", err)
	}
	recorder := &recordingProvider{}
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	translate := work.AiTranslate{
		Reader: work.GetWorkItem{
			Items: itemRepo(), ItemLabels: postgres.NewItemLabelRepository(), Containers: containerRepo(),
			Authorizer: realAuthoriser(ctx, t), UnitOfWork: unitOfWork,
		},
		Providers: recorder, Prompts: prompts,
		Audit:      postgres.NewAuditSink(clockadapter.NewUUIDv7(clockadapter.System{})),
		UnitOfWork: unitOfWork, Clock: clockadapter.System{},
	}
	ask := func(tenant, account shared.ID) error {
		_, err := translate.Execute(ctx, reader(tenant, account),
			work.TranslateCommand{ItemID: w.item, TargetLocale: "de"})
		return err
	}

	if err := ask(w.tenant, w.member); err != nil {
		t.Fatalf("the hub's member asking: %v", err)
	}
	if !strings.Contains(strings.Join(recorder.take(), "\n"), w.title) {
		t.Fatal("the member's translation did not reach the provider with the entry")
	}

	for who, account := range map[string][2]shared.ID{
		"the workspace's owner":     {w.tenant, w.administrator},
		"another workspace's owner": {tenantB, ownerElsewhere(ctx, t)},
	} {
		if err := ask(account[0], account[1]); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%s asking: %v, want not found (T-04)", who, err)
		}
		if sent := recorder.take(); len(sent) != 0 {
			t.Errorf("%s's request reached the provider: %d inputs", who, len(sent))
		}
	}
}
