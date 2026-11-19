// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/service/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// PageFinder finds pages for the search over every kind (UC-WRK-18 check 9) through SearchPages
// - its spaces, its readers, verified pages first among equals (E-8) - narrowed by the
// page.verified and page.owner keys.
type PageFinder struct{ Pages SearchPages }

// Find asks once.
func (f PageFinder) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	query := SearchPagesQuery{Text: q.Words, HubID: q.HubID, Limit: maxSearchHits}
	for _, condition := range q.Conditions {
		switch condition.Field {
		case view.FieldPageVerified:
			wanted, nothing := verifiedWanted(condition)
			if nothing {
				return search.Found{}, nil
			}
			query.Verified = wanted
		case view.FieldPageOwner:
			owner := condition
			query.Owner = &owner
		}
	}
	if strings.TrimSpace(query.Text) == "" && query.Verified == nil && query.Owner == nil {
		// Either state of page.verified alone narrows nothing; every page is no question asked.
		return search.Found{}, nil
	}
	hits, err := f.Pages.Execute(ctx, actor, query)
	if err != nil {
		if errors.Is(err, shared.ErrForbidden) {
			return search.Found{}, nil
		}
		return search.Found{}, err
	}
	found := search.Found{Count: len(hits), More: len(hits) >= maxSearchHits}
	now := f.Pages.Space.Clock.Now()
	for _, hit := range hits[:min(len(hits), q.Size)] {
		updated, created := hit.UpdatedAt, hit.CreatedAt
		row := search.Hit{
			ID: hit.ID, HubID: hit.HubID, Title: hit.Title, Tile: hit.Icon, Snippet: hit.Excerpt, UpdatedAt: &updated,
			CreatedAt: &created, AuthorID: hit.CreatedBy,
		}
		if hit.VerifiedUntil != nil {
			row.State = "REVIEW_DUE"
			if hit.VerifiedUntil.After(now) {
				row.State = "VERIFIED"
			}
		}
		found.Hits = append(found.Hits, row)
	}
	return found, nil
}

// verifiedWanted reads page.verified: the state wanted, nil where either is; nothing where
// neither is (`-page.verified: true, false`).
func verifiedWanted(c view.KindCondition) (wanted *bool, nothing bool) {
	wants := map[bool]bool{}
	for _, value := range c.Values {
		wants[value == "true"] = true
	}
	if c.Negated {
		wants = map[bool]bool{true: !wants[true], false: !wants[false]}
	}
	switch {
	case wants[true] && wants[false]:
		return nil, false
	case wants[true] || wants[false]:
		state := wants[true]
		return &state, false
	}
	return nil, true
}
