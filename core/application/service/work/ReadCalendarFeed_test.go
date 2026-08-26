// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	feed "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// accountShelf is the identity port's fake, cut to the one method a feed read needs.
type accountShelf struct {
	stored map[shared.ID]identity.Account
}

func (a *accountShelf) Find(_ context.Context, id shared.ID) (identity.Account, error) {
	found, ok := a.stored[id]
	if !ok {
		return identity.Account{}, shared.ErrNotFound.WithDetail("accountShelf.not_found")
	}
	return found, nil
}

func (a *accountShelf) FindByEmail(context.Context, string) (identity.Account, error) {
	return identity.Account{}, shared.ErrNotFound
}
func (a *accountShelf) Insert(context.Context, identity.Account) error { return nil }
func (a *accountShelf) UpdatePreferences(context.Context, identity.Account, time.Time) error {
	return nil
}

type feedReadHarness struct {
	read         ReadCalendarFeed
	feeds        *calendarFeeds
	views        *savedViews
	items        *items
	containers   *containers
	accountShelf *accountShelf
	permits      *permitting
}

func newFeedReadHarness(t *testing.T) *feedReadHarness {
	t.Helper()

	h := &feedReadHarness{
		feeds:        newCalendarFeeds(),
		views:        &savedViews{stored: map[shared.ID]view.SavedView{}},
		items:        &items{stored: map[shared.ID]domain.WorkItem{}, lastKey: "a0"},
		containers:   &containers{stored: map[shared.ID]domain.Container{}},
		accountShelf: &accountShelf{stored: map[shared.ID]identity.Account{}},
		permits:      &permitting{},
	}

	h.read = ReadCalendarFeed{
		Feeds: h.feeds, Accounts: h.accountShelf,
		Export: ExportView{
			Views: h.views, Containers: h.containers, Permits: h.permits,
			Query: QueryItems{
				Items: h.items, ItemLabels: &itemLabels{}, Containers: h.containers,
				Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
			},
			ItemLabels: &itemLabels{}, Audit: &sink{}, UnitOfWork: &unitOfWork{},
			Clock: clock.Fixed(now),
		},
		UnitOfWork: &unitOfWork{},
	}

	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.accountShelf.stored[accountID] = identity.Account{
		ID: accountID, TenantID: tenantID, Kind: identity.AccountUser,
		DisplayName: "Anna Beispiel", Status: identity.AccountActive, TimeZone: "Europe/Berlin",
	}
	return h
}

// subscribe puts a view, a feed and a token in place, and answers the token.
func (h *feedReadHarness) subscribe(t *testing.T, owner shared.ID) feed.FeedToken {
	t.Helper()

	saved, err := view.NewSavedView(view.NewSavedViewInput{
		ID: savedViewID, TenantID: tenantID, OwnerID: owner,
		ScopeType: view.ViewScopeCollection, ScopeID: collectionID,
		Name: "This week", Layout: "LIST_EXPANDED",
		Query:   map[string]any{"scope_container_id": collectionID.String()},
		Sharing: view.SharingPrivate,
		Now:     now,
	})
	if err != nil {
		t.Fatalf("the view was refused: %v", err)
	}
	h.views.stored[saved.ID] = saved

	subscription, err := feed.NewCalendarFeed(feed.NewCalendarFeedInput{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000f1"), TenantID: tenantID,
		AccountID: owner, ViewID: saved.ID, Now: now,
	})
	if err != nil {
		t.Fatalf("the feed was refused: %v", err)
	}

	entropy, err := clock.FixedEntropy{Seed: 0x30}.Bytes(feed.FeedTokenSecretBytes)
	if err != nil {
		t.Fatal(err)
	}
	token, err := feed.NewFeedToken(tenantID, entropy)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.feeds.Insert(t.Context(), subscription, token); err != nil {
		t.Fatal(err)
	}
	return token
}

// The plain fetch: the owner's rows, read as the owner, in the owner's zone.
func TestAFeedAnswersTheOwnersRows(t *testing.T) {
	h := newFeedReadHarness(t)
	token := h.subscribe(t, accountID)
	h.items.result = repository.ItemQueryResult{Items: exportedItems(2)}

	exported, err := h.read.Execute(t.Context(), token)
	if err != nil {
		t.Fatalf("the fetch failed: %v", err)
	}
	if len(exported.Items) != 2 {
		t.Errorf("the feed answered %d entries", len(exported.Items))
	}
	if exported.View.Name != "This week" {
		t.Errorf("the feed is called %q", exported.View.Name)
	}
	if exported.TimeZone != "Europe/Berlin" {
		t.Errorf("the selection ran in zone %q, and the owner's is Europe/Berlin", exported.TimeZone)
	}
}

