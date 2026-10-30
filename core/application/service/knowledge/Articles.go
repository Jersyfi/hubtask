// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names and trail codes of a knowledge base published to a portal (UC-DOC-13).
const (
	PublishPageToPortalName     = "PublishPageToPortal"
	UnpublishPageFromPortalName = "UnpublishPageFromPortal"
	SuggestArticlesName         = "SuggestArticles"
	ReadArticleName             = "ReadArticle"
	RateArticleName             = "RateArticle"

	PageArticlePublishedAction   audit.Action = "page.article_published"
	PageArticleUnpublishedAction audit.Action = "page.article_unpublished"
	ArticleReadAction            audit.Action = "page_article.read"
	ArticleRatedAction           audit.Action = "page_article.rated"

	articleTarget = "page_article"
	// maxSuggestionText bounds a portal's question: what a requester has typed so far.
	maxSuggestionText = 500
)

// service reads a collection the portal serves and asks a permission there; a hub or a missing
// container is not found.
func (s Space) service(ctx context.Context, actor appshared.ActorContext, id shared.ID, permission service.Permission, action audit.Action) (work.Container, error) {
	if id.IsZero() {
		return work.Container{}, fieldInvalid("/collection_id", "pages.article_collection_missing")
	}
	var collection work.Container
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		collection, err = s.Containers.Find(ctx, id)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) || (err == nil && collection.Type != work.ContainerCollection) {
		return work.Container{}, appshared.ContainerNotFound(id)
	}
	if err != nil {
		return work.Container{}, err
	}
	return collection, s.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: service.ContainerScopes(collection), Action: action,
		TokenScope: pagesRead, TargetType: articleTarget, TargetID: id,
	})
}

func fieldInvalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

// ArticleOutput is where a page is published (api/openapi.yaml, schema PageArticle).
func ArticleOutput(a domain.Article) usecase.Output {
	return usecase.Output{
		"page_id": a.PageID.String(), "collection_id": a.CollectionID.String(),
		"published_by": idOrNil(a.PublishedBy), "published_at": a.PublishedAt,
		"views": a.Views, "helpful": a.Helpful, "unhelpful": a.Unhelpful,
	}
}

// PublishPageToPortal publishes a page as an article of one service's portal: its requesters read
// it and find it while they type a request. Who manages the hub's members publishes, as they
// would share the page; never a page behind a restriction.
type PublishPageToPortal struct{ Space Space }

// Execute publishes.
func (h PublishPageToPortal) Execute(ctx context.Context, actor appshared.ActorContext, pageID, collectionID shared.ID) (domain.Article, error) {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return domain.Article{}, err
	}
	if h.Space.Articles == nil {
		return domain.Article{}, shared.ErrUnavailable.WithDetail("pages.articles_unavailable")
	}
	place, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return domain.Article{}, err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(place.HubID, service.PermissionManageMembers,
		PageArticlePublishedAction, pagesWrite, articleTarget, place.ID)); err != nil {
		return domain.Article{}, err
	}
	if _, err := h.Space.service(ctx, actor, collectionID, service.PermissionRead, PageArticlePublishedAction); err != nil {
		return domain.Article{}, err
	}
	now := h.Space.Clock.Now()
	var article domain.Article
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		page, err := h.Space.find(ctx, pageID, true)
		if err != nil {
			return err
		}
		readers, err := h.Space.Pages.Readers(ctx, []shared.ID{page.ID})
		if err != nil {
			return err
		}
		if article, err = domain.NewArticle(page.ID, collectionID, actor.AccountID, readers[page.ID], now); err != nil {
			return err
		}
		added, err := h.Space.Articles.Publish(ctx, article)
		if err != nil || !added {
			return err
		}
		return h.Space.record(ctx, actor, PageArticlePublishedAction, pageTarget, page.ID, []audit.Change{
			{Field: "collection_id", Classification: audit.Open, To: collectionID.String()},
		}, now)
	})
	return article, err
}

// Descriptor is the catalogue entry.
func (h PublishPageToPortal) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: PublishPageToPortalName, Permission: service.PermissionManageMembers,
		Summary: "Publishes a page as an article of one service's portal: the requesters of that " +
			"collection read it and are offered it while they type a request. The page alone, as it " +
			"stands at each reading. Never a page behind a restriction; publishing twice changes nothing.",
		SideEffects: "Writes the publication and an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The service whose portal shows it."},
		},
		Audit: usecase.AuditDeclaration{Action: PageArticlePublishedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			ids, err := twoIDs(in, "page_id", "collection_id")
			if err != nil {
				return nil, err
			}
			article, err := h.Execute(ctx, actor, ids[0], ids[1])
			if err != nil {
				return nil, err
			}
			return ArticleOutput(article), nil
		}),
	})
}

