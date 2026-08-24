// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The search, at the level this layer owns: what it asks the repository for, and what it does with
// the answer. Whether the statement finds a German compound word is the database's to answer, and
// is asked of a real one in test/integration.

// searchHarness wires the use case to the fakes, with the entries a search will answer with.
func searchHarness(hits ...repository.ItemHit) (SearchItems, *items, *authorizer, *reader) {
	store := &items{stored: map[shared.ID]domain.WorkItem{}}
	store.hits = repository.ItemHitPage{
		Hits: hits,
		Info: repository.PageInfo{NextCursor: "the-scanned-boundary", HasMore: true},
	}
	permission, permitted := &authorizer{}, &reader{}

	return SearchItems{
		Items: store, Containers: &containers{stored: map[shared.ID]domain.Container{}},
		Authorizer: permission, Anchored: permission, Reader: permitted,
		UnitOfWork: &unitOfWork{},
	}, store, permission, permitted
}

func hitOf(id, collection, hub shared.ID, rank float32) repository.ItemHit {
	return repository.ItemHit{
		Item: domain.WorkItem{
			ID: id, TenantID: tenantID, CollectionID: collection, Type: domain.ItemTask,
			Path: domain.RootPath(id), Depth: 1, Title: "Quarterly report", OrderKey: "a0",
			CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
		},
		HubID: hub, Rank: rank,
	}
}

// The unanchored search: nothing is asked upfront, because there is nothing to ask about, and the
// page is narrowed to what the actor may see afterwards - the shape the trash and the hub level
// already have.
func TestAnUnanchoredSearchIsNarrowedRatherThanRefused(t *testing.T) {
	otherHub := shared.MustParseID("0192f000-0000-7000-8000-00000000001b")
	otherCollection := shared.MustParseID("0192f000-0000-7000-8000-00000000001c")

	handler, store, permission, permitted := searchHarness(
		hitOf(taskID, collectionID, hubID, 0.9),
		hitOf(packageID, otherCollection, otherHub, 0.4),
	)
	// The double keys on the last scope of each path, which for a search is the entry itself.
	permitted.permit = map[shared.ID]bool{taskID: true}

	page, err := handler.Execute(t.Context(), itemActor(), SearchItemsQuery{Words: "quarterly"})
	if err != nil {
		t.Fatalf("searching was refused: %v", err)
	}

	if len(page.Hits) != 1 || page.Hits[0].Item.ID != taskID {
		t.Fatalf("the page holds %d hits, want only the readable one", len(page.Hits))
	}
	if len(permission.requests) != 0 {
		t.Errorf("an unanchored search asked about a scope: %+v", permission.requests)
	}
	if store.searchedText[0].Anchor.Kind != repository.AnchorTenant {
		t.Errorf("the anchor is %q, want the whole tenant", store.searchedText[0].Anchor.Kind)
	}
}

// The path a hit is judged against ends on the entry, and that is not a detail: an entry shared
// with somebody individually is reachable through no other scope, so a path stopping at the
// collection would hide exactly the entries C-04 exists to show.
func TestAHitIsJudgedByThePathThatEndsOnTheEntry(t *testing.T) {
	handler, _, _, permitted := searchHarness(hitOf(taskID, collectionID, hubID, 0.9))

	if _, err := handler.Execute(
		t.Context(), itemActor(), SearchItemsQuery{Words: "quarterly"}); err != nil {
		t.Fatalf("searching was refused: %v", err)
	}

	if len(permitted.asked) != 1 {
		t.Fatalf("%d paths were asked about, want one", len(permitted.asked))
	}
	want := []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID),
		identity.CollectionScope(collectionID), identity.ItemScope(taskID),
	}
	for i, scope := range want {
		if permitted.asked[0][i] != scope {
			t.Errorf("scope %d of the path is %+v, want %+v", i, permitted.asked[0][i], scope)
		}
	}
}

// The cursor is a boundary in what was *scanned*, not in what came back. Narrowing it to the last
// visible hit would skip everything between it and the last row actually read - the walk would then
// silently lose entries the actor may well be allowed to see.
func TestNarrowingASearchLeavesTheCursorAlone(t *testing.T) {
	handler, _, _, permitted := searchHarness(
		hitOf(taskID, collectionID, hubID, 0.9),
		hitOf(packageID, collectionID, hubID, 0.4),
	)
	permitted.permit = map[shared.ID]bool{}

	page, err := handler.Execute(t.Context(), itemActor(), SearchItemsQuery{Words: "quarterly"})
	if err != nil {
		t.Fatalf("searching was refused: %v", err)
	}

	if len(page.Hits) != 0 {
		t.Errorf("%d hits survived a narrowing that permits nothing", len(page.Hits))
	}
	if page.Info.NextCursor != "the-scanned-boundary" || !page.Info.HasMore {
		t.Errorf("the walk was cut short: %+v", page.Info)
	}
}