// Every refusal is the same refusal. That is the security content of this route: an oracle here
// tells whoever is trying tokens which guesses are close (T-21).
func TestEveryReasonAFeedWillNotServeLooksTheSame(t *testing.T) {
	unknown, err := feed.NewFeedToken(tenantID, mustEntropy(t, 0x90))
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]func(*feedReadHarness) feed.FeedToken{
		"a token that names nothing": func(h *feedReadHarness) feed.FeedToken {
			h.subscribe(t, accountID)
			return unknown
		},
		"a revoked feed": func(h *feedReadHarness) feed.FeedToken {
			token := h.subscribe(t, accountID)
			for id := range h.feeds.stored {
				if _, err := h.feeds.Revoke(t.Context(), id, now); err != nil {
					t.Fatal(err)
				}
			}
			return token
		},
		"a feed whose view was deleted": func(h *feedReadHarness) feed.FeedToken {
			token := h.subscribe(t, accountID)
			for id, subscription := range h.feeds.stored {
				subscription.ViewID = ""
				h.feeds.stored[id] = subscription
			}
			return token
		},
		"an owner who is gone": func(h *feedReadHarness) feed.FeedToken {
			token := h.subscribe(t, accountID)
			delete(h.accountShelf.stored, accountID)
			return token
		},
		"an owner who is disabled": func(h *feedReadHarness) feed.FeedToken {
			token := h.subscribe(t, accountID)
			account := h.accountShelf.stored[accountID]
			account.Status = identity.AccountDisabled
			h.accountShelf.stored[accountID] = account
			return token
		},
		"an owner who may no longer see the view": func(h *feedReadHarness) feed.FeedToken {
			token := h.subscribe(t, otherOwner)
			h.accountShelf.stored[otherOwner] = identity.Account{
				ID: otherOwner, TenantID: tenantID, Kind: identity.AccountUser,
				DisplayName: "Bert", Status: identity.AccountActive,
			}
			// The view is shared into a scope this owner has since been removed from.
			saved := h.views.stored[savedViewID]
			saved.OwnerID = accountID
			saved.Sharing = view.SharingScope
			h.views.stored[savedViewID] = saved
			h.permits.allow = false
			return token
		},
		"the empty token": func(h *feedReadHarness) feed.FeedToken {
			return feed.FeedToken{}
		},
	}

	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			h := newFeedReadHarness(t)
			token := arrange(h)

			_, err := h.read.Execute(t.Context(), token)
			refusal := shared.AsError(err)
			if refusal == nil || refusal.DetailCode != "calendar.feed_not_found" {
				t.Fatalf("refused as %v", err)
			}
			// And nothing in the answer says which of the six it was.
			if len(refusal.Params) != 0 || len(refusal.Fields) != 0 {
				t.Errorf("the refusal carries %v / %v", refusal.Params, refusal.Fields)
			}
		})
	}
}

// The feed narrows with its owner: the rows are selected under the owner's authorisation, so a
// query the owner may no longer run answers nothing rather than what it answered last year.
func TestTheFeedReadsAsItsOwnerAtFetchTime(t *testing.T) {
	h := newFeedReadHarness(t)
	token := h.subscribe(t, accountID)
	h.items.result = repository.ItemQueryResult{Items: exportedItems(1)}

	if _, err := h.read.Execute(t.Context(), token); err != nil {
		t.Fatalf("the fetch failed: %v", err)
	}
	if len(h.items.searched) != 1 {
		t.Fatalf("the query ran %d times", len(h.items.searched))
	}
	// The scope the owner's view names, resolved as the owner rather than as nobody.
	if h.items.searched[0].Spec.Scope.ContainerID != collectionID {
		t.Errorf("the query was anchored at %s", h.items.searched[0].Spec.Scope.ContainerID)
	}
}

func mustEntropy(t *testing.T, seed byte) []byte {
	t.Helper()
	drawn, err := clock.FixedEntropy{Seed: seed}.Bytes(feed.FeedTokenSecretBytes)
	if err != nil {
		t.Fatal(err)
	}
	return drawn
}
