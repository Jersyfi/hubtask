// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The search over every kind finds a post only for a person it is for (UC-DOC-17 check 4, B-6):
// READ on the hub and inside the audience, an agent no more than its person - and only once it has
// gone out: a scheduled post or a draft is found by nobody, not even its writer.
func TestTheSearchFindsAPublishedPostForItsAudienceAlone(t *testing.T) {
	ahead := start.Add(time.Hour).Format(time.RFC3339)
	for _, row := range []struct {
		name  string
		in    usecase.Input
		finds []shared.ID
	}{
		{"published to the hub", usecase.Input{"publish_at": start.Format(time.RFC3339)}, []shared.ID{writer, member, guest, robot, agent}},
		{"narrowed to a group", usecase.Input{"publish_at": start.Format(time.RFC3339), "audience": "GROUPS", "group_ids": []any{team.String()}},
			[]shared.ID{writer, member, agent}},
		{"scheduled", usecase.Input{"publish_at": ahead}, nil},
		{"a draft", usecase.Input{}, nil},
	} {
		t.Run(row.name, func(t *testing.T) {
			f := newPostFixture()
			id := f.post(t, row.in)
			f.store.pages[id] = domain.Page{ID: id, HubID: hub, Title: "Release 4.2", Version: 1}
			for _, account := range []shared.ID{writer, member, guest, robot, agent, requester} {
				found, err := PostFinder{Space: f.space}.Find(context.Background(), actorOf(account), search.Question{Words: "release", Size: 5})
				if err != nil {
					t.Fatal(err)
				}
				wants := slices.Contains(row.finds, account)
				if (found.Count == 1) != wants || len(found.Hits) != found.Count {
					t.Errorf("account %s found %+v; wants found %v", account, found, wants)
					continue
				}
				if wants && (found.Hits[0].ID != id || found.Hits[0].HubID != hub || found.Hits[0].Title != "Release 4.2" ||
					found.Hits[0].Snippet != "«Release 4.2»" || found.Hits[0].CreatedAt == nil || found.Hits[0].AuthorID != writer) {
					t.Errorf("account %s found the hit %+v", account, found.Hits[0])
				}
			}
		})
	}
}

// A post whose page is restricted is found by those the restriction admits alone; a hub other than
// the post's, a token that may not read items and words it does not hold find nothing.
func TestTheSearchFindsNoPostBeyondItsPageOrItsWords(t *testing.T) {
	f := newPostFixture()
	id := f.post(t, usecase.Input{"publish_at": start.Format(time.RFC3339)})
	f.store.pages[id] = domain.Page{ID: id, HubID: hub, Title: "Release 4.2", Version: 1, Restricted: true}
	f.store.restrictedTo[id] = []shared.ID{writer}
	finder := PostFinder{Space: f.space}
	find := func(actor appshared.ActorContext, q search.Question) int {
		t.Helper()
		found, err := finder.Find(context.Background(), actor, q)
		if err != nil {
			t.Fatal(err)
		}
		return found.Count
	}
	if n := find(actorOf(writer), search.Question{Words: "release", Size: 5}); n != 1 {
		t.Errorf("the restriction's reader found %d", n)
	}
	if n := find(actorOf(member), search.Question{Words: "release", Size: 5}); n != 0 {
		t.Errorf("somebody the restriction leaves out found %d", n)
	}
	scoped := actorOf(writer)
	scoped.Scopes = []string{"admin:read"}
	for name, q := range map[string]struct {
		actor appshared.ActorContext
		q     search.Question
	}{
		"another hub":        {actorOf(writer), search.Question{Words: "release", HubID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c0ff"), Size: 5}},
		"no items:read":      {scoped, search.Question{Words: "release", Size: 5}},
		"words it lacks":     {actorOf(writer), search.Question{Words: "budget", Size: 5}},
		"keys without words": {actorOf(writer), search.Question{Size: 5}},
	} {
		if n := find(q.actor, q.q); n != 0 {
			t.Errorf("%s found %d", name, n)
		}
	}
}
