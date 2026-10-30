// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PageArticleRepository stores the pages published to a service's portal (migration 0264).
type PageArticleRepository struct{}

// NewPageArticleRepository is the adapter.
func NewPageArticleRepository() PageArticleRepository { return PageArticleRepository{} }

var _ repository.Articles = PageArticleRepository{}

// Publish publishes the page to the collection.
func (PageArticleRepository) Publish(ctx context.Context, article knowledge.Article) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	ids, err := pageUUIDs(article.PageID, article.CollectionID)
	if err != nil {
		return false, err
	}
	by, err := optionalUUID(article.PublishedBy)
	if err != nil {
		return false, err
	}
	rows, err := queries.PublishPageArticle(ctx, sqlc.PublishPageArticleParams{
		PageID: ids[0], CollectionID: ids[1], PublishedBy: by, At: timestampOf(article.PublishedAt),
	})
	if err != nil {
		return false, pageFailed("publishing the article", err)
	}
	return rows > 0, nil
}

// Unpublish takes it back.
func (PageArticleRepository) Unpublish(ctx context.Context, pageID, collectionID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	ids, err := pageUUIDs(pageID, collectionID)
	if err != nil {
		return false, err
	}
	rows, err := queries.UnpublishPageArticle(ctx, sqlc.UnpublishPageArticleParams{PageID: ids[0], CollectionID: ids[1]})
	if err != nil {
		return false, pageFailed("unpublishing the article", err)
	}
	return rows > 0, nil
}

// Of answers where a page is published.
func (PageArticleRepository) Of(ctx context.Context, pageID shared.ID) ([]knowledge.Article, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PageArticles(ctx, key)
	if err != nil {
		return nil, pageFailed("reading the articles", err)
	}
	return mapRows(rows, func(row sqlc.PageArticlesRow) (knowledge.Article, error) {
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return knowledge.Article{}, err
		}
		by, err := optionalID(row.PublishedBy)
		return knowledge.Article{
			PageID: pageID, CollectionID: collection, PublishedBy: by, PublishedAt: timeFrom(row.PublishedAt),
			Views: row.Views, Helpful: row.Helpful, Unhelpful: row.Unhelpful,
		}, err
	})
}

// Published answers whether the page is published to the collection.
func (PageArticleRepository) Published(ctx context.Context, pageID, collectionID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	ids, err := pageUUIDs(pageID, collectionID)
	if err != nil {
		return false, err
	}
	held, err := queries.IsPageArticle(ctx, sqlc.IsPageArticleParams{PageID: ids[0], CollectionID: ids[1]})
	if err != nil {
		return false, pageFailed("reading the article", err)
	}
	return held, nil
}

// Search answers the collection's articles a text finds.
func (PageArticleRepository) Search(ctx context.Context, collectionID shared.ID, text string, limit int) ([]repository.ArticleHit, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(collectionID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchPageArticles(ctx, sqlc.SearchPageArticlesParams{
		Text: text, CollectionID: key, Batch: int32(limit), //nolint:gosec // bounded by knowledge.MaxArticleSuggestions
	})
	if err != nil {
		return nil, pageFailed("searching the articles", err)
	}
	return mapRows(rows, func(row sqlc.SearchPageArticlesRow) (repository.ArticleHit, error) {
		id, err := idFrom(row.ID)
		return repository.ArticleHit{PageID: id, Title: row.Title, Icon: row.Icon, Excerpt: row.Excerpt, UpdatedAt: timeFrom(row.UpdatedAt)}, err
	})
}

// CountView counts one opening.
func (PageArticleRepository) CountView(ctx context.Context, pageID, collectionID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(pageID, collectionID)
	if err != nil {
		return err
	}
	if _, err := queries.CountPageArticleView(ctx, sqlc.CountPageArticleViewParams{PageID: ids[0], CollectionID: ids[1]}); err != nil {
		return pageFailed("counting the article's opening", err)
	}
	return nil
}

// Rate keeps one person's rating.
func (PageArticleRepository) Rate(ctx context.Context, pageID, collectionID, accountID shared.ID, helpful bool, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(pageID, collectionID, accountID)
	if err != nil {
		return err
	}
	if err := queries.RatePageArticle(ctx, sqlc.RatePageArticleParams{
		PageID: ids[0], CollectionID: ids[1], AccountID: ids[2], Helpful: helpful, At: timestampOf(at),
	}); err != nil {
		return pageFailed("rating the article", err)
	}
	return nil
}
