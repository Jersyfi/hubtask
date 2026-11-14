// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

var (
	desk    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b010")
	visitor = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b011")
)

// services knows the hubs and one service collection in the shared hub.
type services struct{ hubs }

func (s services) Find(ctx context.Context, id shared.ID) (work.Container, error) {
	if id == desk {
		return work.Container{ID: desk, Type: work.ContainerCollection, ParentID: hub}, nil
	}
	return s.hubs.Find(ctx, id)
}

// requesterGate lets the visitor REQUEST at the desk and nothing else; everybody else as gate.
type requesterGate struct{ gate }

func (g requesterGate) Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error {
	if actor.AccountID == visitor {
		if request.Permission == service.PermissionRequest && request.TargetID == desk {
			return nil
		}
		return shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return g.gate.Authorize(ctx, actor, request)
}

type shelf struct {
	articles map[[2]shared.ID]*domain.Article
	ratings  map[[3]shared.ID]bool
	pages    *store
}

func (s *shelf) Publish(_ context.Context, a domain.Article) (bool, error) {
	key := [2]shared.ID{a.PageID, a.CollectionID}
	if held := s.articles[key]; held != nil {
		changed := held.WithSubpages != a.WithSubpages
		held.WithSubpages = a.WithSubpages
		return changed, nil
	}
	s.articles[key] = &a
	return true, nil
}
func (s *shelf) Unpublish(_ context.Context, page, collection shared.ID) (bool, error) {
	key := [2]shared.ID{page, collection}
	held := s.articles[key] != nil
	delete(s.articles, key)
	return held, nil
}
func (s *shelf) Of(_ context.Context, page shared.ID) ([]domain.Article, error) {
	var out []domain.Article
	for key, a := range s.articles {
		if key[0] == page {
			held := *a
			for r, helpful := range s.ratings {
				if r[0] == page && r[1] == key[1] {
					if helpful {
						held.Helpful++
					} else {
						held.Unhelpful++
					}
				}
			}
			out = append(out, held)
		}
	}
	return out, nil
}
func (s *shelf) Source(_ context.Context, page, collection shared.ID) (shared.ID, error) {
	for at, depth := page, 0; !at.IsZero() && depth < 16; at, depth = s.pages.pages[at].ParentID, depth+1 {
		if a := s.articles[[2]shared.ID{at, collection}]; a != nil && (depth == 0 || a.WithSubpages) {
			return at, nil
		}
	}
	return "", nil
}
func (s *shelf) Shared(_ context.Context, collection shared.ID) ([]repository.SharedKnowledge, error) {
	out := []repository.SharedKnowledge{}
	for key, a := range s.articles {
		root := s.pages.pages[key[0]]
		if key[1] != collection || root.Restricted {
			continue
		}
		each := repository.SharedKnowledge{PageID: root.ID, Title: root.Title, WithSubpages: a.WithSubpages}
		var under func(parent shared.ID)
		under = func(parent shared.ID) {
			for _, p := range s.pages.pages {
				if p.ParentID == parent && p.DeletedAt == nil && !p.Restricted {
					each.Pages = append(each.Pages, repository.SharedPage{PageID: p.ID, ParentID: parent, Title: p.Title})
					under(p.ID)
				}
			}
		}
		if a.WithSubpages {
			under(root.ID)
		}
		out = append(out, each)
	}
	return out, nil
}
func (s *shelf) Search(_ context.Context, collection shared.ID, text string, _ []string, _ int) ([]repository.ArticleHit, error) {
	var out []repository.ArticleHit
	for key := range s.articles {
		page := s.pages.pages[key[0]]
		if key[1] == collection && page.DeletedAt == nil && !page.Restricted && strings.Contains(page.Body, text) {
			out = append(out, repository.ArticleHit{PageID: page.ID, Title: page.Title})
		}
	}
	return out, nil
}
func (s *shelf) CountView(_ context.Context, page, collection shared.ID) error {
	s.articles[[2]shared.ID{page, collection}].Views++
	return nil
}
func (s *shelf) Rate(_ context.Context, page, collection, account shared.ID, helpful bool, _ time.Time) error {
	s.ratings[[3]shared.ID{page, collection, account}] = helpful
	return nil
}

type plain struct{}

func (plain) NFC(text string) string  { return text }
func (plain) NFKC(text string) string { return text }

// UC-DOC-13 checks 1-3: who manages a hub's members publishes a page to a service; a requester of
// that service who reads no hub is offered it while typing, opens it - counted - and says whether
// it helped, a second answer replacing the first; a page restricted since is no article, and an
// article taken back is not found.
func TestAnArticleReachesTheRequestersOfItsServiceAlone(t *testing.T) {
	f := newFixture()
	articles := &shelf{articles: map[[2]shared.ID]*domain.Article{}, ratings: map[[3]shared.ID]bool{}, pages: f.store}
	f.space.Articles, f.space.Containers, f.space.Authorizer, f.space.Text = articles, services{}, requesterGate{}, plain{}
	ctx := context.Background()
	page := f.create(t, anna, hub, "", "Reset your password", "how to reset the password")

	if _, err := (PublishPageToPortal{Space: f.space}).Execute(ctx, as(anna), page.ID, hub, false); err == nil {
		t.Fatal("a hub was taken for a service")
	}
	article, err := (PublishPageToPortal{Space: f.space}).Execute(ctx, as(anna), page.ID, desk, false)
	if err != nil || article.CollectionID != desk {
		t.Fatalf("PublishPageToPortal: %+v %v", article, err)
	}
	if _, err := (ReadPage{Space: f.space}).Execute(ctx, as(visitor), page.ID); err == nil {
		t.Fatal("the requester read the page in its hub")
	}
	hits, err := (SuggestArticles{Space: f.space}).Execute(ctx, as(visitor), desk, "  password ")
	if err != nil || len(hits) != 1 || hits[0]["page_id"] != page.ID.String() {
		t.Fatalf("SuggestArticles: %v %v", hits, err)
	}
	if _, err := (SuggestArticles{Space: f.space}).Execute(ctx, as(visitor), desk, strings.Repeat("x", 501)); code(err) != "pages.article_text_too_long" {
		t.Fatalf("a long question answered %v", err)
	}
	read, err := (ReadArticle{Space: f.space}).Execute(ctx, as(visitor), desk, page.ID)
	if err != nil || read.Page.Body != "how to reset the password" || read.Source != page.ID {
		t.Fatalf("ReadArticle: %+v %v", read, err)
	}
	if last := f.trail.entries[len(f.trail.entries)-1]; last.Action != ArticleReadAction || last.TargetID != page.ID {
		t.Fatalf("the opening reached the trail as %+v", last)
	}
	for _, helpful := range []bool{true, false} {
		if err := (RateArticle{Space: f.space}).Execute(ctx, as(visitor), desk, page.ID, helpful); err != nil {
			t.Fatal(err)
		}
		// The trail names the rating, never what was said (check 3).
		if last := f.trail.entries[len(f.trail.entries)-1]; last.Action != ArticleRatedAction || len(last.Changes) != 0 {
			t.Fatalf("the rating reached the trail as %+v", last)
		}
	}
	access, err := (ReadPageAccess{Space: f.space}).Execute(ctx, as(anna), page.ID)
	if err != nil || len(access.Articles) != 1 || access.Articles[0].Views != 1 || access.Articles[0].Helpful != 0 || access.Articles[0].Unhelpful != 1 {
		t.Fatalf("the publisher's counts: %+v %v", access.Articles, err)
	}

	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: page.ID, AccountIDs: []shared.ID{anna}}); err != nil {
		t.Fatal(err)
	}
	if _, err := (ReadArticle{Space: f.space}).Execute(ctx, as(visitor), desk, page.ID); code(err) != "pages.article_not_found" {
		t.Fatalf("a restricted page was read as an article: %v", err)
	}
	if _, err := (PublishPageToPortal{Space: f.space}).Execute(ctx, as(anna), f.create(t, anna, hub, page.ID, "Under", "x").ID, desk, false); code(err) != "pages.article_restricted" {
		t.Fatalf("a page behind a restriction was published: %v", err)
	}
	if err := (UnpublishPageFromPortal{Space: f.space}).Execute(ctx, as(anna), page.ID, desk); err != nil {
		t.Fatal(err)
	}
	if err := (UnpublishPageFromPortal{Space: f.space}).Execute(ctx, as(anna), page.ID, desk); code(err) != "pages.article_not_found" {
		t.Fatalf("a second unpublish answered %v", err)
	}
}

