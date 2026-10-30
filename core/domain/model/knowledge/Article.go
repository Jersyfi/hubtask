// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxArticleSuggestions bounds what one question to the portal answers: a requester reads a few
// suggestions while typing, never the whole knowledge base.
const MaxArticleSuggestions = 10

// Article is a page published to one service's portal (UC-DOC-13): the requesters of that
// collection read it, nobody else gains anything. The audience is the service, so one page may be
// published to several, each with its own count of openings and ratings.
type Article struct {
	PageID       shared.ID
	CollectionID shared.ID
	PublishedBy  shared.ID
	PublishedAt  time.Time
	Views        int64
	Helpful      int64
	Unhelpful    int64
}

// NewArticle checks a publication: never of a page behind a restriction, since a portal reaches
// people the restriction does not name.
func NewArticle(page, collection, by shared.ID, readers Readers, now time.Time) (Article, error) {
	if page.IsZero() || by.IsZero() {
		return Article{}, shared.ErrInternal.WithDetail("pages.identity_incomplete")
	}
	if collection.IsZero() {
		return Article{}, invalid("/collection_id", "pages.article_collection_missing")
	}
	if readers.Restricted {
		return Article{}, shared.ErrConflict.WithDetail("pages.article_restricted")
	}
	return Article{PageID: page, CollectionID: collection, PublishedBy: by, PublishedAt: now}, nil
}