// A named collection asks how far the actor reaches into it, exactly as the plain level list does.
// An actor who holds no role on it but has entries inside it shared with them searches those.
func TestASearchInACollectionCarriesTheSharesIntoTheStatement(t *testing.T) {
	handler, store, permission, permitted := searchHarness()
	permission.shares = []shared.ID{taskID}

	collection := collectionFixture(collectionID, hubID, "Shopping")
	handler.Containers.(*containers).stored[collectionID] = collection

	if _, err := handler.Execute(t.Context(), itemActor(), SearchItemsQuery{
		Words: "quarterly", ContainerID: collectionID,
	}); err != nil {
		t.Fatalf("searching was refused: %v", err)
	}

	search := store.searchedText[0]
	if search.Anchor.Kind != repository.AnchorCollection || search.Anchor.CollectionID != collectionID {
		t.Errorf("the anchor is %+v, want the collection", search.Anchor)
	}
	if len(search.RestrictTo) != 1 || search.RestrictTo[0] != taskID {
		t.Errorf("the statement was restricted to %v, want the shared entry", search.RestrictTo)
	}
	// The rows are already restricted to what the actor may see, so asking again per row would be
	// a second answer to a question that has one.
	if permitted.asked != nil {
		t.Errorf("an anchored search narrowed the page as well: %v", permitted.asked)
	}
}

// A named hub is a refusal when it may not be read, because the client named it - the same answer
// the query language gives, and the honest one: an empty page would say the hub is empty.
func TestASearchInAHubTheActorCannotReadIsRefused(t *testing.T) {
	handler, _, permission, _ := searchHarness()
	permission.err = errors.New("not permitted")
	handler.Containers.(*containers).stored[hubID] = hubFixture(hubID, "Home", "a0")

	_, err := handler.Execute(t.Context(), itemActor(), SearchItemsQuery{
		Words: "quarterly", ContainerID: hubID,
	})
	if err == nil {
		t.Fatal("a search in an unreadable hub answered a page")
	}
}

// A scope that is not there is not there, and so is one belonging to another tenant: row level
// security has already made the second look like the first (multi-tenancy.md §2).
func TestASearchInAContainerThatIsNotThereIsNotFound(t *testing.T) {
	handler, _, _, _ := searchHarness()

	_, err := handler.Execute(t.Context(), itemActor(), SearchItemsQuery{
		Words: "quarterly", ContainerID: hubID,
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a missing container answered %v", err)
	}
}

// The words are the whole request, so a search without them is refused by name rather than
// answered with everything - which is what the query endpoint is for, in an order somebody chose.
func TestASearchWithoutWordsIsRefused(t *testing.T) {
	handler, _, _, _ := searchHarness()

	for _, words := range []string{"", "   "} {
		_, err := handler.Execute(t.Context(), itemActor(), SearchItemsQuery{Words: words})

		var domainErr *shared.Error
		if !errors.As(err, &domainErr) || domainErr.DetailCode != "search.words_required" {
			t.Errorf("a search for %q answered %v", words, err)
		}
	}
}

// The language the words are read under is the caller's: stated in the request, or the locale the
// actor is being served in. It is not the entries' - an entry is indexed under its own, which is
// what makes the pair the two halves of ADR-0034.
func TestTheSearchLanguageIsTheCallersAndDefaultsToTheirLocale(t *testing.T) {
	actor := itemActor()
	actor.Locale = "de-AT"

	handler, store, _, _ := searchHarness()
	if _, err := handler.Execute(t.Context(), actor, SearchItemsQuery{Words: "quarterly"}); err != nil {
		t.Fatalf("searching was refused: %v", err)
	}
	if language := store.searchedText[0].Request.Language; language != "de-AT" {
		t.Errorf("the words were read as %q, want the actor's locale", language)
	}

	if _, err := handler.Execute(t.Context(), actor, SearchItemsQuery{
		Words: "quarterly", Language: "en",
	}); err != nil {
		t.Fatalf("searching was refused: %v", err)
	}
	if language := store.searchedText[1].Request.Language; language != "en" {
		t.Errorf("the words were read as %q, want the language the client stated", language)
	}
}

// A size nobody named is the default and one beyond the ceiling is clamped, as everywhere else: a
// client asking for 500 hits wants as many as it can have (api-guidelines.md §4).
func TestTheSearchPageSizeIsClamped(t *testing.T) {
	for _, c := range []struct{ asked, want int }{
		{0, DefaultPageSize}, {-1, DefaultPageSize}, {500, MaxPageSize}, {25, 25},
	} {
		handler, store, _, _ := searchHarness()
		if _, err := handler.Execute(t.Context(), itemActor(), SearchItemsQuery{
			Words: "quarterly", Size: c.asked,
		}); err != nil {
			t.Fatalf("searching was refused: %v", err)
		}
		if size := store.searchedText[0].Request.Size; size != c.want {
			t.Errorf("a size of %d reached the repository as %d, want %d", c.asked, size, c.want)
		}
	}
}

// Read-only throughout: the transaction may be served by a read replica, and a read that opened a
// writable one would pin every search in the product to the primary (multi-tenancy.md §7).
func TestASearchOpensNoWriteTransaction(t *testing.T) {
	handler, _, _, _ := searchHarness(hitOf(taskID, collectionID, hubID, 0.9))

	if _, err := handler.Execute(
		t.Context(), itemActor(), SearchItemsQuery{Words: "quarterly"}); err != nil {
		t.Fatalf("searching was refused: %v", err)
	}
	if uow := handler.UnitOfWork.(*unitOfWork); uow.writes != 0 {
		t.Errorf("%d write transactions were opened", uow.writes)
	}
}