// UC-DOC-13 check 5 (PO-4, E-13): a page shared as a space reaches the service's requesters with
// everything under it - read, found and counted on the space - except a restricted page and what
// lies under that; the same page as a single article keeps its subpages to the hub.
func TestASpaceSharesEverythingUnderItButARestrictedBranch(t *testing.T) {
	f := newFixture()
	articles := &shelf{articles: map[[2]shared.ID]*domain.Article{}, ratings: map[[3]shared.ID]bool{}, pages: f.store}
	f.space.Articles, f.space.Containers, f.space.Authorizer, f.space.Text = articles, services{}, requesterGate{}, plain{}
	ctx := context.Background()
	handbook := f.create(t, anna, hub, "", "Handbook", "the handbook")
	account := f.create(t, anna, hub, handbook.ID, "Account and sign-in", "passkeys")
	lost := f.create(t, anna, hub, account.ID, "Lost device", "revoke the device")
	internal := f.create(t, anna, hub, handbook.ID, "Internal", "for the team")
	below := f.create(t, anna, hub, internal.ID, "Below internal", "still for the team")

	if _, err := (PublishPageToPortal{Space: f.space}).Execute(ctx, as(anna), handbook.ID, desk, false); err != nil {
		t.Fatal(err)
	}
	if _, err := (ReadArticle{Space: f.space}).Execute(ctx, as(visitor), desk, lost.ID); code(err) != "pages.article_not_found" {
		t.Fatalf("a subpage of a single article was read: %v", err)
	}
	space, err := (PublishPageToPortal{Space: f.space}).Execute(ctx, as(anna), handbook.ID, desk, true)
	if err != nil || !space.WithSubpages {
		t.Fatalf("sharing as a space: %+v %v", space, err)
	}
	if last := f.trail.entries[len(f.trail.entries)-1]; last.Action != PageArticlePublishedAction {
		t.Fatalf("widening to a space left no trail: %+v", last)
	}
	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: internal.ID, AccountIDs: []shared.ID{anna}}); err != nil {
		t.Fatal(err)
	}

	read, err := (ReadArticle{Space: f.space}).Execute(ctx, as(visitor), desk, lost.ID)
	if err != nil || read.Source != lost.ID || read.Page.Body != "revoke the device" {
		t.Fatalf("a page under the space: %+v %v", read, err)
	}
	for _, hidden := range []shared.ID{internal.ID, below.ID} {
		if _, err := (ReadArticle{Space: f.space}).Execute(ctx, as(visitor), desk, hidden); code(err) != "pages.article_not_found" {
			t.Fatalf("a restricted branch was read: %v", err)
		}
	}
	if err := (RateArticle{Space: f.space}).Execute(ctx, as(visitor), desk, lost.ID, true); err != nil {
		t.Fatal(err)
	}
	if held := articles.articles[[2]shared.ID{handbook.ID, desk}]; held.Views != 1 {
		t.Fatalf("the opening counted on %+v, not the space", held)
	}
	if !articles.ratings[[3]shared.ID{handbook.ID, desk, visitor}] {
		t.Fatal("the rating did not count on the space")
	}

	spaces, err := (ListPortalKnowledge{Space: f.space}).Execute(ctx, as(visitor), desk)
	if err != nil || len(spaces) != 1 || !spaces[0].WithSubpages {
		t.Fatalf("ListPortalKnowledge: %+v %v", spaces, err)
	}
	names := map[string]bool{}
	for _, p := range spaces[0].Pages {
		names[p.Title] = true
	}
	if !names["Account and sign-in"] || !names["Lost device"] || names["Internal"] || names["Below internal"] {
		t.Fatalf("the space's pages: %+v", spaces[0].Pages)
	}
	if _, err := (ListPortalKnowledge{Space: f.space}).Execute(ctx, as(visitor), hub); err == nil {
		t.Fatal("a hub's knowledge was listed as a portal's")
	}
}