func twoIDs(in usecase.Input, first, second string) ([2]shared.ID, error) {
	var out [2]shared.ID
	for i, field := range []string{first, second} {
		id, err := in.ID(field)
		if err != nil {
			return out, err
		}
		out[i] = id
	}
	return out, nil
}

// UnpublishPageFromPortal takes an article back from one service's portal.
type UnpublishPageFromPortal struct{ Space Space }

// Execute takes it back.
func (h UnpublishPageFromPortal) Execute(ctx context.Context, actor appshared.ActorContext, pageID, collectionID shared.ID) error {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return err
	}
	if h.Space.Articles == nil {
		return shared.ErrNotFound.WithDetail("pages.article_not_found")
	}
	place, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(place.HubID, service.PermissionManageMembers,
		PageArticleUnpublishedAction, pagesWrite, articleTarget, place.ID)); err != nil {
		return err
	}
	now := h.Space.Clock.Now()
	return h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := h.Space.Articles.Unpublish(ctx, place.ID, collectionID)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrNotFound.WithDetail("pages.article_not_found")
		}
		return h.Space.record(ctx, actor, PageArticleUnpublishedAction, pageTarget, place.ID, []audit.Change{
			{Field: "collection_id", Classification: audit.Open, To: collectionID.String()},
		}, now)
	})
}

// Descriptor is the catalogue entry.
func (h UnpublishPageFromPortal) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UnpublishPageFromPortalName, Permission: service.PermissionManageMembers,
		Summary:     "Takes an article back from one service's portal; its openings and ratings go with it.",
		SideEffects: "Removes the publication with its ratings and writes an audit entry.",
		TokenScope:  pagesWrite,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The service."},
		},
		Audit: usecase.AuditDeclaration{Action: PageArticleUnpublishedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			ids, err := twoIDs(in, "page_id", "collection_id")
			if err != nil {
				return nil, err
			}
			return nil, h.Execute(ctx, actor, ids[0], ids[1])
		}),
	})
}

// SuggestArticles answers the articles of a service a requester's words find, while they type a
// request (deflection); without words, the most read. Asked with REQUEST at the collection, so a
// requester who reads no hub reads its articles.
type SuggestArticles struct{ Space Space }

// Execute answers the suggestions.
func (h SuggestArticles) Execute(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, text string) ([]usecase.Output, error) {
	if err := actor.RequireScope(pagesRead); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > maxSuggestionText {
		return nil, fieldInvalid("/text", "pages.article_text_too_long")
	}
	if _, err := h.Space.service(ctx, actor, collectionID, service.PermissionRequest, ArticleReadAction); err != nil {
		return nil, err
	}
	out := []usecase.Output{}
	if h.Space.Articles == nil {
		return out, nil
	}
	err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		hits, err := h.Space.Articles.Search(ctx, collectionID, h.Space.Text.NFC(text), domain.MaxArticleSuggestions)
		for _, hit := range hits {
			out = append(out, usecase.Output{
				"page_id": hit.PageID.String(), "title": hit.Title, "icon": hit.Icon,
				"excerpt": hit.Excerpt, "updated_at": hit.UpdatedAt,
			})
		}
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h SuggestArticles) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SuggestArticlesName, Permission: service.PermissionRequest,
		Summary: "Answers at most ten articles of a service's portal that the words a requester " +
			"has typed find, best first - or, without words, the most read. Only live pages " +
			"published to that service, never one behind a restriction.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The service."},
			{Name: "text", Kind: usecase.KindString, Description: "What the requester has typed so far."},
		},
		Audit: usecase.AuditDeclaration{Action: ArticleReadAction, TargetType: articleTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			hits, err := h.Execute(ctx, actor, id, in.String("text"))
			if err != nil {
				return nil, err
			}
			return usecase.Output{"data": hits}, nil
		}),
	})
}

// article reads a page published to a service the actor may request at, inside a transaction:
// trashed, unpublished or restricted since, it is not found.
func (s Space) article(ctx context.Context, pageID, collectionID shared.ID) (domain.Page, error) {
	held, err := s.Articles.Published(ctx, pageID, collectionID)
	if err != nil {
		return domain.Page{}, err
	}
	if !held {
		return domain.Page{}, shared.ErrNotFound.WithDetail("pages.article_not_found")
	}
	page, err := s.find(ctx, pageID, false)
	if errors.Is(err, shared.ErrNotFound) {
		return domain.Page{}, shared.ErrNotFound.WithDetail("pages.article_not_found")
	}
	if err != nil {
		return domain.Page{}, err
	}
	readers, err := s.Pages.Readers(ctx, []shared.ID{page.ID})
	if err != nil {
		return domain.Page{}, err
	}
	if readers[page.ID].Restricted || page.IsArchived() {
		return domain.Page{}, shared.ErrNotFound.WithDetail("pages.article_not_found")
	}
	return page, nil
}

