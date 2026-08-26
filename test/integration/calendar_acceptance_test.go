// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The sentence D-08 exists to make true, against real memberships and real rows: the feed reads as
// its **owner**, evaluated at fetch time. An owner stripped of the membership that let them see
// the view fetches a feed that has narrowed with them - and the token that worked yesterday is not
// what decides.

type feedAcceptance struct {
	mint work.CreateCalendarFeed
	read work.ReadCalendarFeed
}

func feedAcceptanceHarness(ctx context.Context, t *testing.T) feedAcceptance {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	cursors := pageCursors()
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	sink := postgres.NewAuditSink(ids)
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  unitOfWork, Audit: sink, Clock: fixed,
	}
	feeds := postgres.NewCalendarFeedRepository(
		security.NewFeedTokenHasher(secret.New(feedSecret)))
	views := postgres.NewSavedViewRepository()
	containers := containerRepo()
	itemsRepo := postgres.NewItemRepository(cursors)

	export := work.ExportView{
		Views: views, Containers: containers, Permits: authorizer,
		Query: work.QueryItems{
			Items: itemsRepo, ItemLabels: itemLabelRepo(), Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork, Clock: fixed,
		},
		ItemLabels: itemLabelRepo(), Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
	}

	return feedAcceptance{
		mint: work.CreateCalendarFeed{Writer: work.CalendarFeedWriter{
			Feeds: feeds, Views: views, Containers: containers, Permits: authorizer,
			Audit: sink, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids,
			Entropy: clockadapter.CryptoRandom{},
		}},
		read: work.ReadCalendarFeed{
			Feeds: feeds, Accounts: postgres.NewAccountRepository(), Export: export,
			UnitOfWork: unitOfWork,
		},
	}
}

func subscriber(tenant, account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account,
		AccountName: "Anna", TimeZone: "Europe/Berlin",
		Scopes: []string{"containers:read", "containers:write", "items:read", "items:write"},
	}
}

func TestAFeedNarrowsWithItsOwner(t *testing.T) {
	ctx := context.Background()
	tenant, owner, collection := seedTemplateTenant(ctx, t)
	harness := feedAcceptanceHarness(ctx, t)

	// An entry with a due date, so the feed has something to answer.
	task := seedTask(ctx, t, tenant, owner, collection)
	dated := findItem(ctx, t, tenant, task)
	dated.Due = &workmodel.DueDate{At: created.Add(24 * time.Hour)}
	dated.UpdatedAt = created.Add(time.Hour)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, dated, dated.Version)
	}); err != nil {
		t.Fatalf("seeding the due date: %v", err)
	}

	saved, err := view.NewSavedView(view.NewSavedViewInput{
		ID: freshID(t), TenantID: tenant, OwnerID: owner,
		ScopeType: view.ViewScopeCollection, ScopeID: collection,
		Name: "This week", Layout: "LIST_EXPANDED",
		Query:   map[string]any{"scope_container_id": collection.String()},
		Sharing: view.SharingPrivate,
		Now:     created,
	})
	if err != nil {
		t.Fatalf("the view was refused: %v", err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewSavedViewRepository().Insert(ctx, saved)
	}); err != nil {
		t.Fatalf("writing the view: %v", err)
	}

	minted, err := harness.mint.Execute(ctx, subscriber(tenant, owner), saved.ID)
	if err != nil {
		t.Fatalf("minting the feed failed: %v", err)
	}

	// While the owner may read the collection, the feed answers its entries.
	exported, err := harness.read.Execute(ctx, minted.Token)
	if err != nil {
		t.Fatalf("the fetch failed: %v", err)
	}
	if len(exported.Items) == 0 {
		t.Fatal("the feed answered nothing while its owner could read the collection")
	}

	// The membership goes. The token has not changed, and it is not what decides.
	if _, err := adminPool(ctx, t).Exec(ctx,
		`DELETE FROM membership WHERE tenant_id = $1 AND account_id = $2`,
		tenant.String(), owner.String()); err != nil {
		t.Fatalf("removing the membership: %v", err)
	}

	if _, err := harness.read.Execute(ctx, minted.Token); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a feed whose owner lost the view answered %v", err)
	}
}
