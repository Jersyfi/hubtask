// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

type ratingStore struct{ rows map[shared.ID]domain.Rating }

func (s *ratingStore) Save(_ context.Context, r domain.Rating) (domain.Rating, error) {
	if held, ok := s.rows[r.ItemID]; ok {
		r.ID, r.Version = held.ID, held.Version+1
	}
	s.rows[r.ItemID] = r
	return r, nil
}

func (s *ratingStore) OfItem(_ context.Context, id shared.ID) (domain.Rating, error) {
	r, ok := s.rows[id]
	if !ok {
		return domain.Rating{}, shared.ErrNotFound
	}
	return r, nil
}

type invocations struct{ calls []usecase.Input }

func (i *invocations) Invoke(_ context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	if name != addCommentUseCase || actor.Channel != string(work.ChannelPortal) {
		return nil, errors.New("not through AddComment on the PORTAL door")
	}
	i.calls = append(i.calls, in)
	return usecase.Output{}, nil
}

func requester() appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenantID, AccountID: requesterID, Kind: appshared.ActorUser, Scopes: []string{deskRead, "comments:write"}}
}

// UC-SVC-07 checks 1-3: the requester rates their own resolved request, with words that become
// their comment; an open request and anybody else's request are refused; rating again replaces.
func TestTheRequesterRatesTheirResolvedRequest(t *testing.T) {
	f := newFixture(t)
	ratings := &ratingStore{rows: map[shared.ID]domain.Rating{}}
	comments := &invocations{}
	authorizer := &gate{allow: true}
	rate := RateMyRequest{Portal: Portal{
		Entries: comments, Items: f.items, Containers: containerStore{},
		Desk: Desk{Ratings: ratings, Authorizer: authorizer, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: f.ids},
	}}

	if _, err := rate.Execute(context.Background(), requester(), RateMyRequestCommand{ItemID: itemID, Score: 4}); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("an open request was rated: %v", err)
	}
	f.items.item.Completion.IsCompleted = true
	if _, err := rate.Execute(context.Background(), requester(), RateMyRequestCommand{ItemID: itemID, Score: 2, Comment: "Took long."}); err != nil {
		t.Fatal(err)
	}
	if asked := authorizer.asked[len(authorizer.asked)-1]; asked.Permission != service.PermissionRequest {
		t.Errorf("asked %s, want REQUEST", asked.Permission)
	}
	if len(comments.calls) != 1 || comments.calls[0]["body"] != "Took long." {
		t.Errorf("the words went %+v", comments.calls)
	}
	again, err := rate.Execute(context.Background(), requester(), RateMyRequestCommand{ItemID: itemID, Score: 5})
	if err != nil || again.Score != 5 || again.Version != 2 || len(comments.calls) != 1 {
		t.Errorf("rating again: %+v %v", again, err)
	}
	if _, err := rate.Execute(context.Background(), requester(), RateMyRequestCommand{ItemID: itemID, Score: 9}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a score off the scale: %v", err)
	}
	if _, err := rate.Execute(context.Background(), reader(), RateMyRequestCommand{ItemID: itemID, Score: 3}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody else rated the request: %v", err)
	}
}

// UC-SVC-07 check 3: the requester reads their own score with the request in the portal, null
// while they gave none.
func TestTheRequesterReadsTheirOwnScoreWithTheRequest(t *testing.T) {
	ctx := context.Background()
	portal, items, _ := organisationPortal(ctx, t, false)
	items.item.CreatedBy = requesterID
	read := func() any {
		t.Helper()
		out, err := ReadMyRequest{Portal: portal}.Descriptor().Handler.Invoke(ctx, requester(), usecase.Input{"item_id": itemID.String()})
		if err != nil {
			t.Fatal(err)
		}
		satisfaction, present := out["satisfaction"]
		if !present {
			t.Fatal("the request answers no satisfaction")
		}
		return satisfaction
	}
	if got := read(); got != nil {
		t.Errorf("before a rating the score reads %v, want null", got)
	}
	portal.Desk.Ratings.(*ratingStore).rows[itemID] = domain.Rating{ItemID: itemID, Score: 4, RatedAt: monday}
	if got, _ := read().(usecase.Output); got == nil || got["score"] != 4 {
		t.Errorf("the requester's own score reads %v", got)
	}
}

// failingRatings refuses to store a score, as a database gone for a moment does.
type failingRatings struct{ ratingStore }

func (failingRatings) Save(context.Context, domain.Rating) (domain.Rating, error) {
	return domain.Rating{}, shared.ErrUnavailable
}

// commentsUndone keeps the comments only when the transaction they were written in commits.
type commentsUndone struct {
	inline
	comments *invocations
}

func (u commentsUndone) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	before := len(u.comments.calls)
	if err := fn(ctx); err != nil {
		u.comments.calls = u.comments.calls[:before]
		return err
	}
	return nil
}

// UC-SVC-07 check 1: a score that cannot be stored takes its words with it, so the requester's
// retry writes them once, not a second copy beside the first.
func TestAScoreNotStoredLeavesNoCommentBehind(t *testing.T) {
	f := newFixture(t)
	f.items.item.Completion.IsCompleted = true
	comments := &invocations{}
	rate := RateMyRequest{Portal: Portal{
		Entries: comments, Items: f.items, Containers: containerStore{},
		Desk: Desk{Ratings: &failingRatings{}, Authorizer: &gate{allow: true}, UnitOfWork: commentsUndone{comments: comments},
			Audit: discard{}, Clock: at(monday), IDs: f.ids},
	}}
	if _, err := rate.Execute(context.Background(), requester(), RateMyRequestCommand{ItemID: itemID, Score: 2, Comment: "Took long."}); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("the rating answered %v", err)
	}
	if len(comments.calls) != 0 {
		t.Errorf("a score not stored left its words: %+v", comments.calls)
	}
}