// ReadArticle opens an article in a service's portal and counts the opening.
type ReadArticle struct{ Space Space }

// Execute reads the article.
func (h ReadArticle) Execute(ctx context.Context, actor appshared.ActorContext, collectionID, pageID shared.ID) (domain.Page, error) {
	if err := actor.RequireScope(pagesRead); err != nil {
		return domain.Page{}, err
	}
	if _, err := h.Space.service(ctx, actor, collectionID, service.PermissionRequest, ArticleReadAction); err != nil {
		return domain.Page{}, err
	}
	if h.Space.Articles == nil {
		return domain.Page{}, shared.ErrNotFound.WithDetail("pages.article_not_found")
	}
	var page domain.Page
	err := h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if page, err = h.Space.article(ctx, pageID, collectionID); err != nil {
			return err
		}
		return h.Space.Articles.CountView(ctx, page.ID, collectionID)
	})
	return page, err
}

// Descriptor is the catalogue entry.
func (h ReadArticle) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadArticleName, Permission: service.PermissionRequest,
		Summary: "Opens an article of a service's portal - the page alone, with its Markdown body, " +
			"as it stands now - and counts the opening. Unpublished, trashed, archived or restricted " +
			"since, it is not found.",
		SideEffects: "Counts one opening.",
		TokenScope:  pagesRead,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The service."},
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The article's page."},
		},
		Audit:    usecase.AuditDeclaration{Action: ArticleReadAction, TargetType: articleTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "an opening is counted, not recorded; a page is not an entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			ids, err := twoIDs(in, "collection_id", "page_id")
			if err != nil {
				return nil, err
			}
			page, err := h.Execute(ctx, actor, ids[0], ids[1])
			if err != nil {
				return nil, err
			}
			return usecase.Output{
				"page_id": page.ID.String(), "collection_id": ids[0].String(), "title": page.Title, "icon": page.Icon,
				"body": page.Body, "content_language": nilIfEmpty(page.ContentLanguage), "updated_at": page.UpdatedAt,
			}, nil
		}),
	})
}

// RateArticle keeps a requester's "was this helpful?" of an article, replacing what they said
// before.
type RateArticle struct{ Space Space }

// Execute rates.
func (h RateArticle) Execute(ctx context.Context, actor appshared.ActorContext, collectionID, pageID shared.ID, helpful bool) error {
	if err := actor.RequireScope(pagesRead); err != nil {
		return err
	}
	if _, err := h.Space.service(ctx, actor, collectionID, service.PermissionRequest, ArticleRatedAction); err != nil {
		return err
	}
	if h.Space.Articles == nil {
		return shared.ErrNotFound.WithDetail("pages.article_not_found")
	}
	now := h.Space.Clock.Now()
	return h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		page, err := h.Space.article(ctx, pageID, collectionID)
		if err != nil {
			return err
		}
		return h.Space.Articles.Rate(ctx, page.ID, collectionID, actor.AccountID, helpful, now)
	})
}

// Descriptor is the catalogue entry.
func (h RateArticle) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RateArticleName, Permission: service.PermissionRequest,
		Summary: "Keeps whether an article of a service's portal helped the requester, one answer " +
			"per person - a second replaces the first. Its publisher sees the counts, never who said what.",
		SideEffects: "Writes or replaces the rating.",
		TokenScope:  pagesRead,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The service."},
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The article's page."},
			{Name: "helpful", Kind: usecase.KindBool, Required: true, Description: "Whether it helped."},
		},
		Audit:    usecase.AuditDeclaration{Action: ArticleRatedAction, TargetType: articleTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a rating is counted on the article; a page is not an entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			ids, err := twoIDs(in, "collection_id", "page_id")
			if err != nil {
				return nil, err
			}
			helpful := in.Bool("helpful")
			if err := h.Execute(ctx, actor, ids[0], ids[1], helpful); err != nil {
				return nil, err
			}
			return usecase.Output{"page_id": ids[1].String(), "collection_id": ids[0].String(), "helpful": helpful}, nil
		}),
	})
}
