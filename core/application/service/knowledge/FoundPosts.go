// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PostFinder finds posts for the search over every kind by their title and body words, as the
// page search reads them. A hit goes only to a person the post is for - READ on its hub, inside its
// audience (B-6), and a page restriction that admits them - and only once the post has gone out:
// search finds news, so a draft or a scheduled post is no hit, not even for its writers (B-3).
type PostFinder struct{ Space Space }

// Find judges every candidate at its own hub, as the posts' list does.
func (f PostFinder) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	posts := f.Space.Posting.Posts
	if posts == nil || actor.RequireScope(pagesRead) != nil || strings.TrimSpace(q.Words) == "" {
		return search.Found{}, nil
	}
	// Compared in the form the bodies are stored in (i18n-l10n.md §5).
	words, err := shared.NFC(q.Words, f.Space.Text)
	if err != nil {
		return search.Found{}, err
	}
	now := f.Space.Clock.Now()
	var hits []repository.PostHit
	var readers map[shared.ID]domain.Readers
	err = f.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if hits, err = posts.Search(ctx, words, q.HubID, now, maxSearchHits); err != nil || len(hits) == 0 {
			return err
		}
		ids := make([]shared.ID, 0, len(hits))
		for _, hit := range hits {
			ids = append(ids, hit.Post.PageID)
		}
		readers, err = f.Space.Pages.Readers(ctx, ids)
		return err
	})
	if err != nil || len(hits) == 0 {
		return search.Found{}, err
	}
	judged := map[shared.ID]domain.Reader{}
	found := search.Found{More: len(hits) >= maxSearchHits}
	for _, hit := range hits {
		reader, ok := judged[hit.Post.HubID]
		if !ok {
			// A hub with posts off finds none of them, as its list refuses them with module.off.
			on, err := q.On(ctx, actor, hit.Post.HubID, module.Posts)
			if err != nil {
				return search.Found{}, err
			}
			if on {
				if reader, err = f.Space.postReader(ctx, actor, hit.Post.HubID); err != nil {
					return search.Found{}, err
				}
			}
			judged[hit.Post.HubID] = reader
		}
		if hit.Post.State(now) != domain.PostPublished || !hit.Post.Sees(reader, now) ||
			!readers[hit.Post.PageID].Admits(actor.AccountID) {
			continue
		}
		found.Count++
		if len(found.Hits) >= q.Size {
			continue
		}
		updated, published := hit.UpdatedAt, hit.Post.PublishedAt
		if published == nil {
			published = hit.Post.PublishAt
		}
		found.Hits = append(found.Hits, search.Hit{
			ID: hit.Post.PageID, HubID: hit.Post.HubID, Title: hit.Title, Tile: hit.Icon,
			Snippet: domain.PlainExcerpt(hit.Excerpt), UpdatedAt: &updated, CreatedAt: published, AuthorID: hit.CreatedBy,
		})
	}
	return found, nil
}
