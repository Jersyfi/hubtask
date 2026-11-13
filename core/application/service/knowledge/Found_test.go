// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	st "github.com/Jersyfi/hubtask/core/application/service/search/searchtest"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// keyedSearcher answers fixed hits and keeps the query it was asked.
type keyedSearcher struct {
	hits  []repository.Hit
	asked *repository.PageQuery
}

func (s keyedSearcher) Search(_ context.Context, q repository.PageQuery) ([]repository.Hit, error) {
	*s.asked = q
	return s.hits, nil
}

// A page hit says whether the page is verified or due for review, for its lozenge (E-8), and when
// and by whom it was written, for the search page's order and its *Von* facet.
func TestAPageHitSaysVerifiedOrDueForReview(t *testing.T) {
	f := newFixture()
	now := f.clock.Now()
	ahead, behind := now.Add(time.Hour), now.Add(-time.Hour)
	var asked repository.PageQuery
	index := keyedSearcher{asked: &asked, hits: []repository.Hit{
		{ID: "0192f000-0000-7000-8000-0000000009a1", Title: "A", VerifiedUntil: &ahead, CreatedAt: behind, CreatedBy: "0192f000-0000-7000-8000-0000000009b1"},
		{ID: "0192f000-0000-7000-8000-0000000009a2", Title: "B", VerifiedUntil: &behind},
		{ID: "0192f000-0000-7000-8000-0000000009a3", Title: "C"},
	}}
	found, err := PageFinder{Pages: SearchPages{Space: f.space, Searcher: index}}.Find(context.Background(), as(bert), search.Question{Words: "a", Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if found.Count != 3 || found.Hits[0].State != "VERIFIED" || found.Hits[1].State != "REVIEW_DUE" || found.Hits[2].State != "" {
		t.Fatalf("found %+v", found)
	}
	if found.Hits[0].AuthorID != "0192f000-0000-7000-8000-0000000009b1" || found.Hits[0].CreatedAt == nil || !found.Hits[0].CreatedAt.Equal(behind) {
		t.Errorf("made %v by %q", found.Hits[0].CreatedAt, found.Hits[0].AuthorID)
	}
}

// UC-WRK-18 check 9: page.verified and page.owner reach the page search, which answers them
// without words; a page.verified nothing answers asks nothing.
func TestThePageKeysReachThePageSearch(t *testing.T) {
	cases := []struct {
		name     string
		keys     []view.KindCondition
		verified *bool
		owner    bool
		asks     bool
	}{
		{"verified", []view.KindCondition{{Field: view.FieldPageVerified, Values: []string{"true"}}}, ptrBool(true), false, true},
		{"not verified", []view.KindCondition{{Field: view.FieldPageVerified, Values: []string{"true"}, Negated: true}}, ptrBool(false), false, true},
		{"either", []view.KindCondition{{Field: view.FieldPageVerified, Values: []string{"true", "false"}}}, nil, false, false},
		{"either, by an owner", []view.KindCondition{{Field: view.FieldPageVerified, Values: []string{"true", "false"}}, {Field: view.FieldPageOwner, None: true}}, nil, true, true},
		{"neither", []view.KindCondition{{Field: view.FieldPageVerified, Values: []string{"true", "false"}, Negated: true}}, nil, false, false},
		{"owner", []view.KindCondition{{Field: view.FieldPageOwner, Values: []string{bert.String()}}}, nil, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			asked := repository.PageQuery{Limit: -1}
			index := keyedSearcher{asked: &asked}
			_, err := PageFinder{Pages: SearchPages{Space: newFixture().space, Searcher: index}}.Find(context.Background(), as(bert), search.Question{Conditions: c.keys, Size: 5})
			if err != nil {
				t.Fatal(err)
			}
			if !c.asks {
				if asked.Limit != -1 {
					t.Fatalf("the search was asked %+v", asked)
				}
				return
			}
			if (asked.Verified == nil) != (c.verified == nil) || (c.verified != nil && *asked.Verified != *c.verified) {
				t.Errorf("verified asked as %v, want %v", asked.Verified, c.verified)
			}
			if (asked.Owner != nil) != c.owner || asked.Text != "" {
				t.Errorf("asked %+v", asked)
			}
		})
	}
}

// UC-WRK-18 check 9: a file or comment on a page is found only where the reader may read the page
// - its space, and a restriction that names them -, the passage marked in a comment.
func TestWhatIsOnAPageIsFoundWhereThePageIsRead(t *testing.T) {
	f := newFixture()
	open, closed, restricted := shared.ID("0192f000-0000-7000-8000-0000000009b1"), shared.ID("0192f000-0000-7000-8000-0000000009b2"), shared.ID("0192f000-0000-7000-8000-0000000009b3")
	f.store.pages[open] = domain.Page{ID: open, HubID: hub}
	f.store.pages[closed] = domain.Page{ID: closed, HubID: private}
	f.store.pages[restricted] = domain.Page{ID: restricted, HubID: hub, Restricted: true}
	f.store.restrictedTo[restricted] = []shared.ID{anna}
	row := func(id, on, place shared.ID) searchrepo.Row {
		return searchrepo.Row{ID: id, OnID: on, HubID: place, Title: "Travel policy", Text: "see the travel budget"}
	}
	names := st.Names{Rows: []searchrepo.Row{
		row("0192f000-0000-7000-8000-0000000009c1", open, hub),
		row("0192f000-0000-7000-8000-0000000009c2", closed, private),
		row("0192f000-0000-7000-8000-0000000009c3", restricted, hub),
	}}
	found, err := OnPages{Space: f.space, Names: names, Comments: true}.Find(context.Background(), as(bert), search.Question{Terms: []string{"budget"}, Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if found.Count != 1 || found.Hits[0].OnID != open || found.Hits[0].OnKind != search.KindPage || found.Hits[0].Snippet != "see the travel «budget»" {
		t.Fatalf("found %+v, want the comment on the open page alone", found)
	}
}
